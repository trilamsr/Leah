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

// TestEarconTracker_Buckets_Sized_For_150ms_Target: bucket boundaries must
// straddle the 150ms p95 target so p95 lands inside a bucket, not at +Inf.
func TestEarconTracker_Buckets_Sized_For_150ms_Target(t *testing.T) {
	t.Parallel()
	want := []float64{0.01, 0.025, 0.05, 0.1, 0.15, 0.25, 0.5}
	if len(EarconBuckets) != len(want) {
		t.Fatalf("EarconBuckets len=%d want %d", len(EarconBuckets), len(want))
	}
	for i, b := range want {
		if EarconBuckets[i] != b {
			t.Fatalf("EarconBuckets[%d]=%v want %v", i, EarconBuckets[i], b)
		}
	}
}
