package wake

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// TestEarconTracker_Observe_RecordsHistogram: a normal wake→earcon span
// lands in the leah_voice_wake_to_earcon_seconds histogram with the
// detector-label attached so different wake backends can be compared.
func TestEarconTracker_Observe_RecordsHistogram(t *testing.T) {
	t.Parallel()
	reg := obs.NewRegistry()
	tr := NewEarconTracker(reg, "energy")
	if tr == nil {
		t.Fatal("NewEarconTracker returned nil for non-nil registry")
	}
	start := time.Now()
	tr.WakeDetected(start)
	tr.EarconPlayed(start.Add(80 * time.Millisecond))

	var buf bytes.Buffer
	if err := reg.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "leah_voice_wake_to_earcon_seconds") {
		t.Fatalf("missing histogram in output:\n%s", out)
	}
	if !strings.Contains(out, `detector="energy"`) {
		t.Fatalf("missing detector label in output:\n%s", out)
	}
	if !strings.Contains(out, "leah_voice_wake_to_earcon_seconds_count") {
		t.Fatalf("missing count line:\n%s", out)
	}
}

// TestEarconTracker_NilRegistry_NoPanic: production wiring may omit the
// registry; nil tracker methods must no-op.
func TestEarconTracker_NilRegistry_NoPanic(t *testing.T) {
	t.Parallel()
	tr := NewEarconTracker(nil, "energy")
	if tr != nil {
		t.Fatalf("nil registry must yield nil tracker, got %v", tr)
	}
	tr.WakeDetected(time.Now())
	tr.EarconPlayed(time.Now())
}

// TestEarconTracker_OutOfOrder_NoRecord: EarconPlayed without a preceding
// WakeDetected must NOT record — otherwise stage cleanup paths poison the
// histogram with near-zero values that mask real p95.
func TestEarconTracker_OutOfOrder_NoRecord(t *testing.T) {
	t.Parallel()
	reg := obs.NewRegistry()
	tr := NewEarconTracker(reg, "energy")
	tr.EarconPlayed(time.Now())

	var buf bytes.Buffer
	if err := reg.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	out := buf.String()
	// A registered-but-unobserved histogram is fine; a recorded sample is not.
	if strings.Contains(out, "leah_voice_wake_to_earcon_seconds_count{") {
		// A series line implies an observation was attributed to some label set.
		t.Fatalf("EarconPlayed without WakeDetected recorded a sample:\n%s", out)
	}
}

// TestEarconTracker_Buckets_Sized_For_150ms_Target asserts the histogram
// can actually resolve the MAY-A1 target: the 150ms boundary is present
// (so p95 reports a tight upper bound, not the next slot up), the slice is
// strictly increasing, and a 50ms vs 200ms sample land in different
// cumulative buckets — otherwise on-target vs regressed is indistinguishable.
func TestEarconTracker_Buckets_Sized_For_150ms_Target(t *testing.T) {
	t.Parallel()
	if len(EarconBuckets) < 2 {
		t.Fatalf("EarconBuckets must have ≥2 boundaries, got %v", EarconBuckets)
	}
	last := EarconBuckets[len(EarconBuckets)-1]
	if 0.15 > last {
		t.Fatalf("150ms target lands at +Inf: max bucket=%v", last)
	}
	hasTarget := false
	for _, b := range EarconBuckets {
		if b == 0.15 {
			hasTarget = true
			break
		}
	}
	if !hasTarget {
		t.Fatalf("0.15 missing from EarconBuckets=%v; p95 widens to next boundary", EarconBuckets)
	}
	for i := 1; i < len(EarconBuckets); i++ {
		if EarconBuckets[i] <= EarconBuckets[i-1] {
			t.Fatalf("EarconBuckets not strictly increasing at %d: %v", i, EarconBuckets)
		}
	}

	reg := obs.NewRegistry()
	tr := NewEarconTracker(reg, "energy")
	base := time.Unix(0, 0)
	tr.WakeDetected(base)
	tr.EarconPlayed(base.Add(50 * time.Millisecond))
	tr.WakeDetected(base)
	tr.EarconPlayed(base.Add(200 * time.Millisecond))

	var buf bytes.Buffer
	if err := reg.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `bucket{detector="energy",le="0.05"} 1`) {
		t.Fatalf("50ms sample not in le=0.05 bucket:\n%s", out)
	}
	if !strings.Contains(out, `bucket{detector="energy",le="0.15"} 1`) {
		t.Fatalf("200ms sample leaked under le=0.15:\n%s", out)
	}
	if !strings.Contains(out, `bucket{detector="energy",le="0.25"} 2`) {
		t.Fatalf("200ms sample not in le=0.25 bucket:\n%s", out)
	}
}
