package reminders

import (
	"context"
	"errors"
	"time"

	"github.com/trilam/leah/internal/macos/sqliteopen"
	"github.com/trilam/leah/internal/obs"
)

// PushSource fans WAL-file FSEvents on the Reminders Group Container store
// onto an obs event. Reminders.app + CloudKit sync writes WAL bursts per
// logical change; the debounce collapses each burst into one downstream signal.
type PushSource struct {
	Watcher  sqliteopen.WALWatcher
	ObsEmit  func(obs.Event)
	Debounce time.Duration
	NowFn    func() time.Time
}

func (p *PushSource) Run(ctx context.Context) error {
	if p.Watcher == nil {
		return errors.New("reminders: PushSource.Watcher required")
	}
	now := p.NowFn
	if now == nil {
		now = time.Now
	}
	debounce := p.Debounce
	if debounce < 0 {
		debounce = 0
	}
	ch, err := p.Watcher.Subscribe(ctx)
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
			p.ObsEmit(obs.Event{
				Kind:    "reminders.store_changed",
				Actor:   "reminders",
				Outcome: "ok",
				Payload: obs.RemindersStoreChangedEvent{},
			})
			// Stamp AFTER emit so a panicking ObsEmit cannot poison the next
			// window with a half-applied timestamp.
			lastEmit = t
		}
	}
}
