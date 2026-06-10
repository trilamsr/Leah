package dispatcher

import (
	"bytes"
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/ghclient"
	"github.com/trilam/leah/internal/regattaclient"
)

// validSpec is a minimally well-formed Reasoner output that selfBuild treats as
// a real feature spec (NOT a clarify-abort). Must contain `## Title` to pass
// isClarifyResponse.
const validSpec = `## Title

[SELF-BUILD] add --json flag to leah status

## Motivation

Operator wants scriptable status output. Advances self-improvement loop §2.1.

## Files to create or modify

- internal/dispatcher/status.go
- internal/dispatcher/status_test.go

## Code shape

` + "```go\n" +
	`type Status struct { JSON bool }
` + "```\n" +
	`
## Acceptance criteria

- ` + "`leah status --json`" + ` emits valid JSON.

## Test plan

- TestStatusJSONFlag

## Deferred

- multi-format output (yaml, toml)

## Self-build context

(verbatim block)
`

func TestSelfBuildRefusesNonLeahRepo(t *testing.T) {
	dir := t.TempDir()
	sb := &SelfBuild{
		Reasoner:     &fakeShipReasoner{resp: validSpec},
		GH:           &fakeGh{},
		Audit:        &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:       &budget.Budget{Ceiling: 5.0},
		Out:          &bytes.Buffer{},
		TmpDir:       dir,
		RepoOverride: "trilam/regatta", // operator tried to retarget
	}
	err := sb.Run(context.Background(), "add a flag")
	if !errors.Is(err, ErrSelfBuildRepoLocked) {
		t.Fatalf("want ErrSelfBuildRepoLocked, got %v", err)
	}
	// Audit row recorded the rejection.
	data, _ := os.ReadFile(dir + "/audit.jsonl")
	if !strings.Contains(string(data), `"kind":"self-build"`) {
		t.Errorf("audit missing kind=self-build: %q", data)
	}
	if !strings.Contains(string(data), `"blast_radius":4`) {
		t.Errorf("audit missing blast_radius=4: %q", data)
	}
	if !strings.Contains(string(data), `"outcome":"rejected"`) {
		t.Errorf("audit missing outcome=rejected: %q", data)
	}
}

func TestSelfBuildAuditsBR4(t *testing.T) {
	dir := t.TempDir()
	gh := &fakeGh{createURL: "https://github.com/trilamsr/Leah/issues/42"}
	sb := &SelfBuild{
		Reasoner: &fakeShipReasoner{resp: validSpec},
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		TmpDir:   dir,
	}
	if err := sb.Run(context.Background(), "add --json flag to leah status"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gh.createdRepo != SelfBuildRepo {
		t.Errorf("repo: got %q want %q", gh.createdRepo, SelfBuildRepo)
	}
	data, _ := os.ReadFile(dir + "/audit.jsonl")
	// Two audit rows: ship's BR=3 + self-build's BR=4. Self-build success row required.
	if !strings.Contains(string(data), `"kind":"self-build"`) {
		t.Errorf("audit missing kind=self-build: %q", data)
	}
	if !strings.Contains(string(data), `"blast_radius":4`) {
		t.Errorf("audit missing blast_radius=4: %q", data)
	}
	if !strings.Contains(string(data), `"outcome":"dispatched"`) {
		t.Errorf("audit missing outcome=dispatched: %q", data)
	}
}

func TestSelfBuildPrependsTitle(t *testing.T) {
	dir := t.TempDir()
	gh := &fakeGh{createURL: "https://github.com/trilamsr/Leah/issues/1"}
	sb := &SelfBuild{
		Reasoner: &fakeShipReasoner{resp: validSpec},
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		TmpDir:   dir,
	}
	if err := sb.Run(context.Background(), "add --json flag to leah status"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(gh.createdTitle, SelfBuildTitlePrefix) {
		t.Errorf("title missing %q prefix: %q", SelfBuildTitlePrefix, gh.createdTitle)
	}
}

// TestSelfBuildIncludesRandomAttestation asserts that when an attestation
// file is configured, the dispatched issue body contains the attestation
// block + one of the questions from the file (operator-habituation defense,
// Wave1-E HIGH-2).
func TestSelfBuildIncludesRandomAttestation(t *testing.T) {
	dir := t.TempDir()
	attestPath := filepath.Join(dir, "attestations.txt")
	const q1 = "What's the biggest risk in this diff?"
	const q2 = "Which test case would catch a regression here?"
	if err := os.WriteFile(attestPath, []byte(q1+"\n"+q2+"\n"), 0o600); err != nil {
		t.Fatalf("write attestation file: %v", err)
	}
	gh := &fakeGh{createURL: "https://github.com/trilamsr/Leah/issues/7"}
	sb := &SelfBuild{
		Reasoner:                 &fakeShipReasoner{resp: validSpec},
		GH:                       gh,
		Audit:                    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:                   &budget.Budget{Ceiling: 5.0},
		Out:                      &bytes.Buffer{},
		TmpDir:                   dir,
		AttestationQuestionsPath: attestPath,
		AttestationOperatorLogin: "tri-lamsr",
		// Deterministic seed: rand.Intn(2) returns 1 → q2 with this source.
		// Test only asserts SOME known question appears, so the assertion is
		// stable regardless of which one Intn picks.
		Rand: rand.New(rand.NewSource(1)),
	}
	if err := sb.Run(context.Background(), "add --json flag"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(gh.createdBody, "## Operator merge attestation") {
		t.Errorf("body missing attestation header: %q", gh.createdBody)
	}
	if !strings.Contains(gh.createdBody, "tri-lamsr") {
		t.Errorf("body missing operator login: %q", gh.createdBody)
	}
	if !strings.Contains(gh.createdBody, "Attestation:") {
		t.Errorf("body missing Attestation: comment shape: %q", gh.createdBody)
	}
	if !strings.Contains(gh.createdBody, q1) && !strings.Contains(gh.createdBody, q2) {
		t.Errorf("body missing both candidate questions: %q", gh.createdBody)
	}
	// Audit row should record the attestation question.
	data, _ := os.ReadFile(dir + "/audit.jsonl")
	if !strings.Contains(string(data), "attestation_question=") {
		t.Errorf("audit missing attestation_question field: %q", data)
	}
}

// TestSelfBuildAttestationFailClosed asserts that a configured but empty /
// missing attestation file aborts the dispatch (fail-closed) — the gate
// only disables when AttestationQuestionsPath is left empty.
func TestSelfBuildAttestationFailClosed(t *testing.T) {
	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(emptyPath, []byte("# only comments\n\n"), 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	gh := &fakeGh{createURL: "should-not-be-called"}
	sb := &SelfBuild{
		Reasoner:                 &fakeShipReasoner{resp: validSpec},
		GH:                       gh,
		Audit:                    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:                   &budget.Budget{Ceiling: 5.0},
		Out:                      &bytes.Buffer{},
		TmpDir:                   dir,
		AttestationQuestionsPath: emptyPath,
	}
	if err := sb.Run(context.Background(), "anything"); err == nil {
		t.Fatal("expected error when attestation file has no questions, got nil")
	}
	if gh.createdTitle != "" {
		t.Errorf("issue created despite attestation failure: %q", gh.createdTitle)
	}
}

// TestSelfBuild_ReasonerErrorAuditsFailed asserts that a reasoner-draft
// failure during self-build records a `self-build` outcome=failed row and
// skips the underlying Ship call entirely.
func TestSelfBuild_ReasonerErrorAuditsFailed(t *testing.T) {
	dir := t.TempDir()
	gh := &fakeGh{}
	sb := &SelfBuild{
		Reasoner: errReasoner{},
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		TmpDir:   dir,
	}
	if err := sb.Run(context.Background(), "do x"); err == nil {
		t.Fatal("want reasoner error, got nil")
	}
	if gh.createdTitle != "" {
		t.Errorf("issue created despite reasoner failure: %q", gh.createdTitle)
	}
	data, _ := os.ReadFile(dir + "/audit.jsonl")
	if !strings.Contains(string(data), `"kind":"self-build"`) ||
		!strings.Contains(string(data), `"outcome":"failed"`) {
		t.Errorf("audit missing self-build/failed row: %s", data)
	}
}

// TestSelfBuild_PromptSHARecordedInSuccess asserts promptSHA() output appears
// in the success audit row when PromptPath is set + readable (spec §4.11).
func TestSelfBuild_PromptSHARecordedInSuccess(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte("system prompt content"), 0o600); err != nil {
		t.Fatal(err)
	}
	gh := &fakeGh{createURL: "https://github.com/trilamsr/Leah/issues/1"}
	sb := &SelfBuild{
		Reasoner:   &fakeShipReasoner{resp: validSpec},
		GH:         gh,
		Audit:      &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:     &budget.Budget{Ceiling: 5.0},
		Out:        &bytes.Buffer{},
		TmpDir:     dir,
		PromptPath: promptPath,
	}
	if err := sb.Run(context.Background(), "add feature"); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, _ := os.ReadFile(dir + "/audit.jsonl")
	if !strings.Contains(string(data), "prompt_sha=") {
		t.Errorf("audit missing prompt_sha field: %s", data)
	}
}

// TestSelfBuild_PromptSHAEmptyWhenPathUnset asserts the helper degrades to
// empty (non-fatal) on missing config — the audit row still appears.
func TestSelfBuild_PromptSHAEmptyWhenPathUnset(t *testing.T) {
	sb := &SelfBuild{}
	if got := sb.promptSHA(); got != "" {
		t.Errorf("unset PromptPath should yield empty sha, got %q", got)
	}
}

// TestSelfBuild_PromptSHAEmptyWhenFileMissing asserts an unreadable PromptPath
// returns "" (rather than panic) so the audit row still lands.
func TestSelfBuild_PromptSHAEmptyWhenFileMissing(t *testing.T) {
	sb := &SelfBuild{PromptPath: "/nonexistent/path/that/should/not/exist.md"}
	if got := sb.promptSHA(); got != "" {
		t.Errorf("missing PromptPath should yield empty sha, got %q", got)
	}
}

// TestSelfBuild_PopulatesArgsHash asserts every self-build audit row carries
// a non-empty args_hash matching sha256[:8] of the intent — selflearn.rowKey
// uses (Kind, ArgsHash, Timestamp) for dedup, so a blank hash collapses all
// self-build runs into a single resolver key (H1 from Wave2-5 retro audit).
func TestSelfBuild_PopulatesArgsHash(t *testing.T) {
	const intent = "add --json flag to leah status"
	want := argsHash(intent)

	cases := []struct {
		name       string
		setup      func(dir string) *SelfBuild
		wantOutcome string
	}{
		{
			name: "success",
			setup: func(dir string) *SelfBuild {
				return &SelfBuild{
					Reasoner: &fakeShipReasoner{resp: validSpec},
					GH:       &fakeGh{createURL: "https://github.com/trilamsr/Leah/issues/1"},
					Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
					Budget:   &budget.Budget{Ceiling: 5.0},
					Out:      &bytes.Buffer{},
					TmpDir:   dir,
				}
			},
			wantOutcome: `"outcome":"dispatched"`,
		},
		{
			name: "reasoner-failed",
			setup: func(dir string) *SelfBuild {
				return &SelfBuild{
					Reasoner: errReasoner{},
					GH:       &fakeGh{},
					Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
					Budget:   &budget.Budget{Ceiling: 5.0},
					Out:      &bytes.Buffer{},
					TmpDir:   dir,
				}
			},
			wantOutcome: `"outcome":"failed"`,
		},
		{
			name: "clarify",
			setup: func(dir string) *SelfBuild {
				return &SelfBuild{
					Reasoner: &fakeShipReasoner{resp: "## Clarifying questions\n\n- which pkg?\n"},
					GH:       &fakeGh{},
					Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
					Budget:   &budget.Budget{Ceiling: 5.0},
					Out:      &bytes.Buffer{},
					TmpDir:   dir,
				}
			},
			wantOutcome: `"outcome":"clarify"`,
		},
		{
			name: "rejected-override",
			setup: func(dir string) *SelfBuild {
				return &SelfBuild{
					Reasoner:     &fakeShipReasoner{resp: validSpec},
					GH:           &fakeGh{},
					Audit:        &audit.Logger{Path: dir + "/audit.jsonl"},
					Budget:       &budget.Budget{Ceiling: 5.0},
					Out:          &bytes.Buffer{},
					TmpDir:       dir,
					RepoOverride: "trilam/regatta",
				}
			},
			wantOutcome: `"outcome":"rejected"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			sb := tc.setup(dir)
			_ = sb.Run(context.Background(), intent)
			data, _ := os.ReadFile(dir + "/audit.jsonl")
			// Every self-build row in the file MUST carry args_hash=want.
			// (Some flows write multiple rows: e.g. success path writes ship + self-build.)
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			sawSelfBuild := false
			for _, line := range lines {
				if !strings.Contains(line, `"kind":"self-build"`) {
					continue
				}
				sawSelfBuild = true
				if !strings.Contains(line, `"args_hash":"`+want+`"`) {
					t.Errorf("self-build row missing args_hash=%s: %s", want, line)
				}
			}
			if !sawSelfBuild {
				t.Fatalf("no self-build row found in audit log: %s", data)
			}
			// Sanity: the outcome we expect is in the log.
			if !strings.Contains(string(data), tc.wantOutcome) {
				t.Errorf("missing expected outcome %s in audit: %s", tc.wantOutcome, data)
			}
		})
	}
}

func TestSelfBuildClarifyAbortFilesNoIssue(t *testing.T) {
	dir := t.TempDir()
	clarify := "## Clarifying questions\n\n- Which package?\n- What's the observable acceptance?\n"
	gh := &fakeGh{createURL: "should-not-be-called"}
	out := &bytes.Buffer{}
	sb := &SelfBuild{
		Reasoner: &fakeShipReasoner{resp: clarify},
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      out,
		TmpDir:   dir,
	}
	err := sb.Run(context.Background(), "make leah smarter")
	if !errors.Is(err, ErrSelfBuildClarify) {
		t.Fatalf("want ErrSelfBuildClarify, got %v", err)
	}
	if gh.createdTitle != "" {
		t.Errorf("issue created on clarify-abort: %q", gh.createdTitle)
	}
	if !strings.Contains(out.String(), "Clarifying questions") {
		t.Errorf("clarify output missing questions: %q", out.String())
	}
	data, _ := os.ReadFile(dir + "/audit.jsonl")
	if !strings.Contains(string(data), `"outcome":"clarify"`) {
		t.Errorf("audit missing outcome=clarify: %q", data)
	}
}

// hookGh wraps fakeGh and fires a callback the moment CreateIssue returns.
// Used by TestSelfBuildAuditsDispatchedBeforeWatcher to snapshot the audit
// log at the exact point gh-create succeeds — proving the dispatched row
// landed BEFORE the watcher ran (Defect-2 from closed-loop-live-validation
// research). The callback must NOT block CreateIssue's return — the watcher
// is what we want to delay, not gh.
type hookGh struct {
	createURL string
	onCreate  func()
}

func (h *hookGh) CreateIssue(_ context.Context, _ ghclient.CreateIssueArgs) (string, error) {
	if h.onCreate != nil {
		h.onCreate()
	}
	return h.createURL, nil
}

func (*hookGh) EnsureLabel(_ context.Context, _, _ string) error { return nil }

// blockingRegatta makes List wait until released, then returns the canned
// state. The watcher invokes List once per poll; this lets the test pause
// the watcher mid-loop and verify the BR=4 dispatched audit row already
// landed before the watcher could finish.
type blockingRegatta struct {
	release chan struct{}
	state   string
}

func (b *blockingRegatta) List(ctx context.Context) ([]regattaclient.Agent, error) {
	select {
	case <-b.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return []regattaclient.Agent{{ID: "a1", State: b.state, PR: 99}}, nil
}

// TestSelfBuildAuditsDispatchedBeforeWatcher is the Defect-2 regression test.
// Before the fix, appendAuditSuccess was gated behind inner.Run() returning
// — so an operator who Ctrl-C'd the watcher (common when regatta isn't
// running locally) lost the audit trail of the dispatch entirely. After the
// fix, the BR=4 dispatched row MUST land immediately after gh issue create
// returns success, BEFORE the watcher loop begins.
func TestSelfBuildAuditsDispatchedBeforeWatcher(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	logger := &audit.Logger{Path: auditPath}

	// Snapshot the audit log at the exact moment gh-create returns. The
	// dispatched row must NOT yet be present (Ship's own pending row is
	// written by Ship.Run after CreateIssue; self-build's row is written
	// by SelfBuild AFTER inner.Run returns from the synchronous portion).
	// Snapshot AFTER gh returns + before watcher fires by using a blocking
	// regatta client.
	releaseWatcher := make(chan struct{})
	rc := &blockingRegatta{release: releaseWatcher, state: "merged"}

	gh := &hookGh{createURL: "https://github.com/trilamsr/Leah/issues/99"}

	sb := &SelfBuild{
		Reasoner:  &fakeShipReasoner{resp: validSpec},
		GH:        gh,
		Audit:     logger,
		Budget:    &budget.Budget{Ceiling: 5.0},
		Out:       &bytes.Buffer{},
		TmpDir:    dir,
		Watch:     true,
		Regatta:   rc,
		Heartbeat: &fakeHeartbeat{},
		Notify:    &fakeNotify{},
		PollEvery: 1 * time.Millisecond,
		MaxPolls:  3,
	}

	// Run SelfBuild in a goroutine so the watcher can park on releaseWatcher.
	done := make(chan error, 1)
	go func() { done <- sb.Run(context.Background(), "add --json flag to leah status") }()

	// Poll the audit log until the self-build dispatched row appears. The
	// timeout window is generous (1s) for slow CI; in practice the row
	// lands within microseconds of gh-create returning.
	deadline := time.Now().Add(1 * time.Second)
	var snapshot []byte
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(auditPath)
		if strings.Contains(string(data), `"kind":"self-build"`) &&
			strings.Contains(string(data), `"outcome":"dispatched"`) {
			snapshot = data
			break
		}
		time.Sleep(1 * time.Millisecond) // allow-sleep: scheduler yield in tight audit-row poll
	}
	if snapshot == nil {
		// Read final state for the error message.
		final, _ := os.ReadFile(auditPath)
		t.Fatalf("dispatched self-build row never appeared while watcher was blocked; audit log:\n%s", final)
	}

	// CRITICAL invariant: at the moment we observed the dispatched row, the
	// watcher had not yet returned (it is still blocked on releaseWatcher).
	select {
	case err := <-done:
		t.Fatalf("Run returned (%v) before watcher was released — invariant broken; cannot prove dispatched-row-lands-before-watcher", err)
	default:
		// Good: Run is still parked inside the watcher loop.
	}

	// Release the watcher and let Run complete.
	close(releaseWatcher)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after watcher released")
	}

	// Final audit log should now contain both the dispatched row AND a
	// terminal-outcome self-build row (kind=self-build.outcome).
	final, _ := os.ReadFile(auditPath)
	if !strings.Contains(string(final), `"kind":"self-build.outcome"`) {
		t.Errorf("audit missing self-build.outcome terminal row after watcher returned: %s", final)
	}
}

// TestSelfBuildAuditsDispatchedOnOperatorAbort proves the dispatched row
// survives operator Ctrl-C of the watcher. Before the fix, ctx-cancel
// caused inner.Run to return nil (watcher returns on ctx.Done with no
// error) but the operator-abort case is more brutal — process death.
// We approximate operator abort here with ctx-cancel; the invariant is
// the same: the dispatched row must already exist when abort happens.
func TestSelfBuildAuditsDispatchedOnOperatorAbort(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	logger := &audit.Logger{Path: auditPath}

	releaseWatcher := make(chan struct{})
	defer close(releaseWatcher) // cleanup; not used to release
	rc := &blockingRegatta{release: releaseWatcher, state: "running"}

	gh := &hookGh{createURL: "https://github.com/trilamsr/Leah/issues/100"}

	sb := &SelfBuild{
		Reasoner:  &fakeShipReasoner{resp: validSpec},
		GH:        gh,
		Audit:     logger,
		Budget:    &budget.Budget{Ceiling: 5.0},
		Out:       &bytes.Buffer{},
		TmpDir:    dir,
		Watch:     true,
		Regatta:   rc,
		Heartbeat: &fakeHeartbeat{},
		Notify:    &fakeNotify{},
		PollEvery: 1 * time.Millisecond,
		MaxPolls:  1000,
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- sb.Run(ctx, "add feature x") }()

	// Wait for the dispatched audit row to appear, then simulate operator
	// abort by cancelling the context (the watcher's select returns on
	// ctx.Done).
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(auditPath)
		if strings.Contains(string(data), `"outcome":"dispatched"`) {
			break
		}
		time.Sleep(1 * time.Millisecond) // allow-sleep: scheduler yield in tight audit-row poll
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}

	final, _ := os.ReadFile(auditPath)
	if !strings.Contains(string(final), `"kind":"self-build"`) {
		t.Errorf("audit missing kind=self-build row after operator abort: %s", final)
	}
	if !strings.Contains(string(final), `"outcome":"dispatched"`) {
		t.Errorf("audit missing outcome=dispatched after operator abort: %s", final)
	}
}

// TestPrebakedReasonerReturnsSpecVerbatim pins the SystemPrompt-once contract:
// the inner Reasoner SelfBuild hands to Ship MUST return the pre-drafted spec
// unchanged regardless of the prompt argument. Renaming the type from
// passthrough to prebakedReasoner makes the contract self-documenting; a
// silent swap for a live Reasoner would re-charge the budget + re-prompt with
// the ship template on top of the self-build spec (Wave2-5 retro H2).
func TestPrebakedReasonerReturnsSpecVerbatim(t *testing.T) {
	const spec = "## Title\n\nspec body verbatim\n"
	r := prebakedReasoner{spec: spec}
	got, err := r.Ask(context.Background(), "any prompt the caller passes — must be ignored")
	if err != nil {
		t.Fatalf("Ask returned err: %v", err)
	}
	if got != spec {
		t.Errorf("Ask returned %q, want verbatim spec %q", got, spec)
	}
}

// TestSelfBuildUsesPrebakedReasonerForInnerShip pins that SelfBuild constructs
// the inner Ship with a prebakedReasoner, NOT the outer (live) Reasoner. If a
// future change swaps the wrapper for the outer Reasoner, the budget would be
// charged twice + ship template would re-prompt on top of the self-build spec.
func TestSelfBuildUsesPrebakedReasonerForInnerShip(t *testing.T) {
	dir := t.TempDir()
	outerCalls := &countingReasoner{resp: validSpec}
	gh := &fakeGh{createURL: "https://github.com/trilamsr/Leah/issues/9"}
	sb := &SelfBuild{
		Reasoner: outerCalls,
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		TmpDir:   dir,
	}
	if err := sb.Run(context.Background(), "add a feature"); err != nil {
		t.Fatalf("Run returned err: %v", err)
	}
	// Outer Reasoner gets exactly ONE call: the self-build spec draft. The
	// inner Ship MUST use prebakedReasoner; if it called the outer Reasoner
	// again, the count would be 2 (re-charged budget + ship-template prompt).
	if outerCalls.count != 1 {
		t.Errorf("outer Reasoner called %d times, want 1 (SystemPrompt-once contract)", outerCalls.count)
	}
}

// countingReasoner records how many times Ask is invoked so the test can pin
// the SystemPrompt-once contract.
type countingReasoner struct {
	resp  string
	count int
}

func (c *countingReasoner) Ask(ctx context.Context, _ string) (string, error) {
	c.count++
	return c.resp, nil
}

// Verifies long intent truncates at last whitespace, not mid-word.
func TestDeriveSelfBuildTitle_TruncatesOnWordBoundary(t *testing.T) {
	intent := "add a really long descriptive feature that goes well over the limit"
	got := deriveSelfBuildTitle(intent)

	budget := 60 - len(SelfBuildTitlePrefix)
	if len(got) > budget {
		t.Fatalf("len(got)=%d > budget=%d: %q", len(got), budget, got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("want ... suffix, got %q", got)
	}
	stem := strings.TrimSuffix(got, "...")
	if strings.HasSuffix(stem, " ") {
		t.Fatalf("trailing space before ellipsis: %q", got)
	}
	// Mid-word break means the stem's last token is a prefix of a longer source
	// token. Whole-word truncation guarantees the last stem token appears as a
	// complete token in the input.
	stemTokens := strings.Fields(stem)
	if len(stemTokens) == 0 {
		t.Fatalf("empty stem: %q", got)
	}
	last := stemTokens[len(stemTokens)-1]
	srcTokens := strings.Fields(intent)
	found := false
	for _, tok := range srcTokens {
		if tok == last {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("last stem token %q not a complete word in source; got %q", last, got)
	}
}

// Verifies single-token-over-limit hard truncates instead of infinite-scanning.
func TestDeriveSelfBuildTitle_NoWhitespaceFallback(t *testing.T) {
	intent := "supercalifragilisticexpialidociousplusextraextraextrapaddingtail"
	got := deriveSelfBuildTitle(intent)

	budget := 60 - len(SelfBuildTitlePrefix)
	if len(got) > budget {
		t.Fatalf("len(got)=%d > budget=%d: %q", len(got), budget, got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("want ... suffix, got %q", got)
	}
}

// Verifies truncation landing on whitespace leaves no " ..." artifact.
func TestDeriveSelfBuildTitle_TrailingSpaceTrimmed(t *testing.T) {
	// Craft so the limit-3 byte is a space: 47-char budget, limit-3 = 44.
	// Tokens chosen so a space sits at byte 44.
	intent := "aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk"
	got := deriveSelfBuildTitle(intent)

	if strings.Contains(got, " ...") {
		t.Fatalf("trailing-space artifact: %q", got)
	}
}

// Verifies short intent passes through unchanged.
func TestDeriveSelfBuildTitle_ShortPassThrough(t *testing.T) {
	intent := "tiny intent"
	got := deriveSelfBuildTitle(intent)
	if got != intent {
		t.Fatalf("want %q unchanged, got %q", intent, got)
	}
}

// Verifies intent at exactly the budget passes through with no ellipsis.
func TestDeriveSelfBuildTitle_ExactLimitNoTruncate(t *testing.T) {
	budget := 60 - len(SelfBuildTitlePrefix)
	intent := strings.Repeat("a", budget)
	got := deriveSelfBuildTitle(intent)
	if got != intent {
		t.Fatalf("want %q unchanged, got %q", intent, got)
	}
	if strings.HasSuffix(got, "...") {
		t.Fatalf("unexpected ellipsis at exact limit: %q", got)
	}
}

// Byte-level truncation can split a multibyte rune; `gh issue create` then
// rejects the title or renders mojibake. Result MUST be valid UTF-8.
func TestDeriveSelfBuildTitle_UTF8Emoji_NoCorruption(t *testing.T) {
	intent := strings.Repeat("a", 41) + "🎉 and more padding to force truncation"
	got := deriveSelfBuildTitle(intent)
	if !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8 in result: %q (% x)", got, got)
	}
}

func TestDeriveSelfBuildTitle_UTF8MultibyteAccent(t *testing.T) {
	intent := strings.Repeat("a", 43) + "é and trailing text to force boundary cut"
	got := deriveSelfBuildTitle(intent)
	if !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8 in result: %q (% x)", got, got)
	}
}

func TestDeriveSelfBuildTitle_UTF8CJK(t *testing.T) {
	intent := strings.Repeat("a", 40) + "日本語 padding extras filler more chars"
	got := deriveSelfBuildTitle(intent)
	if !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8 in result: %q (% x)", got, got)
	}
}

// Property check: random multibyte inputs must all yield valid UTF-8.
func TestDeriveSelfBuildTitle_UTF8Property(t *testing.T) {
	runes := []rune{'a', ' ', 'é', '🎉', '日', '本', '語', 'ñ', 'ü', '中'}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		n := 30 + r.Intn(80)
		var b strings.Builder
		for j := 0; j < n; j++ {
			b.WriteRune(runes[r.Intn(len(runes))])
		}
		intent := b.String()
		got := deriveSelfBuildTitle(intent)
		if !utf8.ValidString(got) {
			t.Fatalf("invalid UTF-8 from input %q: %q (% x)", intent, got, got)
		}
	}
}

// Sentinel-absent input is by definition a clarify-response under the new contract.
func TestIsClarifyResponse_MissingSentinel_True(t *testing.T) {
	in := "Sure — which package should the new flag live in, and what is the observable acceptance?\n"
	if !isClarifyResponse(in) {
		t.Fatalf("missing sentinel must be treated as clarify; got false for %q", in)
	}
}

// Sentinel-present input is a spec, not a clarify-response.
func TestIsClarifyResponse_WithSentinel_False(t *testing.T) {
	in := "## leah-selfbuild-spec-v1\n\n## Title\n\n[SELF-BUILD] add x\n"
	if isClarifyResponse(in) {
		t.Fatalf("sentinel-present spec must not be treated as clarify: %q", in)
	}
}

// Sentinel is a literal contract: case-folded variants must not satisfy it.
func TestIsClarifyResponse_CaseSensitive(t *testing.T) {
	in := "## LEAH-SELFBUILD-SPEC-V1\n\n## Title\n\n[SELF-BUILD] add x\n"
	if !isClarifyResponse(in) {
		t.Fatalf("uppercase sentinel must not satisfy literal contract: %q", in)
	}
}

// Drift catch: the prompt-emission surface in dispatcher must carry the sentinel.
func TestSystemPrompt_IncludesSentinel(t *testing.T) {
	if !strings.Contains(selfBuildSpecInstruction, selfBuildSpecSentinel) {
		t.Fatalf("selfBuildSpecInstruction missing sentinel %q: %q", selfBuildSpecSentinel, selfBuildSpecInstruction)
	}
}

// Old H2-heading-shaped responses without sentinel now route as clarify under the new contract.
func TestIsClarifyResponse_OldFormatNoLongerPasses(t *testing.T) {
	in := "## Title\n\n[SELF-BUILD] thing\n\n## Motivation\n\nbecause\n"
	if !isClarifyResponse(in) {
		t.Fatalf("old-format spec without sentinel must now route as clarify: %q", in)
	}
}
