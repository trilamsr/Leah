package selflearn

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
)

// stubRule returns a fixed verdict regardless of entry.
type stubRule struct {
	verdict Outcome
	detail  string
}

func (s stubRule) Resolve(_ context.Context, _ audit.Entry) (Outcome, string) {
	return s.verdict, s.detail
}

// TestResolverUpdatesPendingRow asserts that running the resolver against
// an audit log with one pending row of a known Kind appends one
// resolver.update row with the rule's verdict in Detail (per spec §2).
func TestResolverUpdatesPendingRow(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	logger := &audit.Logger{
		Path: auditPath,
		Now:  func() time.Time { return time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC) },
	}
	if err := logger.Append(audit.Entry{
		Kind:        "ship",
		ArgsHash:    "abc",
		BlastRadius: 3,
		Outcome:     "pending",
		Detail:      "https://github.com/foo/bar/pull/42",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var out bytes.Buffer
	r := &Resolver{
		AuditPath: auditPath,
		Logger: &audit.Logger{
			Path: auditPath,
			Now:  func() time.Time { return time.Date(2026, 6, 9, 11, 0, 0, 0, time.UTC) },
		},
		Rules: map[string]Rule{
			"ship": stubRule{verdict: OutcomeSuccess, detail: "state=MERGED"},
		},
		Since: 7 * 24 * time.Hour,
		Now:   func() time.Time { return time.Date(2026, 6, 9, 11, 0, 0, 0, time.UTC) },
		Out:   &out,
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	rows := readAll(t, auditPath)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows (orig + resolver.update), got %d: %+v", len(rows), rows)
	}
	got := rows[1]
	if got.Kind != "resolver.update" {
		t.Errorf("kind: want resolver.update, got %q", got.Kind)
	}
	if got.Outcome != string(OutcomeSuccess) {
		t.Errorf("outcome: want success, got %q", got.Outcome)
	}
	if !strings.Contains(got.Detail, "ship") || !strings.Contains(got.Detail, "abc") {
		t.Errorf("detail must reference orig (ship,abc): %q", got.Detail)
	}
	if !strings.Contains(got.Detail, "state=MERGED") {
		t.Errorf("detail must include probe result: %q", got.Detail)
	}
}

// TestResolverSkipsAlreadyResolved asserts the resolver does not double-resolve
// a pending row that already has a resolver.update entry (idempotency).
func TestResolverSkipsAlreadyResolved(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	logger := &audit.Logger{
		Path: auditPath,
		Now:  func() time.Time { return time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC) },
	}
	if err := logger.Append(audit.Entry{Kind: "ship", ArgsHash: "abc", Outcome: "pending"}); err != nil {
		t.Fatalf("seed orig: %v", err)
	}
	// Read back the orig timestamp so the dedup key matches.
	rowsBefore := readAll(t, auditPath)
	if len(rowsBefore) != 1 {
		t.Fatalf("seed: want 1 row, got %d", len(rowsBefore))
	}
	origKey := rowKey(rowsBefore[0])
	if err := logger.Append(audit.Entry{
		Kind:    "resolver.update",
		Outcome: "success",
		Detail:  "resolved " + origKey + " -> state=MERGED",
	}); err != nil {
		t.Fatalf("seed resolver.update: %v", err)
	}

	r := &Resolver{
		AuditPath: auditPath,
		Logger:    logger,
		Rules:     map[string]Rule{"ship": stubRule{verdict: OutcomeSuccess, detail: "state=MERGED"}},
		Since:     7 * 24 * time.Hour,
		Now:       func() time.Time { return time.Date(2026, 6, 9, 11, 0, 0, 0, time.UTC) },
		Out:       new(bytes.Buffer),
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	rows := readAll(t, auditPath)
	if len(rows) != 2 {
		t.Errorf("want 2 rows (no new resolver.update), got %d", len(rows))
	}
}

func readAll(t *testing.T, path string) []audit.Entry {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	var out []audit.Entry
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var e audit.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		out = append(out, e)
	}
	return out
}
