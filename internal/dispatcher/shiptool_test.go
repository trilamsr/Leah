package dispatcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/attest"
	"github.com/trilam/leah/internal/audit"
)

type fakeAttestor struct {
	scope string
	err   error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.scope = scope
	return f.err
}

type fakeWriter struct {
	calls int
	title string
	err   error
}

func (f *fakeWriter) Write(_ context.Context, title string) error {
	f.calls++
	f.title = title
	return f.err
}

func newToolAudit(t *testing.T) (*audit.Logger, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	return &audit.Logger{Path: p}, p
}

func TestShipTool_AttestsThenWrites(t *testing.T) {
	at := &fakeAttestor{}
	w := &fakeWriter{}
	a, path := newToolAudit(t)
	st := &ShipTool{Tool: "jira", Attestor: at, Writer: w, Audit: a, Out: os.Stderr}

	if err := st.Run(context.Background(), "ship the thing"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if at.scope != attest.ScopeShipTool {
		t.Errorf("attested scope = %q, want %q", at.scope, attest.ScopeShipTool)
	}
	if w.calls != 1 || w.title != "ship the thing" {
		t.Errorf("writer calls=%d title=%q, want 1 / %q", w.calls, w.title, "ship the thing")
	}
	if got := readAudit(t, path); !strings.Contains(got, `"outcome":"ok"`) || !strings.Contains(got, `"kind":"ship.jira"`) {
		t.Errorf("audit row = %s, want ok ship.jira", got)
	}
}

func TestShipTool_DeclinedDoesNotWrite(t *testing.T) {
	at := &fakeAttestor{err: errors.New("denied")}
	w := &fakeWriter{}
	a, path := newToolAudit(t)
	st := &ShipTool{Tool: "slack", Attestor: at, Writer: w, Audit: a, Out: os.Stderr}

	if err := st.Run(context.Background(), "post this"); err == nil {
		t.Fatal("Run: want error on declined attestation")
	}
	if w.calls != 0 {
		t.Errorf("writer called %d times after decline, want 0", w.calls)
	}
	if got := readAudit(t, path); !strings.Contains(got, `"outcome":"declined"`) {
		t.Errorf("audit row = %s, want declined", got)
	}
}

func TestShipTool_WriteFailureAudited(t *testing.T) {
	at := &fakeAttestor{}
	w := &fakeWriter{err: errors.New("boom")}
	a, path := newToolAudit(t)
	st := &ShipTool{Tool: "notion", Attestor: at, Writer: w, Audit: a, Out: os.Stderr}

	if err := st.Run(context.Background(), "x"); err == nil {
		t.Fatal("Run: want error on write failure")
	}
	if got := readAudit(t, path); !strings.Contains(got, `"outcome":"failed"`) {
		t.Errorf("audit row = %s, want failed", got)
	}
}

func readAudit(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	return string(b)
}
