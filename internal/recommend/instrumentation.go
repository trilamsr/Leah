package recommend

import "github.com/trilam/leah/internal/obs"

// RegisterMetrics cold-seeds the recommendation-engine series from spec
// §4.13 so /metrics surfaces them pre-event.
func RegisterMetrics(registry *obs.Registry) {
	if registry == nil {
		return
	}
	registry.Counter("leah_recommendation_proposed_total").Add(map[string]string{"kind": "cold"}, 0)
	registry.Counter("leah_recommendation_accepted_total").Add(map[string]string{"kind": "cold"}, 0)
	registry.Counter("leah_recommendation_rejected_total").Add(map[string]string{"kind": "cold"}, 0)
	registry.Counter("leah_recommendation_applied_total").Add(map[string]string{"kind": "cold", "outcome": "cold"}, 0)
}

// EmitProposed bumps proposed_total{kind}.
func EmitProposed(registry *obs.Registry, kind string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_recommendation_proposed_total").Inc(map[string]string{"kind": kind})
}

// EmitAccepted bumps accepted_total{kind}.
func EmitAccepted(registry *obs.Registry, kind string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_recommendation_accepted_total").Inc(map[string]string{"kind": kind})
}

// EmitRejected bumps rejected_total{kind}.
func EmitRejected(registry *obs.Registry, kind string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_recommendation_rejected_total").Inc(map[string]string{"kind": kind})
}

// EmitApplied bumps applied_total{kind, outcome}.
func EmitApplied(registry *obs.Registry, kind, outcome string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_recommendation_applied_total").Inc(map[string]string{"kind": kind, "outcome": outcome})
}
