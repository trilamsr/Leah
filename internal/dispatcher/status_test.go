package dispatcher

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestStatusReadsRecentEntries(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/audit.jsonl"
	lines := `{"ts":"2026-06-09T10:00:00Z","kind":"ask","args_hash":"abc","blast_radius":0,"outcome":"success","cost_dollars":0.012}
{"ts":"2026-06-09T10:05:00Z","kind":"ship","args_hash":"def","blast_radius":3,"outcome":"success","cost_dollars":0.087,"detail":"https://github.com/x/y/issues/1"}
{"ts":"2026-06-09T10:10:00Z","kind":"review","args_hash":"pr-1","blast_radius":3,"outcome":"success","detail":"APPROVE a1234567890abcdef"}
`
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	out := &bytes.Buffer{}
	s := &Status{AuditPath: path, Out: out, Limit: 10}
	if err := s.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, want := range []string{"ask", "ship", "review", "APPROVE"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q in: %s", want, out.String())
		}
	}
}

func TestStatusHandlesEmptyAudit(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/audit.jsonl"
	_ = os.WriteFile(path, []byte(""), 0o600)

	out := &bytes.Buffer{}
	s := &Status{AuditPath: path, Out: out, Limit: 10}
	if err := s.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "no activity") {
		t.Errorf("empty output: %q", out.String())
	}
}

func TestStatusHandlesMissingAudit(t *testing.T) {
	out := &bytes.Buffer{}
	s := &Status{AuditPath: "/nonexistent/path.jsonl", Out: out, Limit: 10}
	if err := s.Run(); err != nil {
		t.Fatalf("run should not fail on missing audit: %v", err)
	}
	if !strings.Contains(out.String(), "no activity") {
		t.Errorf("missing-file output: %q", out.String())
	}
}
