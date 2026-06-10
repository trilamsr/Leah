package operatormodel

import (
	"testing"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts operatormodel RPC series surface
// pre-event.
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := obs.NewRegistry()
	RegisterMetrics(r)
	keys := obstest.SnapshotKeys(t, r)
	for _, want := range []string{
		"leah_operatormodel_rpc_total",
		"leah_operatormodel_rpc_latency_seconds",
	} {
		if !obstest.ContainsPrefix(keys, want) {
			t.Fatalf("series %q missing from %v", want, keys)
		}
	}
}

// TestObserve_IncrementsCounter pins the call-site contract.
func TestObserve_IncrementsCounter(t *testing.T) {
	r := obs.NewRegistry()
	Observe(r, "Plan", "ok", 0.1)
	want := "leah_operatormodel_rpc_total|method=Plan,outcome=ok"
	if !obstest.ContainsExact(obstest.SnapshotKeys(t, r), want) {
		t.Fatalf("expected %q", want)
	}
}
