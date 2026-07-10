package onboarding_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
	"github.com/trilam/leah/internal/onboarding"
)

// TestRegisterMetrics_AddsSeries — A9 series surfaces pre-event on /metrics.
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := telemetry.NewRegistry()
	onboarding.RegisterMetrics(r)
	keys := obstest.SnapshotKeys(t, r)
	want := "leah_onboarding_install_to_first_reply_seconds"
	if !obstest.ContainsPrefix(keys, want) {
		t.Fatalf("series %q missing from %v", want, keys)
	}
}

// TestRecordFirstReply_ObservesOnce — observer seals after first sample; A9 is once-per-install.
func TestRecordFirstReply_ObservesOnce(t *testing.T) {
	t.Parallel()
	r := telemetry.NewRegistry()
	onboarding.RegisterMetrics(r)
	obs1 := onboarding.NewFirstReplyObserver(r)

	if !obs1.Record(45 * time.Second) {
		t.Fatalf("first Record returned false, want true")
	}
	if obs1.Record(90 * time.Second) {
		t.Fatalf("second Record returned true, want false (sealed)")
	}
}

// TestRecordFirstReply_NilSafe — nil observer Record is no-op, returns false.
func TestRecordFirstReply_NilSafe(t *testing.T) {
	t.Parallel()
	var obs1 *onboarding.FirstReplyObserver
	if obs1.Record(1 * time.Second) {
		t.Fatalf("nil observer Record returned true")
	}
}

// TestFirstReplyBuckets_Sized_For_5min_SLA — 300s boundary present so p95 lands inside a bucket.
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

// TestRecordFirstReplyIfNotYetRecorded_FullPath — install at T0, reply at T0+5s lands one sample; second call no-ops.
func TestRecordFirstReplyIfNotYetRecorded_FullPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := telemetry.NewRegistry()
	onboarding.RegisterMetrics(r)

	t0 := time.Unix(1_700_000_000, 0).UTC()
	if err := onboarding.MarkInstalled(dir, t0); err != nil {
		t.Fatalf("MarkInstalled: %v", err)
	}
	if !onboarding.RecordFirstReplyIfNotYetRecorded(dir, r, t0.Add(5*time.Second)) {
		t.Fatalf("first RecordFirstReplyIfNotYetRecorded returned false, want true")
	}
	if onboarding.RecordFirstReplyIfNotYetRecorded(dir, r, t0.Add(10*time.Second)) {
		t.Fatalf("second RecordFirstReplyIfNotYetRecorded returned true, want false (sealed)")
	}

	path := filepath.Join(t.TempDir(), "snap.json")
	if err := r.Snapshot(path); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var snap struct {
		Histograms map[string]struct {
			Count int64 `json:"count"`
		} `json:"histograms"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := snap.Histograms["leah_onboarding_install_to_first_reply_seconds"].Count
	if got != 1 {
		t.Fatalf("histogram count = %d, want 1 sample after one Record + one no-op", got)
	}
}

// TestRecordFirstReplyIfNotYetRecorded_NoMarker_NoOp — without install marker the helper returns false.
func TestRecordFirstReplyIfNotYetRecorded_NoMarker_NoOp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r := telemetry.NewRegistry()
	onboarding.RegisterMetrics(r)
	if onboarding.RecordFirstReplyIfNotYetRecorded(dir, r, time.Now()) {
		t.Fatalf("RecordFirstReplyIfNotYetRecorded with no install marker returned true, want false")
	}
}

// TestRecordFirstReplyIfNotYetRecorded_NegativeElapsed_DoesNotBurnSeal — clock skew must not consume the once-per-install slot.
func TestRecordFirstReplyIfNotYetRecorded_NegativeElapsed_DoesNotBurnSeal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	t0 := time.Unix(1_700_000_000, 0).UTC()
	if err := onboarding.MarkInstalled(dir, t0); err != nil {
		t.Fatalf("MarkInstalled: %v", err)
	}
	r := telemetry.NewRegistry()
	onboarding.RegisterMetrics(r)

	if onboarding.RecordFirstReplyIfNotYetRecorded(dir, r, t0.Add(-1*time.Second)) {
		t.Fatalf("negative elapsed returned true, want false")
	}
	// Subsequent valid reply must still land — seal was not burned.
	if !onboarding.RecordFirstReplyIfNotYetRecorded(dir, r, t0.Add(5*time.Second)) {
		t.Fatalf("post-skew valid reply returned false, want true (seal must not have been burned)")
	}
}

// TestRecordFirstReplyIfNotYetRecorded_SealAcrossProcesses — seal file blocks observation even from a fresh registry+observer.
func TestRecordFirstReplyIfNotYetRecorded_SealAcrossProcesses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	t0 := time.Unix(1_700_000_000, 0).UTC()
	if err := onboarding.MarkInstalled(dir, t0); err != nil {
		t.Fatalf("MarkInstalled: %v", err)
	}

	r1 := telemetry.NewRegistry()
	onboarding.RegisterMetrics(r1)
	if !onboarding.RecordFirstReplyIfNotYetRecorded(dir, r1, t0.Add(5*time.Second)) {
		t.Fatalf("first invocation returned false, want true")
	}

	// Second "process" simulated as a fresh registry — only the on-disk seal
	// can stop it; an in-memory atomic.Bool would not survive process exit.
	r2 := telemetry.NewRegistry()
	onboarding.RegisterMetrics(r2)
	if onboarding.RecordFirstReplyIfNotYetRecorded(dir, r2, t0.Add(20*time.Second)) {
		t.Fatalf("second invocation across fresh registry returned true, want false (filesystem-sealed)")
	}
}
