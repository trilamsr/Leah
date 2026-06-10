package dispatcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/trilam/leah/internal/attestation"
	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
)

const selfBuildScope = "self-build"

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

// selfBuildSpecSentinel is the load-bearing literal a Reasoner MUST include
// for its output to be treated as a feature spec. Replaces the prior
// `## title` substring check, which broke whenever the system prompt was
// reworded (closes #9). The `-v1` suffix lets future spec-format changes
// bump cleanly while old-format code paths fail closed.
const selfBuildSpecSentinel = "## leah-selfbuild-spec-v1"

// selfBuildSpecInstruction is appended to the per-call user prompt so the
// sentinel survives system-prompt rewordings. The dispatcher owns this
// contract — not the prompt file — so a `prompts/*.md` edit cannot
// silently break the spec-vs-clarify gate.
const selfBuildSpecInstruction = "When (and only when) you emit a full feature spec, the FIRST line of your response MUST be the literal heading `" + selfBuildSpecSentinel + "` on its own line. Omit it for clarify-only responses."

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
		"Intent:\n"+intent+"\n\nDraft the Leah-feature-spec per the self-build system prompt you were given.\n\n"+selfBuildSpecInstruction)
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

	// SystemPrompt-once contract: the outer s.Reasoner already ran ONCE under
	// the self-build system prompt to produce `spec`. Ship.Run will call
	// Reasoner.Ask a second time with the regatta-issue template — we wrap
	// the spec in prebakedReasoner so that second call returns the spec
	// verbatim without re-charging the budget or re-prompting the model.
	// Swapping prebakedReasoner for the outer Reasoner would silently double
	// both cost and prompt (Wave2-5 retro H2; closes #3).
	inner := &Ship{
		Reasoner: prebakedReasoner{spec: specWithAttestation},
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

// prebakedReasoner is a Reasoner that returns a pre-drafted spec verbatim,
// ignoring the prompt argument. Pins the SystemPrompt-once contract: SelfBuild
// runs the outer Reasoner ONCE with the self-build system prompt, then hands
// the result here so Ship.Run's second Reasoner.Ask call returns the spec
// unchanged instead of re-charging the budget + re-prompting the model with
// the regatta-issue template. Renamed from `passthrough` per Wave2-5 retro H2;
// the old name hid the contract from any future maintainer reaching for a
// "real" Reasoner here.
type prebakedReasoner struct{ spec string }

func (p prebakedReasoner) Ask(ctx context.Context, _ string) (string, error) { return p.spec, nil }

// isClarifyResponse returns true when Reasoner did NOT emit the spec sentinel.
// Sentinel-absence — not H2-heading shape — is the contract; the previous
// substring match on `## title` flipped whenever the system prompt was
// reworded (#9). Fail-closed: any response without the sentinel is treated
// as clarify, so no spurious dispatch can leak through a malformed spec.
func isClarifyResponse(s string) bool {
	return !strings.Contains(s, selfBuildSpecSentinel)
}

// deriveSelfBuildTitle reuses ship's intent classifier but strips redundant
// verb prefixes — every self-build is implicitly a feature add, so [FEAT] would
// double-prefix with [SELF-BUILD]. Truncates at the last whitespace before the
// budget so `gh issue list` reads as complete words, not orphan prepositions
// (closes #8).
func deriveSelfBuildTitle(intent string) string {
	t := strings.TrimSpace(intent)
	budget := 60 - len(SelfBuildTitlePrefix)
	if len(t) <= budget {
		return t
	}
	// Rune-aware: byte-level slicing would split multibyte runes (emoji, CJK,
	// accents) and emit invalid UTF-8 that `gh issue create` rejects.
	cut := budget - 3
	lastSpace := -1
	end := 0
	for i, r := range t {
		next := i + utf8.RuneLen(r)
		if next > cut {
			break
		}
		end = next
		if unicode.IsSpace(r) {
			lastSpace = i
		}
	}
	if lastSpace > 0 {
		end = lastSpace
	}
	return strings.TrimRightFunc(t[:end], unicode.IsSpace) + "..."
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

// pickAttestationQuestion returns "" when path empty (gate disabled); fail-closed otherwise.
func (s *SelfBuild) pickAttestationQuestion() (string, error) {
	if s.AttestationQuestionsPath == "" {
		return "", nil
	}
	pool, err := attestation.Load(s.AttestationQuestionsPath, selfBuildScope)
	if err != nil {
		return "", err
	}
	return pool.Pick(selfBuildScope)
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
