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
	commsout "github.com/trilam/leah/internal/comms/out"
	"github.com/trilam/leah/internal/ctxmgr"
	"github.com/trilam/leah/internal/daemonloop"
	"github.com/trilam/leah/internal/learn"
	"github.com/trilam/leah/internal/learn/rules"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/operatormodel"
	"github.com/trilam/leah/internal/patterns"
)

// buildWeeklyTasks returns the per-week tasks fired by daemonloop on the
// weekly tick. Ordering: resolver back-fill, pattern detection →
// skill-candidates.md, weekly retro report, operator-behavior profile
// rebuild, panic-rate → bug-fix-candidates.md + operator push, restic
// snapshot, quarterly verify-drill.
//
// Each task is built by a per-subsystem helper so future parallel agents
// touch their own file.
func buildWeeklyTasks(sd, auditPath string, a *audit.Logger, out *os.File) []daemonloop.WeeklyTask {
	// One-way feedback edge: resolver outcomes feed the operator model so
	// recommendations reflect recent ship results. The resolver task runs
	// before the operatormodel task (ordering below), draining into this
	// shared observer in between.
	feedback := &operatormodel.FeedbackObserver{}
	return []daemonloop.WeeklyTask{
		buildResolverTask(auditPath, a, out, feedback),
		buildPatternsTask(sd, auditPath, out),
		buildRetroTask(sd, auditPath, out),
		buildOperatorModelTask(sd, auditPath, out, feedback),
		buildPanicDetectTask(sd, out),
		buildBackupSnapshotTask(sd, a, out),
		buildBackupVerifyTask(a, out),
	}
}

// buildResolverTask back-fills outcomes on pending audit rows via the
// regatta.ship rule. Panic-rate detection runs in buildPanicDetectTask —
// keep concerns separate (resolver = audit-row verdicts; panic-detect =
// metric-snapshot deltas). Errors log + skip — the next weekly tick retries.
func buildResolverTask(auditPath string, a *audit.Logger, out *os.File, feedback *operatormodel.FeedbackObserver) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		sink := learn.NewRetroOutcomeSink(64, func(o learn.RetroOutcome) {
			feedback.Observe(operatormodel.ShipOutcome{Verdict: shipVerdict(o), At: time.Now().UTC()})
		})
		defer sink.Close() // flush in-flight before the operatormodel task drains
		r := &learn.Resolver{
			AuditPath: auditPath,
			Logger:    a,
			Rules: map[string]learn.Rule{
				"regatta.ship": rules.RegattaPR{},
			},
			Out:       out,
			OnOutcome: sink.Emit,
		}
		if err := r.Run(ctx); err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: weekly resolver error: %v\n", err)
		}
	}
}

// shipVerdict maps a selflearn resolver verdict to the operator-model
// ship_outcome slot, keeping the import arrow one-way (the translation
// lives at the daemon boundary, not inside operatormodel).
func shipVerdict(o learn.RetroOutcome) string {
	switch o {
	case learn.RetroSuccess:
		return operatormodel.VerdictOK
	case learn.RetroFailed:
		return operatormodel.VerdictFailed
	default:
		return operatormodel.VerdictDangling
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
// learn.Retro for soft-fail attestation reporting.
func buildRetroTask(sd, auditPath string, out *os.File) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		dbPath := filepath.Join(sd, "memory.db")
		store, err := learn.OpenMistakeStore(dbPath)
		if err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: weekly retro store error: %v\n", err)
			return
		}
		defer func() { _ = store.Close() }()
		retro := &learn.Retro{
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
// window. The ctxmgr handle wires context_transition observations
// — without it the class stays silently zero (#10). Cold-start gate inside
// Update() ensures Ready stays false until 50 rows + 7d.
func buildOperatorModelTask(sd, auditPath string, out *os.File, feedback *operatormodel.FeedbackObserver) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		dbPath := filepath.Join(sd, "memory.db")
		store, err := memory.NewStore(dbPath)
		if err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: weekly operatormodel store error: %v\n", err)
			return
		}
		defer func() { _ = store.Close() }()
		// ctxmgr shares memory.db (WAL + busy_timeout co-existence per spec §5.2).
		mgr, err := activectx.Open(dbPath)
		if err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: weekly operatormodel ctxmgr error: %v\n", err)
			return
		}
		defer func() { _ = mgr.Close() }()
		if err := operatormodel.UpdateProfile(ctx, store, auditPath, mgr, feedback); err != nil {
			_, _ = fmt.Fprintf(out, "leah-daemon: weekly operatormodel error: %v\n", err)
		}
	}
}

// buildPanicDetectTask runs panic-rate detection, appends Candidates to
// ~/.leah-state/bug-fix-candidates.md (one BuildIssueBody section per
// candidate w/ UTC timestamp), counts new vs sentinel, and notifies the
// operator when new > 0. NEVER auto-dispatches to regatta — operator
// reads the file + manually runs `leah self-build` (the only structural
// defense against self-modification drift).
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
		if d := commsout.NewDesktop(); d != nil {
			if err := d.Notify(ctx, title, body); err != nil {
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly bug-fix desktop notify error: %v\n", err)
			}
		}
		if p := commsout.NewPushover(); p != nil {
			if err := p.Notify(ctx, title, body); err != nil {
				// Missing credentials degrade silently — see Pushover.Notify.
				_, _ = fmt.Fprintf(out, "leah-daemon: weekly bug-fix pushover skip: %v\n", err)
			}
		}
	}
}
