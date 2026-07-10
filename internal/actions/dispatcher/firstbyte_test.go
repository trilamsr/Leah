package dispatcher

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
)

// TestFirstByteTimer_ObservesOnFirstWrite verifies the histogram fires once on
// the FIRST Write to the wrapped writer and not on subsequent writes.
func TestFirstByteTimer_ObservesOnFirstWrite(t *testing.T) {
	reg := telemetry.NewRegistry()
	var buf bytes.Buffer

	timer := NewFirstByteTimer(reg, "status")
	w := timer.Wrap(&buf)

	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := w.Write([]byte(" world")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := buf.String(); got != "hello world" {
		t.Fatalf("passthrough mismatch: got %q", got)
	}

	snap := snapshotHist(t, reg, "leah_cli_dispatch_to_first_byte_seconds")
	if snap.Count != 1 {
		t.Fatalf("want count=1 (one observation on first write), got %d", snap.Count)
	}
	if !strings.Contains(snap.SeriesKey, "command=status") {
		t.Fatalf("want command=status label, got series key %q", snap.SeriesKey)
	}
}

// TestFirstByteTimer_NoWriteNoObservation guarantees a CLI run that emits
// nothing does NOT pollute the histogram.
func TestFirstByteTimer_NoWriteNoObservation(t *testing.T) {
	reg := telemetry.NewRegistry()
	var buf bytes.Buffer

	timer := NewFirstByteTimer(reg, "ask")
	_ = timer.Wrap(&buf)

	snap := snapshotHist(t, reg, "leah_cli_dispatch_to_first_byte_seconds")
	if snap.Count != 0 {
		t.Fatalf("want count=0 with no writes, got %d", snap.Count)
	}
}

// TestFirstByteTimer_NilRegistryNoOp ensures cmd/leah can omit metrics wiring
// without per-call guards.
func TestFirstByteTimer_NilRegistryNoOp(t *testing.T) {
	var buf bytes.Buffer
	timer := NewFirstByteTimer(nil, "status")
	w := timer.Wrap(&buf)
	if _, err := w.Write([]byte("ok")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != "ok" {
		t.Fatalf("passthrough mismatch: got %q", buf.String())
	}
}

// TestFirstByteTimer_LatencyWithinBucketRange sanity-checks the recorded value
// lands in the first non-zero bucket when the wrapper fires immediately after
// NewFirstByteTimer — guarding against time.Since clock-skew bugs.
func TestFirstByteTimer_LatencyWithinBucketRange(t *testing.T) {
	reg := telemetry.NewRegistry()
	var buf bytes.Buffer

	timer := NewFirstByteTimer(reg, "status")
	time.Sleep(2 * time.Millisecond)
	w := timer.Wrap(&buf)
	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}

	snap := snapshotHist(t, reg, "leah_cli_dispatch_to_first_byte_seconds")
	if snap.Sum <= 0 || snap.Sum > 5 {
		t.Fatalf("recorded latency sum %.4fs outside sane bounds", snap.Sum)
	}
}

type histSnap struct {
	SeriesKey string
	Count     int64
	Sum       float64
}

func snapshotHist(t *testing.T, reg *telemetry.Registry, name string) histSnap {
	t.Helper()
	path := t.TempDir() + "/snap.json"
	if err := reg.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := os.ReadFile(path)
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
	for k, v := range out.Histograms {
		if strings.HasPrefix(k, name) {
			return histSnap{SeriesKey: k, Count: v.Count, Sum: v.Sum}
		}
	}
	return histSnap{}
}
