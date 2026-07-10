package feeds

import "github.com/trilam/leah/internal/obs"

var rpcLatencyBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5}

func RegisterMetrics(registry *telemetry.Registry) {
	if registry == nil {
		return
	}
	registry.Counter("leah_feeds_rpc_total").Declare(map[string]string{"method": "cold", "outcome": "cold"})
	registry.Histogram("leah_feeds_rpc_latency_seconds", rpcLatencyBuckets).Declare(map[string]string{"method": "cold"})
}

func Observe(registry *telemetry.Registry, method, outcome string, latencySec float64) {
	if registry == nil {
		return
	}
	registry.Counter("leah_feeds_rpc_total").Inc(map[string]string{"method": method, "outcome": outcome})
	registry.Histogram("leah_feeds_rpc_latency_seconds", rpcLatencyBuckets).Observe(map[string]string{"method": method}, latencySec)
}
