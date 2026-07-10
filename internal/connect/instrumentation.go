package connect

import "github.com/trilam/leah/internal/obs"

func RegisterMetrics(registry *telemetry.Registry) {
	_ = registry
}

func EmitExchange(registry *telemetry.Registry, provider, outcome string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_connect_exchange_total").Inc(map[string]string{"provider": provider, "outcome": outcome})
}

func EmitRefresh(registry *telemetry.Registry, provider, outcome string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_connect_refresh_total").Inc(map[string]string{"provider": provider, "outcome": outcome})
}

func SetTokenAge(registry *telemetry.Registry, provider string, ageSec float64) {
	if registry == nil {
		return
	}
	registry.Gauge("leah_connect_token_age_seconds").Set(map[string]string{"provider": provider}, ageSec)
}
