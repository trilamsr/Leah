package spotlight

import (
	"context"
	"errors"
	"testing"
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

func TestSpotlight_Name(t *testing.T) {
	s, err := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Name(); got != "Spotlight" {
		t.Fatalf("Name=%q want Spotlight", got)
	}
}

func TestSpotlight_New_RequiresDeps(t *testing.T) {
	if _, err := New(Config{Exec: &fakeExec{}}); err == nil {
		t.Fatal("want error on missing Attestor")
	}
	if _, err := New(Config{Attestor: &fakeAttestor{}}); err == nil {
		t.Fatal("want error on missing Exec")
	}
}

func TestSpotlight_Available_True(t *testing.T) {
	// `sh` is universally on PATH; Bin override avoids depending on macOS-only mdfind.
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}, Bin: "sh"})
	if !s.Available(context.Background()) {
		t.Fatal("Available=false; want true when binary exists on PATH")
	}
}

func TestSpotlight_Available_False(t *testing.T) {
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: &fakeExec{}, Bin: "definitely-not-a-real-binary-zzz"})
	if s.Available(context.Background()) {
		t.Fatal("Available=true; want false when binary missing from PATH")
	}
}

// Denial must short-circuit BEFORE the subprocess is launched — otherwise a
// leaked Spotlight result enters memory before the operator says no.
func TestSpotlight_Query_AttestationDenied_NoExec(t *testing.T) {
	att := &fakeAttestor{err: errors.New("no")}
	ex := &fakeExec{}
	s, _ := New(Config{Attestor: att, Exec: ex})
	_, err := s.Query(context.Background(), Query{Text: "hello"})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if ex.calls != 0 {
		t.Fatalf("exec ran %d times after denial", ex.calls)
	}
}

func TestSpotlight_Query_EmptyTextRejected(t *testing.T) {
	att := &fakeAttestor{}
	ex := &fakeExec{}
	s, _ := New(Config{Attestor: att, Exec: ex})
	_, err := s.Query(context.Background(), Query{Text: "   "})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("err=%v want ErrInvalidQuery", err)
	}
	if att.calls != 0 {
		t.Fatalf("attestor called %d times for invalid query", att.calls)
	}
}

func TestSpotlight_Query_HappyPath(t *testing.T) {
	ex := &fakeExec{stdout: []byte("/Users/a/notes/foo.md\x00/Users/a/bar baz.txt\x00")}
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	items, err := s.Query(context.Background(), Query{Text: "foo"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len=%d want 2", len(items))
	}
	if items[0].URL != "file:///Users/a/notes/foo.md" || items[0].Title != "foo.md" {
		t.Fatalf("row0=%+v", items[0])
	}
	if items[1].Title != "bar baz.txt" {
		t.Fatalf("row1 title=%q want bar baz.txt", items[1].Title)
	}
	if ex.last.name != "mdfind" {
		t.Fatalf("exec name=%q want mdfind", ex.last.name)
	}
	if len(ex.last.args) != 2 || ex.last.args[0] != "-0" || ex.last.args[1] != "foo" {
		t.Fatalf("exec args=%v want [-0 foo]", ex.last.args)
	}
}

func TestSpotlight_Query_LimitApplied(t *testing.T) {
	ex := &fakeExec{stdout: []byte("/a\x00/b\x00/c\x00")}
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	items, err := s.Query(context.Background(), Query{Text: "x", Limit: 2})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len=%d want 2 (limit)", len(items))
	}
}

func TestSpotlight_Query_ExecError(t *testing.T) {
	ex := &fakeExec{err: errors.New("boom")}
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	_, err := s.Query(context.Background(), Query{Text: "x"})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("err=%v want ErrSourceUnavailable", err)
	}
}

// TCC denials surface as ErrPermissionDenied so callers can prompt for access
// instead of generic source-unavailable retries.
func TestSpotlight_Query_PermissionDenied(t *testing.T) {
	ex := &fakeExec{stderr: []byte("mdfind: error -1743 Not authorized"), err: errors.New("exit 1")}
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	_, err := s.Query(context.Background(), Query{Text: "x"})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("err=%v want ErrPermissionDenied", err)
	}
}

func TestSpotlight_Query_EmptyResult(t *testing.T) {
	ex := &fakeExec{stdout: nil}
	s, _ := New(Config{Attestor: &fakeAttestor{}, Exec: ex})
	items, err := s.Query(context.Background(), Query{Text: "nothing"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len=%d want 0", len(items))
	}
}
