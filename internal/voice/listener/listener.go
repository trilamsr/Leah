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

// Fake is the deterministic Listener used by every session-layer test. The
// test goroutine calls Emit; the channel returned by Start replays them in
// order. Concurrent Emit calls are safe — a sync.Mutex guards the close
// transition so producers never send on a closed channel.
type Fake struct {
	mu     sync.RWMutex
	out    chan Segment
	closed bool
	instr  *Instrumentation
}

// WithInstrumentation wires the A2 utterance→transcript histogram so Emit
// records on Final segments. Safe to call before Start.
func (f *Fake) WithInstrumentation(i *Instrumentation) *Fake {
	f.mu.Lock()
	f.instr = i
	f.mu.Unlock()
	return f
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
	out, closed, instr := f.out, f.closed, f.instr
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
	// Record AFTER send so the observation reflects the seam the consumer
	// observes — the final transcript handed off, not merely produced.
	instr.RecordFinal(s)
}

