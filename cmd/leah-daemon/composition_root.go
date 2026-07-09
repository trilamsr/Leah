package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"fmt"
	"io"
	"log/slog"

	"github.com/trilam/leah/internal/a2a"
	"github.com/trilam/leah/internal/attest"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/learn"
	mcpInbound "github.com/trilam/leah/internal/mcp/inbound"
	"github.com/trilam/leah/internal/plugin"
	"github.com/trilam/leah/internal/supervisor"
	"github.com/trilam/leah/internal/sync/discovery"
)

// phase4Producers holds the Phase 4 subsystems constructed at the daemon
// composition root. Field-disjoint from main.go so parallel agents wiring
// Recommendations / Budget / A2A / Plugins IPC each add their handler edge
// against this struct without colliding on main.go.
//
// Default-OFF invariant (Phase 4 §0.2 #3): Sync discovery, A2A server, and
// continuous vision/voice capture are constructed but NOT started here.
// Settings → Sync / Settings → Peers / Settings → Vision toggles drive
// Start() — operator-explicit, never on by default.
type phase4Producers struct {
	Recommender learn.Recommender
	Budget      *budget.Budget
	Verifier    attest.Verifier
	Discovery   discovery.Discovery // default OFF; Settings → Sync starts it
	A2A         *a2a.Server         // default OFF; Settings → Peers starts Listen
	A2AConsent  *a2a.ConsentStore
	PluginHost  plugin.Host
	Supervisor  *supervisor.Supervisor
	MCPInbound  *mcpInbound.Server // default OFF; Settings → Connections issues tokens + starts transport
}

// wirePhase4Producers constructs each Phase 4 producer with deps drawn from
// the daemon's existing composition root. Failures here are non-fatal: a
// stub producer is held in place so downstream IPC edges can probe-and-skip
// rather than crash on nil. Stubs are logged so post-deploy diagnostics
// surface "feature constructed but degraded" without an operator-side panic.
//
// STUBBED (Phase 4 infra not yet shipped):
//   - attest.Config.SelfPath / SelfExpectedSHA256 / SelfSignedBy — release
//     signing pipeline ships separately; verifier returns Unknown until then.
//   - a2a.Server.Identity — ephemeral ed25519 keypair per process; persistent
//     identity ships with the pair-setup CLI.
//   - plugin.Sandbox uses default TaskPolicy{} — bundle-specific policy
//     arrives with the manifest schema v1.
//   - mcp/inbound TokenStore uses InMemory backend — sqlstore mcp_token
//     migration ships with the Settings → Connections issue flow; tokens
//     re-issue on restart until then (server stays OFF by default).
func wirePhase4Producers(db *sql.DB, errOut io.Writer, lg *slog.Logger) phase4Producers {
	p := phase4Producers{
		Recommender: learn.New(db),
		Budget:      budget.New(),
		Verifier:    attest.NewVerifier(attest.Config{}), // STUB: SelfPath unset → Unknown verdict
		Discovery:   discovery.New(),                     // constructed only — operator toggles Start
		A2AConsent:  a2a.NewConsentStore(db),
		Supervisor:  supervisor.New(supervisor.Config{}),
		MCPInbound:  mcpInbound.New(mcpInbound.NewTokenStore(mcpInbound.InMemory())),
	}
	// nil deps → tools error at call (loud misconfig); Settings wires real deps.
	// Recover matches the plugin/a2a soft-fail pattern — a MustRegister collision
	// must not crash the daemon at boot.
	func() {
		defer func() {
			if r := recover(); r != nil {
				_, _ = fmt.Fprintf(errOut, "leah-daemon: mcp inbound register non-fatal: %v\n", r)
				p.MCPInbound = nil
			}
		}()
		mcpInbound.RegisterFirstParty(p.MCPInbound, mcpInbound.FirstPartyDeps{})
	}()

	// a2a.Server: ephemeral identity (STUB — persistent key arrives with pair
	// flow). Handler is nil here; daemon-side ipc edge sets it before Listen.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "leah-daemon: a2a identity non-fatal: %v\n", err)
	} else {
		p.A2A = a2a.NewServer(priv, p.A2AConsent, p.Budget, a2a.CapAsk|a2a.CapMemorySearch, nil)
	}

	host, err := plugin.NewHost(plugin.HostConfig{
		DB:       db,
		Verifier: p.Verifier,
		Sandbox:  plugin.NewSandbox(nil), // STUB: default TaskPolicy
		Quota:    plugin.NewQuotaMeter(nil),
	})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "leah-daemon: plugin host non-fatal: %v\n", err)
	} else {
		p.PluginHost = host
	}

	lg.Info("phase4.producers.wired",
		"recommender", p.Recommender != nil,
		"budget", p.Budget != nil,
		"verifier", p.Verifier != nil,
		"discovery", p.Discovery != nil,
		"a2a_server", p.A2A != nil,
		"plugin_host", p.PluginHost != nil,
		"supervisor", p.Supervisor != nil,
		"mcp_inbound", p.MCPInbound != nil,
		"ambient_capture", "off",
	)
	return p
}
