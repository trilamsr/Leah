package riskscore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type row struct {
	TS      string `json:"ts"`
	Kind    string `json:"kind"`
	Outcome string `json:"outcome"`
}

func writeAudit(t *testing.T, rows []row) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "audit.jsonl")
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	return p
}

// TestFailureRate_BootstrapBelow10Samples pins §5 prior: under-sampled Kinds return 0.1 regardless of empirical rate.
func TestFailureRate_BootstrapBelow10Samples(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	var rows []row
	// 5 same-kind rows, all failures — empirical rate would be 1.0 but bootstrap fires.
	for i := 0; i < 5; i++ {
		rows = append(rows, row{
			TS:      now.Add(-time.Duration(i+1) * time.Hour).Format(time.RFC3339),
			Kind:    "self-build",
			Outcome: "block",
		})
	}
	path := writeAudit(t, rows)
	rate, samples := FailureRate(path, "self-build", now.Add(-90*24*time.Hour))
	if samples != 5 {
		t.Fatalf("samples=%d want 5", samples)
	}
	if rate != 0.1 {
		t.Fatalf("rate=%v want 0.1 (bootstrap prior)", rate)
	}
}

// TestFailureRate_90dWindow pins §5: rows older than `since` are excluded; rows inside window contribute.
func TestFailureRate_90dWindow(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	since := now.Add(-90 * 24 * time.Hour)
	var rows []row
	// 6 in-window failures.
	for i := 0; i < 6; i++ {
		rows = append(rows, row{
			TS:      now.Add(-time.Duration(i+1) * time.Hour).Format(time.RFC3339),
			Kind:    "self-build",
			Outcome: "block",
		})
	}
	// 6 in-window successes.
	for i := 0; i < 6; i++ {
		rows = append(rows, row{
			TS:      now.Add(-time.Duration(i+10) * time.Hour).Format(time.RFC3339),
			Kind:    "self-build",
			Outcome: "merged",
		})
	}
	// 100 out-of-window failures (would skew rate to ~0.94 if window ignored).
	for i := 0; i < 100; i++ {
		rows = append(rows, row{
			TS:      now.Add(-100 * 24 * time.Hour).Format(time.RFC3339),
			Kind:    "self-build",
			Outcome: "failed",
		})
	}
	// Other-kind row inside window (must be ignored).
	rows = append(rows, row{TS: now.Format(time.RFC3339), Kind: "other", Outcome: "block"})

	path := writeAudit(t, rows)
	rate, samples := FailureRate(path, "self-build", since)
	if samples != 12 {
		t.Fatalf("samples=%d want 12 (in-window same-kind only)", samples)
	}
	if rate != 0.5 {
		t.Fatalf("rate=%v want 0.5 (6/12)", rate)
	}
}
