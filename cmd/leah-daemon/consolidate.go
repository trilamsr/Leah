package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/daemonloop"
	"github.com/trilam/leah/internal/memory/store"
	"github.com/trilam/leah/internal/thinking/operatormodel"
)

// wireConsolidation hangs the nightly memory consolidation pass off the
// daily tick. To coexist with the morning-brief daily task (LEAH_BRIEF_DAILY=1,
// hour=8 by default) on the single shared `loop.Daily` slot, this wiring:
//
//  1. wraps every pre-existing Daily task (e.g. brief) in an hour+per-task-
//     tracker gate that fires once per day at the task's expected hour —
//     LEAH_BRIEF_HOUR for brief — so the global DailyHour can be lowered.
//  2. adds the consolidation task gated on LEAH_CONSOLIDATION_HOUR (default
//  3. with its own tracker.
//  3. drops the daemonloop-level DailyHour to the earliest hour (3) and
//     shortens DailyInterval to 1h so the loop checks the gate hourly
//     instead of locking out the second-fire-of-the-day on the first.
//
// Kill-switch via LEAH_CONSOLIDATION=0 is enforced inside ConsolidatePass.Run
// so the daemon never has to special-case it here.
func wireConsolidation(loop *daemonloop.Loop, store *memory.Store, a *audit.Logger, auditPath, sd string) {
	if loop.DailyTracker == "" {
		loop.DailyTracker = filepath.Join(sd, "last-daily.txt")
	}
	// Wrap any pre-existing Daily tasks (brief) with their own hour gate
	// keyed off LEAH_BRIEF_HOUR / loop.DailyHour. The wrapping captures the
	// hour BEFORE we overwrite it below so brief still fires at 8.
	briefHour := loop.DailyHour
	if v := os.Getenv("LEAH_BRIEF_HOUR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
			briefHour = n
		}
	}
	if briefHour > 0 {
		wrapped := make([]daemonloop.WeeklyTask, len(loop.Daily))
		for i, t := range loop.Daily {
			wrapped[i] = hourGate(t, briefHour, filepath.Join(sd, "last-brief.txt"))
		}
		loop.Daily = wrapped
	}

	consHour := 3
	if v := os.Getenv("LEAH_CONSOLIDATION_HOUR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 23 {
			consHour = n
		}
	}
	archivePath := filepath.Join(sd, "consolidated.jsonl")
	task := func(ctx context.Context) {
		cp := &operatormodel.ConsolidatePass{
			Store:       store,
			AuditPath:   auditPath,
			ArchivePath: archivePath,
			Audit:       a,
		}
		if err := cp.Run(ctx); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: consolidation: %v\n", err)
		}
	}
	loop.Daily = append(loop.Daily, hourGate(task, consHour, filepath.Join(sd, "last-consolidation.txt")))

	// Lowest hour wins for the global gate; per-task wrappers above enforce
	// the actual per-task hours. Hourly cadence lets the loop re-check the
	// gate after the first fire so a later task on the list isn't locked
	// out for 24h by an earlier one.
	loop.DailyHour = consHour
	if briefHour > 0 && briefHour < loop.DailyHour {
		loop.DailyHour = briefHour
	}
	loop.DailyInterval = time.Hour
}

// hourGate wraps task in a closure that runs at most once per local
// calendar day, only when time.Now().Hour() == hour. Per-task tracker keeps
// the gate independent of the loop-level DailyTracker so two tasks sharing
// loop.Daily can fire on the same day at different hours without one
// locking the other out.
func hourGate(task daemonloop.WeeklyTask, hour int, tracker string) daemonloop.WeeklyTask {
	return func(ctx context.Context) {
		now := time.Now()
		if now.Hour() != hour {
			return
		}
		if ranToday(tracker, now) {
			return
		}
		task(ctx)
		_ = writeTracker(tracker, now)
	}
}

func ranToday(path string, now time.Time) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	last, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return last.Local().Format("2006-01-02") == now.Local().Format("2006-01-02")
}

func writeTracker(path string, now time.Time) error {
	return os.WriteFile(path, []byte(now.Format(time.RFC3339)), 0o600)
}
