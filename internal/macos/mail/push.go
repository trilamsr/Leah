package mail

import (
	"context"
	"errors"
	"time"

	"github.com/trilam/leah/internal/macos/sqliteopen"
	"github.com/trilam/leah/internal/obs"
)

// PushSource fans WAL-file FSEvents on Envelope Index-wal onto an obs event.
// Mail.app batches its writes — one SyncMailboxes pass produces dozens of WAL
// append events; the debounce collapses them into one downstream signal.
type PushSource struct {
	Watcher  sqliteopen.WALWatcher
	ObsEmit  func(obs.Event)
	Debounce time.Duration
	NowFn    func() time.Time
}

func (p *PushSource) Run(ctx context.Context) error {
	if p.Watcher == nil {
		return errors.New("mail: PushSource.Watcher required")
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
				Kind:    "mail_changed",
				Actor:   "mail",
				Outcome: "ok",
				Payload: obs.MailChangedEvent{},
			})
			// Stamp AFTER emit so a panicking ObsEmit cannot poison the next
			// window with a half-applied timestamp.
			lastEmit = t
		}
	}
}
