package learn

import "github.com/trilam/leah/internal/obs"

var resolveLatencyBuckets = []float64{0.01, 0.05, 0.1, 0.5, 1, 5}

func RegisterMetrics(registry *telemetry.Registry) {
	if registry == nil {
		return
	}
	registry.Counter("leah_selflearn_resolve_total").Declare(map[string]string{"outcome": "cold"})
	registry.Histogram("leah_selflearn_resolve_latency_seconds", resolveLatencyBuckets).Declare(nil)
}

func EmitResolve(registry *telemetry.Registry, outcome string, latencySec float64) {
	if registry == nil {
		return
	}
	registry.Counter("leah_selflearn_resolve_total").Inc(map[string]string{"outcome": outcome})
	registry.Histogram("leah_selflearn_resolve_latency_seconds", resolveLatencyBuckets).Observe(nil, latencySec)
}
