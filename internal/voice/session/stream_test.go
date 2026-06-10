package session_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/voice/listener"
	"github.com/trilam/leah/internal/voice/session"
)

// fakeStreamReasoner emits canned text chunks on AskStream; satisfies the
// session.StreamReasoner interface so Session takes the streaming path.
type fakeStreamReasoner struct {
	chunks    []string
	preDelay  time.Duration // delay before first chunk
	gapDelay  time.Duration // delay between subsequent chunks
	emitCount int32
}

func (f *fakeStreamReasoner) Ask(ctx context.Context, _ string) (string, error) {
	return strings.Join(f.chunks, ""), nil
}

func (f *fakeStreamReasoner) AskStream(ctx context.Context, _ string) (<-chan string, error) {
	out := make(chan string, len(f.chunks))
	go func() {
		defer close(out)
		if f.preDelay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(f.preDelay):
			}
		}
		for i, c := range f.chunks {
			if i > 0 && f.gapDelay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(f.gapDelay):
				}
			}
			select {
			case <-ctx.Done():
				return
			case out <- c:
				atomic.AddInt32(&f.emitCount, 1)
			}
		}
	}()
	return out, nil
}

// chunkTTS records each Speak call's text + timestamp so tests assert
// sentence-boundary chunking and first-speak latency.
type chunkTTS struct {
	mu        sync.Mutex
	spoken    []string
	at        []time.Time
	start     time.Time
	gate      chan struct{} // when non-nil, Speak blocks on gate or ctx
	cancelled int32
}

func (c *chunkTTS) Speak(ctx context.Context, text string) error {
	c.mu.Lock()
	if c.start.IsZero() {
		c.start = time.Now()
	}
	c.spoken = append(c.spoken, text)
	c.at = append(c.at, time.Now())
	gate := c.gate
	c.mu.Unlock()
	if gate == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		atomic.AddInt32(&c.cancelled, 1)
		return ctx.Err()
	case <-gate:
		return nil
	}
}

func (c *chunkTTS) chunks() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.spoken))
	copy(out, c.spoken)
	return out
}

func (c *chunkTTS) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.spoken)
}

func (c *chunkTTS) firstSpeakAfter(t time.Time) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	// First chunk after the attestation prompt — find the second spoken at
	// timestamp.
	if len(c.at) < 2 {
		return -1
	}
	return c.at[1].Sub(t)
}

func newStreamSession(t *testing.T, fl *listener.Fake, tts *chunkTTS, rs *fakeStreamReasoner) *session.Session {
	t.Helper()
	return &session.Session{
		Listen:    fl,
		Speak:     tts,
		Reason:    rs,
		Attest:    &fakeAttestor{},
		IdleAfter: 5 * time.Second,
		Now:       time.Now,
	}
}

// TestSession_StreamsFirstSentence_Under500ms — first speakable sentence
// boundary in the stream must reach Speak well before the whole reply finishes.
// The reasoner emits "Hello world. " in the first chunk and then stalls 200ms
// before the trailing tail; first Speak must fire near the boundary, not after
// the stall.
func TestSession_StreamsFirstSentence_Under500ms(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	tts := &chunkTTS{}
	rs := &fakeStreamReasoner{
		chunks:   []string{"Hello world. ", "Trailing tail without terminator"},
		gapDelay: 250 * time.Millisecond,
	}
	s := newStreamSession(t, fl, tts, rs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	waitForChunk(t, tts, 1) // attestation prompt
	fl.Emit(listener.Segment{Text: "yes leah", Final: true})

	utterAt := time.Now()
	fl.Emit(listener.Segment{Text: "say hi", Final: true})

	// Expect first reply chunk to land within 500ms (gapDelay is 250ms before
	// the trailing tail, so sentence-boundary emission must happen first).
	waitForChunk(t, tts, 2)
	dt := tts.firstSpeakAfter(utterAt)
	if dt < 0 {
		t.Fatalf("no reply chunk recorded")
	}
	if dt > 500*time.Millisecond {
		t.Errorf("first sentence latency %v > 500ms", dt)
	}

	cancel()
	<-done
}

// TestSession_TrailingFlush — the final chunk arriving without a sentence
// terminator must still reach Speak via Flush().
func TestSession_TrailingFlush(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	tts := &chunkTTS{}
	rs := &fakeStreamReasoner{
		chunks: []string{"First sentence here. ", "no terminator at end"},
	}
	s := newStreamSession(t, fl, tts, rs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	waitForChunk(t, tts, 1) // prompt
	fl.Emit(listener.Segment{Text: "yes leah", Final: true})
	fl.Emit(listener.Segment{Text: "explain", Final: true})

	// Need at least: prompt + sentence + trailing flush = 3 chunks.
	waitForChunk(t, tts, 3)
	got := tts.chunks()
	last := got[len(got)-1]
	if !strings.Contains(last, "no terminator at end") {
		t.Errorf("trailing flush missing; chunks=%q", got)
	}

	cancel()
	<-done
}

// TestSession_BargeInCancelsMidStream — a Final segment mid-stream must cancel
// the in-flight reply ctx so the stream goroutine and any in-flight chunked
// Speak unwind cleanly.
func TestSession_BargeInCancelsMidStream(t *testing.T) {
	t.Parallel()
	fl := listener.NewFake()
	tts := &chunkTTS{gate: make(chan struct{})}
	rs := &fakeStreamReasoner{
		chunks:   []string{"Sentence one. ", "Sentence two. ", "Sentence three."},
		gapDelay: 50 * time.Millisecond,
	}
	s := newStreamSession(t, fl, tts, rs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	waitForChunk(t, tts, 1) // prompt (also gated)
	tts.mu.Lock()
	close(tts.gate)
	tts.gate = make(chan struct{})
	tts.mu.Unlock()

	fl.Emit(listener.Segment{Text: "yes leah", Final: true})
	fl.Emit(listener.Segment{Text: "tell me about birds", Final: true})

	// Wait for first reply chunk to be in flight (Speak holds the gate).
	waitForChunk(t, tts, 2)

	// Barge-in mid-stream.
	fl.Emit(listener.Segment{Text: "stop", Final: true})

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&tts.cancelled) == 0 {
		select {
		case <-deadline:
			t.Fatalf("TTS was not cancelled by barge-in mid-stream (count=%d)", tts.count())
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	<-done
}

// waitForChunk blocks until tts.count() >= n or test deadline hits.
func waitForChunk(t *testing.T, tts *chunkTTS, n int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for tts.count() < n {
		select {
		case <-deadline:
			t.Fatalf("waitForChunk(%d): got %d", n, tts.count())
		case <-time.After(5 * time.Millisecond):
		}
	}
}
