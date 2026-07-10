package dispatcher

import (
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// TestProgressTimer_ObservesOnFirstEmit verifies the histogram fires once on
// the FIRST Emit call and not on subsequent ticks.
func TestProgressTimer_ObservesOnFirstEmit(t *testing.T) {
	reg := telemetry.NewRegistry()
	timer := NewProgressTimer(reg, "ask")

	timer.Emit()
	timer.Emit()
	timer.Emit()

	snap := snapshotHist(t, reg, "leah_cli_dispatch_to_first_progress_seconds")
	if snap.Count != 1 {
		t.Fatalf("want count=1 (one observation on first emit), got %d", snap.Count)
	}
	if !strings.Contains(snap.SeriesKey, "command=ask") {
		t.Fatalf("want command=ask label, got series key %q", snap.SeriesKey)
	}
}

// TestProgressTimer_NoEmitNoObservation guarantees a command that finishes
// before any progress signal does NOT pollute the histogram.
func TestProgressTimer_NoEmitNoObservation(t *testing.T) {
	reg := telemetry.NewRegistry()
	_ = NewProgressTimer(reg, "ship")

	snap := snapshotHist(t, reg, "leah_cli_dispatch_to_first_progress_seconds")
	if snap.Count != 0 {
		t.Fatalf("want count=0 with no emits, got %d", snap.Count)
	}
}

// TestProgressTimer_NilRegistryNoOp ensures cmd/leah can omit metrics wiring
// without per-call guards.
func TestProgressTimer_NilRegistryNoOp(t *testing.T) {
	timer := NewProgressTimer(nil, "ask")
	timer.Emit()
	timer.Emit()
}

// TestProgressTimer_NilReceiverNoOp lets callers store a *ProgressTimer that
// may be nil (e.g. test paths) and call Emit without a guard.
func TestProgressTimer_NilReceiverNoOp(t *testing.T) {
	var timer *ProgressTimer
	timer.Emit()
}

// TestProgressTimer_LatencyWithinBucketRange sanity-checks the recorded value
// lands in a sane band when Emit fires shortly after NewProgressTimer.
func TestProgressTimer_LatencyWithinBucketRange(t *testing.T) {
	reg := telemetry.NewRegistry()
	timer := NewProgressTimer(reg, "brief")
	time.Sleep(2 * time.Millisecond)
	timer.Emit()

	snap := snapshotHist(t, reg, "leah_cli_dispatch_to_first_progress_seconds")
	if snap.Sum <= 0 || snap.Sum > 2 {
		t.Fatalf("recorded latency sum %.4fs outside sane bounds", snap.Sum)
	}
}
