package operatormodel

import "github.com/trilam/leah/internal/obs"

var rpcLatencyBuckets = []float64{0.01, 0.05, 0.1, 0.5, 1, 5}

func RegisterMetrics(registry *obs.Registry) {
	if registry == nil {
		return
	}
	registry.Counter("leah_operatormodel_rpc_total").Declare(map[string]string{"method": "cold", "outcome": "cold"})
	registry.Histogram("leah_operatormodel_rpc_latency_seconds", rpcLatencyBuckets).Declare(map[string]string{"method": "cold"})
}

func Observe(registry *obs.Registry, method, outcome string, latencySec float64) {
	if registry == nil {
		return
	}
	registry.Counter("leah_operatormodel_rpc_total").Inc(map[string]string{"method": method, "outcome": outcome})
	registry.Histogram("leah_operatormodel_rpc_latency_seconds", rpcLatencyBuckets).Observe(map[string]string{"method": method}, latencySec)
}
