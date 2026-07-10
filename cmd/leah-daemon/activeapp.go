package main

import (
	"context"
	"log/slog"

	"github.com/trilam/leah/internal/macos/activeapp"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/osevent"
)

func startActiveAppPush(ctx context.Context, lg *slog.Logger, registry *telemetry.Registry, blocklist []string) {
	if blocklist == nil {
		blocklist = activeapp.DefaultBlocklist
	}
	src, err := osevent.NewSource(osevent.Config{Blocklist: blocklist})
	if err != nil {
		lg.Warn("activeapp push disabled", "err", err)
		return
	}
	p := &activeapp.PushSource{
		Source:  src,
		ObsEmit: func(e telemetry.Event) { telemetry.EmitEvent(ctx, e) },
	}
	telemetry.SafeGo(lg, registry, "activeapp-push", func() {
		if err := p.Run(ctx); err != nil && ctx.Err() == nil {
			lg.Warn("activeapp push exit", "err", err)
		}
		_ = src.Close()
	})
}
