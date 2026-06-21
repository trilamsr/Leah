package web

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSLA_ReadsA1ToA9FromMetricsSnapshot fakes a latest.json with all nine
// SLA histograms and asserts Snapshot.SLA surfaces one row per A1-A9 with
// p50/p95 derived from the bucket distribution and a count. Single test:
// every wired histogram is invisible until this lands.
func TestSLA_ReadsA1ToA9FromMetricsSnapshot(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "latest.json")

	// One observation per histogram in the 0.5-1.0 bucket. The exact bucket
	// edges differ per histogram in production; the reader handles arbitrary
	// edges. Here we use 0.1/0.5/1/+Inf for every A* and put the sample in 1.
	hist := func() map[string]any {
		return map[string]any{
			"count": 1,
			"sum":   0.7,
			"buckets": map[string]int64{
				"0.1": 0, "0.5": 0, "1": 1, "+Inf": 0,
			},
		}
	}
	payload := map[string]any{
		"ts":       "2026-06-21T00:00:00Z",
		"counters": map[string]int64{},
		"gauges":   map[string]float64{},
		"histograms": map[string]any{
			"leah_voice_wake_to_earcon_seconds":             hist(),
			"leah_voice_utterance_to_transcript_seconds":    hist(),
			"leah_intent_classify_seconds":                  hist(),
			"leah_voice_intent_to_first_audio_seconds":      hist(),
			"leah_hud_state_to_widget_seconds":              hist(),
			"leah_cli_dispatch_to_first_byte_seconds":       hist(),
			"leah_cli_dispatch_to_first_progress_seconds":   hist(),
			"leah_voice_barge_in_cancel_seconds":            hist(),
			"leah_onboarding_install_to_first_reply_seconds": hist(),
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(snap, raw, 0o600); err != nil {
		t.Fatalf("write snap: %v", err)
	}

	s := &Server{MetricsPath: snap}
	state := s.Snapshot(context.Background())
	if len(state.SLA) != 9 {
		t.Fatalf("SLA rows: got %d, want 9 (A1-A9)", len(state.SLA))
	}
	// Stable ordered A1..A9 — UI relies on order for the labelled grid.
	wantOrder := []string{
		"A1", "A2", "A3", "A4", "A5", "A6", "A7", "A8", "A9",
	}
	for i, row := range state.SLA {
		if row.Label != wantOrder[i] {
			t.Errorf("row[%d].Label: got %q, want %q", i, row.Label, wantOrder[i])
		}
		if row.Count != 1 {
			t.Errorf("row[%d] %s Count: got %d, want 1", i, row.Label, row.Count)
		}
		// The single observation lives in the 1.0 bucket so both p50 and p95
		// must fall in (0.5, 1.0]. Strict equality is brittle across linear-
		// interpolation tweaks; bounded check guards the contract.
		if row.P50 <= 0.5 || row.P50 > 1.0 {
			t.Errorf("row[%d] %s P50: got %v, want in (0.5, 1.0]", i, row.Label, row.P50)
		}
		if row.P95 <= 0.5 || row.P95 > 1.0 {
			t.Errorf("row[%d] %s P95: got %v, want in (0.5, 1.0]", i, row.Label, row.P95)
		}
		if !strings.HasPrefix(row.Histogram, "leah_") {
			t.Errorf("row[%d] Histogram: got %q, want a leah_* name", i, row.Histogram)
		}
	}
}

// TestSLA_EmptyWhenSnapshotMissing asserts an unwired MetricsPath produces
// nine zero-valued rows (still visible in UI, communicating "no data yet")
// rather than an empty slice — operator should see the SLA grid scaffold
// even before any voice/cli traffic.
func TestSLA_EmptyWhenSnapshotMissing(t *testing.T) {
	s := &Server{}
	state := s.Snapshot(context.Background())
	if len(state.SLA) != 9 {
		t.Fatalf("SLA rows: got %d, want 9 zero-rows", len(state.SLA))
	}
	for _, row := range state.SLA {
		if row.Count != 0 || row.P50 != 0 || row.P95 != 0 {
			t.Errorf("expected zero row, got %+v", row)
		}
	}
}

// TestSLA_LabeledSeriesAggregated asserts multiple label sets for the same
// histogram name (e.g. leah_intent_classify_seconds{kind=A}, {kind=B}) merge
// into ONE A3 row — operator wants the cross-kind SLA, not one row per kind.
func TestSLA_LabeledSeriesAggregated(t *testing.T) {
	dir := t.TempDir()
	snap := filepath.Join(dir, "latest.json")
	payload := map[string]any{
		"histograms": map[string]any{
			"leah_intent_classify_seconds|kind=A": map[string]any{
				"count": 2, "sum": 0.4,
				"buckets": map[string]int64{"0.1": 2, "0.5": 0, "1": 0, "+Inf": 0},
			},
			"leah_intent_classify_seconds|kind=B": map[string]any{
				"count": 3, "sum": 1.5,
				"buckets": map[string]int64{"0.1": 0, "0.5": 3, "1": 0, "+Inf": 0},
			},
		},
	}
	raw, _ := json.Marshal(payload)
	if err := os.WriteFile(snap, raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := &Server{MetricsPath: snap}
	state := s.Snapshot(context.Background())
	var a3 SLARow
	for _, r := range state.SLA {
		if r.Label == "A3" {
			a3 = r
			break
		}
	}
	if a3.Count != 5 {
		t.Errorf("A3 Count: got %d, want 5 (2+3 across both label sets)", a3.Count)
	}
}
