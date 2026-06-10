package listener

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Segment is one STT emission. Partial transcripts stream with Final=false;
// the end-of-utterance VAD trigger yields exactly one Final=true Segment.
type Segment struct {
	Text       string
	Final      bool
	StartedAt  time.Time
	FinishedAt time.Time
}

// Listener opens the microphone and streams Segments until ctx is cancelled.
// Returning the channel (not a handler) keeps the session-layer arbitration
// in §4 of the spec single-threaded: one select over wake, segments, TTS-done.
type Listener interface {
	Start(ctx context.Context) (<-chan Segment, error)
}

// ErrNotImplemented is returned by the W11 real-backend placeholder. W12
// replaces the body with whisper-stream wiring; tests assert the sentinel so
// the transition is mechanical.
var ErrNotImplemented = errors.New("voice/listener: real backend not implemented (W12 owns whisper-stream wiring)")

// Fake is the deterministic Listener used by every session-layer test. The
// test goroutine calls Emit; the channel returned by Start replays them in
// order. Concurrent Emit calls are safe — a sync.Mutex guards the close
// transition so producers never send on a closed channel.
type Fake struct {
	mu     sync.RWMutex
	out    chan Segment
	closed bool
}

// NewFake returns a Fake ready for Start. Reusing a Fake across Start calls
// is undefined — tests instantiate one per scenario.
func NewFake() *Fake {
	return &Fake{}
}

// Start hands back the segment channel. ctx-cancel closes the channel from a
// dedicated goroutine so readers see a clean end-of-stream.
func (f *Fake) Start(ctx context.Context) (<-chan Segment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.out != nil {
		return nil, errors.New("voice/listener: Fake already started")
	}
	// Small buffer mirrors a typical whisper-stream pipe — partials cluster
	// (~10/s) but the session reader is single-select-loop fast.
	f.out = make(chan Segment, 16)
	go func() {
		<-ctx.Done()
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.closed {
			close(f.out)
			f.closed = true
		}
	}()
	return f.out, nil
}

// Emit sends a Segment, blocking on a slow reader so backpressure mirrors
// the real whisper-stream pipe. Returns immediately if Start was never called
// or the channel is closed — a post-cancel Emit is a no-op, not a panic.
func (f *Fake) Emit(s Segment) {
	f.mu.RLock()
	out, closed := f.out, f.closed
	f.mu.RUnlock()
	if out == nil || closed {
		return
	}
	defer func() {
		// guard against a race where ctx-cancel closes out between the
		// RLock check and the send; recover keeps Emit a no-op.
		_ = recover()
	}()
	out <- s
}

// Real is the production Listener — W11 ships only the shell. W12 replaces
// Start with the whisper-stream subprocess pipeline (spec §3.3).
type Real struct{}

// NewReal returns the placeholder Real. Calling Start returns
// ErrNotImplemented until W12.
func NewReal() *Real { return &Real{} }

// Start is a W12 stub; see ErrNotImplemented.
func (r *Real) Start(_ context.Context) (<-chan Segment, error) {
	return nil, ErrNotImplemented
}
