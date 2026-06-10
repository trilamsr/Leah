package selflearn

import "github.com/trilam/leah/internal/obs"

// RegisterMetrics cold-seeds the selflearn series added in spec §4.9
// (resolve_total, resolve_latency_seconds) so /metrics surfaces them
// pre-event. The pre-existing dangling_selfbuilds_total gauge is owned
// by its own emitter.
func RegisterMetrics(registry *obs.Registry) {
	if registry == nil {
		return
	}
	registry.Counter("leah_selflearn_resolve_total").Add(map[string]string{"outcome": "cold"}, 0)
	registry.Histogram("leah_selflearn_resolve_latency_seconds", []float64{0.01, 0.05, 0.1, 0.5, 1, 5}).Observe(nil, 0)
}

// EmitResolve records one resolve attempt with its outcome and latency.
func EmitResolve(registry *obs.Registry, outcome string, latencySec float64) {
	if registry == nil {
		return
	}
	registry.Counter("leah_selflearn_resolve_total").Inc(map[string]string{"outcome": outcome})
	registry.Histogram("leah_selflearn_resolve_latency_seconds", []float64{0.01, 0.05, 0.1, 0.5, 1, 5}).Observe(nil, latencySec)
}
