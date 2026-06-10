package dispatcher

import (
	"context"

	"github.com/trilam/leah/internal/obs"
)

func Counter(registry *obs.Registry) func(string) {
	if registry == nil {
		return nil
	}
	c := registry.Counter("leah_dispatcher_ship_total")
	return func(outcome string) {
		c.Inc(map[string]string{"outcome": outcome})
	}
}

type SelfChecker struct {
	Reasoner Reasoner
	Probe    string
}

func (c *SelfChecker) SelfCheck(ctx context.Context) error {
	if c == nil || c.Reasoner == nil {
		return nil
	}
	probe := c.Probe
	if probe == "" {
		probe = "ping"
	}
	_, err := c.Reasoner.Ask(ctx, probe)
	return err
}

func EmitShipCount(registry *obs.Registry, outcome string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_dispatcher_ship_total").Inc(map[string]string{"outcome": outcome})
}
