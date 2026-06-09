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
}

func (f *fakeGh) CreateIssue(ctx context.Context, a ghclient.CreateIssueArgs) (string, error) {
	f.createdRepo = a.Repo
	f.createdTitle = a.Title
	if body, err := os.ReadFile(a.BodyFile); err == nil {
		f.createdBody = string(body)
	}
	f.createdLabel = a.Labels
	return f.createURL, nil
}

type fakeShipReasoner struct{ resp string }

func (f *fakeShipReasoner) Ask(ctx context.Context, user string) (string, error) {
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
