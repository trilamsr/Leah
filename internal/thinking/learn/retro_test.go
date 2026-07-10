package learn

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
)

// TestRetroIncludesAttestationViolations asserts the H3 gate section
// renders a per-PR bullet when the wired AttestationScanner returns
// violations; otherwise prints "(none)".
func TestRetroIncludesAttestationViolations(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	dbPath := filepath.Join(dir, "memory.db")

	store, err := OpenMistakeStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Hit: scanner returns 1 violation.
	r := &Retro{
		AuditPath: auditPath,
		Store:     store,
		Now:       func() time.Time { return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC) },
		AttestationScanner: func(_ context.Context, _ string) ([]AttestationViolation, error) {
			return []AttestationViolation{{
				Repo:     "trilamsr/Leah",
				PRNumber: 42,
				URL:      "https://github.com/trilamsr/Leah/pull/42",
			}}, nil
		},
	}
	md, err := r.Generate("")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(md, "## Attestation gate violations") {
		t.Errorf("missing attestation header: %s", md)
	}
	if !strings.Contains(md, "https://github.com/trilamsr/Leah/pull/42") {
		t.Errorf("missing PR URL bullet: %s", md)
	}

	// Empty: scanner returns no violations → (none) bullet.
	r.AttestationScanner = func(_ context.Context, _ string) ([]AttestationViolation, error) {
		return nil, nil
	}
	md, err = r.Generate("")
	if err != nil {
		t.Fatalf("Generate (empty): %v", err)
	}
	if !strings.Contains(md, "## Attestation gate violations") || !strings.Contains(md, "(none)") {
		t.Errorf("empty scanner output should render '(none)': %s", md)
	}
}

// TestRetroIncludesMistakes asserts the markdown retro report contains a
// mistakes section listing logged root_cause entries for the requested
// ISO week (spec §4.1).
func TestRetroIncludesMistakes(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	dbPath := filepath.Join(dir, "memory.db")

	// week 2026-W23 = 2026-06-01 (Mon) .. 2026-06-07 (Sun)
	weekTime := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)

	logger := &audit.Logger{
		Path: auditPath,
		Now:  func() time.Time { return weekTime },
	}
	if err := logger.Append(audit.Entry{
		Kind:        "ship",
		ArgsHash:    "abc",
		BlastRadius: 3,
		Outcome:     "success",
		CostDollars: 0.18,
		Detail:      "https://github.com/foo/bar/pull/42",
	}); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	store, err := OpenMistakeStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	store.Now = func() time.Time { return weekTime }
	if _, err := store.Add(Mistake{
		AuditTS:    weekTime.Format(time.RFC3339),
		AuditKind:  "ship",
		AuditHash:  "abc",
		RootCause:  "wrong-pr",
		Prevention: "double-check the repo arg",
	}); err != nil {
		t.Fatalf("Add mistake: %v", err)
	}

	r := &Retro{
		AuditPath: auditPath,
		Store:     store,
		Now:       func() time.Time { return weekTime },
	}
	md, err := r.Generate("2026-23")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(md, "2026-23") {
		t.Errorf("must include week tag: %s", md)
	}
	if !strings.Contains(md, "## Mistakes") {
		t.Errorf("must include Mistakes header: %s", md)
	}
	if !strings.Contains(md, "wrong-pr") {
		t.Errorf("must include logged root_cause: %s", md)
	}
	if !strings.Contains(md, "double-check") {
		t.Errorf("must include prevention text: %s", md)
	}
	if !strings.Contains(md, "## Wins") {
		t.Errorf("must include Wins header: %s", md)
	}
}
