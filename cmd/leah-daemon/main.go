// Package main is the leah-daemon composition root. Per-subsystem wiring
// (notifier, weekly tasks, backup, brief, dashboard, metrics snapshotter,
// attestation adapter) lives in sibling files (notify.go, weekly.go,
// backup.go, brief.go, dashboard.go, obs.go, attestation.go, state.go) so
// parallel agents adding new daemon wiring touch their own file instead of
// stacking diffs onto this one.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget/monthly"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/eval"
	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/macos/activeapp"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/onboarding"
	"github.com/trilam/leah/internal/reasoner"
	"github.com/trilam/leah/internal/recommend"
	"github.com/trilam/leah/internal/regattaclient"
	"github.com/trilam/leah/internal/tts"
	"github.com/trilam/leah/internal/tts/apple"
	"github.com/trilam/leah/internal/tts/elevenlabs"
	"github.com/trilam/leah/internal/voice"
	"github.com/trilam/leah/internal/watchdog"
)

func main() {
	dashboardAddr := flag.String("dashboard", "", "if non-empty, serve JARVIS dashboard at this addr (e.g. 127.0.0.1:8080); default off")
	logLevel := flag.String("log-level", "info", "log verbosity: debug|info|warn|error")
	flag.Parse()

	// Priority: explicit -log-level flag > LEAH_LOG_LEVEL env > default "info".
	// flag.Visit walks only flags that were actually set on the command line,
	// so passing -log-level=info (the default value) still overrides env.
	var flagSet bool
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "log-level" {
			flagSet = true
		}
	})
	if flagSet || os.Getenv("LEAH_LOG_LEVEL") == "" {
		_ = os.Setenv("LEAH_LOG_LEVEL", *logLevel)
	}

	pollEvery := 30 * time.Second
	if v := os.Getenv("LEAH_DAEMON_POLL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pollEvery = time.Duration(n) * time.Second
		}
	}

	sd := stateDir()
	// A9 SLA anchor: idempotent — relaunch / crash recovery cannot reset.
	_ = onboarding.MarkInstalled(sd, time.Now())
	auditPath := filepath.Join(sd, "audit.jsonl")
	a := &audit.Logger{Path: auditPath}
	rc := regattaclient.New()

	errRing := obs.NewErrorRing(10)
	lg, closeLog, err := obs.NewLoggerWithRing(errRing)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: obs logger: %v\n", err)
		os.Exit(1)
	}
	defer closeLog()
	registry := obs.NewRegistry()
	health := obs.NewHealthRegistry()
	_ = os.MkdirAll(filepath.Join(sd, "panics"), 0o700)

	// Voice opt-in: LEAH_VOICE_ENABLED=1 wires VoiceNotify alongside Desktop
	// via Fanout. Default OFF — terminal-state transitions stay silent until
	// operator opts in (avoids unexpected TTS on every PR merge).
	logVoiceState(os.Stdout)

	// One shared discord adapter: the brief pushes through it and the dashboard
	// spam panel reads its limiter, so the panel reflects real send pressure.
	discordAdpt := connectedDiscordAdapter()

	loop := daemonloop.New(rc, watchdog.New(), buildNotifier(), a, os.Stdout, pollEvery)
	loop.WeeklyTracker = filepath.Join(sd, "last-weekly.txt")
	loop.WeeklyHour = 9
	if v := os.Getenv("LEAH_DAEMON_WEEKLY_HOUR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
			loop.WeeklyHour = n
		}
	}
	loop.Weekly = buildWeeklyTasks(sd, auditPath, a, os.Stdout)
	wireBriefSchedule(loop, sd, buildBriefTask(sd, rc, os.Stdout), os.Stdout)
	wireDegradedPull(loop, sd, rc, os.Stdout)

	store, err := memory.NewStore(filepath.Join(sd, "memory.db"))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: memory: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	var chain *voice.ChainTTS
	if t, ok := voice.NewTTS().(*voice.ChainTTS); ok {
		chain = t
	}

	wireObs(registry, health, a, store, loop, chain)
	wireConsolidation(loop, store, a, auditPath, sd)

	// Optional producers constructed at the composition root so the orphan-
	// package scan sees them live; ambient capture (sync, a2a, vision live)
	// stays OFF until operator toggles via Settings. Field-disjoint owner:
	// composition_root.go; IPC edges land separately without colliding.
	p4 := wirePhase4Producers(store.DB(), os.Stderr, lg)
	defer func() {
		if p4.Discovery != nil {
			p4.Discovery.Stop()
		}
		if p4.A2A != nil {
			_ = p4.A2A.Stop(context.Background())
		}
		if p4.Supervisor != nil {
			_ = p4.Supervisor.Close()
		}
	}()
	_ = p4 // IPC edges (Recommendations / Budget / Plugins panes) consume in follow-up

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Detect regatta mode at boot, wire Gated wrapper if a transport is
	// available; ModeNone is a graceful skip (operator runs `leah connect
	// regatta`). Return value is intentionally discarded — no daemon-side
	// caller consumes Ship/Review yet.
	if _, err := bootRegatta(ctx, bootRegattaOpts{
		Attestor: noopAttestor{},
		Audit:    newAuditLoggerSink(a),
		Logger:   os.Stderr,
		Registry: registry,
	}); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: regatta boot non-fatal: %v\n", err)
	}

	startActiveAppPush(ctx, lg, registry, activeapp.DefaultBlocklist)
	obs.SafeGo(lg, registry, "pushsource-runtime", func() {
		runPushSources(ctx, lg, registry, nil, func(kind string, _ []byte) { lg.Debug("push frame", "kind", kind) })
	})

	bus := obs.NewBroadcaster()
	obs.SetDefaultBroadcaster(bus)
	engine := recommend.NewMemoryEngine(a)
	engine.RegisterMatcher(identitySignalMatcher{})
	stopRec, _ := startRecommendDispatcher(ctx, lg, registry, bus, engine)
	defer stopRec()
	stopInbound, _ := startInboundDiscord(ctx, inboundOpts{StateDir: sd, Engine: engine, Audit: a})
	defer stopInbound()
	if br, err := recommend.StartSignalBridge(ctx, bus, engine, recommend.SignalBridgeOptions{
		Kinds: []string{
			"safari.history_changed", "messages_changed", "mail_changed", "notes_changed",
			"contact_store_changed", "workspace.active_app_changed", "focus.state_changed",
		},
	}); err == nil {
		defer br.Stop()
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: signal bridge non-fatal: %v\n", err)
	}

	// Daemon-side 1-minute rollover timer so an idle daemon rolls the month
	// within ≤60 s of midnight. Cap defaults to $50; LEAH_COST_MONTH_CAP
	// overrides.
	cmCap := 50.0
	if v := os.Getenv("LEAH_COST_MONTH_CAP"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 {
			cmCap = parsed
		}
	}
	if cm, err := monthly.OpenAt(filepath.Join(sd, "cost-month.json"), cmCap, time.Now()); err == nil {
		obs.SafeGo(lg, registry, "monthly-cost-rollover", func() {
			cm.RunRolloverLoop(ctx, time.Minute, func(err error) {
				_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: monthly-cost rollover: %v\n", err)
			})
		})
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: monthly-cost open non-fatal: %v\n", err)
	}

	snapPath := startMetricsSnapshotter(ctx, lg, registry, sd)
	startLogRetention(ctx, lg, registry, sd)

	if *dashboardAddr != "" {
		closeDash, err := startDashboard(ctx, *dashboardAddr, sd, auditPath, snapPath, rc, loop, registry, health, store, discordAdpt)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: %v\n", err)
			os.Exit(1)
		}
		defer closeDash()
	}

	home, err := os.UserHomeDir()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: home dir: %v\n", err)
		os.Exit(1)
	}
	sockPath := filepath.Join(home, "Library", "Caches", "Leah", "leah.sock")
	sonnet, err := reasoner.NewAnthropicClient()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: reasoning backend init: %v (ask/answer path will error until key is set)\n", err)
		sonnet = nil
	}
	// store.DB() already has conversation_turn from the schema migration above.
	// knowledge.Graph wiring is deferred; nil skips RAG prepend.
	ttsClass := tts.NewBlockwordClassifier()
	ttsCloud, ttsLocal := buildTTSProviders()
	ipcDeps := IPCDeps{
		A2A:  newA2AIPCAdapter(p4.A2A),
		Sync: newSyncIPCAdapter(newDiscoveryEngine(p4.Discovery), store.DB()),
	}
	obs.SafeGo(lg, registry, "ipc-server", func() {
		_ = ipc.NewServer(sockPath, newIPCHandlerWithDeps(sonnet, store.DB(), nil, errRing, ttsCloud, ttsLocal, ttsClass, nil, ipcDeps)).Serve(ctx)
	})
	startMCPPublish(ctx, lg, registry, os.Stderr)

	evalDone := make(chan struct{})
	close(evalDone) // default: noop close-on-exit when LEAH_EVAL is unset
	if os.Getenv("LEAH_EVAL") == "1" && sonnet != nil {
		if evalStore, err := eval.OpenStore(filepath.Join(sd, "eval.db")); err == nil {
			evalInterval := time.Hour
			if v := os.Getenv("LEAH_EVAL_INTERVAL_SECONDS"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					evalInterval = time.Duration(n) * time.Second
				}
			}
			evalDir := os.Getenv("LEAH_EVAL_FIXTURES_DIR")
			if evalDir == "" {
				evalDir = filepath.Join(sd, "eval-fixtures")
			}
			evalDone = make(chan struct{})
			sched := &eval.Scheduler{
				Asker:       evalAskerFunc(func(ctx context.Context, q string) (string, error) { return sonnet.OneShot(ctx, "", q) }),
				FixturesDir: evalDir,
				Store:       evalStore,
				Interval:    evalInterval,
			}
			obs.SafeGo(lg, registry, "eval-scheduler", func() {
				defer close(evalDone)
				defer func() { _ = evalStore.Close() }()
				sched.Run(ctx)
			})
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: eval store open non-fatal: %v\n", err)
		}
	}

	if err := loop.Run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: %v\n", err)
		os.Exit(1)
	}
	<-evalDone
}

type evalAskerFunc func(context.Context, string) (string, error)

func (f evalAskerFunc) Ask(ctx context.Context, q string) (string, error) { return f(ctx, q) }

// buildTTSProviders constructs the §17.17 cloud + local pair. Missing
// LEAH_ELEVENLABS_API_KEY degrades to Apple-only — operator can still ask
// Leah out loud for sensitive content; cloud arrives once they `leah connect`.
// Apple's emit fn is a no-op: handleTTSSpeak short-circuits on Name()=="apple-ava"
// and emits the IPC frame itself, so the provider-side emit would only duplicate.
func buildTTSProviders() (cloud, local tts.Provider) {
	if c, err := elevenlabs.FromEnv(nil); err == nil {
		cloud = c
	}
	local = apple.New(func(ipc.Frame) {})
	return cloud, local
}
