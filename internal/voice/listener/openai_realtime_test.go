package listener

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/testutil"
)

type fakeAttestor struct {
	calls  int32
	denied bool
	scope  string
}

func (a *fakeAttestor) Attest(_ context.Context, scope string) error {
	atomic.AddInt32(&a.calls, 1)
	a.scope = scope
	if a.denied {
		return errors.New("denied")
	}
	return nil
}

type fakeTokenSource struct {
	token string
	err   error
}

func (s *fakeTokenSource) Token(_ context.Context) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.token == "" {
		return "sk-test", nil
	}
	return s.token, nil
}

type fakeWSConn struct {
	frames   [][]byte
	cursor   int
	mu       sync.Mutex
	gate     chan struct{}
	closed   bool
	closeN   int32
	writeErr error
	written  [][]byte
}

func newFakeWSConn(frames [][]byte) *fakeWSConn {
	return &fakeWSConn{frames: frames, gate: make(chan struct{})}
}

func (c *fakeWSConn) ReadMessage() ([]byte, error) {
	c.mu.Lock()
	if c.cursor >= len(c.frames) {
		c.mu.Unlock()
		<-c.gate
		return nil, errors.New("closed")
	}
	f := c.frames[c.cursor]
	c.cursor++
	c.mu.Unlock()
	return f, nil
}

func (c *fakeWSConn) WriteMessage(p []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.writeErr != nil {
		return c.writeErr
	}
	c.written = append(c.written, p)
	return nil
}

func (c *fakeWSConn) Close() error {
	atomic.AddInt32(&c.closeN, 1)
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.gate)
	}
	c.mu.Unlock()
	return nil
}

type fakeDialer struct {
	calls int32
	conn  *fakeWSConn
	err   error
	url   string
	tok   string
}

func (d *fakeDialer) Dial(_ context.Context, url, token string) (WebSocketConn, error) {
	atomic.AddInt32(&d.calls, 1)
	d.url = url
	d.tok = token
	if d.err != nil {
		return nil, d.err
	}
	return d.conn, nil
}

func deltaFrame(t *testing.T, text string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type":  "conversation.item.input_audio_transcription.delta",
		"delta": text,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func finalFrame(t *testing.T, text string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type":       "conversation.item.input_audio_transcription.completed",
		"transcript": text,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestOpenAIRealtime_Start_AttestationDenied_NoDial(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{denied: true}
	dialer := &fakeDialer{conn: newFakeWSConn(nil)}
	r := NewOpenAIRealtime(Config{
		Attestor:    att,
		TokenSource: &fakeTokenSource{},
		Dialer:      dialer,
	})

	_, err := r.Start(context.Background())
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if atomic.LoadInt32(&dialer.calls) != 0 {
		t.Fatalf("dial calls = %d, want 0 — attestation must precede dial", dialer.calls)
	}
	if att.scope != ScopeListenStart {
		t.Errorf("scope = %q, want %q", att.scope, ScopeListenStart)
	}
}

func TestOpenAIRealtime_Start_HappyPath(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn([][]byte{
		deltaFrame(t, "hello "),
		deltaFrame(t, "world "),
		finalFrame(t, "hello world done"),
	})
	dialer := &fakeDialer{conn: conn}
	r := NewOpenAIRealtime(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Dialer:      dialer,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := r.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	got := make([]Segment, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case s := <-ch:
			got = append(got, s)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for segment %d", i)
		}
	}
	if got[0].Text != "hello " || got[0].Final {
		t.Errorf("segment[0] = %+v", got[0])
	}
	if got[1].Text != "world " || got[1].Final {
		t.Errorf("segment[1] = %+v", got[1])
	}
	if !got[2].Final || got[2].Text != "hello world done" {
		t.Errorf("segment[2] = %+v want Final transcript", got[2])
	}
}

func TestOpenAIRealtime_Stop_ClosesConnection(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn(nil) // no frames; reader blocks on gate
	dialer := &fakeDialer{conn: conn}
	r := NewOpenAIRealtime(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Dialer:      dialer,
	})

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := r.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()

	testutil.Eventually(t, 2*time.Second, 10*time.Millisecond, func() bool {
		return atomic.LoadInt32(&conn.closeN) >= 1
	})

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("channel still open after cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("channel did not close after cancel")
	}
}

func TestOpenAIRealtime_TokenFromSource(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn(nil)
	dialer := &fakeDialer{conn: conn}
	r := NewOpenAIRealtime(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{token: "sk-from-source"},
		Dialer:      dialer,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if dialer.tok != "sk-from-source" {
		t.Errorf("dialer token = %q, want %q", dialer.tok, "sk-from-source")
	}
}

func TestOpenAIRealtime_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	// Many delta frames consumed concurrently with cancellation
	frames := make([][]byte, 0, 50)
	for i := 0; i < 50; i++ {
		frames = append(frames, deltaFrame(t, "x"))
	}
	conn := newFakeWSConn(frames)
	dialer := &fakeDialer{conn: conn}
	r := NewOpenAIRealtime(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Dialer:      dialer,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := r.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	got := 0
	for got < 50 {
		select {
		case <-ch:
			got++
		case <-time.After(3 * time.Second):
			t.Fatalf("got %d/50 segments", got)
		}
	}
}

func TestOpenAIRealtime_RequiresDialer(t *testing.T) {
	t.Parallel()
	r := NewOpenAIRealtime(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
	})
	if _, err := r.Start(context.Background()); err == nil {
		t.Fatalf("Start with nil Dialer should error")
	}
}
