package onboarding_test

import (
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
	"github.com/trilam/leah/internal/onboarding"
)

// TestRegisterMetrics_AddsSeries — A9 install→first-reply histogram surfaces
// pre-event so /metrics scrape returns the series even before the first ask.
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := obs.NewRegistry()
	onboarding.RegisterMetrics(r)
	keys := obstest.SnapshotKeys(t, r)
	want := "leah_onboarding_install_to_first_reply_seconds"
	if !obstest.ContainsPrefix(keys, want) {
		t.Fatalf("series %q missing from %v", want, keys)
	}
}

// TestRecordFirstReply_ObservesOnce — first call records, subsequent calls
// drop. A9 measures the time-to-first-useful-reply, which is by definition
// a once-per-install event.
func TestRecordFirstReply_ObservesOnce(t *testing.T) {
	t.Parallel()
	r := obs.NewRegistry()
	onboarding.RegisterMetrics(r)
	obs1 := onboarding.NewFirstReplyObserver(r)

	if !obs1.Record(45 * time.Second) {
		t.Fatalf("first Record returned false, want true")
	}
	if obs1.Record(90 * time.Second) {
		t.Fatalf("second Record returned true, want false (sealed)")
	}
}

// TestRecordFirstReply_NilSafe — registry-less callers get nil observer; Record
// returns false and is a no-op.
func TestRecordFirstReply_NilSafe(t *testing.T) {
	t.Parallel()
	var obs1 *onboarding.FirstReplyObserver
	if obs1.Record(1 * time.Second) {
		t.Fatalf("nil observer Record returned true")
	}
}

// TestFirstReplyBuckets_Sized_For_5min_SLA — A9 SLA is 5 min (300s). The
// boundary must be present so p95 reports a tight upper bound, not +Inf.
func TestFirstReplyBuckets_Sized_For_5min_SLA(t *testing.T) {
	t.Parallel()
	hasTarget := false
	for _, b := range onboarding.FirstReplyBuckets {
		if b == 300 {
			hasTarget = true
			break
		}
	}
	if !hasTarget {
		t.Fatalf("300s SLA boundary missing from %v", onboarding.FirstReplyBuckets)
	}
}
