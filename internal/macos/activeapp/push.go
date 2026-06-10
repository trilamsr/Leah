package activeapp

import (
	"context"
	"errors"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/osevent"
)

// PushSource emits its own kind (never hud.state — that contract is owned
// by the HUD state machine).
type PushSource struct {
	Source  osevent.Source
	ObsEmit func(obs.Event)
}

func (p *PushSource) Run(ctx context.Context) error {
	if p.Source == nil {
		return errors.New("activeapp: PushSource.Source required")
	}
	ch, err := p.Source.Subscribe(ctx, osevent.WorkspaceActiveAppChanged)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if p.ObsEmit == nil {
				continue
			}
			bid, _ := ev.Detail["bundle_id"].(string)
			if bid == "" {
				continue
			}
			name, _ := ev.Detail["name"].(string)
			p.ObsEmit(obs.Event{
				Kind:    "workspace.active_app_changed",
				Actor:   "activeapp",
				Outcome: "ok",
				Payload: obs.WorkspaceActiveAppEvent{BundleID: bid, Name: name},
			})
		}
	}
}
