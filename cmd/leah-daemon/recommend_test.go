package main

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/recommend"
	"github.com/trilam/leah/internal/testutil"
)

// fakeSignalEngine satisfies recommend.SignalEngine for wiring-tests; only
// OnSignal is asserted — Propose/Accept/Reject/Apply are no-ops.
type fakeSignalEngine struct {
	mu   sync.Mutex
	seen []recommend.Signal
}

func (f *fakeSignalEngine) OnSignal(_ context.Context, sig recommend.Signal) ([]recommend.Recommendation, error) {
	f.mu.Lock()
	f.seen = append(f.seen, sig)
	f.mu.Unlock()
	return nil, nil
}

func (f *fakeSignalEngine) Propose(context.Context) ([]recommend.Recommendation, error) {
	return nil, nil
}
func (f *fakeSignalEngine) Accept(context.Context, string) error                  { return nil }
func (f *fakeSignalEngine) Reject(context.Context, string) error                  { return nil }
func (f *fakeSignalEngine) Apply(context.Context, recommend.Recommendation) error { return nil }

func TestStartRecommendDispatcher_WiresToEngine(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := obs.NewRegistry()
	bus := obs.NewBroadcaster()
	eng := &fakeSignalEngine{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop, err := startRecommendDispatcher(ctx, lg, reg, bus, eng)
	if err != nil {
		t.Fatalf("startRecommendDispatcher: %v", err)
	}
	defer stop()

	bus.Emit(obs.Event{Kind: "voice.speak", TS: time.Unix(1700000000, 0).UTC(), Detail: "wake"})

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		eng.mu.Lock()
		defer eng.mu.Unlock()
		return len(eng.seen) >= 1
	})
	eng.mu.Lock()
	defer eng.mu.Unlock()
	if got := eng.seen[0]; got.Kind != "voice.speak" || got.Detail != "wake" {
		t.Fatalf("translation mismatch: %+v", got)
	}
}

func TestStartRecommendDispatcher_CtxCancel_Stops(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := obs.NewRegistry()
	bus := obs.NewBroadcaster()
	eng := &fakeSignalEngine{}

	ctx, cancel := context.WithCancel(context.Background())
	stop, err := startRecommendDispatcher(ctx, lg, reg, bus, eng)
	if err != nil {
		t.Fatalf("startRecommendDispatcher: %v", err)
	}
	cancel()
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not return within 1s after ctx cancel")
	}
}
