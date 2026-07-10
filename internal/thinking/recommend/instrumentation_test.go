package recommend

import (
	"testing"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/platform/telemetry/obstest"
)

// TestRegisterMetrics_AddsSeries asserts recommendation-engine series
// surface pre-event (spec §4.13).
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := telemetry.NewRegistry()
	RegisterMetrics(r)
	keys := obstest.SnapshotKeys(t, r)
	for _, want := range []string{
		"leah_recommendation_proposed_total",
		"leah_recommendation_accepted_total",
		"leah_recommendation_rejected_total",
		"leah_recommendation_applied_total",
	} {
		if !obstest.ContainsPrefix(keys, want) {
			t.Fatalf("series %q missing from %v", want, keys)
		}
	}
}

// TestEmitApplied_IncrementsCounter pins the call-site contract.
func TestEmitApplied_IncrementsCounter(t *testing.T) {
	r := telemetry.NewRegistry()
	EmitApplied(r, "habit", "ok")
	want := "leah_recommendation_applied_total|kind=habit,outcome=ok"
	if !obstest.ContainsExact(obstest.SnapshotKeys(t, r), want) {
		t.Fatalf("expected %q", want)
	}
}
