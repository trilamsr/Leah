package git

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeExec records the last command and returns canned stdout/err.
type fakeExec struct {
	stdout []byte
	err    error
	gotCmd string
	gotArg []string
}

func (f *fakeExec) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.gotCmd = name
	f.gotArg = args
	return f.stdout, nil, f.err
}

func TestGitSource_RecentCommits_FiltersByAuthor(t *testing.T) {
	// Two commits separated by record-sep \x1e; fields by unit-sep \x1f.
	log := "abc123\x1ffix: parser bug\x1fextra body line\x1e" +
		"def456\x1ffeat: new flag\x1f\x1e"
	fx := &fakeExec{stdout: []byte(log)}
	s := New(Config{Exec: fx, Author: "tri@maydow.com", Dir: "/tmp/repo"})

	since := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	facts, err := s.Candidates(context.Background(), since)
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	if len(facts) != 2 {
		t.Fatalf("facts len = %d, want 2: %+v", len(facts), facts)
	}
	if facts[0].Slot != "commit" || facts[0].Origin != "abc123" {
		t.Errorf("fact[0] = %+v", facts[0])
	}
	if !strings.Contains(facts[0].Summary, "fix: parser bug") {
		t.Errorf("fact[0].Summary missing subject: %q", facts[0].Summary)
	}

	// Verify the shell-out used --author and an RFC3339 --since.
	got := strings.Join(fx.gotArg, " ")
	if !strings.Contains(got, "--author=tri@maydow.com") {
		t.Errorf("missing --author arg: %v", fx.gotArg)
	}
	if !strings.Contains(got, "--since=2026-06-20T00:00:00Z") {
		t.Errorf("missing --since RFC3339: %v", fx.gotArg)
	}
}

func TestGitSource_NoCommits_ReturnsEmpty(t *testing.T) {
	fx := &fakeExec{stdout: []byte("")}
	s := New(Config{Exec: fx, Author: "x@x", Dir: "/tmp"})
	facts, err := s.Candidates(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(facts) != 0 {
		t.Fatalf("want empty, got %+v", facts)
	}
}

func TestGitSource_ExecError_WrappedAsSourceError(t *testing.T) {
	fx := &fakeExec{err: errors.New("git: not a repo")}
	s := New(Config{Exec: fx, Author: "x@x", Dir: "/tmp"})
	_, err := s.Candidates(context.Background(), time.Now())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "git source") {
		t.Errorf("error not wrapped with package label: %v", err)
	}
}

func TestGitSource_Name(t *testing.T) {
	s := New(Config{Exec: &fakeExec{}, Author: "x@x", Dir: "/tmp"})
	if s.Name() != "git" {
		t.Errorf("Name = %q, want git", s.Name())
	}
}
