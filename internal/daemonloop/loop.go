// Package daemonloop is the always-on leah-daemon poll engine. Per-tick it
// pings the healthcheck URL, diffs regatta agent state for terminal
// transitions, and fires WeeklyTask funcs at a 7d cadence. Cold-start
// suppresses notifications so a daemon restart never floods.
package daemonloop

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
)

// WeeklyTask runs on the weekly tick. Tasks execute in their own goroutine
// per tick — they MUST NOT block the per-30s poll.
type WeeklyTask func(ctx context.Context)

// defaultWeeklyInterval is the cadence for firing weekly tasks when a
// Loop does not specify its own. Each Loop carries its own WeeklyInterval
// field — there is no package-level mutable state to avoid the race the
// 2026-06-09 audit H6 flagged (parallel tests mutating a shared var
// while concurrent tick goroutines read it).
const defaultWeeklyInterval = 7 * 24 * time.Hour

// defaultDailyInterval is the cadence for firing daily tasks (morning
// brief) when a Loop does not specify its own. Same per-Loop-field
// pattern as defaultWeeklyInterval — no package-level mutable state.
const defaultDailyInterval = 24 * time.Hour

// RegattaClient is the subset of regattaclient.Client the loop needs;
// abstracted so tests can stub state diffs without spawning the regatta binary.
type RegattaClient interface {
	List(ctx context.Context) ([]regattaclient.Agent, error)
}

// Heartbeat is the per-tick keep-alive surface (typically watchdog.Heartbeat
// posting to healthchecks.io). Failures are non-fatal.
type Heartbeat interface {
	Ping(ctx context.Context) error
}

// Loop polls regatta state every PollEvery and notifies on terminal-state
// transitions. Single-goroutine; in-memory diff; cold-start does not notify.
type Loop struct {
	Regatta   RegattaClient
	Heartbeat Heartbeat
	Notify    contracts.Notifier
	Audit     *audit.Logger
	Out       io.Writer
	PollEvery time.Duration

	// Weekly fires once per weeklyInterval. Each task runs in its own
	// goroutine — slow tasks DO NOT block the per-30s tick. Concurrent
	// invocations are gated by weeklyMu (TryLock — skip if in flight).
	Weekly []WeeklyTask
	// WeeklyTracker is the file storing the last-fired RFC3339 timestamp.
	// Missing or unparseable file = fire on next tick.
	WeeklyTracker string
	// WeeklyHour gates the weekly tick to fire only at-or-after this
	// hour-of-day (local time). Zero (or unset) = no hour gate.
	WeeklyHour int
	// WeeklyInterval overrides the 7-day cadence per Loop instance. Zero
	// = defaultWeeklyInterval. Per-Loop field (not a package var) so test
	// instances cannot race the production tick goroutine.
	WeeklyInterval time.Duration

	// Daily fires once per dailyInterval — used for the morning-brief task.
	// Same per-task-goroutine + per-task-recover + dailyMu TryLock pattern
	// as Weekly. Independent tracker so weekly + daily never race.
	Daily []WeeklyTask
	// DailyTracker is the file storing the last daily-fire RFC3339 timestamp.
	DailyTracker string
	// DailyHour gates the daily tick to fire only at-or-after this hour-of-day
	// (local time). Same semantics as WeeklyHour.
	DailyHour int
	// DailyInterval overrides the 24h cadence per Loop. Zero = defaultDailyInterval.
	DailyInterval time.Duration

	prevState map[string]string
	cold      bool
	weeklyMu  sync.Mutex
	dailyMu   sync.Mutex
	// regattaLastErr is the last Regatta.List error string; an identical
	// error on subsequent ticks suppresses the ERROR log to silence the F3
	// 30s spam when the regatta binary is absent. Empty = last tick succeeded
	// (or no tick yet); flipping from non-empty → empty fires a one-shot
	// "regatta available" recovery INFO log.
	regattaLastErr string
	// lastTick records the wall-clock instant of the most recent tick so
	// the dashboard's Heartbeat surface reflects real loop liveness instead
	// of the request time (BB-RETRO L3 #18). Zero pointer = no tick yet.
	lastTick atomic.Pointer[time.Time]
	// OnTick fires after lastTick is stamped each tick; wired by main.go to
	// feed leah_daemonloop_tick_total + last_tick gauge. nil-safe.
	OnTick func(time.Time)
}

// LastTick returns the wall-clock instant of the most recent tick, or the
// zero time before the first tick has completed. Thread-safe for arbitrary
// concurrent readers vs the single tick writer.
func (l *Loop) LastTick() time.Time {
	if p := l.lastTick.Load(); p != nil {
		return *p
	}
	return time.Time{}
}

// New constructs a Loop with empty prevState + cold=true so the first tick
// seeds state without emitting transitions. WeeklyTracker / WeeklyHour /
// Weekly are set by the caller before Run.
func New(rc RegattaClient, hb Heartbeat, nf contracts.Notifier, a *audit.Logger, out io.Writer, pollEvery time.Duration) *Loop {
	return &Loop{
		Regatta:   rc,
		Heartbeat: hb,
		Notify:    nf,
		Audit:     a,
		Out:       out,
		PollEvery: pollEvery,
		prevState: map[string]string{},
		cold:      true,
	}
}

// Run blocks until ctx is canceled.
func (l *Loop) Run(ctx context.Context) error {
	_, _ = fmt.Fprintln(l.Out, "leah-daemon: starting; poll interval:", l.PollEvery)
	for {
		l.tick(ctx)
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintln(l.Out, "leah-daemon: shutdown")
			return nil
		case <-time.After(l.PollEvery):
		}
	}
}

func (l *Loop) tick(ctx context.Context) {
	lg := obs.LoggerFromCtx(ctx).With("package", "daemonloop")
	lg.Debug("daemon.tick")
	// Record at entry — guarantees liveness signal even if Heartbeat.Ping
	// or Regatta.List stalls on a slow network.
	now := time.Now()
	l.lastTick.Store(&now)
	if l.OnTick != nil {
		l.OnTick(now)
	}
	if l.Heartbeat != nil {
		_ = l.Heartbeat.Ping(ctx)
	}
	agents, err := l.Regatta.List(ctx)
	if err != nil {
		// F3: log first-of-streak only; identical repeats are silent
		// (silenced 30s ERROR spam when regatta binary missing). A new
		// error message resets the streak so a different failure is
		// still surfaced.
		if msg := err.Error(); msg != l.regattaLastErr {
			lg.Error("daemon.regatta.list_error", "err", msg)
			_, _ = fmt.Fprintf(l.Out, "leah-daemon: regatta list error: %v\n", err)
			l.regattaLastErr = msg
		}
		// Fall through to weekly tick — regatta unreachability MUST NOT
		// gate weekly self-learn/patterns/retro work.
	} else {
		if l.regattaLastErr != "" {
			lg.Info("daemon.regatta.available")
			_, _ = fmt.Fprintln(l.Out, "leah-daemon: regatta available")
			l.regattaLastErr = ""
		}
		current := map[string]string{}
		for _, a := range agents {
			current[a.ID] = a.State
		}
		if !l.cold {
			for id, newState := range current {
				oldState, existed := l.prevState[id]
				if existed && oldState != newState && isTerminal(newState) {
					l.notifyTransition(ctx, id, oldState, newState, agents)
				}
			}
		}
		l.prevState = current
		l.cold = false
	}

	l.maybeFireWeekly(ctx)
	l.maybeFireDaily(ctx)
}

// maybeFireWeekly checks the WeeklyTracker file and, if >= weeklyInterval has
// elapsed since the last fire (or the tracker is missing/unparseable), runs
// all WeeklyTask funcs in a background goroutine and rewrites the tracker.
// Concurrent invocations short-circuit on weeklyMu TryLock.
func (l *Loop) maybeFireWeekly(ctx context.Context) {
	if len(l.Weekly) == 0 || l.WeeklyTracker == "" {
		return
	}
	if !l.weeklyMu.TryLock() {
		return // weekly already in flight
	}

	now := time.Now().UTC()
	interval := l.WeeklyInterval
	if interval <= 0 {
		interval = defaultWeeklyInterval
	}
	last, ok := readWeeklyTracker(l.WeeklyTracker)
	if ok && now.Sub(last) < interval {
		l.weeklyMu.Unlock()
		_, _ = fmt.Fprintf(l.Out, "leah-daemon: weekly skipped (last ran %s, <%v ago)\n",
			last.Format(time.RFC3339), interval)
		return
	}
	if l.WeeklyHour > 0 && time.Now().Hour() < l.WeeklyHour {
		l.weeklyMu.Unlock()
		_, _ = fmt.Fprintf(l.Out, "leah-daemon: weekly deferred (hour %d < WeeklyHour %d)\n",
			time.Now().Hour(), l.WeeklyHour)
		return
	}

	if err := writeWeeklyTracker(l.WeeklyTracker, now); err != nil {
		_, _ = fmt.Fprintf(l.Out, "leah-daemon: weekly tracker write error: %v\n", err)
		l.weeklyMu.Unlock()
		return
	}
	tasks := l.Weekly
	_, _ = fmt.Fprintf(l.Out, "leah-daemon: weekly: running %d task(s)\n", len(tasks))
	go func() {
		defer l.weeklyMu.Unlock()
		for _, t := range tasks {
			func() {
				defer func() {
					if r := recover(); r != nil {
						_, _ = fmt.Fprintf(l.Out, "leah-daemon: weekly task panic: %v\n", r)
					}
				}()
				t(ctx)
			}()
		}
	}()
}

// maybeFireDaily mirrors maybeFireWeekly with the 24h cadence + a separate
// tracker + mutex. Independent state prevents interleaving with weekly fires
// (the morning brief MUST run regardless of weekly-retro cadence and vice-versa).
func (l *Loop) maybeFireDaily(ctx context.Context) {
	if len(l.Daily) == 0 || l.DailyTracker == "" {
		return
	}
	if !l.dailyMu.TryLock() {
		return // daily already in flight
	}

	now := time.Now().UTC()
	interval := l.DailyInterval
	if interval <= 0 {
		interval = defaultDailyInterval
	}
	last, ok := readWeeklyTracker(l.DailyTracker)
	if ok && now.Sub(last) < interval {
		l.dailyMu.Unlock()
		_, _ = fmt.Fprintf(l.Out, "leah-daemon: daily skipped (last ran %s, <%v ago)\n",
			last.Format(time.RFC3339), interval)
		return
	}
	if l.DailyHour > 0 && time.Now().Hour() < l.DailyHour {
		l.dailyMu.Unlock()
		_, _ = fmt.Fprintf(l.Out, "leah-daemon: daily deferred (hour %d < DailyHour %d)\n",
			time.Now().Hour(), l.DailyHour)
		return
	}

	if err := writeWeeklyTracker(l.DailyTracker, now); err != nil {
		_, _ = fmt.Fprintf(l.Out, "leah-daemon: daily tracker write error: %v\n", err)
		l.dailyMu.Unlock()
		return
	}
	tasks := l.Daily
	_, _ = fmt.Fprintf(l.Out, "leah-daemon: daily: running %d task(s)\n", len(tasks))
	go func() {
		defer l.dailyMu.Unlock()
		for _, t := range tasks {
			func() {
				defer func() {
					if r := recover(); r != nil {
						_, _ = fmt.Fprintf(l.Out, "leah-daemon: daily task panic: %v\n", r)
					}
				}()
				t(ctx)
			}()
		}
	}()
}

func readWeeklyTracker(path string) (time.Time, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func writeWeeklyTracker(path string, now time.Time) error {
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o700)
	}
	return os.WriteFile(path, []byte(now.Format(time.RFC3339)), 0o600)
}

func isTerminal(state string) bool {
	switch state {
	case "merged", "escalated", "failed", "stuck", "killed":
		return true
	}
	return false
}

func (l *Loop) notifyTransition(ctx context.Context, id, from, to string, agents []regattaclient.Agent) {
	var pr int
	for _, a := range agents {
		if a.ID == id {
			pr = a.PR
			break
		}
	}
	title := "Leah"
	body := fmt.Sprintf("agent %s: %s → %s (PR #%d)", id, from, to, pr)
	obs.LoggerFromCtx(ctx).Info("daemon.transition",
		"package", "daemonloop",
		"agent_id", id,
		"from", from,
		"to", to,
		"pr", pr,
	)
	_ = l.Notify.Notify(ctx, title, body)
	_ = l.Audit.Append(audit.Entry{
		Kind:        "daemon.transition",
		BlastRadius: 0,
		Outcome:     "observed",
		Detail:      body,
	})
	_, _ = fmt.Fprintln(l.Out, body)
}
