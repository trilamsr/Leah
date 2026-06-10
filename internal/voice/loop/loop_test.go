package loop_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/testutil"
	"github.com/trilam/leah/internal/voice/listener"
	"github.com/trilam/leah/internal/voice/loop"
)

// startedListener wraps listener.Fake to publish a ready signal once the
// loop has called Start — fixes the pre-Start Emit-drop race deterministically.
type startedListener struct {
	*listener.Fake
	ready chan struct{}
	once  sync.Once
}

func newStartedListener() *startedListener {
	return &startedListener{Fake: listener.NewFake(), ready: make(chan struct{})}
}

func (s *startedListener) Start(ctx context.Context) (<-chan listener.Segment, error) {
	ch, err := s.Fake.Start(ctx)
	s.once.Do(func() { close(s.ready) })
	return ch, err
}

func (s *startedListener) waitReady(t *testing.T) {
	t.Helper()
	select {
	case <-s.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("listener.Start never called")
	}
}

type fakeTTS struct {
	mu        sync.Mutex
	spoken    []string
	cancelled int32
	gate      chan struct{}
	auto      bool
}

func (f *fakeTTS) Speak(ctx context.Context, text string) error {
	f.mu.Lock()
	f.spoken = append(f.spoken, text)
	g, a := f.gate, f.auto
	f.mu.Unlock()
	if a {
		return nil
	}
	if g == nil {
		g = make(chan struct{})
	}
	select {
	case <-ctx.Done():
		atomic.AddInt32(&f.cancelled, 1)
		return ctx.Err()
	case <-g:
		return nil
	}
}

func (f *fakeTTS) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.spoken)
}

func (f *fakeTTS) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.spoken) == 0 {
		return ""
	}
	return f.spoken[len(f.spoken)-1]
}

func (f *fakeTTS) cancels() int32 { return atomic.LoadInt32(&f.cancelled) }

type fakeReasoner struct {
	reply  string
	delay  time.Duration
	err    error
	called int32
}

func (r *fakeReasoner) Ask(ctx context.Context, _ string) (string, error) {
	atomic.AddInt32(&r.called, 1)
	if r.err != nil {
		return "", r.err
	}
	if r.delay == 0 {
		return r.reply, nil
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(r.delay):
		return r.reply, nil
	}
}

func (r *fakeReasoner) callCount() int32 { return atomic.LoadInt32(&r.called) }

type fakeIntent struct {
	handle   bool
	err      error
	captured chan string
}

func (f *fakeIntent) Recognize(_ context.Context, t string) (bool, error) {
	if f.captured != nil {
		f.captured <- t
	}
	return f.handle, f.err
}

type wakeChan chan struct{}

func (w wakeChan) Events() <-chan struct{} { return w }

func TestLoop_WakeToFinalToReasonerToTTS_HappyPath(t *testing.T) {
	fl := newStartedListener()
	tts := &fakeTTS{auto: true}
	rs := &fakeReasoner{reply: "the time is now"}
	wake := make(wakeChan, 1)

	l := loop.New(loop.Config{Listener: fl, Reasoner: rs, TTS: tts, Wake: wake})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()
	fl.waitReady(t)

	wake <- struct{}{}
	// Wake send is buffered; loop's select may not have consumed it before the
	// next Emit. Settle by polling on the side-effect we actually want.
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		fl.Emit(listener.Segment{Text: "what time is it", Final: true})
		return tts.count() >= 1
	})

	if got := tts.last(); got != "the time is now" {
		t.Fatalf("tts got %q want %q", got, "the time is now")
	}
	if rs.callCount() < 1 {
		t.Fatalf("reasoner called %d times want >= 1", rs.callCount())
	}

	cancel()
	<-done
}

func TestLoop_BargeIn_CancelsTTS(t *testing.T) {
	fl := newStartedListener()
	tts := &fakeTTS{}
	rs := &fakeReasoner{reply: "first reply"}

	l := loop.New(loop.Config{Listener: fl, Reasoner: rs, TTS: tts})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()
	fl.waitReady(t)

	fl.Emit(listener.Segment{Text: "hello", Final: true})
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		return tts.count() == 1
	})

	rs.reply = "second reply"
	fl.Emit(listener.Segment{Text: "wait stop", Final: true})
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		return tts.cancels() >= 1
	})

	cancel()
	<-done
}

func TestLoop_TripIntent_RoutedNotReasoner(t *testing.T) {
	fl := newStartedListener()
	tts := &fakeTTS{auto: true}
	rs := &fakeReasoner{reply: "should not run"}
	ic := &fakeIntent{handle: true, captured: make(chan string, 1)}

	l := loop.New(loop.Config{Listener: fl, Reasoner: rs, TTS: tts, IntentRouter: ic})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()
	fl.waitReady(t)

	fl.Emit(listener.Segment{Text: "what's on my drive to seattle", Final: true})

	select {
	case got := <-ic.captured:
		if got != "what's on my drive to seattle" {
			t.Fatalf("intent got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("intent router not called")
	}

	// Give the goroutine a fair chance to call the reasoner if the routing
	// branch were wrong; settle on observable absence rather than wall-clock.
	testutil.Eventually(t, 200*time.Millisecond, 10*time.Millisecond, func() bool {
		return rs.callCount() == 0
	})
	if rs.callCount() != 0 {
		t.Fatalf("reasoner called %d times, want 0", rs.callCount())
	}

	cancel()
	<-done
}

func TestLoop_ReasonerError_AnnouncesError(t *testing.T) {
	fl := newStartedListener()
	tts := &fakeTTS{auto: true}
	rs := &fakeReasoner{err: errors.New("upstream 500")}

	l := loop.New(loop.Config{Listener: fl, Reasoner: rs, TTS: tts})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()
	fl.waitReady(t)

	fl.Emit(listener.Segment{Text: "anything", Final: true})

	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		return tts.count() == 1
	})
	if got := tts.last(); got == "" || got == "upstream 500" {
		t.Fatalf("unexpected announcement %q", got)
	}

	cancel()
	<-done
}

func TestLoop_CtxCancel_StopsLoop(t *testing.T) {
	fl := newStartedListener()
	tts := &fakeTTS{auto: true}
	rs := &fakeReasoner{reply: "x"}

	l := loop.New(loop.Config{Listener: fl, Reasoner: rs, TTS: tts})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()
	fl.waitReady(t)

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit on ctx cancel")
	}
}
