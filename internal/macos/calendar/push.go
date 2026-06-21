package calendar

import (
	"context"
	"errors"
	"time"

	"github.com/trilam/leah/internal/macos/sqliteopen"
	"github.com/trilam/leah/internal/obs"
)

// PushSource fans WAL-file FSEvents on Calendar.sqlitedb-wal onto an obs
// event. EventKit + iCloud sync writes multi-page WAL bursts per logical
// mutation; the debounce collapses each burst into one downstream signal.
// NowFn is test-injectable so the suppression window is wall-clock-independent.
type PushSource struct {
	Watcher  sqliteopen.WALWatcher
	ObsEmit  func(obs.Event)
	Debounce time.Duration
	NowFn    func() time.Time
}

func (p *PushSource) Run(ctx context.Context) error {
	if p.Watcher == nil {
		return errors.New("calendar: PushSource.Watcher required")
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
				Kind:    "calendar.store_changed",
				Actor:   "calendar",
				Outcome: "ok",
				Payload: obs.CalendarStoreChangedEvent{},
			})
			// Stamp AFTER emit so a panicking ObsEmit cannot poison the next
			// window with a half-applied timestamp.
			lastEmit = t
		}
	}
}
