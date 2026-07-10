package learn

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
)

// TestDanglingSelfBuild_DetectsAfter7d asserts a dispatched row 8d old with no paired outcome surfaces a candidate.
func TestDanglingSelfBuild_DetectsAfter7d(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	dispatchTS := now.Add(-8 * 24 * time.Hour)

	logger := &audit.Logger{Path: auditPath, Now: func() time.Time { return dispatchTS }}
	mustAppend(t, logger, audit.Entry{
		Kind:        "self-build",
		ArgsHash:    "h1",
		BlastRadius: 4,
		Outcome:     "dispatched",
		Detail:      "prompt_sha=abc url=https://github.com/trilamsr/Leah/issues/99",
	})

	got, err := DetectDanglingSelfBuild(auditPath, func() time.Time { return now })
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 dangling candidate, got %d: %+v", len(got), got)
	}
	if got[0].IssueURL != "https://github.com/trilamsr/Leah/issues/99" {
		t.Errorf("issue url: got %q", got[0].IssueURL)
	}
	if got[0].ArgsHash != "h1" {
		t.Errorf("args hash: got %q", got[0].ArgsHash)
	}
}

// TestDanglingSelfBuild_QuietBefore7d asserts a dispatched row 6d old does not surface yet.
func TestDanglingSelfBuild_QuietBefore7d(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	dispatchTS := now.Add(-6 * 24 * time.Hour)

	logger := &audit.Logger{Path: auditPath, Now: func() time.Time { return dispatchTS }}
	mustAppend(t, logger, audit.Entry{
		Kind:        "self-build",
		ArgsHash:    "h1",
		BlastRadius: 4,
		Outcome:     "dispatched",
		Detail:      "url=https://github.com/trilamsr/Leah/issues/77",
	})

	got, err := DetectDanglingSelfBuild(auditPath, func() time.Time { return now })
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 candidates (still within 7d), got %d: %+v", len(got), got)
	}
}

// TestDanglingSelfBuild_PairedOutcomeIgnored asserts a dispatched row with a matching self-build.outcome is not flagged.
func TestDanglingSelfBuild_PairedOutcomeIgnored(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	dispatchTS := now.Add(-10 * 24 * time.Hour)
	outcomeTS := dispatchTS.Add(2 * time.Hour)

	mustAppend(t, &audit.Logger{Path: auditPath, Now: func() time.Time { return dispatchTS }}, audit.Entry{
		Kind:        "self-build",
		ArgsHash:    "h1",
		BlastRadius: 4,
		Outcome:     "dispatched",
		Detail:      "url=https://github.com/trilamsr/Leah/issues/42",
	})
	mustAppend(t, &audit.Logger{Path: auditPath, Now: func() time.Time { return outcomeTS }}, audit.Entry{
		Kind:        "self-build.outcome",
		ArgsHash:    "h1",
		BlastRadius: 4,
		Outcome:     "MERGED",
		Detail:      "state=MERGED url=https://github.com/trilamsr/Leah/issues/42",
	})

	got, err := DetectDanglingSelfBuild(auditPath, func() time.Time { return now })
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("paired outcome must suppress candidate, got %d: %+v", len(got), got)
	}
}

// TestDanglingSelfBuild_MultipleDispatchedSameURL asserts a re-dispatched URL with one paired outcome is fully resolved.
func TestDanglingSelfBuild_MultipleDispatchedSameURL(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	firstTS := now.Add(-12 * 24 * time.Hour)
	secondTS := now.Add(-9 * 24 * time.Hour)
	outcomeTS := now.Add(-1 * 24 * time.Hour)

	mustAppend(t, &audit.Logger{Path: auditPath, Now: func() time.Time { return firstTS }}, audit.Entry{
		Kind: "self-build", ArgsHash: "h1", BlastRadius: 4, Outcome: "dispatched",
		Detail: "url=https://github.com/trilamsr/Leah/issues/55",
	})
	mustAppend(t, &audit.Logger{Path: auditPath, Now: func() time.Time { return secondTS }}, audit.Entry{
		Kind: "self-build", ArgsHash: "h2", BlastRadius: 4, Outcome: "dispatched",
		Detail: "url=https://github.com/trilamsr/Leah/issues/55",
	})
	mustAppend(t, &audit.Logger{Path: auditPath, Now: func() time.Time { return outcomeTS }}, audit.Entry{
		Kind: "self-build.outcome", ArgsHash: "h2", BlastRadius: 4, Outcome: "MERGED",
		Detail: "state=MERGED url=https://github.com/trilamsr/Leah/issues/55",
	})

	got, err := DetectDanglingSelfBuild(auditPath, func() time.Time { return now })
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	// One outcome row for the URL clears every dispatched row for the same URL —
	// retro defect-signal asks "did this self-build ever finish?", and one terminal
	// answer suffices. Per-args_hash matching would re-flag the first dispatch as a
	// "lost attempt", which the operator already knows about (the URL is the same).
	if len(got) != 0 {
		t.Fatalf("outcome on same URL must clear earlier dispatches, got %d: %+v", len(got), got)
	}
}

func mustAppend(t *testing.T, l *audit.Logger, e audit.Entry) {
	t.Helper()
	if err := l.Append(e); err != nil {
		t.Fatalf("append: %v", err)
	}
}
