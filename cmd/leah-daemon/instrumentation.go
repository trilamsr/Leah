package main

import (
	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/dispatcher"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/voice"
)

func wireObs(
	registry *obs.Registry,
	health *obs.HealthRegistry,
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

	health.Register("audit", &audit.SelfChecker{Logger: a})
	health.Register("memory", &memory.SelfChecker{Store: store})
	health.Register("daemonloop", &daemonloop.SelfChecker{Loop: loop})
	health.Register("voice", &voice.SelfChecker{Chain: chain})
	health.Register("dispatcher", &dispatcher.SelfChecker{Reasoner: nil})
}
