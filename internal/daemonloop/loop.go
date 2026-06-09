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
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
)

// WeeklyTask runs on the weekly tick. Tasks execute in their own goroutine
// per tick — they MUST NOT block the per-30s poll.
type WeeklyTask func(ctx context.Context)

// weeklyInterval is the cadence for firing weekly tasks. Exposed as a
// package var for tests; defaults to 7 days.
var weeklyInterval = 7 * 24 * time.Hour

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

// Notifier sends a desktop / phone push on terminal-state transitions.
// notify.Desktop + notify.Pushover both satisfy this.
type Notifier interface {
	Notify(ctx context.Context, title, body string) error
}

// Loop polls regatta state every PollEvery and notifies on terminal-state
// transitions. Single-goroutine; in-memory diff; cold-start does not notify.
type Loop struct {
	Regatta   RegattaClient
	Heartbeat Heartbeat
	Notify    Notifier
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

	prevState map[string]string
	cold      bool
	weeklyMu  sync.Mutex
}

// New constructs a Loop with empty prevState + cold=true so the first tick
// seeds state without emitting transitions. WeeklyTracker / WeeklyHour /
// Weekly are set by the caller before Run.
func New(rc RegattaClient, hb Heartbeat, nf Notifier, a *audit.Logger, out io.Writer, pollEvery time.Duration) *Loop {
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
	if l.Heartbeat != nil {
		_ = l.Heartbeat.Ping(ctx)
	}
	agents, err := l.Regatta.List(ctx)
	if err != nil {
		lg.Error("daemon.regatta.list_error", "err", err.Error())
		_, _ = fmt.Fprintf(l.Out, "leah-daemon: regatta list error: %v\n", err)
		// Fall through to weekly tick — regatta unreachability MUST NOT
		// gate weekly self-learn/patterns/retro work.
	} else {
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
	last, ok := readWeeklyTracker(l.WeeklyTracker)
	if ok && now.Sub(last) < weeklyInterval {
		l.weeklyMu.Unlock()
		_, _ = fmt.Fprintf(l.Out, "leah-daemon: weekly skipped (last ran %s, <%v ago)\n",
			last.Format(time.RFC3339), weeklyInterval)
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
