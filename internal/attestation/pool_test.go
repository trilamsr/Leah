package attestation

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Sentinel + happy-path file-load: count matches non-blank, non-comment lines.
func TestPool_LoadFromFile_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "questions.txt")
	body := "# header comment\n" +
		"What is the biggest risk?\n" +
		"\n" +
		"Which test catches a regression?\n" +
		"# trailing comment\n" +
		"Who owns the rollback?\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	p, err := Load(path, "self-build")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := p.Len(); got != 3 {
		t.Errorf("Len = %d, want 3", got)
	}
}

// Same scope → same question across calls; load-bearing for audit-row replay.
func TestPool_PickByScope_Deterministic(t *testing.T) {
	p := mustPool(t, []string{"q1", "q2", "q3", "q4", "q5"}, []string{"self-build"})
	first, err := p.Pick("self-build")
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	for i := 0; i < 10; i++ {
		got, err := p.Pick("self-build")
		if err != nil {
			t.Fatalf("Pick iter %d: %v", i, err)
		}
		if got != first {
			t.Fatalf("Pick non-deterministic: iter %d returned %q, first was %q", i, got, first)
		}
	}
}

// Distinct scopes must yield distinct questions — chosen scopes don't collide on this fixture.
func TestPool_PickByScope_DistinctScopes_DistinctQuestions(t *testing.T) {
	scopes := []string{"gmail.list", "gmail.send", "gcal.create", "self-build"}
	p := mustPool(t, []string{"q1", "q2", "q3", "q4", "q5", "q6", "q7", "q8"}, scopes)
	seen := map[string]string{}
	for _, sc := range scopes {
		q, err := p.Pick(sc)
		if err != nil {
			t.Fatalf("Pick(%q): %v", sc, err)
		}
		if prev, ok := seen[q]; ok {
			t.Fatalf("collision: scopes %q and %q both picked %q", prev, sc, q)
		}
		seen[q] = sc
	}
}

// Unknown scope must surface ErrUnknownScope, not silently fall back to questions[0].
func TestPool_UnknownScope_ReturnsError(t *testing.T) {
	p := mustPool(t, []string{"q1", "q2"}, []string{"self-build"})
	got, err := p.Pick("not-registered")
	if !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("err = %v, want ErrUnknownScope", err)
	}
	if got != "" {
		t.Errorf("returned %q, want empty on error", got)
	}
}

// Missing file must surface a wrapped error, not nil — fail-closed.
func TestPool_LoadMissingFile_ReturnsError(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.txt"), "self-build")
	if err == nil {
		t.Fatal("Load missing file: want error, got nil")
	}
}

// Empty pool (all-comments / blank file) must error — operator-habituation defense.
func TestPool_LoadEmptyPool_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(path, []byte("# only comments\n\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Load(path, "self-build")
	if !errors.Is(err, ErrEmptyPool) {
		t.Fatalf("err = %v, want ErrEmptyPool", err)
	}
}

// mustPool builds a pool from in-memory fixtures via the test helper.
func mustPool(t *testing.T, questions []string, scopes []string) *Pool {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "q.txt")
	var body string
	for _, q := range questions {
		body += q + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	p, err := Load(path, scopes...)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return p
}
