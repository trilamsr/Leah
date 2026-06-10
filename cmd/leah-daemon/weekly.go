package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/ctxmgr"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/notify"
	"github.com/trilam/leah/internal/operatormodel"
	"github.com/trilam/leah/internal/patterns"
	"github.com/trilam/leah/internal/selflearn"
	"github.com/trilam/leah/internal/selflearn/rules"
)

// buildWeeklyTasks returns the per-week tasks fired by daemonloop on the
// weekly tick. Ordering: resolver back-fill, pattern detection →
// skill-candidates.md, weekly retro report, operator-behavior profile
// rebuild, panic-rate → bug-fix-candidates.md + operator push, restic
// snapshot, quarterly verify-drill.
//
// Each task is built by a per-subsystem helper so future parallel agents
// touch their own file (split closes Wave 7 god-file retros KK / JJ / MM).
func buildWeeklyTasks(sd, auditPath string, a *audit.Logger, out *os.File) []daemonloop.WeeklyTask {
	return []daemonloop.WeeklyTask{
		buildResolverTask(auditPath, a, out),
		buildPatternsTask(sd, auditPath, out),
		buildRetroTask(sd, auditPath, out),
		buildOperatorModelTask(sd, auditPath, out),
		buildPanicDetectTask(sd, out),
		buildBackupSnapshotTask(sd, a, out),
		buildBackupVerifyTask(a, out),
	}
}

// buildResolverTask back-fills outcomes on pending audit rows via the
// regatta.ship rule. Panic-rate detection runs in buildPanicDetectTask —
// keep concerns separate (resolver = audit-row verdicts; panic-detect =
// metric-snapshot deltas). Errors log + skip — the next weekly tick retries.
func buildResolverTask(auditPath string, a *audit.Logger, out *os.File) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		r := &selflearn.Resolver{
			AuditPath: auditPath,
			Logger:    a,
			Rules: map[string]selflearn.Rule{
				"regatta.ship": rules.RegattaPR{},
			},
			Out: out,
		}
		if err := r.Run(ctx); err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: weekly resolver error: %v\n", err)
		}
	}
}

// buildPatternsTask scans the last 7d of audit entries, clusters repeat
// errors, and writes proposals to ~/.leah-state/skill-candidates.md.
func buildPatternsTask(sd, auditPath string, out *os.File) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
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
	}
}

// buildRetroTask writes the weekly self-retro to ~/.leah-state/
// retro-YYYY-WW.md, threading the attestation-scanner adapter into
// selflearn.Retro for soft-fail attestation reporting.
func buildRetroTask(sd, auditPath string, out *os.File) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
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
	}
}

// buildOperatorModelTask rebuilds operator_profile rows from the audit
// window (Wave 2-J). The ctxmgr handle wires context_transition observations
// — without it the class stays silently zero (#10). Cold-start gate inside
// Update() ensures Ready stays false until 50 rows + 7d.
func buildOperatorModelTask(sd, auditPath string, out *os.File) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		dbPath := filepath.Join(sd, "memory.db")
		store, err := memory.NewStore(dbPath)
		if err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: weekly operatormodel store error: %v\n", err)
			return
		}
		defer func() { _ = store.Close() }()
		// ctxmgr shares memory.db (WAL + busy_timeout co-existence per spec §5.2).
		mgr, err := ctxmgr.Open(dbPath)
		if err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: weekly operatormodel ctxmgr error: %v\n", err)
			return
		}
		defer func() { _ = mgr.Close() }()
		if err := operatormodel.UpdateProfile(ctx, store, auditPath, mgr); err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: weekly operatormodel error: %v\n", err)
		}
	}
}

// buildPanicDetectTask runs panic-rate detection, appends Candidates to
// ~/.leah-state/bug-fix-candidates.md (one BuildIssueBody section per
// candidate w/ UTC timestamp), counts new vs sentinel, and notifies the
// operator when new > 0. NEVER auto-dispatches to regatta — operator
// reads the file + manually runs `leah self-build` (the only structural
// defense against self-modification drift; see
// docs/specs/2026-06-09-bug-fix-self-build-hook.md).
func buildPanicDetectTask(sd string, out *os.File) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		detectors := []rules.PanicRateRule{{}}
		var found []rules.Candidate
		for _, d := range detectors {
			cands, err := d.Detect(ctx, sd)
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
		if newCount == 0 {
			return
		}
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
}
