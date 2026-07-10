package activeapp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/input/osevent"
	"github.com/trilam/leah/internal/platform/testutil"
)

type recorder struct {
	mu    sync.Mutex
	calls []telemetry.Event
}

func (r *recorder) emit(e telemetry.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, e)
}

func (r *recorder) snapshot() []telemetry.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]telemetry.Event(nil), r.calls...)
}

func runPush(t *testing.T, src osevent.Source, rec *recorder) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	p := &PushSource{Source: src, ObsEmit: rec.emit}
	errCh := make(chan error, 1)
	go func() { errCh <- p.Run(ctx) }()
	return cancel, errCh
}

func TestPushSource_Run_EmitsTypedPayload(t *testing.T) {
	t.Parallel()
	src := osevent.NewFake(osevent.Config{})
	t.Cleanup(func() { _ = src.Close() })

	rec := &recorder{}
	cancel, _ := runPush(t, src, rec)
	t.Cleanup(cancel)

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		src.Inject(osevent.Event{
			Kind:   osevent.WorkspaceActiveAppChanged,
			Detail: map[string]any{"bundle_id": "com.apple.Safari", "name": "Safari"},
		})
		return len(rec.snapshot()) >= 1
	})

	got := rec.snapshot()
	if got[0].Kind != "workspace.active_app_changed" {
		t.Fatalf("kind=%q want workspace.active_app_changed", got[0].Kind)
	}
	p, ok := got[0].Payload.(telemetry.WorkspaceActiveAppEvent)
	if !ok {
		t.Fatalf("payload=%T want obs.WorkspaceActiveAppEvent", got[0].Payload)
	}
	if p.BundleID != "com.apple.Safari" || p.Name != "Safari" {
		t.Fatalf("payload=%+v want {BundleID:com.apple.Safari Name:Safari}", p)
	}
}

// B1 regression gate: activeapp must NEVER emit hud.state. That kind has a
// typed contract (HUDStateEvent) consumed by ambient.js — any string detail
// resets the pill to "ambient" and injects phantom signals into the recommender.
func TestPushSource_Run_NeverEmitsHUDState(t *testing.T) {
	t.Parallel()
	src := osevent.NewFake(osevent.Config{})
	t.Cleanup(func() { _ = src.Close() })

	rec := &recorder{}
	cancel, _ := runPush(t, src, rec)
	t.Cleanup(cancel)

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		src.Inject(osevent.Event{
			Kind:   osevent.WorkspaceActiveAppChanged,
			Detail: map[string]any{"bundle_id": "com.apple.Safari", "name": "Safari"},
		})
		return len(rec.snapshot()) >= 1
	})
	for _, e := range rec.snapshot() {
		if e.Kind == "hud.state" {
			t.Fatalf("activeapp emitted hud.state: %+v (contract owned by HUD state machine)", e)
		}
	}
}

func TestPushSource_CtxCancel_StopsLoop(t *testing.T) {
	t.Parallel()
	src := osevent.NewFake(osevent.Config{})
	t.Cleanup(func() { _ = src.Close() })

	p := &PushSource{Source: src, ObsEmit: func(telemetry.Event) {}}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- p.Run(ctx) }()

	cancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Run returned nil after cancel; want ctx.Err()")
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not exit after ctx cancel")
	}
}

// Source-level blocklist (Fake suppresses) — PushSource must never see the event.
func TestPushSource_BlocklistedApp_NotEmitted(t *testing.T) {
	t.Parallel()
	src := osevent.NewFake(osevent.Config{Blocklist: []string{"com.bank."}})
	t.Cleanup(func() { _ = src.Close() })

	rec := &recorder{}
	cancel, _ := runPush(t, src, rec)
	t.Cleanup(cancel)

	src.Inject(osevent.Event{
		Kind:   osevent.WorkspaceActiveAppChanged,
		Detail: map[string]any{"bundle_id": "com.bank.chase", "name": "Chase"},
	})
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		src.Inject(osevent.Event{
			Kind:   osevent.WorkspaceActiveAppChanged,
			Detail: map[string]any{"bundle_id": "com.apple.Safari", "name": "Safari"},
		})
		return len(rec.snapshot()) >= 1
	})

	got := rec.snapshot()
	if len(got) != 1 {
		t.Fatalf("emits=%d want 1 (safari only): %+v", len(got), got)
	}
	p, _ := got[0].Payload.(telemetry.WorkspaceActiveAppEvent)
	if p.BundleID != "com.apple.Safari" {
		t.Fatalf("emit=%+v want safari only", got[0])
	}
}

func TestPushSource_Run_RequiresSource(t *testing.T) {
	t.Parallel()
	p := &PushSource{ObsEmit: func(telemetry.Event) {}}
	if err := p.Run(context.Background()); err == nil {
		t.Fatal("Run with nil Source: want error")
	}
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// Coalesce: NSWorkspace re-fires the same bundle on focus-without-switch
// (e.g. window raise inside the same app). Downstream recommender treats every
// emit as a fresh signal, so duplicates inflate scores.
func TestPushSource_Coalesce_DropsRepeatBundleID(t *testing.T) {
	t.Parallel()
	src := osevent.NewFake(osevent.Config{})
	t.Cleanup(func() { _ = src.Close() })

	rec := &recorder{}
	clk := &clock{t: time.Unix(0, 0)}
	p := &PushSource{
		Source:   src,
		ObsEmit:  rec.emit,
		Debounce: 0, // isolate coalesce
		NowFn:    clk.now,
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = p.Run(ctx) }()

	ev := osevent.Event{Kind: osevent.WorkspaceActiveAppChanged, Detail: map[string]any{"bundle_id": "com.apple.Safari", "name": "Safari"}}
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		src.Inject(ev)
		return len(rec.snapshot()) >= 1
	})
	for i := 0; i < 5; i++ {
		clk.advance(time.Second) // past debounce window
		src.Inject(ev)
	}
	// Inject a sentinel distinct bundle; once it lands the dup spam has flushed.
	clk.advance(time.Second)
	src.Inject(osevent.Event{Kind: osevent.WorkspaceActiveAppChanged, Detail: map[string]any{"bundle_id": "com.apple.Notes", "name": "Notes"}})
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		for _, e := range rec.snapshot() {
			if p, _ := e.Payload.(telemetry.WorkspaceActiveAppEvent); p.BundleID == "com.apple.Notes" {
				return true
			}
		}
		return false
	})

	if got := len(rec.snapshot()); got != 2 {
		t.Fatalf("emits=%d want 2 (1 safari coalesced + 1 notes sentinel)", got)
	}
}

// Debounce: rapid app-switch storms (cmd-tab through 5 apps in 200ms) should
// emit only after motion settles, otherwise the recommender thrashes.
func TestPushSource_Debounce_SuppressesRapidSwitches(t *testing.T) {
	t.Parallel()
	src := osevent.NewFake(osevent.Config{})
	t.Cleanup(func() { _ = src.Close() })

	rec := &recorder{}
	clk := &clock{t: time.Unix(0, 0)}
	p := &PushSource{
		Source:   src,
		ObsEmit:  rec.emit,
		Debounce: 250 * time.Millisecond,
		NowFn:    clk.now,
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = p.Run(ctx) }()

	// First switch always passes (no prior emit to debounce against).
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		src.Inject(osevent.Event{Kind: osevent.WorkspaceActiveAppChanged, Detail: map[string]any{"bundle_id": "com.apple.Safari", "name": "Safari"}})
		return len(rec.snapshot()) >= 1
	})

	// Storm of distinct bundles within 100ms of the first emit. Advance + inject
	// + bounded settle for each — the settle window must elapse before the next
	// clock advance, else the loop could read now() AFTER the advance and
	// falsely emit a storm bundle past the debounce gate.
	for _, b := range []string{"com.google.Chrome", "com.tinyspeck.slackmacgap", "com.apple.Terminal"} {
		clk.advance(50 * time.Millisecond)
		src.Inject(osevent.Event{Kind: osevent.WorkspaceActiveAppChanged, Detail: map[string]any{"bundle_id": b, "name": b}})
		time.Sleep(20 * time.Millisecond) // allow-sleep: bounded settle so the loop reads now() at the current clock value, not a later advance
		if len(rec.snapshot()) != 1 {
			t.Fatalf("storm bundle %s leaked an emit: %+v", b, rec.snapshot())
		}
	}
	// After the debounce window elapses, the next switch passes through —
	// arrival of Notes proves the storm fully drained.
	clk.advance(300 * time.Millisecond)
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		src.Inject(osevent.Event{Kind: osevent.WorkspaceActiveAppChanged, Detail: map[string]any{"bundle_id": "com.apple.Notes", "name": "Notes"}})
		return len(rec.snapshot()) >= 2
	})

	got := rec.snapshot()
	if len(got) != 2 {
		t.Fatalf("emits=%d want 2 (safari + notes only, storm debounced): %+v", len(got), got)
	}
	if p, _ := got[0].Payload.(telemetry.WorkspaceActiveAppEvent); p.BundleID != "com.apple.Safari" {
		t.Fatalf("emit[0]=%+v want safari", got[0])
	}
	if p, _ := got[1].Payload.(telemetry.WorkspaceActiveAppEvent); p.BundleID != "com.apple.Notes" {
		t.Fatalf("emit[1]=%+v want notes", got[1])
	}
}
