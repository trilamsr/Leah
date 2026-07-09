package rules

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDetectAbovePanicThresholdReturnsCandidate asserts a per-name delta
// at or above the threshold becomes a Candidate carrying the panic-file
// path + raw stack body (#bug-fix-self-build-hook).
func TestDetectAbovePanicThresholdReturnsCandidate(t *testing.T) {
	dir := t.TempDir()
	writeMetrics(t, dir, map[string]int64{
		"leah_panic_total|name=daemon.tick": 5,
	})
	writeBaseline(t, dir, map[string]int64{
		"leah_panic_total|name=daemon.tick": 1,
	})
	writePanicFile(t, dir, "daemon.tick", "panic: nil map\n\ngoroutine 1 [running]:\nfoo\n")

	r := PanicRateRule{Threshold: 3}
	cands, err := r.Detect(context.Background(), dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(cands))
	}
	c := cands[0]
	if c.Name != "daemon.tick" {
		t.Errorf("name = %q, want daemon.tick", c.Name)
	}
	if c.Delta != 4 {
		t.Errorf("delta = %d, want 4", c.Delta)
	}
	if !strings.Contains(c.PanicStack, "nil map") {
		t.Errorf("stack missing raw body: %q", c.PanicStack)
	}
}

// TestDetectBelowThresholdReturnsEmpty asserts sub-threshold deltas
// emit no candidates (zero noise floor).
func TestDetectBelowThresholdReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeMetrics(t, dir, map[string]int64{
		"leah_panic_total|name=daemon.tick": 2,
	})
	writeBaseline(t, dir, map[string]int64{
		"leah_panic_total|name=daemon.tick": 1,
	})

	r := PanicRateRule{Threshold: 3}
	cands, err := r.Detect(context.Background(), dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(cands) != 0 {
		t.Fatalf("want 0 candidates, got %d", len(cands))
	}
}

// TestBuildIssueBodyIncludesStackAndLogs asserts the regatta-issue body
// embeds raw panic stack verbatim with no Leah-narrative wrapper that
// could prompt-inject the downstream agent.
func TestBuildIssueBodyIncludesStackAndLogs(t *testing.T) {
	c := Candidate{
		Name:       "daemon.tick",
		Delta:      7,
		PanicFile:  "/tmp/p.txt",
		PanicStack: "panic: nil map deref\n\ngoroutine 12 [running]:\nfoo.bar()",
	}
	body := BuildIssueBody(c)
	if !strings.Contains(body, "daemon.tick") {
		t.Errorf("body missing name: %q", body)
	}
	if !strings.Contains(body, "7") {
		t.Errorf("body missing delta: %q", body)
	}
	if !strings.Contains(body, "nil map deref") {
		t.Errorf("body missing raw stack: %q", body)
	}
	if !strings.Contains(body, "/tmp/p.txt") {
		t.Errorf("body missing panic-file path: %q", body)
	}
}

// TestDetectHandlesMissingMetricsFileGracefully asserts cold-start (no
// metrics snapshot yet) returns (nil, nil) — first run primes baseline.
func TestDetectHandlesMissingMetricsFileGracefully(t *testing.T) {
	dir := t.TempDir()
	r := PanicRateRule{Threshold: 3}
	cands, err := r.Detect(context.Background(), dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(cands) != 0 {
		t.Fatalf("want 0 candidates on cold start, got %d", len(cands))
	}
}

// TestDetectMissingPanicFileEmitsCandidateWithoutStack asserts a
// threshold-crossing delta still emits a candidate even when no panic
// file exists for that name (operator still sees the signal).
func TestDetectMissingPanicFileEmitsCandidateWithoutStack(t *testing.T) {
	dir := t.TempDir()
	writeMetrics(t, dir, map[string]int64{
		"leah_panic_total|name=lost.goroutine": 4,
	})
	writeBaseline(t, dir, map[string]int64{
		"leah_panic_total|name=lost.goroutine": 0,
	})

	r := PanicRateRule{Threshold: 3}
	cands, err := r.Detect(context.Background(), dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(cands))
	}
	if cands[0].PanicStack != "" {
		t.Errorf("expected empty stack when no panic file, got %q", cands[0].PanicStack)
	}
}

// --- helpers ---

func writeMetrics(t *testing.T, stateDir string, counters map[string]int64) {
	t.Helper()
	dir := filepath.Join(stateDir, "metrics")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir metrics: %v", err)
	}
	snap := map[string]any{
		"ts":       time.Now().UTC().Format(time.RFC3339),
		"counters": counters,
	}
	b, _ := json.Marshal(snap)
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), b, 0o600); err != nil {
		t.Fatalf("write metrics: %v", err)
	}
}

func writeBaseline(t *testing.T, stateDir string, counters map[string]int64) {
	t.Helper()
	dir := filepath.Join(stateDir, "selflearn")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir selflearn: %v", err)
	}
	b, _ := json.Marshal(map[string]any{"counters": counters})
	if err := os.WriteFile(filepath.Join(dir, "panic-baseline.json"), b, 0o600); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
}

func writePanicFile(t *testing.T, stateDir, name, body string) {
	t.Helper()
	dir := filepath.Join(stateDir, "panics")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir panics: %v", err)
	}
	fname := time.Now().UTC().Format("2006-01-02T15-04-05.000") + "-" + name + ".txt"
	if err := os.WriteFile(filepath.Join(dir, fname), []byte(body), 0o600); err != nil {
		t.Fatalf("write panic: %v", err)
	}
}
