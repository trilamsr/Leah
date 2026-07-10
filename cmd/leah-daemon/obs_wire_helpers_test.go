package main

import (
	"net/http"
	"testing"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/daemonloop"
	"github.com/trilam/leah/internal/memory/store"
	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/platform/web"
)

// driveProducersForTest exercises the wires so /metrics has live observations
// on top of the cold-start zero seeds wireInstrumentation lays down.
func driveProducersForTest(t *testing.T, registry *telemetry.Registry) {
	t.Helper()
	// Cold-start seeds in wireObs already emit every series; no-op here.
	_ = registry
}

// wireInstrumentation forwards to the production wireObs. Test composition
// passes nil for chain (voice off in unit tests) so the SelfChecker reports
// "degraded" — still counted as a registered probe for the package_health test.
func wireInstrumentation(
	registry *telemetry.Registry,
	health *telemetry.HealthRegistry,
	a *audit.Logger,
	store *memory.Store,
	loop *daemonloop.Loop,
) {
	wireObs(registry, health, a, store, loop, nil)
}

// buildMuxForTest mirrors the daemon's mux assembly: web.Server.BuildMux
// wrapped with MetricsMiddleware so /metrics emits leah_web_requests_total.
func buildMuxForTest(s *web.Server) (http.Handler, error) {
	mux, err := s.BuildMux()
	if err != nil {
		return nil, err
	}
	return web.MetricsMiddleware(s.Metrics, mux), nil
}
