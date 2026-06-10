package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/web"
)

// driveProducersForTest exercises every package's production code path that
// should emit a metric. GREEN implements via per-package package-level
// helper calls so we are testing real production wires, not test doubles.
func driveProducersForTest(t *testing.T, registry *obs.Registry) {
	t.Helper()
	_ = registry
}

// wireInstrumentation binds counters + SelfCheckers across daemonloop /
// dispatcher / audit / memory / voice / web / attestation onto the shared
// registry + health surfaces. GREEN replaces this no-op with the real wires.
func wireInstrumentation(
	registry *obs.Registry,
	health *obs.HealthRegistry,
	a *audit.Logger,
	store *memory.Store,
	loop *daemonloop.Loop,
) {
	_, _, _, _, _ = registry, health, a, store, loop
}

// buildMuxForTest mirrors web.Server.buildMux's two endpoints we care about
// (/metrics, /health) so the in-package tests assert against the same handler
// graph production would build. Reproduced here rather than exporting a
// public mux constructor.
func buildMuxForTest(s *web.Server) (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		if s.Metrics == nil {
			http.Error(w, "metrics not wired", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_ = s.Metrics.WritePrometheus(w)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if s.Health == nil {
			http.Error(w, "health not wired", http.StatusServiceUnavailable)
			return
		}
		rep := s.Health.Probe(r.Context(), 2*time.Second)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rep)
	})
	return mux, nil
}
