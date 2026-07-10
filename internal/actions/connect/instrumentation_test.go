package connect

import (
	"testing"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/platform/telemetry/obstest"
)

// TestRegisterMetrics_NoPhantomColdSeries asserts the connect-level
// RegisterMetrics no longer emits provider=cold series (F4): adapters
// own the per-provider seed via connectadapter.Register.
func TestRegisterMetrics_NoPhantomColdSeries(t *testing.T) {
	r := telemetry.NewRegistry()
	RegisterMetrics(r)
	keys := obstest.SnapshotKeys(t, r)
	for _, k := range keys {
		if k != "" {
			t.Fatalf("RegisterMetrics introduced series %q; expected no-op", k)
		}
	}
}

// TestEmitExchange_BumpsCounter pins the EmitExchange call-site contract.
func TestEmitExchange_BumpsCounter(t *testing.T) {
	r := telemetry.NewRegistry()
	EmitExchange(r, "gmail", "ok")
	want := "leah_connect_exchange_total|outcome=ok,provider=gmail"
	if !obstest.ContainsExact(obstest.SnapshotKeys(t, r), want) {
		t.Fatalf("expected counter key %q", want)
	}
}
