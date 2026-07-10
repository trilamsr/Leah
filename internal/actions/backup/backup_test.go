package backup

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// fakeExec records every Run/LookPath call and returns canned answers.
// Mirrors internal/voice fakeExec — no real restic binary required.
type fakeExec struct {
	mu       sync.Mutex
	runs     []runCall
	runErr   map[string]error
	runOut   map[string][]byte
	lookupOK map[string]bool
}

type runCall struct {
	name string
	args []string
}

func newFakeExec() *fakeExec {
	return &fakeExec{
		runErr:   map[string]error{},
		runOut:   map[string][]byte{},
		lookupOK: map[string]bool{},
	}
}

func (f *fakeExec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, runCall{name: name, args: args})
	return f.runOut[name], f.runErr[name]
}

func (f *fakeExec) LookPath(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lookupOK[name] {
		return "/usr/local/bin/" + name, nil
	}
	return "", errors.New("not found")
}

// TestInitCallsExpectedRestic asserts Init shells `restic -r <repo> init`.
func TestInitCallsExpectedRestic(t *testing.T) {
	fe := newFakeExec()
	r := &Restic{Exec: fe}
	if err := r.Init(context.Background(), "/tmp/repo"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if len(fe.runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(fe.runs))
	}
	if fe.runs[0].name != "restic" {
		t.Errorf("bin = %q, want restic", fe.runs[0].name)
	}
	if !contains(fe.runs[0].args, "init") || !contains(fe.runs[0].args, "/tmp/repo") {
		t.Errorf("args missing init/repo: %v", fe.runs[0].args)
	}
}

// TestSnapshotCallsExpectedRestic asserts Snapshot shells
// `restic -r <repo> backup <statePath>`.
func TestSnapshotCallsExpectedRestic(t *testing.T) {
	fe := newFakeExec()
	r := &Restic{Exec: fe}
	if err := r.Snapshot(context.Background(), "/tmp/repo", "/home/.leah-state"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(fe.runs) != 1 || fe.runs[0].name != "restic" {
		t.Fatalf("runs = %v", fe.runs)
	}
	args := fe.runs[0].args
	if !contains(args, "backup") || !contains(args, "/home/.leah-state") || !contains(args, "/tmp/repo") {
		t.Errorf("args missing backup/path/repo: %v", args)
	}
}

// TestRestoreCallsExpectedRestic asserts Restore shells
// `restic -r <repo> restore latest --target <target>`.
func TestRestoreCallsExpectedRestic(t *testing.T) {
	fe := newFakeExec()
	r := &Restic{Exec: fe}
	if err := r.Restore(context.Background(), "/tmp/repo", "/tmp/restored"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(fe.runs) != 1 {
		t.Fatalf("runs = %d", len(fe.runs))
	}
	args := fe.runs[0].args
	if !contains(args, "restore") || !contains(args, "latest") ||
		!contains(args, "--target") || !contains(args, "/tmp/restored") {
		t.Errorf("args missing restore/latest/--target/path: %v", args)
	}
}

// TestVerifyCallsExpectedRestic asserts Verify shells `restic -r <repo> check`.
func TestVerifyCallsExpectedRestic(t *testing.T) {
	fe := newFakeExec()
	r := &Restic{Exec: fe}
	if err := r.Verify(context.Background(), "/tmp/repo"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(fe.runs) != 1 || !contains(fe.runs[0].args, "check") {
		t.Errorf("expected check call, got %v", fe.runs)
	}
}

// TestSnapshotPropagatesError wraps the binary's stderr in a friendly error.
func TestSnapshotPropagatesError(t *testing.T) {
	fe := newFakeExec()
	fe.runErr["restic"] = errors.New("restic: locked repo")
	r := &Restic{Exec: fe}
	err := r.Snapshot(context.Background(), "/tmp/repo", "/x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "snapshot") {
		t.Errorf("error missing operation context: %v", err)
	}
}

// TestRejectEmptyArgs catches the empty-repo footgun (restic would interpret
// missing repo as $RESTIC_REPOSITORY, silently writing to the wrong target).
func TestRejectEmptyArgs(t *testing.T) {
	fe := newFakeExec()
	r := &Restic{Exec: fe}
	if err := r.Init(context.Background(), ""); err == nil {
		t.Error("init empty repo: want error")
	}
	if err := r.Snapshot(context.Background(), "", "/x"); err == nil {
		t.Error("snapshot empty repo: want error")
	}
	if err := r.Snapshot(context.Background(), "/r", ""); err == nil {
		t.Error("snapshot empty path: want error")
	}
	if err := r.Restore(context.Background(), "/r", ""); err == nil {
		t.Error("restore empty target: want error")
	}
}

// TestAvailable returns true when `restic` is on PATH.
func TestAvailable(t *testing.T) {
	fe := newFakeExec()
	fe.lookupOK["restic"] = true
	r := &Restic{Exec: fe}
	if !r.Available() {
		t.Error("Available = false, want true")
	}
	fe2 := newFakeExec()
	r2 := &Restic{Exec: fe2}
	if r2.Available() {
		t.Error("Available = true, want false")
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
