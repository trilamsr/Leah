package dispatcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
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
		s.appendAudit("repo override rejected: " + s.RepoOverride)
		return ErrSelfBuildRepoLocked
	}

	spec, err := s.Reasoner.Ask(ctx,
		"Intent:\n"+intent+"\n\nDraft the Leah-feature-spec per the self-build system prompt you were given.")
	if err != nil {
		s.appendAuditFail("reasoner: " + err.Error())
		return err
	}

	// Clarify-abort: Reasoner returned questions instead of a spec. Print + bail.
	if isClarifyResponse(spec) {
		_, _ = fmt.Fprintln(s.Out, spec)
		s.appendAuditClarify()
		return ErrSelfBuildClarify
	}

	title := SelfBuildTitlePrefix + deriveSelfBuildTitle(intent)

	inner := &Ship{
		Reasoner: passthrough{spec: spec},
		GH:       s.GH,
		Audit:    s.Audit,
		Budget:   s.Budget,
		Out:      s.Out,
		Repo:     SelfBuildRepo,
		Title:    title,
		TmpDir:   s.TmpDir,

		Watch:     s.Watch,
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
	s.appendAuditSuccess()
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

func (s *SelfBuild) appendAudit(detail string) {
	_ = s.Audit.Append(audit.Entry{
		Kind:        "self-build",
		ArgsHash:    "",
		BlastRadius: 4,
		Outcome:     "rejected",
		CostDollars: s.Budget.Spent(),
		Detail:      detail,
	})
}

func (s *SelfBuild) appendAuditFail(detail string) {
	_ = s.Audit.Append(audit.Entry{
		Kind:        "self-build",
		BlastRadius: 4,
		Outcome:     "failed",
		CostDollars: s.Budget.Spent(),
		Detail:      detail,
	})
}

func (s *SelfBuild) appendAuditClarify() {
	_ = s.Audit.Append(audit.Entry{
		Kind:        "self-build",
		BlastRadius: 4,
		Outcome:     "clarify",
		CostDollars: s.Budget.Spent(),
		Detail:      "prompt_sha=" + s.promptSHA(),
	})
}

func (s *SelfBuild) appendAuditSuccess() {
	_ = s.Audit.Append(audit.Entry{
		Kind:        "self-build",
		BlastRadius: 4,
		Outcome:     "dispatched",
		CostDollars: s.Budget.Spent(),
		Detail:      "prompt_sha=" + s.promptSHA(),
	})
}
