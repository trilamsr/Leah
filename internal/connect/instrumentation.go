package connect

import "github.com/trilam/leah/internal/obs"

// RegisterMetrics cold-seeds the shared leah_connect_* series from spec
// §4.7. Per-adapter packages register their own provider label; this
// seed exists so the series surface in /metrics even when no adapter has
// run.
func RegisterMetrics(registry *obs.Registry) {
	if registry == nil {
		return
	}
	registry.Counter("leah_connect_api_call_total").Add(map[string]string{"provider": "cold", "endpoint": "cold"}, 0)
	registry.Counter("leah_connect_exchange_total").Add(map[string]string{"provider": "cold", "outcome": "cold"}, 0)
	registry.Counter("leah_connect_refresh_total").Add(map[string]string{"provider": "cold", "outcome": "cold"}, 0)
	registry.Gauge("leah_connect_token_age_seconds").Set(map[string]string{"provider": "cold"}, 0)
	registry.Histogram("leah_connect_api_latency_seconds", []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5}).Observe(map[string]string{"provider": "cold"}, 0)
}

// EmitExchange bumps exchange_total{provider, outcome}.
func EmitExchange(registry *obs.Registry, provider, outcome string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_connect_exchange_total").Inc(map[string]string{"provider": provider, "outcome": outcome})
}

// EmitRefresh bumps refresh_total{provider, outcome}.
func EmitRefresh(registry *obs.Registry, provider, outcome string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_connect_refresh_total").Inc(map[string]string{"provider": provider, "outcome": outcome})
}

// SetTokenAge writes token_age_seconds{provider}.
func SetTokenAge(registry *obs.Registry, provider string, ageSec float64) {
	if registry == nil {
		return
	}
	registry.Gauge("leah_connect_token_age_seconds").Set(map[string]string{"provider": provider}, ageSec)
}
