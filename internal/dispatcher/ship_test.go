package dispatcher

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/ghclient"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
)

type fakeGh struct {
	createdRepo  string
	createdTitle string
	createdBody  string
	createdLabel []string
	createURL    string

	// createErrs is consumed FIFO on each CreateIssue call. A non-nil entry
	// returns that error before the success path; an exhausted/empty slice
	// returns createURL. Lets a single test drive the "fail-then-succeed"
	// label-retry shape without forking the fake.
	createErrs  []error
	createCalls int

	// ensureLabelCalls captures every (repo, label) pair EnsureLabel saw.
	// ensureLabelErr (when set) is returned on each invocation.
	ensureLabelCalls []ensureLabelCall
	ensureLabelErr   error
}

type ensureLabelCall struct {
	repo  string
	label string
}

func (f *fakeGh) CreateIssue(ctx context.Context, a ghclient.CreateIssueArgs) (string, error) {
	f.createCalls++
	if len(f.createErrs) > 0 {
		err := f.createErrs[0]
		f.createErrs = f.createErrs[1:]
		if err != nil {
			return "", err
		}
	}
	f.createdRepo = a.Repo
	f.createdTitle = a.Title
	if body, err := os.ReadFile(a.BodyFile); err == nil {
		f.createdBody = string(body)
	}
	f.createdLabel = a.Labels
	return f.createURL, nil
}

func (f *fakeGh) EnsureLabel(ctx context.Context, repo, label string) error {
	f.ensureLabelCalls = append(f.ensureLabelCalls, ensureLabelCall{repo: repo, label: label})
	return f.ensureLabelErr
}

type fakeShipReasoner struct {
	resp       string
	lastPrompt string
}

func (f *fakeShipReasoner) Ask(ctx context.Context, user string) (string, error) {
	f.lastPrompt = user
	return f.resp, nil
}

func TestShipDraftsBodyFilesIssueAuditsCorrectly(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	a := &audit.Logger{Path: auditPath}
	b := &budget.Budget{Ceiling: 5.0}
	r := &fakeShipReasoner{resp: "## Context\n\nFix it.\n\n## What to do\n\n- Do thing\n\n## Acceptance\n\n- Thing done\n"}
	gh := &fakeGh{createURL: "https://github.com/trilam/regatta/issues/1234"}
	out := &bytes.Buffer{}

	ship := &Ship{
		Reasoner: r,
		GH:       gh,
		Audit:    a,
		Budget:   b,
		Out:      out,
		Repo:     "trilam/regatta",
		Title:    "[FIX] BUG-1086: stamp headless flag",
		TmpDir:   dir,
	}
	if err := ship.Run(context.Background(), "fix bug 1086"); err != nil {
		t.Fatalf("run: %v", err)
	}

	if gh.createdRepo != "trilam/regatta" {
		t.Errorf("repo: %q", gh.createdRepo)
	}
	if !strings.HasPrefix(gh.createdTitle, "[FIX]") {
		t.Errorf("title: %q", gh.createdTitle)
	}
	if !strings.Contains(gh.createdBody, "## Context") {
		t.Errorf("body missing Context: %q", gh.createdBody)
	}
	if !strings.Contains(gh.createdBody, "leah-dispatched:") {
		t.Errorf("body missing correlation comment: %q", gh.createdBody)
	}
	if len(gh.createdLabel) == 0 || gh.createdLabel[0] != "ready-for-agent" {
		t.Errorf("labels: %v", gh.createdLabel)
	}
	if !strings.Contains(out.String(), "https://github.com/trilam/regatta/issues/1234") {
		t.Errorf("output missing url: %q", out.String())
	}

	data, _ := os.ReadFile(auditPath)
	if !strings.Contains(string(data), `"kind":"ship"`) {
		t.Errorf("audit missing kind=ship: %q", data)
	}
	if !strings.Contains(string(data), `"blast_radius":3`) {
		t.Errorf("audit missing blast_radius=3: %q", data)
	}
}

// TestShipEmitsObsLogsAt4Stages asserts the dispatcher emits draft.start,
// write.complete, and issue.created INFO events (Wave2-K obs
// instrumentation contract — 4th stage watch.start is gated on Watch=true
// and exercised in the watch tests).
func TestShipEmitsObsLogsAt4Stages(t *testing.T) {
	var buf bytes.Buffer
	lg := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := obs.WithLogger(context.Background(), lg)

	dir := t.TempDir()
	ship := &Ship{
		Reasoner: &fakeShipReasoner{resp: "body"},
		GH:       &fakeGh{createURL: "https://x"},
		Audit:    &audit.Logger{Path: dir + "/a.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		Repo:     "t/r",
		Title:    "[FIX] x",
		TmpDir:   dir,
	}
	if err := ship.Run(ctx, "x"); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"dispatcher.draft.start", "dispatcher.write.complete", "dispatcher.issue.created"} {
		if !strings.Contains(buf.String(), `"msg":"`+want+`"`) {
			t.Errorf("missing %s in: %s", want, buf.String())
		}
	}
}

type fakeRegatta struct {
	listResp []regattaclient.Agent
}

func (f *fakeRegatta) List(ctx context.Context) ([]regattaclient.Agent, error) {
	return f.listResp, nil
}

type fakeHeartbeat struct {
	pings int
}

func (f *fakeHeartbeat) Ping(ctx context.Context) error {
	f.pings++
	return nil
}

type fakeNotify struct {
	calls []string
}

func (f *fakeNotify) Notify(ctx context.Context, title, body string) error {
	f.calls = append(f.calls, title+":"+body)
	return nil
}

func TestShipWatcherPingsHeartbeatAndNotifiesOnMerge(t *testing.T) {
	dir := t.TempDir()
	a := &audit.Logger{Path: dir + "/audit.jsonl"}
	b := &budget.Budget{Ceiling: 5.0}
	r := &fakeShipReasoner{resp: "## Context\n\nFix.\n\n## What to do\n\n- x\n\n## Acceptance\n\n- y\n"}
	gh := &fakeGh{createURL: "https://github.com/x/r/issues/1"}
	out := &bytes.Buffer{}

	rc := &fakeRegatta{listResp: []regattaclient.Agent{
		{ID: "a1", Branch: "regatta/agent-1", State: "merged", PR: 1234},
	}}
	hb := &fakeHeartbeat{}
	nf := &fakeNotify{}

	ship := &Ship{
		Reasoner:  r,
		GH:        gh,
		Audit:     a,
		Budget:    b,
		Out:       out,
		Repo:      "x/r",
		TmpDir:    dir,
		Watch:     true,
		Regatta:   rc,
		Heartbeat: hb,
		Notify:    nf,
		PollEvery: 1 * time.Millisecond,
		MaxPolls:  2,
	}
	if err := ship.Run(context.Background(), "fix"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if hb.pings == 0 {
		t.Error("heartbeat never pinged")
	}
	if len(nf.calls) == 0 {
		t.Error("notifier never called")
	}
	if !strings.Contains(strings.Join(nf.calls, " "), "merged") {
		t.Errorf("notify calls: %v", nf.calls)
	}
}

// errRegatta returns the given error on every List call.
type errRegatta struct{ err error }

func (e errRegatta) List(_ context.Context) ([]regattaclient.Agent, error) {
	return nil, e.err
}

// TestShipWatcherMirrorsRegattaListErrorToObsLogger asserts the watcher
// reports regatta-list failures through the obs logger (lg.Warn) in
// addition to stdout, so a daemon-mode watcher with no attached terminal
// does not lose the error signal. Audit M5.
func TestShipWatcherMirrorsRegattaListErrorToObsLogger(t *testing.T) {
	dir := t.TempDir()
	a := &audit.Logger{Path: dir + "/audit.jsonl"}
	b := &budget.Budget{Ceiling: 5.0}
	r := &fakeShipReasoner{resp: "## Context\n\nx\n\n## What to do\n\n- x\n\n## Acceptance\n\n- y\n"}
	gh := &fakeGh{createURL: "https://github.com/x/r/issues/1"}
	out := &bytes.Buffer{}

	var logBuf bytes.Buffer
	lg := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := obs.WithLogger(context.Background(), lg)

	rc := errRegatta{err: errSynthetic("regatta unreachable")}

	ship := &Ship{
		Reasoner:  r,
		GH:        gh,
		Audit:     a,
		Budget:    b,
		Out:       out,
		Repo:      "x/r",
		TmpDir:    dir,
		Watch:     true,
		Regatta:   rc,
		Heartbeat: &fakeHeartbeat{},
		Notify:    &fakeNotify{},
		PollEvery: 1 * time.Millisecond,
		MaxPolls:  1,
	}
	if err := ship.Run(ctx, "fix"); err != nil {
		t.Fatalf("run: %v", err)
	}
	logs := logBuf.String()
	if !strings.Contains(logs, `"msg":"dispatcher.watch.regatta_list_error"`) {
		t.Errorf("obs logger missing watch.regatta_list_error event:\n%s", logs)
	}
	if !strings.Contains(logs, "regatta unreachable") {
		t.Errorf("obs logger missing underlying error: %s", logs)
	}
}

func TestDeriveTitleVerbRouting(t *testing.T) {
	cases := map[string]string{
		"fix bug 1086":         "[FIX] fix bug 1086",
		"add dashboard polish": "[FEAT] add dashboard polish",
		"doc the api":          "[DOCS] doc the api",
		"refactor handler":     "[REFACTOR] refactor handler",
		"[FIX] BUG-100":        "[FIX] BUG-100",
	}
	for in, want := range cases {
		got := deriveTitle(in)
		if got != want {
			t.Errorf("deriveTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

// errSynthetic is a tiny string-error used by failure-path tests below.
type errSynthetic string

func (e errSynthetic) Error() string { return string(e) }

// errGh always fails CreateIssue with a synthetic error.
type errGh struct{}

func (errGh) CreateIssue(_ context.Context, _ ghclient.CreateIssueArgs) (string, error) {
	return "", errSynthetic("gh create-issue exploded")
}

func (errGh) EnsureLabel(_ context.Context, _, _ string) error { return nil }

// errReasoner always fails Ask with a synthetic error.
type errReasoner struct{}

func (errReasoner) Ask(_ context.Context, _ string) (string, error) {
	return "", errSynthetic("reasoner exploded")
}

// TestShipRun_ReasonerErrorAuditsFailedAndReturns asserts the reasoner-draft
// failure path records a `ship` audit row with outcome=failed and skips the
// GH call entirely.
func TestShipRun_ReasonerErrorAuditsFailedAndReturns(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	gh := &fakeGh{}
	ship := &Ship{
		Reasoner: errReasoner{},
		GH:       gh,
		Audit:    &audit.Logger{Path: auditPath},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		Repo:     "x/y",
		TmpDir:   dir,
	}
	err := ship.Run(context.Background(), "do thing")
	if err == nil {
		t.Fatal("want reasoner error, got nil")
	}
	if gh.createdTitle != "" {
		t.Errorf("issue created despite reasoner failure: %q", gh.createdTitle)
	}
	data, _ := os.ReadFile(auditPath)
	if !strings.Contains(string(data), `"outcome":"failed"`) {
		t.Errorf("audit missing outcome=failed: %s", data)
	}
}

// TestShipRun_GHErrorAuditsFailed asserts a gh-create failure records a
// failed-outcome row referencing the gh error path.
func TestShipRun_GHErrorAuditsFailed(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	ship := &Ship{
		Reasoner: &fakeShipReasoner{resp: "draft body"},
		GH:       errGh{},
		Audit:    &audit.Logger{Path: auditPath},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		Repo:     "x/y",
		TmpDir:   dir,
	}
	if err := ship.Run(context.Background(), "do thing"); err == nil {
		t.Fatal("want gh error, got nil")
	}
	data, _ := os.ReadFile(auditPath)
	if !strings.Contains(string(data), `"outcome":"failed"`) {
		t.Errorf("audit missing outcome=failed: %s", data)
	}
	if !strings.Contains(string(data), "gh issue create") {
		t.Errorf("audit detail should reference gh create: %s", data)
	}
}

// TestShipRun_BodyFileWriteFailureAuditsFailed asserts an unwritable TmpDir
// surfaces a failed audit row and skips the gh call.
func TestShipRun_BodyFileWriteFailureAuditsFailed(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	gh := &fakeGh{}
	// Point TmpDir at a path nested under a regular file — os.WriteFile fails.
	blocker := dir + "/blocker"
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	ship := &Ship{
		Reasoner: &fakeShipReasoner{resp: "draft"},
		GH:       gh,
		Audit:    &audit.Logger{Path: auditPath},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		Repo:     "x/y",
		TmpDir:   blocker, // not-a-directory → WriteFile errors
	}
	if err := ship.Run(context.Background(), "thing"); err == nil {
		t.Fatal("want write error, got nil")
	}
	if gh.createdTitle != "" {
		t.Errorf("issue created despite write failure: %q", gh.createdTitle)
	}
	data, _ := os.ReadFile(auditPath)
	if !strings.Contains(string(data), `"outcome":"failed"`) {
		t.Errorf("audit missing outcome=failed: %s", data)
	}
}

// TestShipWithContext_PrependsContextToReasonerPrompt asserts the Ship.Context
// field is prepended verbatim to the Reasoner draft prompt before the
// "Intent:" line — the --from-pr / --from-issue / --from-thread surface.
func TestShipWithContext_PrependsContextToReasonerPrompt(t *testing.T) {
	dir := t.TempDir()
	r := &fakeShipReasoner{resp: "drafted body"}
	ship := &Ship{
		Reasoner: r,
		GH:       &fakeGh{createURL: "https://x/1"},
		Audit:    &audit.Logger{Path: dir + "/a.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		Repo:     "r/r",
		TmpDir:   dir,
		Context:  "## Context from PR #42\n\nfoo bar\n\n",
	}
	if err := ship.Run(context.Background(), "tie this together"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(r.lastPrompt, "Context from PR #42") {
		t.Errorf("expected context block in prompt, got: %q", r.lastPrompt)
	}
	if !strings.Contains(r.lastPrompt, "Intent:\ntie this together") {
		t.Errorf("expected intent line in prompt, got: %q", r.lastPrompt)
	}
	// Context block must precede Intent line.
	ctxIdx := strings.Index(r.lastPrompt, "Context from PR")
	intentIdx := strings.Index(r.lastPrompt, "Intent:")
	if ctxIdx < 0 || intentIdx < 0 || ctxIdx >= intentIdx {
		t.Errorf("context must precede intent, ctx@%d intent@%d", ctxIdx, intentIdx)
	}
}

// TestShipRun_NeutralizesCommentCloseInDraft asserts the substitution that
// prevents a draft body's literal `-->` from accidentally closing the
// leah-dispatched HTML comment marker.
func TestShipRun_NeutralizesCommentCloseInDraft(t *testing.T) {
	dir := t.TempDir()
	gh := &fakeGh{createURL: "https://example/1"}
	hostile := "## Context\n\nUser said: --> oops -->\n"
	ship := &Ship{
		Reasoner: &fakeShipReasoner{resp: hostile},
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		Repo:     "x/y",
		TmpDir:   dir,
	}
	if err := ship.Run(context.Background(), "x"); err != nil {
		t.Fatalf("run: %v", err)
	}
	// Drafted body's two `-->` occurrences neutralized → only the marker block
	// (two trailing HTML comments) provides the literal `-->` tokens.
	got := strings.Count(gh.createdBody, "-->")
	if got != 2 {
		t.Errorf("expected exactly 2 trailing `-->` (the markers), got %d in body: %q", got, gh.createdBody)
	}
	if !strings.Contains(gh.createdBody, "leah-dispatched:") {
		t.Errorf("body missing marker: %q", gh.createdBody)
	}
}

// TestShipAutoCreatesMissingLabel asserts that when the first `gh issue
// create` fails because `ready-for-agent` does not exist on the target repo,
// Ship.Run calls EnsureLabel and retries CreateIssue exactly once. The
// retry MUST succeed end-to-end: audit row written as pending, URL printed
// to Out, no error returned. Closes Defect-1 in
// docs/research/2026-06-09-closed-loop-live-validation.md (operator burned
// $0.029 on a Reasoner draft + manual retry when the label was missing).
func TestShipAutoCreatesMissingLabel(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	gh := &fakeGh{
		createURL: "https://github.com/x/y/issues/7",
		createErrs: []error{
			// First call: gh's actual stderr surface when --label names a
			// label that does not exist on the repo.
			errSynthetic(`gh issue create: gh: 'ready-for-agent' not found`),
		},
	}
	out := &bytes.Buffer{}
	ship := &Ship{
		Reasoner: &fakeShipReasoner{resp: "draft"},
		GH:       gh,
		Audit:    &audit.Logger{Path: auditPath},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      out,
		Repo:     "x/y",
		TmpDir:   dir,
	}
	if err := ship.Run(context.Background(), "do the thing"); err != nil {
		t.Fatalf("run should retry after label-not-found, got: %v", err)
	}
	if gh.createCalls != 2 {
		t.Errorf("want exactly 2 CreateIssue calls (initial + retry), got %d", gh.createCalls)
	}
	if len(gh.ensureLabelCalls) != 1 {
		t.Fatalf("want 1 EnsureLabel call, got %d: %+v", len(gh.ensureLabelCalls), gh.ensureLabelCalls)
	}
	if gh.ensureLabelCalls[0].repo != "x/y" || gh.ensureLabelCalls[0].label != "ready-for-agent" {
		t.Errorf("EnsureLabel called with %+v; want {x/y, ready-for-agent}", gh.ensureLabelCalls[0])
	}
	if !strings.Contains(out.String(), "https://github.com/x/y/issues/7") {
		t.Errorf("expected retried URL on stdout, got: %q", out.String())
	}
	data, _ := os.ReadFile(auditPath)
	if !strings.Contains(string(data), `"outcome":"pending"`) {
		t.Errorf("audit should record pending after successful retry: %s", data)
	}
}

// TestShipDoesNotRetryOnNonLabelError asserts a generic CreateIssue failure
// (auth error, network, etc.) is NOT retried — EnsureLabel must not fire so
// the operator sees the real root cause instead of masked symptoms.
func TestShipDoesNotRetryOnNonLabelError(t *testing.T) {
	dir := t.TempDir()
	gh := &fakeGh{
		createURL:  "https://x/1",
		createErrs: []error{errSynthetic("gh: not authenticated")},
	}
	ship := &Ship{
		Reasoner: &fakeShipReasoner{resp: "draft"},
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		Repo:     "x/y",
		TmpDir:   dir,
	}
	if err := ship.Run(context.Background(), "x"); err == nil {
		t.Fatal("want auth error to bubble up, got nil")
	}
	if gh.createCalls != 1 {
		t.Errorf("want 1 CreateIssue call (no retry on non-label error), got %d", gh.createCalls)
	}
	if len(gh.ensureLabelCalls) != 0 {
		t.Errorf("EnsureLabel should not fire on non-label error, got: %+v", gh.ensureLabelCalls)
	}
}

// TestShipRetryFailureBubblesOriginalError asserts that when the retry after
// EnsureLabel also fails, the ORIGINAL error is preserved so the operator
// sees the actionable root cause (not a second-attempt noise). Audit row
// records outcome=failed and references the original message.
func TestShipRetryFailureBubblesOriginalError(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	gh := &fakeGh{
		createURL: "https://x/1",
		createErrs: []error{
			errSynthetic("gh issue create: gh: 'ready-for-agent' not found"),
			errSynthetic("gh issue create: gh: still broken"),
		},
	}
	ship := &Ship{
		Reasoner: &fakeShipReasoner{resp: "draft"},
		GH:       gh,
		Audit:    &audit.Logger{Path: auditPath},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		Repo:     "x/y",
		TmpDir:   dir,
	}
	err := ship.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("want error after retry also fails, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("want original 'not found' preserved, got: %v", err)
	}
	if gh.createCalls != 2 {
		t.Errorf("want 2 CreateIssue calls, got %d", gh.createCalls)
	}
	data, _ := os.ReadFile(auditPath)
	if !strings.Contains(string(data), `"outcome":"failed"`) {
		t.Errorf("audit missing outcome=failed: %s", data)
	}
}
