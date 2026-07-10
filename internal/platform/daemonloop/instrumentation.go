package daemonloop

import (
	"context"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
)

func Bind(l *Loop, registry *telemetry.Registry) {
	if registry == nil || l == nil {
		return
	}
	tickC := registry.Counter("leah_daemonloop_tick_total")
	ageG := registry.Gauge("leah_daemonloop_last_tick_age_seconds")
	l.OnTick = func(now time.Time) {
		tickC.Inc(nil)
		ageG.Set(nil, 0)
		_ = now
	}
}

type SelfChecker struct {
	Loop      *Loop
	Threshold time.Duration
	Now       func() time.Time
}

func (c *SelfChecker) SelfCheck(ctx context.Context) error {
	_ = ctx
	if c == nil || c.Loop == nil {
		return nil
	}
	thr := c.Threshold
	if thr <= 0 {
		thr = 5 * time.Minute
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	last := c.Loop.LastTick()
	if last.IsZero() {
		return nil
	}
	if age := now().Sub(last); age > thr {
		return fmt.Errorf("daemonloop stale: last tick %v ago (> %v)", age, thr)
	}
	return nil
}
