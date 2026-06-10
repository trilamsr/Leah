package main

import (
	"context"
	"log/slog"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/recommend"
)

// startRecommendDispatcher subscribes recommend.SignalDispatcher to bus and
// drives engine.OnSignal for every translated event. Returned stop drains
// the pump on ctx-cancel-or-daemon-shutdown. Caller owns bus + engine
// lifecycle; bus must be the default broadcaster so obs.EmitEvent reaches it.
func startRecommendDispatcher(ctx context.Context, lg *slog.Logger, registry *obs.Registry, bus *obs.Broadcaster, engine recommend.SignalEngine) (func(), error) {
	d := recommend.NewSignalDispatcher(engine, bus, nil).WithRegistry(registry)
	if err := d.Start(ctx); err != nil {
		lg.Warn("recommend dispatcher start failed", "err", err)
		return func() {}, err
	}
	return d.Stop, nil
}
