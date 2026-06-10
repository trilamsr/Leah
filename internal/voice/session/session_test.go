package session_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/voice/listener"
	"github.com/trilam/leah/internal/voice/session"
)

// fakeTTS is a barge-in-aware TTS double: Speak blocks until ctx is cancelled
// OR a configurable "done" channel closes, so tests can model an in-flight
// utterance that the session must cancel.
type fakeTTS struct {
	mu        sync.Mutex
	spoken    []string
	cancelled int32
	gate      chan struct{} // closed in tests to let Speak return success
	autoDone  bool          // when true, Speak returns nil immediately
}

func (f *fakeTTS) Speak(ctx context.Context, text string) error {
	f.mu.Lock()
	f.spoken = append(f.spoken, text)
	gate := f.gate
	auto := f.autoDone
	f.mu.Unlock()
	if auto {
		return nil
	}
	if gate == nil {
		gate = make(chan struct{})
	}
	select {
	case <-ctx.Done():
		atomic.AddInt32(&f.cancelled, 1)
		return ctx.Err()
	case <-gate:
		return nil
	}
}

func (f *fakeTTS) spokenCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.spoken)
}

func (f *fakeTTS) cancelCount() int32 { return atomic.LoadInt32(&f.cancelled) }

// fakeReasoner returns canned text after an optional delay; honors ctx cancel.
type fakeReasoner struct {
	reply string
	delay time.Duration
	err   error
}

func (r *fakeReasoner) Ask(ctx context.Context, _ string) (string, error) {
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

// fakeAttestor lets tests deny or accept the session-arming phrase.
type fakeAttestor struct{ deny bool }

func (a *fakeAttestor) Attest(_ context.Context, _ string) error {
	if a.deny {
		return errors.New("attest: denied")
	}
	return nil
}

func newSession(t *testing.T, fl *listener.Fake, tts *fakeTTS, rs *fakeReasoner, att *fakeAttestor) *session.Session {
	t.Helper()
	return &session.Session{
		Listen:    fl,
		Speak:     tts,
		Reason:    rs,
		Attest:    att,
		IdleAfter: 50 * time.Millisecond,
		Now:       time.Now,
	}
}

// TestSession_IdleToListening_OnWake: post-attest, a final segment drives idle→listening→thinking→speaking.
func TestSession_IdleToListening_OnWake(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	tts := &fakeTTS{autoDone: true}
	rs := &fakeReasoner{reply: "ok"}
	s := newSession(t, fl, tts, rs, &fakeAttestor{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// Attestation prompt is spoken first; supply the "yes leah" final.
	waitForSpoken(t, tts, 1)
	fl.Emit(listener.Segment{Text: "yes leah", Final: true})
	// Now operator utterance.
	fl.Emit(listener.Segment{Text: "what time is it", Final: true})
	waitForSpoken(t, tts, 2)

	cancel()
	<-done
	if got := tts.spokenCount(); got < 2 {
		t.Fatalf("spokenCount = %d, want >= 2 (prompt + reply)", got)
	}
}

// TestSession_BargeIn_CancelsTTS: a Final segment mid-TTS cancels in-flight Speak.
func TestSession_BargeIn_CancelsTTS(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	tts := &fakeTTS{gate: make(chan struct{})}
	rs := &fakeReasoner{reply: "long reply"}
	s := newSession(t, fl, tts, rs, &fakeAttestor{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	waitForSpoken(t, tts, 1) // attestation prompt is gated; release it
	tts.mu.Lock()
	close(tts.gate)
	tts.gate = make(chan struct{})
	tts.mu.Unlock()
	fl.Emit(listener.Segment{Text: "yes leah", Final: true})

	// First utterance → reasoner → speaking (blocked on gate).
	fl.Emit(listener.Segment{Text: "tell me a story", Final: true})
	waitForSpoken(t, tts, 2)

	// Barge-in: a second final segment must cancel the in-flight Speak ctx.
	fl.Emit(listener.Segment{Text: "stop", Final: true})

	deadline := time.After(2 * time.Second)
	for tts.cancelCount() == 0 {
		select {
		case <-deadline:
			t.Fatalf("TTS was not cancelled by barge-in (spoken=%d, cancelled=%d)", tts.spokenCount(), tts.cancelCount())
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	<-done
}

// TestSession_IdleTimeout: an armed session with no input transitions out of armed before exit.
func TestSession_IdleTimeout(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	tts := &fakeTTS{autoDone: true}
	rs := &fakeReasoner{reply: "ok"}
	s := newSession(t, fl, tts, rs, &fakeAttestor{})
	s.IdleAfter = 30 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	waitForSpoken(t, tts, 1)
	fl.Emit(listener.Segment{Text: "yes leah", Final: true})

	// No further input — wait past IdleAfter; session must transition to requires_wake
	// (the spec §9 "armed → idle-timeout → requires_wake" rule) which speaks the
	// "going to sleep" TTS.
	deadline := time.After(2 * time.Second)
	for tts.spokenCount() < 2 {
		select {
		case <-deadline:
			t.Fatalf("idle-timeout did not fire (spoken=%d)", tts.spokenCount())
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	<-done
}

// TestSession_AttestationDenied_ReturnsToIdle: wrong phrase terminates with denial.
func TestSession_AttestationDenied_ReturnsToIdle(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	tts := &fakeTTS{autoDone: true}
	rs := &fakeReasoner{reply: "should not be called"}
	s := newSession(t, fl, tts, rs, &fakeAttestor{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	waitForSpoken(t, tts, 1) // prompt
	fl.Emit(listener.Segment{Text: "no thanks", Final: true})

	select {
	case err := <-done:
		if !errors.Is(err, session.ErrAttestationDenied) {
			t.Fatalf("Run err = %v, want ErrAttestationDenied", err)
		}
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatalf("session did not terminate on denied attestation")
	}
}

// TestSession_ConcurrentSafe: stress emit + cancel under -race.
func TestSession_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	tts := &fakeTTS{autoDone: true}
	rs := &fakeReasoner{reply: "ok"}
	s := newSession(t, fl, tts, rs, &fakeAttestor{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	waitForSpoken(t, tts, 1)
	fl.Emit(listener.Segment{Text: "yes leah", Final: true})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				fl.Emit(listener.Segment{Text: "ping", Final: j%3 == 0})
			}
		}(i)
	}
	wg.Wait()
	cancel()
	<-done
}

// waitForSpoken blocks until tts.spokenCount() >= n or test deadline hits.
func waitForSpoken(t *testing.T, tts *fakeTTS, n int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for tts.spokenCount() < n {
		select {
		case <-deadline:
			t.Fatalf("waitForSpoken(%d): got %d", n, tts.spokenCount())
		case <-time.After(5 * time.Millisecond):
		}
	}
}
