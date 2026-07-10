package main

import (
	"github.com/trilam/leah/internal/actions/adapters/discord"
	"github.com/trilam/leah/internal/actions/adapters/facetime"
	"github.com/trilam/leah/internal/actions/adapters/flights"
	"github.com/trilam/leah/internal/actions/adapters/gcal"
	"github.com/trilam/leah/internal/actions/adapters/gmail"
	"github.com/trilam/leah/internal/actions/adapters/imessage"
	"github.com/trilam/leah/internal/actions/adapters/maps"
	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/actions/connect"
	"github.com/trilam/leah/internal/platform/daemonloop"
	"github.com/trilam/leah/internal/actions/dispatcher"
	"github.com/trilam/leah/internal/memory/feeds"
	"github.com/trilam/leah/internal/thinking/intent"
	"github.com/trilam/leah/internal/thinking/learn"
	"github.com/trilam/leah/internal/memory/store"
	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/platform/onboarding"
	"github.com/trilam/leah/internal/thinking/operatormodel"
	"github.com/trilam/leah/internal/thinking/recommend"
	"github.com/trilam/leah/internal/input/voice"
	"github.com/trilam/leah/internal/input/voice/listener"
	voiceloop "github.com/trilam/leah/internal/input/voice/loop"
)

func wireObs(
	registry *telemetry.Registry,
	health *telemetry.HealthRegistry,
	a *audit.Logger,
	store *memory.Store,
	loop *daemonloop.Loop,
	chain *voice.ChainTTS,
) {
	audit.Bind(a, registry)
	daemonloop.Bind(loop, registry)
	if chain != nil {
		voice.Bind(chain, registry)
	}
	// Cold-start zero seeds so /metrics surfaces every series pre-event.
	registry.Counter("leah_audit_append_total").Add(nil, 0)
	registry.Counter("leah_daemonloop_tick_total").Add(nil, 0)
	registry.Counter("leah_dispatcher_ship_total").Add(map[string]string{"outcome": "cold"}, 0)
	registry.Counter("leah_memory_queries_total").Add(map[string]string{"table": "cold"}, 0)
	registry.Counter("leah_voice_speak_total").Add(map[string]string{"backend": "cold"}, 0)
	registry.Counter("leah_attestation_attempts_total").Add(map[string]string{"scope": "cold", "outcome": "cold"}, 0)
	registry.Counter("leah_web_requests_total").Add(map[string]string{"path": "cold", "status": "0"}, 0)

	// Metric inventory backfill — register per-package series.
	gmail.RegisterMetrics(registry)
	gcal.RegisterMetrics(registry)
	imessage.RegisterMetrics(registry)
	facetime.RegisterMetrics(registry)
	discord.RegisterMetrics(registry)
	maps.RegisterMetrics(registry)
	flights.RegisterMetrics(registry)
	connect.RegisterMetrics(registry)
	feeds.RegisterMetrics(registry)
	intent.RegisterMetrics(registry)
	recommend.RegisterMetrics(registry)
	learn.RegisterMetrics(registry)
	operatormodel.RegisterMetrics(registry)
	listener.RegisterMetrics(registry)
	voiceloop.RegisterMetrics(registry)
	onboarding.RegisterMetrics(registry)

	health.Register("audit", &audit.SelfChecker{Logger: a})
	health.Register("memory", &memory.SelfChecker{Store: store})
	health.Register("daemonloop", &daemonloop.SelfChecker{Loop: loop})
	health.Register("voice", &voice.SelfChecker{Chain: chain})
	health.Register("dispatcher", &dispatcher.SelfChecker{Reasoner: nil})
}
