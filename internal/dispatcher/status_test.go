package dispatcher

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func TestStatusJSONEmitsArray(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/audit.jsonl"
	lines := `{"ts":"2026-06-09T10:00:00Z","kind":"ask","args_hash":"abc","blast_radius":0,"outcome":"success","cost_dollars":0.012}
{"ts":"2026-06-09T10:05:00Z","kind":"ship","args_hash":"def","blast_radius":3,"outcome":"pending","detail":"https://github.com/x/y/issues/1"}
`
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}

	out := &bytes.Buffer{}
	s := &Status{AuditPath: path, Out: out, Limit: 10, JSON: true}
	if err := s.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("parse output: %v\n%s", err, out.String())
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	if got[0]["kind"] != "ask" || got[1]["kind"] != "ship" {
		t.Errorf("entries: %+v", got)
	}
}

func TestStatusTruncatesLongDetailInTableMode(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/audit.jsonl"
	longDetail := strings.Repeat("x", 200)
	lines := fmt.Sprintf(`{"ts":"2026-06-09T10:00:00Z","kind":"ask","args_hash":"abc","blast_radius":0,"outcome":"failed","detail":%q}`+"\n", longDetail)
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	s := &Status{AuditPath: path, Out: out, Limit: 10}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), strings.Repeat("x", 100)) {
		t.Errorf("detail not truncated: %s", out.String())
	}
	if !strings.Contains(out.String(), "...") {
		t.Errorf("missing ellipsis: %s", out.String())
	}
}

func TestStatusJSONEmptyAuditEmitsEmptyArray(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/audit.jsonl"
	_ = os.WriteFile(path, []byte(""), 0o600)

	out := &bytes.Buffer{}
	s := &Status{AuditPath: path, Out: out, Limit: 10, JSON: true}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	trimmed := strings.TrimSpace(out.String())
	if trimmed != "[]" && trimmed != "null" {
		t.Errorf("empty want [] or null, got %q", trimmed)
	}
}
