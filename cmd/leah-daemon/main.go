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
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/notify"
	"github.com/trilam/leah/internal/operatormodel"
	"github.com/trilam/leah/internal/patterns"
	"github.com/trilam/leah/internal/regattaclient"
	"github.com/trilam/leah/internal/selflearn"
	"github.com/trilam/leah/internal/selflearn/rules"
	"github.com/trilam/leah/internal/watchdog"
	"github.com/trilam/leah/internal/web"
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

	loop := daemonloop.New(
		rc,
		watchdog.New(),
		notify.NewDesktop(),
		a,
		os.Stdout,
		pollEvery,
	)
	loop.WeeklyTracker = filepath.Join(sd, "last-weekly.txt")
	loop.WeeklyHour = 9
	if v := os.Getenv("LEAH_DAEMON_WEEKLY_HOUR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
			loop.WeeklyHour = n
		}
	}
	loop.Weekly = buildWeeklyTasks(sd, auditPath, a, os.Stdout)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if *dashboardAddr != "" {
		store, err := memory.NewStore(filepath.Join(sd, "memory.db"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "leah-daemon: memory store: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = store.Close() }()
		startedAt := time.Now()
		srv := &web.Server{
			Addr:      *dashboardAddr,
			AuditPath: auditPath,
			Memory:    store,
			Regatta:   rc,
			Budget:    budget.New(),
			StartTime: startedAt,
			Heartbeat: func() time.Time { return time.Now() }, // TODO: wire daemonloop last-tick when exposed
		}
		go func() {
			if err := srv.Start(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "leah-daemon: dashboard: %v\n", err)
			}
		}()
		fmt.Fprintf(os.Stdout, "leah-daemon: dashboard at http://%s/dashboard\n", *dashboardAddr)
	}

	if err := loop.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "leah-daemon: %v\n", err)
		os.Exit(1)
	}
}

// buildWeeklyTasks returns the per-week tasks fired by daemonloop on the
// weekly tick: resolver back-fill, pattern detection → skill-candidates.md,
// weekly retro report, and operator-behavior profile rebuild.
func buildWeeklyTasks(sd, auditPath string, a *audit.Logger, out *os.File) []daemonloop.WeeklyTask {
	return []daemonloop.WeeklyTask{
		// 1. Resolver back-fills outcomes on pending audit rows.
		func(ctx context.Context) {
			r := &selflearn.Resolver{
				AuditPath: auditPath,
				Logger:    a,
				Rules: map[string]selflearn.Rule{
					"regatta.ship": rules.RegattaPR{},
				},
				Out: out,
			}
			if err := r.Run(ctx); err != nil {
				fmt.Fprintf(out, "leah-daemon: weekly resolver error: %v\n", err)
			}
		},
		// 2. Pattern detect → skill-candidates.md
		func(ctx context.Context) {
			since := time.Now().Add(-7 * 24 * time.Hour)
			clusters, err := patterns.Detect(auditPath, since)
			if err != nil {
				fmt.Fprintf(out, "leah-daemon: weekly patterns error: %v\n", err)
				return
			}
			md := patterns.Propose(clusters)
			path := filepath.Join(sd, "skill-candidates.md")
			if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
				fmt.Fprintf(out, "leah-daemon: weekly patterns write error: %v\n", err)
			}
		},
		// 3. Retro → retro-YYYY-WW.md
		func(ctx context.Context) {
			dbPath := filepath.Join(sd, "memory.db")
			store, err := selflearn.OpenMistakeStore(dbPath)
			if err != nil {
				fmt.Fprintf(out, "leah-daemon: weekly retro store error: %v\n", err)
				return
			}
			defer func() { _ = store.Close() }()
			retro := &selflearn.Retro{AuditPath: auditPath, Store: store}
			y, w := time.Now().UTC().ISOWeek()
			week := fmt.Sprintf("%04d-%02d", y, w)
			md, err := retro.Generate(week)
			if err != nil {
				fmt.Fprintf(out, "leah-daemon: weekly retro error: %v\n", err)
				return
			}
			path := filepath.Join(sd, fmt.Sprintf("retro-%s.md", week))
			if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
				fmt.Fprintf(out, "leah-daemon: weekly retro write error: %v\n", err)
			}
		},
		// 4. Operator-behavior profile rebuild (Wave 2-J).
		// Rebuilds operator_profile rows from the audit window; cold-start
		// gate inside Update() ensures Ready stays false until 50 rows + 7d.
		func(ctx context.Context) {
			dbPath := filepath.Join(sd, "memory.db")
			store, err := memory.NewStore(dbPath)
			if err != nil {
				fmt.Fprintf(out, "leah-daemon: weekly operatormodel store error: %v\n", err)
				return
			}
			defer func() { _ = store.Close() }()
			if err := operatormodel.UpdateProfile(ctx, store, auditPath); err != nil {
				fmt.Fprintf(out, "leah-daemon: weekly operatormodel error: %v\n", err)
			}
		},
	}
}

func stateDir() string {
	d := os.Getenv("LEAH_STATE_DIR")
	if d == "" {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".leah-state")
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir state dir: %v\n", err)
		os.Exit(1)
	}
	return d
}
