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

// TestRunCommand_LongRunningCommandsEmitProgress pins the wireup contract:
// commands flagged long-running in longRunningCLICommand MUST emit exactly one
// observation to leah_cli_dispatch_to_first_progress_seconds per invocation.
func TestRunCommand_LongRunningCommandsEmitProgress(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)

	reg := telemetry.NewRegistry()
	// Missing args returns 2 before any LLM call — fine; we're pinning the
	// pre-dispatch progress emit, not subcommand behavior.
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
	found := false
	for k, v := range out.Histograms {
		if strings.HasPrefix(k, "leah_cli_dispatch_to_first_progress_seconds") &&
			strings.Contains(k, "command=ask") {
			if v.Count != 1 {
				t.Fatalf("want count=1 for ask progress, got %d", v.Count)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("ask: no leah_cli_dispatch_to_first_progress_seconds series; got %v", out.Histograms)
	}
}

// TestRunCommand_ShortCommandsDoNotEmitProgress guards against widening the
// long-running list to commands whose latency is already covered by first-byte.
func TestRunCommand_ShortCommandsDoNotEmitProgress(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)

	reg := telemetry.NewRegistry()
	_ = runCommand(context.Background(), reg, []string{"status"})

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
		if strings.HasPrefix(k, "leah_cli_dispatch_to_first_progress_seconds") &&
			strings.Contains(k, "command=status") && v.Count != 0 {
			t.Fatalf("status tripped progress histogram (count=%d); short commands MUST be excluded", v.Count)
		}
	}
}
