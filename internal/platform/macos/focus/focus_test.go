package focus

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeAttestor struct {
	err   error
	calls int
}

func (f *fakeAttestor) Attest(_ context.Context, _ string) error {
	f.calls++
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

func TestFocus_Name(t *testing.T) {
	f, err := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Name(); got != "Focus" {
		t.Fatalf("Name=%q want Focus", got)
	}
}

func TestFocus_New_RequiresDeps(t *testing.T) {
	if _, err := New(Config{Exec: &fakeExec{}}); err == nil {
		t.Fatal("want error on missing Attestor")
	}
	if _, err := New(Config{Attestor: &fakeAttestor{}}); err == nil {
		t.Fatal("want error on missing Exec")
	}
}

func TestFocus_Available_True(t *testing.T) {
	f, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{stdout: []byte("{}")}})
	if !f.Available(context.Background()) {
		t.Fatal("Available=false; want true")
	}
}

func TestFocus_Available_False(t *testing.T) {
	f, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{err: errors.New("no domain")}})
	if f.Available(context.Background()) {
		t.Fatal("Available=true; want false on exec error")
	}
}

// Denial must short-circuit BEFORE the subprocess is launched — otherwise
// Focus mode (potentially revealing meeting/sleep schedule) leaks before
// the operator says no.
func TestFocus_Query_AttestationDenied_NoExec(t *testing.T) {
	att := &fakeAttestor{err: errors.New("no")}
	ex := &fakeExec{}
	f, _ := New(Config{Attestor: att, Exec: ex})
	_, err := f.Query(context.Background())
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if ex.calls != 0 {
		t.Fatalf("exec ran %d times after denial", ex.calls)
	}
}

func TestFocus_Query_HappyPath_Active(t *testing.T) {
	out := `{
    active = 1;
    mode = "Work";
    dndStart = "2026-06-10 09:00:00 +0000";
}`
	ex := &fakeExec{stdout: []byte(out)}
	f, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	s, err := f.Query(context.Background())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !s.Active {
		t.Fatalf("Active=false; want true")
	}
	if s.Mode != "Work" {
		t.Fatalf("Mode=%q want Work", s.Mode)
	}
	want := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	if !s.ActiveSince.Equal(want) {
		t.Fatalf("ActiveSince=%v want %v", s.ActiveSince, want)
	}
	if ex.last.name != "defaults" {
		t.Fatalf("exec name=%q want defaults", ex.last.name)
	}
}

func TestFocus_Query_HappyPath_Inactive(t *testing.T) {
	out := `{
    active = 0;
}`
	ex := &fakeExec{stdout: []byte(out)}
	f, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	s, err := f.Query(context.Background())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if s.Active {
		t.Fatal("Active=true; want false")
	}
	if !s.ActiveSince.IsZero() {
		t.Fatalf("ActiveSince=%v want zero", s.ActiveSince)
	}
}

func TestFocus_Query_ExecError(t *testing.T) {
	ex := &fakeExec{err: errors.New("boom")}
	f, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	_, err := f.Query(context.Background())
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

// Unknown plist keys must not break the parser — Apple adds fields between
// macOS minor releases.
func TestFocus_Query_UnknownKeysIgnored(t *testing.T) {
	out := `{
    active = 1;
    mode = "Personal";
    futureKey = "something";
    nested = {
        irrelevant = 1;
    };
}`
	ex := &fakeExec{stdout: []byte(out)}
	f, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	s, err := f.Query(context.Background())
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if !s.Active || s.Mode != "Personal" {
		t.Fatalf("state=%+v want Active=true Mode=Personal", s)
	}
}

// TCC denials surface as ErrPermissionDenied so callers route to the
// permissions-help prompt instead of generic source-unavailable retries.
func TestFocus_Query_PermissionDenied(t *testing.T) {
	ex := &fakeExec{stderr: []byte("defaults read: -1743 Not authorized"), err: errors.New("exit 1")}
	f, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	_, err := f.Query(context.Background())
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err=%v want ErrPermissionDenied", err)
	}
}
