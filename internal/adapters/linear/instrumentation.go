package linear

import "github.com/trilam/leah/internal/obs"

const provider = "linear"

// RegisterMetrics cold-seeds the shared leah_connect_* series with this
// adapter's provider label so /metrics surfaces them pre-event.
func RegisterMetrics(registry *obs.Registry) {
	if registry == nil {
		return
	}
	registry.Counter("leah_connect_api_call_total").Add(map[string]string{"provider": provider, "endpoint": "cold"}, 0)
	registry.Counter("leah_connect_exchange_total").Add(map[string]string{"provider": provider, "outcome": "cold"}, 0)
	registry.Counter("leah_connect_refresh_total").Add(map[string]string{"provider": provider, "outcome": "cold"}, 0)
	registry.Gauge("leah_connect_token_age_seconds").Set(map[string]string{"provider": provider}, 0)
	registry.Histogram("leah_connect_api_latency_seconds", []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5}).Observe(map[string]string{"provider": provider}, 0)
}

// Observe records one adapter RPC against the shared connect series.
func Observe(registry *obs.Registry, endpoint, outcome string, latencySec float64) {
	if registry == nil {
		return
	}
	registry.Counter("leah_connect_api_call_total").Inc(map[string]string{"provider": provider, "endpoint": endpoint})
	registry.Counter("leah_connect_exchange_total").Inc(map[string]string{"provider": provider, "outcome": outcome})
	registry.Histogram("leah_connect_api_latency_seconds", []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5}).Observe(map[string]string{"provider": provider}, latencySec)
}
