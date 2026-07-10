package contacts

import (
	"context"
	"errors"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/osevent"
)

// PushSource fans CNContactStoreDidChangeNotification onto an obs event.
// Debounce collapses the multi-fire burst CN posts per write — Apple does not
// promise one notification per change. NowFn is test-injectable so the
// suppression window is wall-clock-independent in unit tests.
type PushSource struct {
	Source   osevent.Source
	ObsEmit  func(telemetry.Event)
	Debounce time.Duration
	NowFn    func() time.Time
}

func (p *PushSource) Run(ctx context.Context) error {
	if p.Source == nil {
		return errors.New("contacts: PushSource.Source required")
	}
	now := p.NowFn
	if now == nil {
		now = time.Now
	}
	debounce := p.Debounce
	if debounce < 0 {
		debounce = 0
	}
	ch, err := p.Source.Subscribe(ctx, osevent.ContactStoreChanged)
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
				Kind:    "contact_store_changed",
				Actor:   "contacts",
				Outcome: "ok",
				Payload: telemetry.ContactStoreChangedEvent{},
			})
			// Stamp AFTER emit so a panicking ObsEmit cannot poison the next
			// window with a half-applied timestamp.
			lastEmit = t
		}
	}
}
