package activeapp

import (
	"context"
	"errors"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/input/osevent"
)

// DefaultDebounce: rapid cmd-tab through multiple apps fires NSWorkspace events
// faster than the recommender can act on. 250ms drops the storm without
// noticeably delaying a real intentional switch.
const DefaultDebounce = 250 * time.Millisecond

// PushSource emits its own kind (never hud.state — that contract is owned
// by the HUD state machine). Coalesce (same bundle twice in a row) + debounce
// (rapid distinct switches) are applied here, not in the osevent pump, so the
// pump stays a faithful mirror of NSWorkspace for other consumers.
type PushSource struct {
	Source   osevent.Source
	ObsEmit  func(telemetry.Event)
	Debounce time.Duration    // 0 disables; <0 treated as 0
	NowFn    func() time.Time // injectable for hermetic tests
}

func (p *PushSource) Run(ctx context.Context) error {
	if p.Source == nil {
		return errors.New("activeapp: PushSource.Source required")
	}
	ch, err := p.Source.Subscribe(ctx, osevent.WorkspaceActiveAppChanged)
	if err != nil {
		return err
	}
	now := p.NowFn
	if now == nil {
		now = time.Now
	}
	debounce := p.Debounce
	if debounce < 0 {
		debounce = 0
	}

	var lastBundle string
	var lastEmit time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				// Source.Subscribe closes ch on ctx.Done; if cancel raced the
				// select, the !ok branch can win even though Run was canceled.
				// Surface ctx.Err so callers can distinguish cancel from EOF.
				if err := ctx.Err(); err != nil {
					return err
				}
				return nil
			}
			if p.ObsEmit == nil {
				continue
			}
			bid, _ := ev.Detail["bundle_id"].(string)
			if bid == "" {
				continue
			}
			if bid == lastBundle {
				continue
			}
			t := now()
			if debounce > 0 && !lastEmit.IsZero() && t.Sub(lastEmit) < debounce {
				continue
			}
			name, _ := ev.Detail["name"].(string)
			p.ObsEmit(telemetry.Event{
				Kind:    "workspace.active_app_changed",
				Actor:   "activeapp",
				Outcome: "ok",
				Payload: telemetry.WorkspaceActiveAppEvent{BundleID: bid, Name: name},
			})
			lastBundle = bid
			lastEmit = t
		}
	}
}
