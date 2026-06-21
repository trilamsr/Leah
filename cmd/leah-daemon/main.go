// Package main is the leah-daemon composition root. Per-subsystem wiring
// (notifier, weekly tasks, backup, brief, dashboard, metrics snapshotter,
// attestation adapter) lives in sibling files (notify.go, weekly.go,
// backup.go, brief.go, dashboard.go, obs.go, attestation.go, state.go) so
// parallel agents adding new daemon wiring touch their own file instead of
// stacking diffs onto this one (Wave 7 god-file retro: KK Fanout / JJ
// brief cron / MM backup tasks all collided on main.go).
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
	"github.com/trilam/leah/internal/costmonth"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/macos/activeapp"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/recommend"
	"github.com/trilam/leah/internal/regattaclient"
	"github.com/trilam/leah/internal/voice"
	"github.com/trilam/leah/internal/watchdog"
)

func main() {
	dashboardAddr := flag.String("dashboard", "", "if non-empty, serve JARVIS dashboard at this addr (e.g. 127.0.0.1:8080); default off")
	flag.Parse()

	pollEvery := 30 * time.Second
	if v := os.Getenv("LEAH_DAEMON_POLL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pollEvery = time.Duration(n) * time.Second
		}
	}

	sd := stateDir()
	auditPath := filepath.Join(sd, "audit.jsonl")
	a := &audit.Logger{Path: auditPath}
	rc := regattaclient.New()

	lg, closeLog, err := obs.NewLogger()
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
	wireBriefSchedule(loop, sd, buildBriefTask(sd, rc, os.Stdout, discordAdpt), os.Stdout)
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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// W41: detect regatta mode at boot, wire Gated wrapper if a transport is
	// available; ModeNone is a graceful skip (operator runs `leah connect
	// regatta`). Return value is intentionally discarded — no daemon-side
	// caller consumes Ship/Review yet; subsequent waves will thread it through.
	if _, err := bootRegatta(ctx, bootRegattaOpts{
		Attestor: noopAttestor{},
		Audit:    newAuditLoggerSink(a),
		Logger:   os.Stderr,
		Registry: registry,
	}); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: regatta boot non-fatal: %v\n", err)
	}

	startActiveAppPush(ctx, lg, registry, activeapp.DefaultBlocklist)

	bus := obs.NewBroadcaster()
	obs.SetDefaultBroadcaster(bus)
	engine := recommend.NewMemoryEngine(a)
	engine.RegisterMatcher(identitySignalMatcher{})
	stopRec, _ := startRecommendDispatcher(ctx, lg, registry, bus, engine)
	defer stopRec()

	// W94: spec §6.3 mandates a daemon-side 1-minute rollover timer so a
	// daemon-idle midnight rolls the month within ≤60 s. Cap defaults to
	// $50; LEAH_COST_MONTH_CAP overrides per spec §7.1.
	cmCap := 50.0
	if v := os.Getenv("LEAH_COST_MONTH_CAP"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 {
			cmCap = parsed
		}
	}
	if cm, err := costmonth.OpenAt(filepath.Join(sd, "cost-month.json"), cmCap, time.Now()); err == nil {
		go cm.RunRolloverLoop(ctx, time.Minute, func(err error) {
			_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: costmonth rollover: %v\n", err)
		})
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: costmonth open non-fatal: %v\n", err)
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

	if err := loop.Run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: %v\n", err)
		os.Exit(1)
	}
}
