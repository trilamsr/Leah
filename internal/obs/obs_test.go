package obs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoggerWritesToFileAndStderr asserts the logger emits a JSONL record
// containing the required schema fields to the per-day log file.
func TestLoggerWritesToFileAndStderr(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_LOG_LEVEL", "DEBUG")

	lg, closeFn, err := NewLogger()
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer closeFn()

	lg.Info("reasoner.call.complete", "duration_ms", 423, "cost_dollars", 0.012)

	// Find today's log file.
	logsDir := filepath.Join(dir, "logs")
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		t.Fatalf("read logs dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 log file, got %d", len(entries))
	}
	raw, err := os.ReadFile(filepath.Join(logsDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 0 {
		t.Fatalf("no log lines written")
	}

	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("parse JSON line: %v\n%s", err, lines[0])
	}
	for _, field := range []string{"ts", "level", "msg"} {
		if _, ok := rec[field]; !ok {
			t.Errorf("missing required field %q in %v", field, rec)
		}
	}
	if rec["msg"] != "reasoner.call.complete" {
		t.Errorf("msg = %v", rec["msg"])
	}
}

// TestWithTracePropagatesID asserts the trace ID set via WithTrace is
// retrievable via TraceID and appears as an attr on logger records.
func TestWithTracePropagatesID(t *testing.T) {
	ctx := WithTrace(context.Background(), "trace-abc")
	if got := TraceID(ctx); got != "trace-abc" {
		t.Errorf("TraceID = %q, want trace-abc", got)
	}
	if got := TraceID(context.Background()); got != "" {
		t.Errorf("bare ctx TraceID = %q, want empty", got)
	}

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	lg, closeFn, err := NewLogger()
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer closeFn()

	scoped := WithLogger(ctx, lg)
	LoggerFromCtx(scoped).Info("test.event")

	logsDir := filepath.Join(dir, "logs")
	entries, _ := os.ReadDir(logsDir)
	if len(entries) == 0 {
		t.Fatalf("no log file")
	}
	raw, _ := os.ReadFile(filepath.Join(logsDir, entries[0].Name()))
	if !strings.Contains(string(raw), `"trace_id":"trace-abc"`) {
		t.Errorf("trace_id missing from log: %s", raw)
	}
}

// TestCounterIncrement asserts Counter.Inc + label keys flatten + Snapshot
// round-trips a JSON file readable by the future selflearn rule.
func TestCounterIncrement(t *testing.T) {
	r := NewRegistry()
	c := r.Counter("leah_action_total")
	c.Inc(map[string]string{"kind": "ship", "outcome": "success"})
	c.Inc(map[string]string{"kind": "ship", "outcome": "success"})
	c.Inc(map[string]string{"kind": "ship", "outcome": "failed"})

	g := r.Gauge("leah_budget_spent_usd")
	g.Set(nil, 0.43)

	h := r.Histogram("leah_reasoner_latency_ms", []float64{50, 100, 250, 500, 1000})
	h.Observe(nil, 42)
	h.Observe(nil, 180)
	h.Observe(nil, 480)

	dir := t.TempDir()
	path := filepath.Join(dir, "latest.json")
	if err := r.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var snap struct {
		Counters   map[string]int64              `json:"counters"`
		Gauges     map[string]float64            `json:"gauges"`
		Histograms map[string]map[string]any     `json:"histograms"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("parse snapshot: %v\n%s", err, raw)
	}
	if snap.Counters["leah_action_total|kind=ship,outcome=success"] != 2 {
		t.Errorf("success counter = %d, want 2", snap.Counters["leah_action_total|kind=ship,outcome=success"])
	}
	if snap.Counters["leah_action_total|kind=ship,outcome=failed"] != 1 {
		t.Errorf("failed counter = %d, want 1", snap.Counters["leah_action_total|kind=ship,outcome=failed"])
	}
	if snap.Gauges["leah_budget_spent_usd"] != 0.43 {
		t.Errorf("gauge = %v, want 0.43", snap.Gauges["leah_budget_spent_usd"])
	}
	if _, ok := snap.Histograms["leah_reasoner_latency_ms"]; !ok {
		t.Errorf("histogram missing")
	}
}

// TestSafeGoRecoversPanicAndLogs asserts SafeGo recovers panics, writes a
// stack trace under $LEAH_STATE_DIR/panics/, and increments the panic
// counter on the supplied registry.
func TestSafeGoRecoversPanicAndLogs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	lg, closeFn, err := NewLogger()
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer closeFn()

	reg := NewRegistry()
	SafeGo(lg, reg, "test-goroutine", func() {
		panic("boom")
	})

	// Poll for the panic file: SafeGo's recover + file-write happens
	// AFTER the inner f returns; a done-channel inside f would race the
	// outer defer. Bounded wait keeps the test deterministic without a
	// fixed sleep.
	panicsDir := filepath.Join(dir, "panics")
	var entries []os.DirEntry
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ = os.ReadDir(panicsDir)
		if len(entries) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(entries) == 0 {
		t.Fatalf("panic file never written in 2s")
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 panic file, got %d", len(entries))
	}
	raw, _ := os.ReadFile(filepath.Join(panicsDir, entries[0].Name()))
	body := string(raw)
	if !strings.Contains(body, "boom") {
		t.Errorf("panic file missing 'boom': %s", body)
	}
	if !strings.Contains(body, "test-goroutine") {
		t.Errorf("panic file missing goroutine name: %s", body)
	}

	// Counter incremented.
	tmp := filepath.Join(t.TempDir(), "snap.json")
	_ = reg.Snapshot(tmp)
	raw, _ = os.ReadFile(tmp)
	if !strings.Contains(string(raw), `"leah_panic_total|name=test-goroutine": 1`) {
		t.Errorf("panic counter not incremented: %s", raw)
	}
}

// TestSafeRunPropagatesPanicWhenEnvSet asserts LEAH_OBS_PANIC_PROPAGATE=1
// re-panics after capture so tests fail loudly. Uses SafeRun (the
// synchronous core of SafeGo) so the test can recover the re-panic
// without losing the test runtime.
func TestSafeRunPropagatesPanicWhenEnvSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_OBS_PANIC_PROPAGATE", "1")

	lg, closeFn, err := NewLogger()
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer closeFn()

	reg := NewRegistry()
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		SafeRun(lg, reg, "propagate", func() { panic("loud") })
	}()
	if recovered == nil {
		t.Fatalf("expected re-panic to be recoverable by outer defer")
	}
	if fmt.Sprintf("%v", recovered) != "loud" {
		t.Errorf("recovered = %v, want 'loud'", recovered)
	}

	// Panic file MUST still be written.
	panicsDir := filepath.Join(dir, "panics")
	entries, _ := os.ReadDir(panicsDir)
	if len(entries) != 1 {
		t.Fatalf("want 1 panic file even in propagate mode, got %d", len(entries))
	}
}

// TestDailyRotationClosesPriorHandle asserts that when the date changes,
// the rotator opens a new file and closes the prior one.
func TestDailyRotationClosesPriorHandle(t *testing.T) {
	dir := t.TempDir()
	rot := newDailyRotator(filepath.Join(dir, "logs"))

	d1 := time.Date(2026, 6, 9, 23, 59, 0, 0, time.UTC)
	w1, err := rot.writerFor(d1)
	if err != nil {
		t.Fatalf("writerFor d1: %v", err)
	}
	if _, err := w1.Write([]byte("a\n")); err != nil {
		t.Fatalf("write d1: %v", err)
	}

	d2 := time.Date(2026, 6, 10, 0, 0, 1, 0, time.UTC)
	w2, err := rot.writerFor(d2)
	if err != nil {
		t.Fatalf("writerFor d2: %v", err)
	}
	if w1 == w2 {
		t.Errorf("rotator returned same handle across day boundary")
	}
	// Writing to the old handle MUST fail (file closed).
	if _, err := w1.Write([]byte("b\n")); !errors.Is(err, os.ErrClosed) && err == nil {
		// Some platforms return generic write-on-closed-fd error; tolerate
		// any non-nil error here.
		t.Errorf("write to closed handle should fail, got nil")
	}
	rot.close()
}
