package recommend_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/recommend"
)

// captureMatcher records every Signal MemoryEngine.OnSignal hands it.
type captureMatcher struct {
	mu      sync.Mutex
	signals []recommend.Signal
	wake    chan struct{}
}

func newCaptureMatcher() *captureMatcher {
	return &captureMatcher{wake: make(chan struct{}, 16)}
}

func (m *captureMatcher) Match(_ context.Context, sig recommend.Signal) ([]recommend.Recommendation, error) {
	m.mu.Lock()
	m.signals = append(m.signals, sig)
	m.mu.Unlock()
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return nil, nil
}

func (m *captureMatcher) snapshot() []recommend.Signal {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]recommend.Signal, len(m.signals))
	copy(out, m.signals)
	return out
}

func waitN(t *testing.T, m *captureMatcher, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if len(m.snapshot()) >= want {
			return
		}
		select {
		case <-m.wake:
		case <-deadline:
			t.Fatalf("waitN: got %d signals, want %d", len(m.snapshot()), want)
		}
	}
}

func TestSignalBridge_AllowlistKindsReachEngine(t *testing.T) {
	bus := obs.NewBroadcaster()
	engine := recommend.NewMemoryEngine(nil)
	m := newCaptureMatcher()
	engine.RegisterMatcher(m)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	br, err := recommend.StartSignalBridge(ctx, bus, engine, recommend.SignalBridgeOptions{
		Kinds: []string{"safari.history_changed", "focus.state_changed"},
	})
	if err != nil {
		t.Fatalf("StartSignalBridge: %v", err)
	}
	defer br.Stop()

	bus.Emit(obs.Event{Kind: "safari.history_changed", Actor: "macos.safari", Detail: "hist"})
	bus.Emit(obs.Event{Kind: "focus.state_changed", Actor: "macos.focus", Detail: "dnd"})

	waitN(t, m, 2, 2*time.Second)
	got := m.snapshot()
	if got[0].Kind != "safari.history_changed" || got[1].Kind != "focus.state_changed" {
		t.Fatalf("kinds wrong: %+v", got)
	}
	if got[0].Detail != "hist" || got[1].Detail != "dnd" {
		t.Fatalf("detail not preserved: %+v", got)
	}
	if got[0].At.IsZero() {
		t.Fatalf("At zero — bridge must stamp time from event")
	}
}

func TestSignalBridge_NonAllowlistKindFiltered(t *testing.T) {
	bus := obs.NewBroadcaster()
	engine := recommend.NewMemoryEngine(nil)
	m := newCaptureMatcher()
	engine.RegisterMatcher(m)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	br, err := recommend.StartSignalBridge(ctx, bus, engine, recommend.SignalBridgeOptions{
		Kinds: []string{"safari.history_changed"},
	})
	if err != nil {
		t.Fatalf("StartSignalBridge: %v", err)
	}
	defer br.Stop()

	bus.Emit(obs.Event{Kind: "hud.state", Actor: "hud"})
	bus.Emit(obs.Event{Kind: "safari.history_changed", Actor: "macos.safari"})

	waitN(t, m, 1, 2*time.Second)
	if got := m.snapshot(); len(got) != 1 || got[0].Kind != "safari.history_changed" {
		t.Fatalf("filter leaked: %+v", got)
	}
}

func TestSignalBridge_ContextCancelStops(t *testing.T) {
	bus := obs.NewBroadcaster()
	engine := recommend.NewMemoryEngine(nil)
	m := newCaptureMatcher()
	engine.RegisterMatcher(m)

	ctx, cancel := context.WithCancel(context.Background())
	br, err := recommend.StartSignalBridge(ctx, bus, engine, recommend.SignalBridgeOptions{
		Kinds: []string{"safari.history_changed"},
	})
	if err != nil {
		t.Fatalf("StartSignalBridge: %v", err)
	}

	cancel()
	if err := br.Wait(2 * time.Second); err != nil {
		t.Fatalf("Wait after cancel: %v", err)
	}

	bus.Emit(obs.Event{Kind: "safari.history_changed"})
	// Bridge stopped — no signals should land. Short sleep then snapshot.
	time.Sleep(50 * time.Millisecond)
	if got := m.snapshot(); len(got) != 0 {
		t.Fatalf("signals after cancel: %+v", got)
	}
}

func TestSignalBridge_RequiresKinds(t *testing.T) {
	bus := obs.NewBroadcaster()
	engine := recommend.NewMemoryEngine(nil)
	if _, err := recommend.StartSignalBridge(context.Background(), bus, engine, recommend.SignalBridgeOptions{}); err == nil {
		t.Fatalf("expected error for empty Kinds")
	}
}

func TestSignalBridge_StopIsIdempotent(t *testing.T) {
	bus := obs.NewBroadcaster()
	engine := recommend.NewMemoryEngine(nil)
	br, err := recommend.StartSignalBridge(context.Background(), bus, engine, recommend.SignalBridgeOptions{
		Kinds: []string{"safari.history_changed"},
	})
	if err != nil {
		t.Fatalf("StartSignalBridge: %v", err)
	}
	br.Stop()
	br.Stop()
}
