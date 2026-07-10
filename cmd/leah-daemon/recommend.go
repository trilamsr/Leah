package main

import (
	"context"
	"log/slog"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/thinking/recommend"
)

func startRecommendDispatcher(ctx context.Context, lg *slog.Logger, registry *telemetry.Registry, bus *telemetry.Broadcaster, engine recommend.SignalEngine) (func(), error) {
	d := recommend.NewSignalDispatcher(engine, bus, nil).WithRegistry(registry)
	if err := d.Start(ctx); err != nil {
		lg.Warn("recommend dispatcher start failed", "err", err)
		return func() {}, err
	}
	return d.Stop, nil
}

// identitySignalMatcher is a placeholder until pattern-bearing matchers land.
type identitySignalMatcher struct{}

func (identitySignalMatcher) Match(_ context.Context, sig recommend.Signal) ([]recommend.Recommendation, error) {
	return []recommend.Recommendation{{
		ID:         sig.Kind + ":" + sig.Detail,
		Pattern:    sig.Kind + ":" + sig.Detail,
		Tier:       recommend.TierSilent,
		Source:     "signal.identity",
		Confidence: 0.5,
		CreatedAt:  sig.At,
	}}, nil
}
