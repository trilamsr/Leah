package daemonloop

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
)

type fakeRegatta struct {
	resps [][]regattaclient.Agent
	call  int
}

func (f *fakeRegatta) List(ctx context.Context) ([]regattaclient.Agent, error) {
	if f.call >= len(f.resps) {
		return f.resps[len(f.resps)-1], nil
	}
	r := f.resps[f.call]
	f.call++
	return r, nil
}

type fakeHb struct{ pings int }

func (f *fakeHb) Ping(ctx context.Context) error { f.pings++; return nil }

type fakeNf struct{ calls []string }

func (f *fakeNf) Notify(ctx context.Context, title, body string) error {
	f.calls = append(f.calls, title+":"+body)
	return nil
}

// TestTickEmitsObsLogOnTransition asserts the daemonloop tick emits the
// daemon.transition INFO event with agent_id / from / to / pr attrs
// (Wave2-K obs instrumentation contract).
func TestTickEmitsObsLogOnTransition(t *testing.T) {
	var buf bytes.Buffer
	lg := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := obs.WithLogger(context.Background(), lg)

	rc := &fakeRegatta{resps: [][]regattaclient.Agent{
		{{ID: "a1", State: "running", PR: 0}},
		{{ID: "a1", State: "merged", PR: 42}},
	}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	l := New(rc, &fakeHb{}, &fakeNf{}, a, &bytes.Buffer{}, time.Millisecond)
	l.tick(ctx)
	l.tick(ctx)

	out := buf.String()
	if !strings.Contains(out, `"msg":"daemon.tick"`) {
		t.Errorf("missing daemon.tick: %s", out)
	}
	if !strings.Contains(out, `"msg":"daemon.transition"`) {
		t.Errorf("missing daemon.transition: %s", out)
	}
	if !strings.Contains(out, `"to":"merged"`) {
		t.Errorf("missing to=merged attr: %s", out)
	}
}

func TestLoopColdStartDoesNotNotify(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{
		{{ID: "a1", State: "merged", PR: 1}},
	}}
	hb := &fakeHb{}
	nf := &fakeNf{}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}

	l := New(rc, hb, nf, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.tick(context.Background())
	if len(nf.calls) != 0 {
		t.Errorf("cold start should not notify; got %v", nf.calls)
	}
	if hb.pings != 1 {
		t.Errorf("heartbeat should fire on cold tick; got %d", hb.pings)
	}
}

func TestLoopNotifiesOnTerminalTransition(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{
		{{ID: "a1", State: "running", PR: 0}},
		{{ID: "a1", State: "merged", PR: 1234}},
	}}
	dir := t.TempDir()
	a := &audit.Logger{Path: dir + "/audit.jsonl"}
	nf := &fakeNf{}

	l := New(rc, &fakeHb{}, nf, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.tick(context.Background())
	l.tick(context.Background())

	if len(nf.calls) != 1 {
		t.Fatalf("want 1 notify call, got %d: %v", len(nf.calls), nf.calls)
	}
	if !strings.Contains(nf.calls[0], "merged") || !strings.Contains(nf.calls[0], "PR #1234") {
		t.Errorf("notify body: %v", nf.calls[0])
	}

	data, _ := os.ReadFile(dir + "/audit.jsonl")
	if !strings.Contains(string(data), `"kind":"daemon.transition"`) {
		t.Errorf("audit missing daemon.transition: %s", data)
	}
}

func TestLoopIgnoresNonTerminalTransition(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{
		{{ID: "a1", State: "running", PR: 0}},
		{{ID: "a1", State: "spawning", PR: 0}},
	}}
	nf := &fakeNf{}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}

	l := New(rc, &fakeHb{}, nf, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.tick(context.Background())
	l.tick(context.Background())

	if len(nf.calls) != 0 {
		t.Errorf("non-terminal change should not notify; got %v", nf.calls)
	}
}

func TestWeeklyTaskFiresAfter7Days(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-weekly.txt"

	// Seed tracker to 8d ago so weekly should fire.
	old := time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if err := os.WriteFile(tracker, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	var fired int
	var mu sync.Mutex
	done := make(chan struct{}, 1)
	weekly := func(ctx context.Context) {
		mu.Lock()
		fired++
		mu.Unlock()
		select {
		case done <- struct{}{}:
		default:
		}
	}

	l := New(rc, &fakeHb{}, &fakeNf{}, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.WeeklyTracker = tracker
	l.Weekly = []WeeklyTask{weekly}
	l.tick(context.Background())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("weekly task did not fire after 7d boundary")
	}
	mu.Lock()
	if fired != 1 {
		t.Errorf("want 1 weekly fire, got %d", fired)
	}
	mu.Unlock()

	// Tracker file should be updated to ~now.
	data, err := os.ReadFile(tracker)
	if err != nil {
		t.Fatalf("read tracker: %v", err)
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse tracker: %v", err)
	}
	if time.Since(ts) > time.Minute {
		t.Errorf("tracker stale: %v", ts)
	}
}

// TestWeeklyIntervalOverridePerLoop asserts each Loop carries its own
// WeeklyInterval — no shared package-level mutable state. Catches the
// race the audit's H6 flagged: parallel tests mutating a package var
// while concurrent goroutines read it. Setting WeeklyInterval to a
// short window on one Loop instance must not bleed into others.
func TestWeeklyIntervalOverridePerLoop(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-weekly.txt"

	// Seed tracker to 1h ago so the default-7d Loop would skip, but a
	// 30-minute-interval Loop would fire.
	old := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	if err := os.WriteFile(tracker, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	var fired atomic.Int32
	done := make(chan struct{}, 1)
	weekly := func(ctx context.Context) {
		fired.Add(1)
		select {
		case done <- struct{}{}:
		default:
		}
	}

	short := New(rc, &fakeHb{}, &fakeNf{}, a, &bytes.Buffer{}, time.Millisecond)
	short.WeeklyTracker = tracker
	short.Weekly = []WeeklyTask{weekly}
	short.WeeklyInterval = 30 * time.Minute
	short.tick(context.Background())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("short-interval weekly did not fire")
	}
	if got := fired.Load(); got != 1 {
		t.Errorf("fired = %d, want 1", got)
	}

	// Build a second Loop with the default interval against the same
	// tracker (which just got rewritten to "now") — it must NOT fire and
	// must NOT see any state leaked from `short`.
	def := New(rc, &fakeHb{}, &fakeNf{}, a, &bytes.Buffer{}, time.Millisecond)
	def.WeeklyTracker = tracker
	def.Weekly = []WeeklyTask{weekly}
	def.tick(context.Background())
	time.Sleep(50 * time.Millisecond)
	if got := fired.Load(); got != 1 {
		t.Errorf("default-interval Loop fired (leak): fired=%d, want 1", got)
	}
}

func TestWeeklyTaskDoesNotFireWithin7Days(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-weekly.txt"

	// Seed tracker to 1d ago — weekly should NOT fire.
	recent := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	if err := os.WriteFile(tracker, []byte(recent), 0o600); err != nil {
		t.Fatal(err)
	}

	var fired int
	var mu sync.Mutex
	weekly := func(ctx context.Context) {
		mu.Lock()
		fired++
		mu.Unlock()
	}

	l := New(rc, &fakeHb{}, &fakeNf{}, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.WeeklyTracker = tracker
	l.Weekly = []WeeklyTask{weekly}
	l.tick(context.Background())

	// Allow any goroutine that would have fired to run.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if fired != 0 {
		t.Errorf("weekly should not fire within 7d; got %d", fired)
	}
	mu.Unlock()

	// Tracker should be unchanged.
	data, _ := os.ReadFile(tracker)
	if strings.TrimSpace(string(data)) != recent {
		t.Errorf("tracker changed unexpectedly: %s", data)
	}
}

func TestWeeklyTaskFiresOnFirstRunWhenTrackerMissing(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-weekly.txt" // does not exist

	done := make(chan struct{}, 1)
	weekly := func(ctx context.Context) {
		select {
		case done <- struct{}{}:
		default:
		}
	}

	l := New(rc, &fakeHb{}, &fakeNf{}, a, &bytes.Buffer{}, 1*time.Millisecond)
	l.WeeklyTracker = tracker
	l.Weekly = []WeeklyTask{weekly}
	l.tick(context.Background())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("weekly task did not fire on first run (missing tracker)")
	}
	if _, err := os.Stat(tracker); err != nil {
		t.Errorf("tracker should be created: %v", err)
	}
}

// TestWeeklyTask_PanicDoesNotKillSubsequent asserts a panicking weekly task
// is recovered + later tasks in the same tick still run (per-task recover).
func TestWeeklyTask_PanicDoesNotKillSubsequent(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-weekly.txt" // missing → fires immediately

	done := make(chan struct{}, 1)
	var ranAfter atomic.Bool
	panicker := func(ctx context.Context) { panic("synthetic weekly panic") }
	survivor := func(ctx context.Context) {
		ranAfter.Store(true)
		select {
		case done <- struct{}{}:
		default:
		}
	}

	out := &bytes.Buffer{}
	l := New(rc, &fakeHb{}, &fakeNf{}, a, out, 1*time.Millisecond)
	l.WeeklyTracker = tracker
	l.Weekly = []WeeklyTask{panicker, survivor}
	l.tick(context.Background())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("survivor task did not run after panicker panicked")
	}
	if !ranAfter.Load() {
		t.Error("survivor task did not run")
	}
	if !strings.Contains(out.String(), "weekly task panic") {
		t.Errorf("output missing panic log: %q", out.String())
	}
}

// TestWeeklyHourGate asserts WeeklyHour defers the weekly task when the
// current hour is below the threshold (operator-quiet-hours guard).
func TestWeeklyHourGate(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}
	dir := t.TempDir()
	tracker := dir + "/last-weekly.txt" // missing → would normally fire

	var fired atomic.Int32
	weekly := func(ctx context.Context) { fired.Add(1) }
	out := &bytes.Buffer{}

	l := New(rc, &fakeHb{}, &fakeNf{}, a, out, 1*time.Millisecond)
	l.WeeklyTracker = tracker
	l.Weekly = []WeeklyTask{weekly}
	// Hours go 0-23; 25 is unreachable so the gate must always defer.
	l.WeeklyHour = 25
	l.tick(context.Background())

	if fired.Load() != 0 {
		t.Errorf("WeeklyHour=25 should always defer; fired=%d", fired.Load())
	}
	if !strings.Contains(out.String(), "deferred") {
		t.Errorf("output should mention deferred: %q", out.String())
	}
	if _, err := os.Stat(tracker); err == nil {
		t.Errorf("tracker should NOT be written when weekly deferred")
	}
}

// TestLoopContinuesWhenRegattaErrors asserts a Regatta.List failure logs but
// does not kill the tick — heartbeat must still run.
func TestLoopContinuesWhenRegattaErrors(t *testing.T) {
	rc := &errRegatta{}
	hb := &fakeHb{}
	out := &bytes.Buffer{}
	a := &audit.Logger{Path: t.TempDir() + "/audit.jsonl"}

	l := New(rc, hb, &fakeNf{}, a, out, 1*time.Millisecond)
	l.tick(context.Background())
	if hb.pings != 1 {
		t.Errorf("heartbeat should fire even when regatta errors; got %d", hb.pings)
	}
	if !strings.Contains(out.String(), "regatta list error") {
		t.Errorf("output should surface regatta error: %q", out.String())
	}
}

// errRegatta returns an error on every List call.
type errRegatta struct{}

func (errRegatta) List(_ context.Context) ([]regattaclient.Agent, error) {
	return nil, errString("synthetic regatta failure")
}

type errString string

func (e errString) Error() string { return string(e) }

func TestLoopRespectsContextCancellation(t *testing.T) {
	rc := &fakeRegatta{resps: [][]regattaclient.Agent{{}}}
	l := New(rc, &fakeHb{}, &fakeNf{}, &audit.Logger{Path: t.TempDir() + "/audit.jsonl"},
		&bytes.Buffer{}, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not exit after cancel")
	}
}
