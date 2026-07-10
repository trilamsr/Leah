package focus

import (
	"context"
	"errors"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/input/osevent"
)

// PushSource fans Focus/DnD state-change notifications onto an obs event.
// Debounce collapses the multi-fire burst pmset / DND-Assertions write per
// toggle into one downstream signal. NowFn is test-injectable so the
// suppression window is wall-clock-independent in unit tests.
type PushSource struct {
	Source   osevent.Source
	ObsEmit  func(telemetry.Event)
	Debounce time.Duration
	NowFn    func() time.Time
}

func (p *PushSource) Run(ctx context.Context) error {
	if p.Source == nil {
		return errors.New("focus: PushSource.Source required")
	}
	now := p.NowFn
	if now == nil {
		now = time.Now
	}
	debounce := p.Debounce
	if debounce < 0 {
		debounce = 0
	}
	ch, err := p.Source.Subscribe(ctx, osevent.FocusStateChanged)
	if err != nil {
		return err
	}
	var lastEmit time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-ch:
			if !ok {
				return nil
			}
			if p.ObsEmit == nil {
				continue
			}
			t := now()
			if debounce > 0 && !lastEmit.IsZero() && t.Sub(lastEmit) < debounce {
				continue
			}
			p.ObsEmit(telemetry.Event{
				Kind:    "focus.state_changed",
				Actor:   "focus",
				Outcome: "ok",
				Payload: telemetry.FocusStateChangedEvent{},
			})
			// Stamp AFTER emit so a panicking ObsEmit cannot poison the next
			// window with a half-applied timestamp.
			lastEmit = t
		}
	}
}
