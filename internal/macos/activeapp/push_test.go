package activeapp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/osevent"
	"github.com/trilam/leah/internal/testutil"
)

type recorder struct {
	mu    sync.Mutex
	calls []obs.Event
}

func (r *recorder) emit(e obs.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, e)
}

func (r *recorder) snapshot() []obs.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]obs.Event(nil), r.calls...)
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
	p, ok := got[0].Payload.(obs.WorkspaceActiveAppEvent)
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

	p := &PushSource{Source: src, ObsEmit: func(obs.Event) {}}
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
	p, _ := got[0].Payload.(obs.WorkspaceActiveAppEvent)
	if p.BundleID != "com.apple.Safari" {
		t.Fatalf("emit=%+v want safari only", got[0])
	}
}

func TestPushSource_Run_RequiresSource(t *testing.T) {
	t.Parallel()
	p := &PushSource{ObsEmit: func(obs.Event) {}}
	if err := p.Run(context.Background()); err == nil {
		t.Fatal("Run with nil Source: want error")
	}
}
