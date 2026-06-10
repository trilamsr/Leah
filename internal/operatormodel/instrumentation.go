package operatormodel

import "github.com/trilam/leah/internal/obs"

// RegisterMetrics cold-seeds the operatormodel RPC series so /metrics
// surfaces them pre-event.
func RegisterMetrics(registry *obs.Registry) {
	if registry == nil {
		return
	}
	registry.Counter("leah_operatormodel_rpc_total").Add(map[string]string{"method": "cold", "outcome": "cold"}, 0)
	registry.Histogram("leah_operatormodel_rpc_latency_seconds", []float64{0.01, 0.05, 0.1, 0.5, 1, 5}).Observe(map[string]string{"method": "cold"}, 0)
}

// Observe records one operatormodel RPC with its method, outcome, and latency.
func Observe(registry *obs.Registry, method, outcome string, latencySec float64) {
	if registry == nil {
		return
	}
	registry.Counter("leah_operatormodel_rpc_total").Inc(map[string]string{"method": method, "outcome": outcome})
	registry.Histogram("leah_operatormodel_rpc_latency_seconds", []float64{0.01, 0.05, 0.1, 0.5, 1, 5}).Observe(map[string]string{"method": method}, latencySec)
}
