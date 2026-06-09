package ghclient

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeExec struct {
	calls   [][]string
	respOut string
	respErr error
}

func (f *fakeExec) Run(ctx context.Context, args []string) (stdout string, err error) {
	f.calls = append(f.calls, args)
	return f.respOut, f.respErr
}

func TestCreateIssueShellsCorrectArgs(t *testing.T) {
	fe := &fakeExec{respOut: "https://github.com/trilam/regatta/issues/1234\n"}
	c := &Client{Exec: fe}

	url, err := c.CreateIssue(context.Background(), CreateIssueArgs{
		Repo:     "trilam/regatta",
		Title:    "[FIX] something",
		BodyFile: "/tmp/body.md",
		Labels:   []string{"ready-for-agent", "bug"},
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if url != "https://github.com/trilam/regatta/issues/1234" {
		t.Errorf("url: %q", url)
	}
	if len(fe.calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(fe.calls))
	}
	got := strings.Join(fe.calls[0], " ")
	for _, want := range []string{"issue", "create", "--repo trilam/regatta",
		"--title [FIX] something", "--body-file /tmp/body.md",
		"--label ready-for-agent", "--label bug"} {
		if !strings.Contains(got, want) {
			t.Errorf("call missing %q in: %s", want, got)
		}
	}
}

func TestCreateIssueReturnsErrorOnFailure(t *testing.T) {
	fe := &fakeExec{respErr: errors.New("gh: not authenticated")}
	c := &Client{Exec: fe}

	_, err := c.CreateIssue(context.Background(), CreateIssueArgs{
		Repo: "x/y", Title: "t", BodyFile: "/tmp/b",
	})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestViewPRReturnsJSON(t *testing.T) {
	fe := &fakeExec{respOut: `{"number":1112,"state":"OPEN","mergedAt":null}`}
	c := &Client{Exec: fe}

	got, err := c.ViewPR(context.Background(), "trilam/regatta", 1112,
		[]string{"number", "state", "mergedAt"})
	if err != nil {
		t.Fatal(err)
	}
	if got["state"] != "OPEN" {
		t.Errorf("state: %v", got["state"])
	}
	call := strings.Join(fe.calls[0], " ")
	if !strings.Contains(call, "--json number,state,mergedAt") {
		t.Errorf("json fields: %s", call)
	}
}
