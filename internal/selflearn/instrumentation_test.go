package selflearn

import (
	"testing"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts spec §4.9 resolve series surface
// pre-event.
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := obs.NewRegistry()
	RegisterMetrics(r)
	keys := obstest.SnapshotKeys(t, r)
	for _, want := range []string{
		"leah_selflearn_resolve_total",
		"leah_selflearn_resolve_latency_seconds",
	} {
		if !obstest.ContainsPrefix(keys, want) {
			t.Fatalf("series %q missing from %v", want, keys)
		}
	}
}

// TestEmitResolve_IncrementsCounter pins the call-site contract.
func TestEmitResolve_IncrementsCounter(t *testing.T) {
	r := obs.NewRegistry()
	EmitResolve(r, "ok", 0.1)
	want := "leah_selflearn_resolve_total|outcome=ok"
	if !obstest.ContainsExact(obstest.SnapshotKeys(t, r), want) {
		t.Fatalf("expected %q", want)
	}
}
