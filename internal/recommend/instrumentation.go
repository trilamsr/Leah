package recommend

import "github.com/trilam/leah/internal/obs"

func RegisterMetrics(registry *obs.Registry) {
	if registry == nil {
		return
	}
	registry.Counter("leah_recommendation_proposed_total").Declare(map[string]string{"kind": "cold"})
	registry.Counter("leah_recommendation_accepted_total").Declare(map[string]string{"kind": "cold"})
	registry.Counter("leah_recommendation_rejected_total").Declare(map[string]string{"kind": "cold"})
	registry.Counter("leah_recommendation_applied_total").Declare(map[string]string{"kind": "cold", "outcome": "cold"})
}

func EmitProposed(registry *obs.Registry, kind string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_recommendation_proposed_total").Inc(map[string]string{"kind": kind})
}

func EmitAccepted(registry *obs.Registry, kind string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_recommendation_accepted_total").Inc(map[string]string{"kind": kind})
}

func EmitRejected(registry *obs.Registry, kind string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_recommendation_rejected_total").Inc(map[string]string{"kind": kind})
}

func EmitApplied(registry *obs.Registry, kind, outcome string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_recommendation_applied_total").Inc(map[string]string{"kind": kind, "outcome": outcome})
}
