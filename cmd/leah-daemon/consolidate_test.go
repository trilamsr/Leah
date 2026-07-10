package main

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/daemonloop"
	"github.com/trilam/leah/internal/memory/store"
)

// TestConsolidation_BriefCoexistence_3am_vs_8am asserts wireConsolidation
// and wireBriefSchedule with LEAH_BRIEF_DAILY=1 coexist on loop.Daily —
// each task fires at its own hour via the hourGate wrapper, and the lower
// global DailyHour + 1h DailyInterval lets the loop re-check the daily gate
// after the 3am fire so the 8am brief is not locked out for 24h.
func TestConsolidation_BriefCoexistence_3am_vs_8am(t *testing.T) {
	t.Setenv("LEAH_BRIEF_DAILY", "1")
	sd := t.TempDir()
	auditPath := filepath.Join(sd, "audit.jsonl")
	store, err := memory.NewStore(filepath.Join(sd, "memory.db"))
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	defer func() { _ = store.Close() }()
	a := &audit.Logger{Path: auditPath}

	loop := &daemonloop.Loop{}
	var briefFired, consFired atomic.Int32
	briefTask := func(_ context.Context) { briefFired.Add(1) }
	wireBriefSchedule(loop, sd, briefTask, os.Stdout)
	// Replace the brief task with our counter — wireBriefSchedule wraps the
	// raw task; we want to assert the WRAP (post-wireConsolidation hourGate)
	// only fires at hour 8.
	loop.Daily = []daemonloop.WeeklyTask{briefTask}

	wireConsolidation(loop, store, a, auditPath, sd)
	// Replace consolidation task with counter — same reason.
	consTask := func(_ context.Context) { consFired.Add(1) }
	// wireConsolidation appended its task LAST; swap it.
	loop.Daily[len(loop.Daily)-1] = hourGate(consTask, 3, filepath.Join(sd, "last-consolidation.txt"))
	loop.Daily[0] = hourGate(briefTask, 8, filepath.Join(sd, "last-brief.txt"))

	// DailyHour must be the EARLIEST hour of the two (3) so the loop doesn't
	// defer the consolidation fire; DailyInterval must be sub-day (1h) so
	// the loop re-checks the gate after consolidation fires at 3am and the
	// brief still has a chance at 8am.
	if loop.DailyHour != 3 {
		t.Errorf("DailyHour = %d, want 3 (earliest of 3am consolidation, 8am brief)", loop.DailyHour)
	}
	if loop.DailyInterval >= 24*time.Hour {
		t.Errorf("DailyInterval = %v, want <24h (hourly re-check)", loop.DailyInterval)
	}
	if len(loop.Daily) != 2 {
		t.Fatalf("Daily task count = %d, want 2 (brief + consolidation)", len(loop.Daily))
	}

	// Drive the wrapped tasks at synthetic clocks. The hourGate uses
	// time.Now() directly so we can't dependency-inject — instead we assert
	// the hour-gate behavior at the CURRENT hour, then validate the
	// per-task tracker prevents double-fire on the same day.
	now := time.Now()
	switch now.Hour() {
	case 3:
		loop.Daily[1](context.Background())
		if consFired.Load() != 1 {
			t.Errorf("consolidation: fired=%d at hour 3, want 1", consFired.Load())
		}
		loop.Daily[0](context.Background())
		if briefFired.Load() != 0 {
			t.Errorf("brief: fired=%d at hour 3, want 0 (gate skips non-8 hour)", briefFired.Load())
		}
	case 8:
		loop.Daily[0](context.Background())
		if briefFired.Load() != 1 {
			t.Errorf("brief: fired=%d at hour 8, want 1", briefFired.Load())
		}
		loop.Daily[1](context.Background())
		if consFired.Load() != 0 {
			t.Errorf("consolidation: fired=%d at hour 8, want 0 (gate skips non-3 hour)", consFired.Load())
		}
	default:
		// At any other hour, both gates skip — assert neither fires.
		loop.Daily[0](context.Background())
		loop.Daily[1](context.Background())
		if briefFired.Load() != 0 || consFired.Load() != 0 {
			t.Errorf("off-hour fires: brief=%d cons=%d, want 0,0 at hour=%d",
				briefFired.Load(), consFired.Load(), now.Hour())
		}
	}
}

// TestHourGate_OncePerDay asserts the per-task tracker prevents a second
// fire on the same calendar day even when the hour still matches (e.g. a
// daemon restart at 03:30 after a 03:00 fire).
func TestHourGate_OncePerDay(t *testing.T) {
	sd := t.TempDir()
	tracker := filepath.Join(sd, "last.txt")
	now := time.Now()
	hour := now.Hour()

	var fired atomic.Int32
	task := hourGate(func(_ context.Context) { fired.Add(1) }, hour, tracker)
	task(context.Background())
	task(context.Background())
	if fired.Load() != 1 {
		t.Fatalf("fired = %d, want 1 (tracker must dedupe same-day)", fired.Load())
	}
}
