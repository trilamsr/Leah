package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/backup"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/notify"
	"github.com/trilam/leah/internal/obs"
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

	// obs wiring: logger + metrics registry + panic dir. SafeGo on
	// snapshotter so a write failure can't crash the daemon.
	lg, closeLog, err := obs.NewLogger()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: obs logger: %v\n", err)
		os.Exit(1)
	}
	defer closeLog()
	registry := obs.NewRegistry()
	_ = os.MkdirAll(filepath.Join(sd, "panics"), 0o700)

	// Voice opt-in: LEAH_VOICE_ENABLED=1 wires VoiceNotify alongside Desktop
	// via Fanout. Default OFF — terminal-state transitions stay silent until
	// operator opts in (avoids unexpected TTS on every PR merge).
	nf := buildNotifier()
	if os.Getenv("LEAH_VOICE_ENABLED") == "1" {
		_, _ = fmt.Fprintln(os.Stdout, "leah-daemon: voice notifier enabled (LEAH_VOICE_ENABLED=1)")
	} else {
		_, _ = fmt.Fprintln(os.Stdout, "leah-daemon: voice notifier disabled (set LEAH_VOICE_ENABLED=1 to enable)")
	}

	loop := daemonloop.New(
		rc,
		watchdog.New(),
		nf,
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

	// 60s metrics snapshot to ~/.leah-state/metrics/latest.json. SafeGo
	// recovers panics into the panic dir + counter, never kills the daemon.
	snapPath := filepath.Join(sd, "metrics", "latest.json")
	_ = os.MkdirAll(filepath.Dir(snapPath), 0o700)
	obs.SafeGo(lg, registry, "metrics-snapshotter", func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := registry.Snapshot(snapPath); err != nil {
					lg.Error("metrics snapshot failed", "err", err)
				}
			}
		}
	})

	if *dashboardAddr != "" {
		store, err := memory.NewStore(filepath.Join(sd, "memory.db"))
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: memory store: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = store.Close() }()
		startedAt := time.Now()
		srv := &web.Server{
			Addr:        *dashboardAddr,
			AuditPath:   auditPath,
			MetricsPath: snapPath,
			Memory:      store,
			Regatta:     rc,
			Budget:      budget.New(),
			StartTime:   startedAt,
			Heartbeat:   func() time.Time { return time.Now() }, // TODO: wire daemonloop last-tick when exposed
			// 10s memoization absorbs the dashboard's 3s poll cadence so /api/state
			// re-scans audit.jsonl + sqlite at most ~once every 10s (H4 audit fix).
			CacheTTL: 10 * time.Second,
		}
		go func() {
			if err := srv.Start(ctx); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: dashboard: %v\n", err)
			}
		}()
		_, _ = fmt.Fprintf(os.Stdout, "leah-daemon: dashboard at http://%s/dashboard\n", *dashboardAddr)
	}

	if err := loop.Run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: %v\n", err)
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
				PanicDetectors: []selflearn.PanicDetector{rules.PanicRateRule{}},
				Out:            out,
			}
			if err := r.Run(ctx); err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly resolver error: %v\n", err)
			}
		},
		// 2. Pattern detect → skill-candidates.md
		func(ctx context.Context) {
			since := time.Now().Add(-7 * 24 * time.Hour)
			clusters, err := patterns.Detect(auditPath, since)
			if err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly patterns error: %v\n", err)
				return
			}
			md := patterns.Propose(clusters)
			path := filepath.Join(sd, "skill-candidates.md")
			if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly patterns write error: %v\n", err)
			}
		},
		// 3. Retro → retro-YYYY-WW.md
		func(ctx context.Context) {
			dbPath := filepath.Join(sd, "memory.db")
			store, err := selflearn.OpenMistakeStore(dbPath)
			if err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly retro store error: %v\n", err)
				return
			}
			defer func() { _ = store.Close() }()
			retro := &selflearn.Retro{
				AuditPath:          auditPath,
				Store:              store,
				AttestationScanner: daemonAttestationScanner(),
			}
			y, w := time.Now().UTC().ISOWeek()
			week := fmt.Sprintf("%04d-%02d", y, w)
			md, err := retro.Generate(week)
			if err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly retro error: %v\n", err)
				return
			}
			path := filepath.Join(sd, fmt.Sprintf("retro-%s.md", week))
			if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly retro write error: %v\n", err)
			}
		},
		// 4. Operator-behavior profile rebuild (Wave 2-J).
		// Rebuilds operator_profile rows from the audit window; cold-start
		// gate inside Update() ensures Ready stays false until 50 rows + 7d.
		func(ctx context.Context) {
			dbPath := filepath.Join(sd, "memory.db")
			store, err := memory.NewStore(dbPath)
			if err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly operatormodel store error: %v\n", err)
				return
			}
			defer func() { _ = store.Close() }()
			if err := operatormodel.UpdateProfile(ctx, store, auditPath); err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly operatormodel error: %v\n", err)
			}
		},
		// 5. Panic-rate detection → bug-fix-candidates.md + operator push.
		// Iterates resolver.PanicDetectors, appends Candidates to
		// ~/.leah-state/bug-fix-candidates.md (one BuildIssueBody section
		// per candidate w/ UTC timestamp), counts new vs sentinel, and
		// notifies the operator when new > 0. NEVER auto-dispatches to
		// regatta — operator reads the file + manually runs `leah
		// self-build` (the only structural defense against
		// self-modification drift; see
		// docs/specs/2026-06-09-bug-fix-self-build-hook.md).
		func(ctx context.Context) {
			detectors := []selflearn.PanicDetector{rules.PanicRateRule{}}
			var found []rules.Candidate
			for _, d := range detectors {
				pr, ok := d.(rules.PanicRateRule)
				if !ok {
					continue
				}
				cands, err := pr.Detect(ctx, sd)
				if err != nil {
					_, _ = fmt.Fprintf(out, "leah-daemon: weekly panic-detect %s error: %v\n", d.Name(), err)
					continue
				}
				found = append(found, cands...)
			}

			candPath := filepath.Join(sd, "bug-fix-candidates.md")
			if len(found) > 0 {
				var b strings.Builder
				ts := time.Now().UTC().Format(time.RFC3339)
				for _, c := range found {
					_, _ = fmt.Fprintf(&b, "\n<!-- candidate appended %s -->\n", ts)
					b.WriteString(rules.BuildIssueBody(c))
					b.WriteString("\n---\n")
				}
				f, err := os.OpenFile(candPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
				if err != nil {
					_, _ = fmt.Fprintf(out, "leah-daemon: weekly bug-fix-candidates open error: %v\n", err)
				} else {
					if _, err := f.WriteString(b.String()); err != nil {
						_, _ = fmt.Fprintf(out, "leah-daemon: weekly bug-fix-candidates write error: %v\n", err)
					}
					_ = f.Close()
				}
			}

			// Compare against last-week sentinel to derive NEW count.
			sentinelPath := filepath.Join(sd, "bug-fix-last-count.txt")
			prev := 0
			if raw, err := os.ReadFile(sentinelPath); err == nil {
				if n, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
					prev = n
				}
			}
			total := prev + len(found)
			if err := os.WriteFile(sentinelPath, []byte(strconv.Itoa(total)), 0o600); err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly bug-fix sentinel write error: %v\n", err)
			}

			newCount := len(found)
			if newCount > 0 {
				title := "Leah: bug-fix candidates"
				body := fmt.Sprintf("Leah noticed %d bug candidates this week — review ~/.leah-state/bug-fix-candidates.md + run leah self-build", newCount)
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly panic-detect: %d new candidates → %s\n", newCount, candPath)
				if d := notify.NewDesktop(); d != nil {
					if err := d.Notify(ctx, title, body); err != nil {
						_, _ = fmt.Fprintf(out, "leah-daemon: weekly bug-fix desktop notify error: %v\n", err)
					}
				}
				if p := notify.NewPushover(); p != nil {
					if err := p.Notify(ctx, title, body); err != nil {
						// Missing credentials degrade silently — see Pushover.Notify.
						_, _ = fmt.Fprintf(out, "leah-daemon: weekly bug-fix pushover skip: %v\n", err)
					}
				}
			}
		},
		// 6. Restic snapshot of ~/.leah-state to local + B2 (whichever
		// repos are configured AND reachable). Soft-fails when restic
		// isn't installed or no repos are configured — operator runs
		// `brew install restic && scripts/restic-init.sh` to enable.
		// Per docs/research/2026-06-09-adopt-vs-build-survey.md §12
		// (restic 0.19.0, BSD-2).
		func(ctx context.Context) {
			r := &backup.Restic{}
			if !r.Available() {
				return // silent — operator hasn't installed restic yet
			}
			repos := selectBackupRepos()
			if len(repos) == 0 {
				return // silent — operator hasn't configured a repo yet
			}
			for _, repo := range repos {
				if err := r.Snapshot(ctx, repo, sd); err != nil {
					_, _ = fmt.Fprintf(out, "leah-daemon: weekly backup %s error: %v\n", repo, err)
					_ = a.Append(audit.Entry{Kind: "backup.snapshot", ArgsHash: repo, BlastRadius: 2, Outcome: "failed", Detail: err.Error()})
					continue
				}
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly backup %s: ok\n", repo)
				_ = a.Append(audit.Entry{Kind: "backup.snapshot", ArgsHash: repo, BlastRadius: 2, Outcome: "success"})
			}
		},
		// 7. Quarterly verify-drill (`restic check`): only fires on the
		// first Sunday of a calendar quarter (Jan / Apr / Jul / Oct).
		// Inside the weekly tick because the daemon has no quarterly
		// cadence primitive; the date gate makes it fire ~4×/year.
		// Catches bit-rot before the operator needs the restore.
		func(ctx context.Context) {
			if !isFirstSundayOfQuarter(time.Now()) {
				return
			}
			r := &backup.Restic{}
			if !r.Available() {
				return
			}
			for _, repo := range selectBackupRepos() {
				if err := r.Verify(ctx, repo); err != nil {
					_, _ = fmt.Fprintf(out, "leah-daemon: quarterly verify %s error: %v\n", repo, err)
					_ = a.Append(audit.Entry{Kind: "backup.verify", ArgsHash: repo, BlastRadius: 1, Outcome: "failed", Detail: err.Error()})
					if d := notify.NewDesktop(); d != nil {
						_ = d.Notify(ctx, "Leah: backup verify FAILED", fmt.Sprintf("repo %s — see audit log", repo))
					}
					continue
				}
				_, _ = fmt.Fprintf(out, "leah-daemon: quarterly verify %s: ok\n", repo)
				_ = a.Append(audit.Entry{Kind: "backup.verify", ArgsHash: repo, BlastRadius: 1, Outcome: "success"})
			}
		},
	}
}

// selectBackupRepos returns local + B2 repos that are both configured AND
// (for local) currently mounted. Skipping unmounted USB volumes prevents
// restic from writing to a phantom mount point.
func selectBackupRepos() []string {
	var out []string
	local := os.Getenv("LEAH_BACKUP_LOCAL_PATH")
	if local == "" {
		local = "/Volumes/leah-backup"
	}
	if st, err := os.Stat(local); err == nil && st.IsDir() {
		out = append(out, local)
	}
	if b2 := os.Getenv("LEAH_BACKUP_B2_REPO"); b2 != "" {
		out = append(out, b2)
	}
	return out
}

// isFirstSundayOfQuarter reports whether t falls on the first Sunday of a
// quarter month (Jan, Apr, Jul, Oct). Gates the quarterly verify drill.
func isFirstSundayOfQuarter(t time.Time) bool {
	m := t.Month()
	if m != time.January && m != time.April && m != time.July && m != time.October {
		return false
	}
	return t.Weekday() == time.Sunday && t.Day() <= 7
}

// daemonAttestationScanner adapts rules.AttestationGate.Scan to the
// selflearn.Retro.AttestationScanner shape. Field-copy adapter mirrors
// cmd/leah/retro.go::attestationScannerAdapter; the duplication is
// intentional (composition root per binary).
func daemonAttestationScanner() func(context.Context, string) ([]selflearn.AttestationViolation, error) {
	g := rules.AttestationGate{}
	return func(ctx context.Context, path string) ([]selflearn.AttestationViolation, error) {
		raw, err := g.Scan(ctx, path)
		if err != nil {
			return nil, err
		}
		out := make([]selflearn.AttestationViolation, 0, len(raw))
		for _, v := range raw {
			out = append(out, selflearn.AttestationViolation{
				Repo: v.Repo, PRNumber: v.PRNumber, URL: v.URL,
			})
		}
		return out, nil
	}
}

// buildNotifier returns the composition root for daemon notifications.
// Always includes Desktop; appends VoiceNotify when LEAH_VOICE_ENABLED=1.
// Fanout dispatches to every wrapped notifier and joins errors so a TTS
// chain failure cannot suppress the desktop banner.
func buildNotifier() daemonloop.Notifier {
	desktop := notify.NewDesktop()
	if os.Getenv("LEAH_VOICE_ENABLED") != "1" {
		return desktop
	}
	return &notify.Fanout{Notifiers: []notify.Notifier{desktop, notify.NewVoice()}}
}

func stateDir() string {
	d := os.Getenv("LEAH_STATE_DIR")
	if d == "" {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".leah-state")
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "mkdir state dir: %v\n", err)
		os.Exit(1)
	}
	return d
}
