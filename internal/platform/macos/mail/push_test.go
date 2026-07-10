package mail

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/macos/sqliteopen"
	"github.com/trilam/leah/internal/platform/telemetry"
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

func runPush(t *testing.T, w sqliteopen.WALWatcher, rec *recorder, debounce time.Duration, clk *fakeClock) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	p := &PushSource{Watcher: w, ObsEmit: rec.emit, Debounce: debounce, NowFn: clk.Now}
	go func() { _ = p.Run(ctx) }()
	return cancel
}

func TestPushSource_Run_EmitsTypedPayload(t *testing.T) {
	t.Parallel()
	w := sqliteopen.NewFakeWALWatcher()
	t.Cleanup(func() { _ = w.Close() })

	clk := &fakeClock{now: time.Unix(1_000_000, 0)}
	rec := &recorder{}
	cancel := runPush(t, w, rec, 0, clk)
	t.Cleanup(cancel)

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		w.Inject(sqliteopen.WALEvent{})
		return len(rec.snapshot()) >= 1
	})

	got := rec.snapshot()
	if got[0].Kind != "mail_changed" {
		t.Fatalf("kind=%q want mail_changed", got[0].Kind)
	}
	if _, ok := got[0].Payload.(telemetry.MailChangedEvent); !ok {
		t.Fatalf("payload=%T want obs.MailChangedEvent", got[0].Payload)
	}
}

// Mail.app emits a WAL-append burst per SyncMailboxes pass — burst inside the
// debounce window must collapse to one downstream signal.
func TestPushSource_Debounce_CoalescesBurst(t *testing.T) {
	t.Parallel()
	w := sqliteopen.NewFakeWALWatcher()
	t.Cleanup(func() { _ = w.Close() })

	clk := &fakeClock{now: time.Unix(1_000_000, 0)}
	rec := &recorder{}
	cancel := runPush(t, w, rec, 500*time.Millisecond, clk)
	t.Cleanup(cancel)

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		w.Inject(sqliteopen.WALEvent{})
		return len(rec.snapshot()) >= 1
	})
	for i := 0; i < 5; i++ {
		w.Inject(sqliteopen.WALEvent{})
	}
	time.Sleep(50 * time.Millisecond) // allow-sleep: bounded debounce-suppression settle; not a polling loop
	if n := len(rec.snapshot()); n != 1 {
		t.Fatalf("emits=%d want 1 (burst coalesced)", n)
	}
}

func TestPushSource_Debounce_EmitsAfterWindow(t *testing.T) {
	t.Parallel()
	w := sqliteopen.NewFakeWALWatcher()
	t.Cleanup(func() { _ = w.Close() })

	clk := &fakeClock{now: time.Unix(1_000_000, 0)}
	rec := &recorder{}
	cancel := runPush(t, w, rec, 500*time.Millisecond, clk)
	t.Cleanup(cancel)

	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		w.Inject(sqliteopen.WALEvent{})
		return len(rec.snapshot()) >= 1
	})
	clk.advance(time.Second)
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		w.Inject(sqliteopen.WALEvent{})
		return len(rec.snapshot()) >= 2
	})

	if n := len(rec.snapshot()); n < 2 {
		t.Fatalf("emits=%d want >=2 (post-window emit)", n)
	}
}

func TestPushSource_CtxCancel_StopsLoop(t *testing.T) {
	t.Parallel()
	w := sqliteopen.NewFakeWALWatcher()
	t.Cleanup(func() { _ = w.Close() })

	clk := &fakeClock{now: time.Unix(1_000_000, 0)}
	p := &PushSource{Watcher: w, ObsEmit: func(telemetry.Event) {}, NowFn: clk.Now}
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

func TestPushSource_Run_RequiresWatcher(t *testing.T) {
	t.Parallel()
	p := &PushSource{ObsEmit: func(telemetry.Event) {}}
	if err := p.Run(context.Background()); err == nil {
		t.Fatal("Run with nil Watcher: want error")
	}
}
