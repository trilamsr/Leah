package contacts

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

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func runPush(t *testing.T, src osevent.Source, rec *recorder, debounce time.Duration, clk *fakeClock) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	p := &PushSource{Source: src, ObsEmit: rec.emit, Debounce: debounce, NowFn: clk.Now}
	go func() { _ = p.Run(ctx) }()
	return cancel
}

func TestPushSource_Run_EmitsTypedPayload(t *testing.T) {
	t.Parallel()
	src := osevent.NewFake(osevent.Config{})
	t.Cleanup(func() { _ = src.Close() })

	clk := &fakeClock{now: time.Unix(1_000_000, 0)}
	rec := &recorder{}
	cancel := runPush(t, src, rec, 0, clk)
	t.Cleanup(cancel)

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		src.Inject(osevent.Event{Kind: osevent.ContactStoreChanged})
		return len(rec.snapshot()) >= 1
	})

	got := rec.snapshot()
	if got[0].Kind != "contact_store_changed" {
		t.Fatalf("kind=%q want contact_store_changed", got[0].Kind)
	}
	if _, ok := got[0].Payload.(obs.ContactStoreChangedEvent); !ok {
		t.Fatalf("payload=%T want obs.ContactStoreChangedEvent", got[0].Payload)
	}
}

// CN fires many times per write — second emit inside the debounce window must
// be suppressed; the burst collapses to one downstream signal.
func TestPushSource_Debounce_CoalescesBurst(t *testing.T) {
	t.Parallel()
	src := osevent.NewFake(osevent.Config{})
	t.Cleanup(func() { _ = src.Close() })

	clk := &fakeClock{now: time.Unix(1_000_000, 0)}
	rec := &recorder{}
	cancel := runPush(t, src, rec, 500*time.Millisecond, clk)
	t.Cleanup(cancel)

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		src.Inject(osevent.Event{Kind: osevent.ContactStoreChanged})
		return len(rec.snapshot()) >= 1
	})
	// Burst inside debounce window — must collapse to the single initial emit.
	for i := 0; i < 5; i++ {
		src.Inject(osevent.Event{Kind: osevent.ContactStoreChanged})
	}
	// Settle pass: 50ms covers the push goroutine draining 5 events; emit count
	// stays at 1 if debounce is honored.
	time.Sleep(50 * time.Millisecond) // allow-sleep: bounded debounce-suppression settle; not a polling loop
	if n := len(rec.snapshot()); n != 1 {
		t.Fatalf("emits=%d want 1 (burst coalesced)", n)
	}
}

// After the debounce window elapses, the next event must emit again — debounce
// suppresses inside-window only, not the steady state.
func TestPushSource_Debounce_EmitsAfterWindow(t *testing.T) {
	t.Parallel()
	src := osevent.NewFake(osevent.Config{})
	t.Cleanup(func() { _ = src.Close() })

	clk := &fakeClock{now: time.Unix(1_000_000, 0)}
	rec := &recorder{}
	cancel := runPush(t, src, rec, 500*time.Millisecond, clk)
	t.Cleanup(cancel)

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		src.Inject(osevent.Event{Kind: osevent.ContactStoreChanged})
		return len(rec.snapshot()) >= 1
	})
	clk.advance(time.Second)
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		src.Inject(osevent.Event{Kind: osevent.ContactStoreChanged})
		return len(rec.snapshot()) >= 2
	})

	if n := len(rec.snapshot()); n < 2 {
		t.Fatalf("emits=%d want >=2 (post-window emit)", n)
	}
}

func TestPushSource_CtxCancel_StopsLoop(t *testing.T) {
	t.Parallel()
	src := osevent.NewFake(osevent.Config{})
	t.Cleanup(func() { _ = src.Close() })

	clk := &fakeClock{now: time.Unix(1_000_000, 0)}
	p := &PushSource{Source: src, ObsEmit: func(obs.Event) {}, NowFn: clk.Now}
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

func TestPushSource_Run_RequiresSource(t *testing.T) {
	t.Parallel()
	p := &PushSource{ObsEmit: func(obs.Event) {}}
	if err := p.Run(context.Background()); err == nil {
		t.Fatal("Run with nil Source: want error")
	}
}
