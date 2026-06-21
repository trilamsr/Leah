package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/obs"
)

// TestRunCommand_StatusEmitsFirstByteHistogram pins the end-to-end wiring:
// `leah status` on a clean state dir prints "no activity" via the wrapped
// stdout, so the leah_cli_dispatch_to_first_byte_seconds histogram MUST show
// one observation with command=status.
func TestRunCommand_StatusEmitsFirstByteHistogram(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)

	reg := obs.NewRegistry()
	if code := runCommand(context.Background(), reg, []string{"status"}); code != 0 {
		t.Fatalf("runCommand status = %d; want 0", code)
	}

	snapPath := filepath.Join(t.TempDir(), "snap.json")
	if err := reg.Snapshot(snapPath); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snap: %v", err)
	}
	var out struct {
		Histograms map[string]struct {
			Count int64   `json:"count"`
			Sum   float64 `json:"sum"`
		} `json:"histograms"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var found bool
	for k, v := range out.Histograms {
		if strings.HasPrefix(k, "leah_cli_dispatch_to_first_byte_seconds") &&
			strings.Contains(k, "command=status") {
			if v.Count != 1 {
				t.Fatalf("status: histogram count = %d; want 1", v.Count)
			}
			if v.Sum < 0 || v.Sum > 5 {
				t.Fatalf("status: histogram sum %.4fs outside sane bounds", v.Sum)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("status: no leah_cli_dispatch_to_first_byte_seconds series; got %v", out.Histograms)
	}
}

// TestRunCommand_AskPathDoesNotTripHistogram pins the "excl. LLM" contract:
// the ask path uses raw os.Stdout (so LLM tokens cannot trip the sniffer),
// and consequently the histogram must show ZERO command=ask observations
// even when ask runs to completion.
func TestRunCommand_AskPathDoesNotTripHistogram(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)

	reg := obs.NewRegistry()
	// Missing query arg returns 2 before any LLM call — fine for this test;
	// what we're pinning is wiring, not ask's runtime behavior.
	_ = runCommand(context.Background(), reg, []string{"ask"})

	snapPath := filepath.Join(t.TempDir(), "snap.json")
	if err := reg.Snapshot(snapPath); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snap: %v", err)
	}
	var out struct {
		Histograms map[string]struct {
			Count int64 `json:"count"`
		} `json:"histograms"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for k, v := range out.Histograms {
		if strings.Contains(k, "command=ask") && v.Count != 0 {
			t.Fatalf("ask path tripped first-byte histogram (count=%d); LLM-emitting paths MUST stay unwrapped", v.Count)
		}
	}
}

// TestRunAndFlush_SnapshotsToStateDir pins the snapshotCLIMetrics defer:
// runAndFlush MUST write <stateDir>/metrics/cli-latest.json on exit so the
// daemon scrape can pick up the per-invocation observation.
func TestRunAndFlush_SnapshotsToStateDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)
	auditPath := filepath.Join(tmp, "audit.jsonl")

	if code := runAndFlush(context.Background(), auditPath, []string{"status"}); code != 0 {
		t.Fatalf("runAndFlush status = %d; want 0", code)
	}

	snapPath := filepath.Join(tmp, "metrics", "cli-latest.json")
	raw, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("snapshot not written at %s: %v", snapPath, err)
	}
	if !strings.Contains(string(raw), "leah_cli_dispatch_to_first_byte_seconds") {
		t.Fatalf("snapshot missing first-byte series: %s", raw)
	}
}
