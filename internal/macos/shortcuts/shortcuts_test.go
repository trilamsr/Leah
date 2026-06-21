package shortcuts

import (
	"context"
	"errors"
	"testing"
)

type fakeAttestor struct {
	err    error
	calls  int
	scopes []string
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.calls++
	f.scopes = append(f.scopes, scope)
	return f.err
}

type fakeExec struct {
	stdout []byte
	stderr []byte
	err    error
	calls  int
	last   struct {
		name string
		args []string
	}
}

func (f *fakeExec) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls++
	f.last.name = name
	f.last.args = args
	return f.stdout, f.stderr, f.err
}

func TestShortcuts_Name(t *testing.T) {
	s, err := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Name(); got != "Shortcuts" {
		t.Fatalf("Name=%q want Shortcuts", got)
	}
}

func TestShortcuts_New_RequiresDeps(t *testing.T) {
	if _, err := New(Config{Exec: &fakeExec{}}); err == nil {
		t.Fatal("want error on missing Attestor")
	}
	if _, err := New(Config{Attestor: &fakeAttestor{}}); err == nil {
		t.Fatal("want error on missing Exec")
	}
}

func TestShortcuts_Available_True(t *testing.T) {
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}, Bin: "sh"})
	if !s.Available(context.Background()) {
		t.Fatal("Available=false; want true when binary exists on PATH")
	}
}

func TestShortcuts_Available_False(t *testing.T) {
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}, Bin: "definitely-not-a-real-binary-zzz"})
	if s.Available(context.Background()) {
		t.Fatal("Available=true; want false when binary missing from PATH")
	}
}

func TestShortcuts_List_HappyPath(t *testing.T) {
	ex := &fakeExec{stdout: []byte("Daily Briefing\nSend Location\n\nStart Focus\n")}
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	names, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"Daily Briefing", "Send Location", "Start Focus"}
	if len(names) != len(want) {
		t.Fatalf("len=%d want %d (%v)", len(names), len(want), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names[%d]=%q want %q", i, names[i], want[i])
		}
	}
	if ex.last.name != "shortcuts" {
		t.Fatalf("exec name=%q want shortcuts", ex.last.name)
	}
	if len(ex.last.args) != 1 || ex.last.args[0] != "list" {
		t.Fatalf("exec args=%v want [list]", ex.last.args)
	}
}

func TestShortcuts_List_Empty(t *testing.T) {
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{stdout: nil}})
	names, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("len=%d want 0", len(names))
	}
}

func TestShortcuts_List_AttestationDenied_NoExec(t *testing.T) {
	att := &fakeAttestor{err: errors.New("no")}
	ex := &fakeExec{}
	s, _ := New(Config{Attestor: att, Exec: ex})
	if _, err := s.List(context.Background()); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if ex.calls != 0 {
		t.Fatalf("exec ran %d times after denial", ex.calls)
	}
}

func TestShortcuts_List_Scope(t *testing.T) {
	att := &fakeAttestor{}
	s, _ := New(Config{Attestor: att, Exec: &fakeExec{}})
	if _, err := s.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(att.scopes) != 1 || att.scopes[0] != ScopeList {
		t.Fatalf("scopes=%v want [%s]", att.scopes, ScopeList)
	}
}

func TestShortcuts_List_ExecError(t *testing.T) {
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{err: errors.New("boom")}})
	if _, err := s.List(context.Background()); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

func TestShortcuts_Run_HappyPath(t *testing.T) {
	ex := &fakeExec{}
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	if err := s.Run(context.Background(), "Daily Briefing"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ex.last.name != "shortcuts" {
		t.Fatalf("exec name=%q want shortcuts", ex.last.name)
	}
	if len(ex.last.args) != 2 || ex.last.args[0] != "run" || ex.last.args[1] != "Daily Briefing" {
		t.Fatalf("exec args=%v want [run Daily Briefing]", ex.last.args)
	}
}

// Run is a side-effect action; its consent scope MUST differ from the read-only
// list scope so the operator can grant listing without granting execution.
func TestShortcuts_Run_Scope(t *testing.T) {
	att := &fakeAttestor{}
	s, _ := New(Config{Attestor: att, Exec: &fakeExec{}})
	if err := s.Run(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if len(att.scopes) != 1 || att.scopes[0] != ScopeRun {
		t.Fatalf("scopes=%v want [%s]", att.scopes, ScopeRun)
	}
	if ScopeRun == ScopeList {
		t.Fatal("run and list share a scope; action must gate separately")
	}
}

// Denial must short-circuit BEFORE the side effect fires.
func TestShortcuts_Run_AttestationDenied_NoExec(t *testing.T) {
	att := &fakeAttestor{err: errors.New("no")}
	ex := &fakeExec{}
	s, _ := New(Config{Attestor: att, Exec: ex})
	if err := s.Run(context.Background(), "x"); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if ex.calls != 0 {
		t.Fatalf("exec ran %d times after denial", ex.calls)
	}
}

func TestShortcuts_Run_EmptyNameRejected(t *testing.T) {
	att := &fakeAttestor{}
	ex := &fakeExec{}
	s, _ := New(Config{Attestor: att, Exec: ex})
	if err := s.Run(context.Background(), "  "); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err=%v want ErrInvalidName", err)
	}
	if att.calls != 0 {
		t.Fatalf("attestor called %d times for empty name", att.calls)
	}
}

func TestShortcuts_Run_NotFound(t *testing.T) {
	ex := &fakeExec{stderr: []byte("No shortcut named Foo"), err: errors.New("exit 1")}
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	if err := s.Run(context.Background(), "Foo"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

func TestShortcuts_Run_ExecError(t *testing.T) {
	ex := &fakeExec{stderr: []byte("kaboom"), err: errors.New("exit 1")}
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	if err := s.Run(context.Background(), "Foo"); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}
