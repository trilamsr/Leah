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
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	snapPath := startMetricsSnapshotter(ctx, lg, registry, sd)

	if *dashboardAddr != "" {
		closeDash, err := startDashboard(ctx, *dashboardAddr, sd, auditPath, snapPath, rc, loop, registry, health)
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
