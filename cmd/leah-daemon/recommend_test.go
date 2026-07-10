package main

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
	"github.com/trilam/leah/internal/thinking/recommend"
	"github.com/trilam/leah/internal/platform/testutil"
	"github.com/trilam/leah/internal/platform/web"
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
	reg := telemetry.NewRegistry()
	bus := telemetry.NewBroadcaster()
	eng := &fakeSignalEngine{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop, err := startRecommendDispatcher(ctx, lg, reg, bus, eng)
	if err != nil {
		t.Fatalf("startRecommendDispatcher: %v", err)
	}
	defer stop()

	bus.Emit(telemetry.Event{Kind: "voice.speak", TS: time.Unix(1700000000, 0).UTC(), Detail: "wake"})

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

// TestStartRecommendDispatcher_EmitEventReachesEngine catches the orphan-bus
// bug: web.Server.ensureBroadcaster must NOT clobber an already-set default,
// or obs.EmitEvent fans out to a broadcaster the dispatcher doesn't subscribe.
func TestStartRecommendDispatcher_EmitEventReachesEngine(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := telemetry.NewRegistry()
	bus := telemetry.NewBroadcaster()
	telemetry.SetDefaultBroadcaster(bus)
	t.Cleanup(func() { telemetry.SetDefaultBroadcaster(nil) })

	eng := &fakeSignalEngine{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := startRecommendDispatcher(ctx, lg, reg, bus, eng)
	if err != nil {
		t.Fatalf("startRecommendDispatcher: %v", err)
	}
	defer stop()

	// A dashboard build must NOT replace the daemon's default broadcaster.
	srv := &web.Server{Addr: "127.0.0.1:0"}
	if _, err := srv.BuildMux(); err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	telemetry.EmitEvent(ctx, telemetry.Event{Kind: "voice.speak", Detail: "wake"})

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		eng.mu.Lock()
		defer eng.mu.Unlock()
		return len(eng.seen) >= 1
	})
}

// TestStartRecommendDispatcher_EndToEnd_ProducesRecommendation exercises the
// full bus → dispatcher → engine → matcher → Seed pipeline with the same
// matcher main.go registers.
func TestStartRecommendDispatcher_EndToEnd_ProducesRecommendation(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := telemetry.NewRegistry()
	bus := telemetry.NewBroadcaster()
	telemetry.SetDefaultBroadcaster(bus)
	t.Cleanup(func() { telemetry.SetDefaultBroadcaster(nil) })

	eng := recommend.NewMemoryEngine(nil)
	eng.RegisterMatcher(identitySignalMatcher{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop, err := startRecommendDispatcher(ctx, lg, reg, bus, eng)
	if err != nil {
		t.Fatalf("startRecommendDispatcher: %v", err)
	}
	defer stop()

	telemetry.EmitEvent(ctx, telemetry.Event{Kind: "voice.speak", Detail: "wake"})

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		recs, _ := eng.Propose(ctx)
		return len(recs) >= 1
	})
}

func TestStartRecommendDispatcher_CtxCancel_Stops(t *testing.T) {
	lg := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := telemetry.NewRegistry()
	bus := telemetry.NewBroadcaster()
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
