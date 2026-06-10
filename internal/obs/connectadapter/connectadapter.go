// Package connectadapter wraps the leah_connect_* series (spec §4.7) for
// per-provider adapters. One factory call per adapter replaces ~28 lines
// of near-identical boilerplate per package.
//
// Separation matters: ObserveAPI bumps api_call_total + api_latency_seconds,
// ObserveExchange bumps exchange_total only. Conflating them — as the
// initial W80 backfill did — would propagate to every adapter wiring PR.
package connectadapter

import "github.com/trilam/leah/internal/obs"

var latencyBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5}

// Metrics emits the shared leah_connect_* series for one provider.
type Metrics struct {
	provider string
	reg      *obs.Registry
}

// For returns a Metrics bound to provider. Nil registry is tolerated —
// methods become no-ops, matching the existing adapter convention.
func For(provider string, reg *obs.Registry) *Metrics {
	return &Metrics{provider: provider, reg: reg}
}

// Register declares all five series at this provider so /metrics surfaces
// them pre-event. Histograms use Declare (no fake 0-sample).
func (m *Metrics) Register() {
	if m == nil || m.reg == nil {
		return
	}
	p := m.provider
	m.reg.Counter("leah_connect_api_call_total").Declare(map[string]string{"provider": p, "endpoint": "cold"})
	m.reg.Counter("leah_connect_exchange_total").Declare(map[string]string{"provider": p, "outcome": "cold"})
	m.reg.Counter("leah_connect_refresh_total").Declare(map[string]string{"provider": p, "outcome": "cold"})
	m.reg.Gauge("leah_connect_token_age_seconds").Declare(map[string]string{"provider": p})
	m.reg.Histogram("leah_connect_api_latency_seconds", latencyBuckets).Declare(map[string]string{"provider": p})
}

// ObserveAPI records one adapter RPC against api_call_total and
// api_latency_seconds. outcome is not a label on these series (spec §4.7),
// callers route failures via ObserveExchange/ObserveRefresh as appropriate.
func (m *Metrics) ObserveAPI(endpoint string, latencySec float64) {
	if m == nil || m.reg == nil {
		return
	}
	m.reg.Counter("leah_connect_api_call_total").Inc(map[string]string{"provider": m.provider, "endpoint": endpoint})
	m.reg.Histogram("leah_connect_api_latency_seconds", latencyBuckets).Observe(map[string]string{"provider": m.provider}, latencySec)
}

// ObserveExchange bumps exchange_total{provider, outcome} for a real OAuth
// exchange event. Do NOT call from generic RPC paths — see spec §4.7.
func (m *Metrics) ObserveExchange(outcome string) {
	if m == nil || m.reg == nil {
		return
	}
	m.reg.Counter("leah_connect_exchange_total").Inc(map[string]string{"provider": m.provider, "outcome": outcome})
}

// ObserveRefresh bumps refresh_total{provider, outcome} for a real token
// refresh event.
func (m *Metrics) ObserveRefresh(outcome string) {
	if m == nil || m.reg == nil {
		return
	}
	m.reg.Counter("leah_connect_refresh_total").Inc(map[string]string{"provider": m.provider, "outcome": outcome})
}

// SetTokenAge writes token_age_seconds{provider}.
func (m *Metrics) SetTokenAge(ageSec float64) {
	if m == nil || m.reg == nil {
		return
	}
	m.reg.Gauge("leah_connect_token_age_seconds").Set(map[string]string{"provider": m.provider}, ageSec)
}
