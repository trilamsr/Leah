package dispatcher

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
)

// SelfBuildRepo is the only repo SelfBuild ever targets. Hard-coded to prevent
// accidental self-build dispatch into a customer repo, a fork, or trilam/regatta
// itself. Operator's `--repo` flag is refused with ErrSelfBuildRepoLocked.
const SelfBuildRepo = "trilamsr/Leah"

// SelfBuildTitlePrefix tags every self-build PR so operator can distinguish
// Leah-built PRs from operator-built ones in `gh pr list`.
const SelfBuildTitlePrefix = "[SELF-BUILD] "

// ErrSelfBuildRepoLocked is returned when a caller attempts to override SelfBuildRepo.
var ErrSelfBuildRepoLocked = errors.New("self-build: --repo not allowed; repo is locked to " + SelfBuildRepo)

// ErrSelfBuildClarify is returned when Reasoner emits a clarifying-question
// abort instead of a feature spec. SelfBuild prints the questions and files
// NO issue.
var ErrSelfBuildClarify = errors.New("self-build: reasoner requested clarification; no issue filed")

// SelfBuild wraps Ship for self-modifying feature dispatch against trilamsr/Leah.
// All Ship fields it owns are constrained at Run time — Repo, Title, and the
// audit row's BlastRadius are set by SelfBuild, not by the caller.
type SelfBuild struct {
	Reasoner Reasoner
	GH       GHClient
	Audit    *audit.Logger
	Budget   *budget.Budget
	Out      io.Writer
	TmpDir   string

	// PromptPath is the absolute path to prompts/self-build-feature.md, used
	// to record the prompt's sha256 in the audit row (spec §4.11). May be empty
	// for tests; production CLI wiring sets it.
	PromptPath string

	// AttestationQuestionsPath points at prompts/self-build-attestations.txt
	// (one question per line). SelfBuild picks one at random + appends an
	// operator-attestation block to the issue body. Empty disables the gate
	// — useful in tests; production CLI wiring sets it.
	AttestationQuestionsPath string

	// AttestationOperatorLogin is the GitHub login that must answer the
	// attestation question in the PR. Defaults to "tri-lamsr" (the single
	// owner of trilamsr/Leah) when empty.
	AttestationOperatorLogin string

	// Rand is the random source for picking a question (tests inject a
	// deterministic source). nil → math/rand default.
	Rand *rand.Rand

	// lastQuestion is populated by Run after the attestation question is
	// chosen; surfaced in the dispatched-success audit row. Not exported.
	lastQuestion string

	// RepoOverride MUST be empty; any non-empty value triggers ErrSelfBuildRepoLocked.
	RepoOverride string

	// Watcher (forwarded to underlying Ship).
	Watch     bool
	Regatta   RegattaClient
	Heartbeat HeartbeatPinger
	Notify    Notifier
	PollEvery time.Duration
	MaxPolls  int
}

// Run drafts a Leah-feature-spec from operator intent, then dispatches a regatta
// issue against SelfBuildRepo with BR=4 audit. Returns ErrSelfBuildRepoLocked if
// RepoOverride is set, ErrSelfBuildClarify if Reasoner aborts.
func (s *SelfBuild) Run(ctx context.Context, intent string) error {
	if s.RepoOverride != "" {
		s.appendAudit(intent, "repo override rejected: "+s.RepoOverride)
		return ErrSelfBuildRepoLocked
	}

	spec, err := s.Reasoner.Ask(ctx,
		"Intent:\n"+intent+"\n\nDraft the Leah-feature-spec per the self-build system prompt you were given.")
	if err != nil {
		s.appendAuditFail(intent, "reasoner: "+err.Error())
		return err
	}

	// Clarify-abort: Reasoner returned questions instead of a spec. Print + bail.
	if isClarifyResponse(spec) {
		_, _ = fmt.Fprintln(s.Out, spec)
		s.appendAuditClarify(intent)
		return ErrSelfBuildClarify
	}

	question, err := s.pickAttestationQuestion()
	if err != nil {
		s.appendAuditFail(intent, "attestation: "+err.Error())
		return err
	}
	specWithAttestation := spec
	if question != "" {
		specWithAttestation = spec + "\n\n" + s.attestationBlock(question)
	}
	s.lastQuestion = question

	title := SelfBuildTitlePrefix + deriveSelfBuildTitle(intent)

	inner := &Ship{
		Reasoner: passthrough{spec: specWithAttestation},
		GH:       s.GH,
		Audit:    s.Audit,
		Budget:   s.Budget,
		Out:      s.Out,
		Repo:     SelfBuildRepo,
		Title:    title,
		TmpDir:   s.TmpDir,

		// Watch=false on inner Ship: dispatched audit row MUST land before watcher (Defect-2).
		Watch:     false,
		Regatta:   s.Regatta,
		Heartbeat: s.Heartbeat,
		Notify:    s.Notify,
		PollEvery: s.PollEvery,
		MaxPolls:  s.MaxPolls,
	}

	// Ship writes a BR=3 "ship" audit row; we follow up with the BR=4 "self-build"
	// row so retro queries (WHERE kind = 'self-build') count exactly the self-builds
	// without colliding with operator-initiated `leah ship` dispatches.
	if err := inner.Run(ctx, intent); err != nil {
		return err
	}

	// Land dispatched row before watcher so operator-abort still leaves a self-build trace (Defect-2).
	s.appendAuditSuccess(intent, inner.LastURL)

	// Outcome row pairs with dispatched; dangling dispatched-without-outcome is flagged by selflearn.
	if s.Watch {
		state := inner.watch(ctx)
		s.appendAuditOutcome(intent, state, inner.LastURL)
	}
	return nil
}

// passthrough is a Reasoner that returns a pre-drafted spec unchanged. SelfBuild
// runs Reasoner ONCE with the self-build system prompt, then hands the result
// to Ship which would otherwise call Reasoner again with the regatta-issue
// template.
type passthrough struct{ spec string }

func (p passthrough) Ask(ctx context.Context, _ string) (string, error) { return p.spec, nil }

// isClarifyResponse returns true when Reasoner returned a clarifying-question
// block instead of a feature spec. Spec §3 / §6 says Reasoner emits a `##
// Clarifying questions` H2 with all other sections empty in this case.
func isClarifyResponse(s string) bool {
	low := strings.ToLower(s)
	// Must mention clarifying questions AND lack a Title H2 (the spec's first
	// required section). Belt-and-suspenders: the system prompt says emit
	// clarify-only, but defend against partial outputs.
	hasClarify := strings.Contains(low, "## clarifying questions")
	hasTitle := strings.Contains(low, "## title")
	return hasClarify && !hasTitle
}

// deriveSelfBuildTitle reuses ship's intent classifier but strips redundant
// verb prefixes — every self-build is implicitly a feature add, so [FEAT] would
// double-prefix with [SELF-BUILD].
func deriveSelfBuildTitle(intent string) string {
	t := strings.TrimSpace(intent)
	if len(t) > 60-len(SelfBuildTitlePrefix) {
		t = t[:60-len(SelfBuildTitlePrefix)-3] + "..."
	}
	return t
}

// promptSHA returns a short prefix of the sha256 of the prompt file, recorded
// in the audit row's detail field (spec §4.11). Returns "" if PromptPath is
// empty or the file is unreadable — non-fatal.
func (s *SelfBuild) promptSHA() string {
	if s.PromptPath == "" {
		return ""
	}
	data, err := os.ReadFile(s.PromptPath)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// appendAudit, appendAuditFail, appendAuditClarify, appendAuditSuccess all
// thread the original operator intent through argsHash() so the row's
// args_hash matches Ship.Run's hash for the same dispatch. selflearn.rowKey
// = (Kind, ArgsHash, Timestamp); a blank hash collapses every self-build
// run into one resolver key (Wave2-5 retro H1).
func (s *SelfBuild) appendAudit(intent, detail string) {
	_ = s.Audit.Append(audit.Entry{
		Kind:        "self-build",
		ArgsHash:    argsHash(intent),
		BlastRadius: 4,
		Outcome:     "rejected",
		CostDollars: s.Budget.Spent(),
		Detail:      detail,
	})
}

func (s *SelfBuild) appendAuditFail(intent, detail string) {
	_ = s.Audit.Append(audit.Entry{
		Kind:        "self-build",
		ArgsHash:    argsHash(intent),
		BlastRadius: 4,
		Outcome:     "failed",
		CostDollars: s.Budget.Spent(),
		Detail:      detail,
	})
}

func (s *SelfBuild) appendAuditClarify(intent string) {
	_ = s.Audit.Append(audit.Entry{
		Kind:        "self-build",
		ArgsHash:    argsHash(intent),
		BlastRadius: 4,
		Outcome:     "clarify",
		CostDollars: s.Budget.Spent(),
		Detail:      "prompt_sha=" + s.promptSHA(),
	})
}

// appendAuditOutcome records the terminal watcher state for a self-build
// dispatch on a separate kind=self-build.outcome row referencing the same
// args_hash as the dispatched row. selflearn correlates the pair via
// (ArgsHash, Kind ∈ {self-build, self-build.outcome}); a dangling
// dispatched row with no outcome after N days flags an operator-abort or
// regatta-watcher failure.
func (s *SelfBuild) appendAuditOutcome(intent, state, issueURL string) {
	if state == "" {
		// No Regatta client wired → nothing observed → nothing to record.
		return
	}
	detail := "state=" + state
	if issueURL != "" {
		detail += " url=" + issueURL
	}
	_ = s.Audit.Append(audit.Entry{
		Kind:        "self-build.outcome",
		ArgsHash:    argsHash(intent),
		BlastRadius: 4,
		Outcome:     state,
		CostDollars: s.Budget.Spent(),
		Detail:      detail,
	})
}

func (s *SelfBuild) appendAuditSuccess(intent, issueURL string) {
	detail := "prompt_sha=" + s.promptSHA()
	if issueURL != "" {
		// Recorded so the H3 attestation gate can map a self-build dispatch to
		// its PR. Format: url=<full issue URL>. The gate uses gh issue view to
		// hop from issue → closing PR.
		detail += " url=" + issueURL
	}
	if s.lastQuestion != "" {
		// Question text can contain quotes / commas; truncate to keep audit
		// rows scannable. Operator can re-derive the full text from the PR body.
		q := s.lastQuestion
		if len(q) > 80 {
			q = q[:77] + "..."
		}
		detail += " attestation_question=" + strconv.Quote(q)
	}
	_ = s.Audit.Append(audit.Entry{
		Kind:        "self-build",
		ArgsHash:    argsHash(intent),
		BlastRadius: 4,
		Outcome:     "dispatched",
		CostDollars: s.Budget.Spent(),
		Detail:      detail,
	})
}

// pickAttestationQuestion reads AttestationQuestionsPath and returns one
// randomly-selected line (whitespace-trimmed, blanks + comments dropped).
// Returns "" when the path is empty (gate disabled). Returns an error when
// the path is set but unreadable / yields no questions — the gate is
// fail-closed by design (operator habituation defense per Wave1-E HIGH-2).
func (s *SelfBuild) pickAttestationQuestion() (string, error) {
	if s.AttestationQuestionsPath == "" {
		return "", nil
	}
	f, err := os.Open(s.AttestationQuestionsPath)
	if err != nil {
		return "", fmt.Errorf("open attestation file: %w", err)
	}
	defer func() { _ = f.Close() }()
	var qs []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		qs = append(qs, line)
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan attestation file: %w", err)
	}
	if len(qs) == 0 {
		return "", errors.New("attestation file has no questions")
	}
	r := s.Rand
	if r == nil {
		r = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return qs[r.Intn(len(qs))], nil
}

// attestationBlock renders the markdown footer appended to the issue body.
// The block is operator-actionable: it names the GitHub login that must
// answer and the exact comment shape ("Attestation: …") that downstream
// retro tooling will grep for.
func (s *SelfBuild) attestationBlock(question string) string {
	login := s.AttestationOperatorLogin
	if login == "" {
		login = "tri-lamsr"
	}
	var sb strings.Builder
	sb.WriteString("---\n\n")
	sb.WriteString("## Operator merge attestation\n\n")
	sb.WriteString("Before merging this PR, operator (`")
	sb.WriteString(login)
	sb.WriteString("`) must answer the following question in a PR comment whose first line starts with `Attestation:`.\n\n")
	sb.WriteString("> ")
	sb.WriteString(question)
	sb.WriteString("\n\n")
	sb.WriteString("PRs merged without an `Attestation:` comment from `")
	sb.WriteString(login)
	sb.WriteString("` will be flagged in the next weekly retro (self-build habituation defense).\n")
	return sb.String()
}
