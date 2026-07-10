package listener

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/testutil"
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

func failedFrame(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type": "conversation.item.input_audio_transcription.failed",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestOpenAIRealtimeDecoder_Start_AttestationDenied_NoDial(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{denied: true}
	dialer := &fakeDialer{conn: newFakeWSConn(nil)}
	r := NewOpenAIRealtimeDecoder(Config{
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

func TestOpenAIRealtimeDecoder_Start_HappyPath(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn([][]byte{
		deltaFrame(t, "hello "),
		deltaFrame(t, "world "),
		finalFrame(t, "hello world done"),
	})
	dialer := &fakeDialer{conn: conn}
	r := NewOpenAIRealtimeDecoder(Config{
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

func TestOpenAIRealtimeDecoder_Stop_ClosesConnection(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn(nil)
	dialer := &fakeDialer{conn: conn}
	r := NewOpenAIRealtimeDecoder(Config{
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

	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("channel did not close after cancel")
		}
	}
}

func TestOpenAIRealtimeDecoder_TokenFromSource(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn(nil)
	dialer := &fakeDialer{conn: conn}
	r := NewOpenAIRealtimeDecoder(Config{
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

func TestOpenAIRealtimeDecoder_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	frames := make([][]byte, 0, 50)
	for i := 0; i < 50; i++ {
		frames = append(frames, deltaFrame(t, "x"))
	}
	conn := newFakeWSConn(frames)
	dialer := &fakeDialer{conn: conn}
	r := NewOpenAIRealtimeDecoder(Config{
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

func TestOpenAIRealtimeDecoder_RequiresDialer(t *testing.T) {
	t.Parallel()
	r := NewOpenAIRealtimeDecoder(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
	})
	if _, err := r.Start(context.Background()); err == nil {
		t.Fatalf("Start with nil Dialer should error")
	}
}

// Review #1: validation must run BEFORE started flag flips.
func TestOpenAIRealtimeDecoder_NilConfig_DoesNotPoisonStartedFlag(t *testing.T) {
	t.Parallel()
	r := NewOpenAIRealtimeDecoder(Config{
		TokenSource: &fakeTokenSource{},
		Dialer:      &fakeDialer{conn: newFakeWSConn(nil)},
	})
	if _, err := r.Start(context.Background()); err == nil {
		t.Fatalf("expected error on broken config")
	}
	r.cfg.Attestor = &fakeAttestor{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := r.Start(ctx); err != nil {
		t.Fatalf("retry after repair failed: %v", err)
	}
}

// Review #2/#4: utterance duration semantics — segments share non-zero StartedAt.
func TestOpenAIRealtimeDecoder_UtteranceDurationCarried(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn([][]byte{
		deltaFrame(t, "hello "),
		deltaFrame(t, "world"),
		finalFrame(t, "hello world"),
	})
	r := NewOpenAIRealtimeDecoder(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Dialer:      &fakeDialer{conn: conn},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := r.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	segs := make([]Segment, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case s := <-ch:
			segs = append(segs, s)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout segment %d", i)
		}
	}

	for i, s := range segs {
		if s.StartedAt.IsZero() {
			t.Errorf("segment[%d].StartedAt is zero (lost utterance start)", i)
		}
		if s.FinishedAt.Before(s.StartedAt) {
			t.Errorf("segment[%d] FinishedAt %v < StartedAt %v", i, s.FinishedAt, s.StartedAt)
		}
	}
	if !segs[0].StartedAt.Equal(segs[1].StartedAt) || !segs[1].StartedAt.Equal(segs[2].StartedAt) {
		t.Errorf("utterance StartedAt drifted across segments: %v %v %v",
			segs[0].StartedAt, segs[1].StartedAt, segs[2].StartedAt)
	}
	if segs[2].FinishedAt.Equal(segs[2].StartedAt) {
		t.Errorf("Final StartedAt==FinishedAt — duration destroyed")
	}
}

// Review #3: SendAudio must panic until W14b uplink ships.
func TestOpenAIRealtimeDecoder_SendAudio_PanicsUntilW14b(t *testing.T) {
	t.Parallel()
	r := NewOpenAIRealtimeDecoder(Config{})
	defer func() {
		v := recover()
		if v == nil {
			t.Fatalf("SendAudio did not panic")
		}
		err, ok := v.(error)
		if !ok || !errors.Is(err, ErrUplinkNotShipped) {
			t.Fatalf("panic value = %v, want ErrUplinkNotShipped", v)
		}
	}()
	_ = r.SendAudio([]byte{1, 2, 3})
}

// Review #6: ws:// rejected, wss:// accepted.
func TestOpenAIRealtimeDecoder_RejectsInsecureScheme(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		url     string
		wantErr error
		wantOK  bool
	}{
		{"insecure_ws", "ws://api.openai.com/v1/realtime", ErrInsecureScheme, false},
		{"http_scheme", "http://api.openai.com/v1/realtime", ErrInsecureScheme, false},
		{"secure_wss", "wss://api.openai.com/v1/realtime", nil, true},
		{"empty_uses_default", "", nil, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			conn := newFakeWSConn(nil)
			r := NewOpenAIRealtimeDecoder(Config{
				Attestor:    &fakeAttestor{},
				TokenSource: &fakeTokenSource{},
				Dialer:      &fakeDialer{conn: conn},
				URL:         tc.url,
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := r.Start(ctx)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("Start(%q) = %v, want ok", tc.url, err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Start(%q) err = %v, want %v", tc.url, err, tc.wantErr)
			}
		})
	}
}

// Review #7: spec §11 secret-shape redaction at the listener layer.
func TestOpenAIRealtimeDecoder_RedactsSecretShape(t *testing.T) {
	t.Parallel()
	secret := "sk-abcdefghijklmnopqrstuvwxyz0123"
	conn := newFakeWSConn([][]byte{
		finalFrame(t, "my key is "+secret),
	})
	r := NewOpenAIRealtimeDecoder(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Dialer:      &fakeDialer{conn: conn},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := r.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case s := <-ch:
		if strings.Contains(s.Text, secret) {
			t.Fatalf("secret leaked: %q", s.Text)
		}
		if s.Text != "[REDACTED]" {
			t.Fatalf("expected [REDACTED], got %q", s.Text)
		}
		if !s.Final {
			t.Fatalf("redacted segment must still be Final to unblock session")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
}

// Review #7: secret-in-delta dropped; only the scrubbed Final reaches the loop.
func TestOpenAIRealtimeDecoder_RedactsSecretInDelta(t *testing.T) {
	t.Parallel()
	secret := "ghp_abcdefghijklmnopqrstuvwxyz0123"
	conn := newFakeWSConn([][]byte{
		deltaFrame(t, "leah remember "+secret),
		finalFrame(t, "leah remember [SCRUBBED]"),
	})
	r := NewOpenAIRealtimeDecoder(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Dialer:      &fakeDialer{conn: conn},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := r.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case s := <-ch:
		if !s.Final {
			t.Fatalf("expected Final first (delta with secret must be dropped), got partial: %q", s.Text)
		}
		if strings.Contains(s.Text, secret) {
			t.Fatalf("secret leaked: %q", s.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
}

// Review #10: …transcription.failed emits synthetic Final + counter bump.
func TestOpenAIRealtimeDecoder_FailedEvent_EmitsSyntheticFinal(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn([][]byte{
		deltaFrame(t, "hello "),
		failedFrame(t),
	})
	r := NewOpenAIRealtimeDecoder(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Dialer:      &fakeDialer{conn: conn},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := r.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case s := <-ch:
		if s.Final {
			t.Fatalf("expected partial first")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout delta")
	}
	select {
	case s := <-ch:
		if !s.Final {
			t.Fatalf("expected synthetic Final after .failed, got %+v", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout synthetic Final")
	}
	if r.Metrics().FailedEvents() < 1 {
		t.Fatalf("FailedEvents = %d, want >=1", r.Metrics().FailedEvents())
	}
}

// Review #12: ReadMessage error emits synthetic Final before channel close.
func TestOpenAIRealtimeDecoder_ReadError_EmitsSyntheticFinal(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn([][]byte{
		deltaFrame(t, "in progress "),
	})
	r := NewOpenAIRealtimeDecoder(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Dialer:      &fakeDialer{conn: conn},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := r.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout delta")
	}
	_ = conn.Close()

	deadline := time.After(2 * time.Second)
	sawFinal := false
	for {
		select {
		case s, ok := <-ch:
			if !ok {
				if !sawFinal {
					t.Fatalf("channel closed without synthetic Final")
				}
				return
			}
			if s.Final {
				sawFinal = true
			}
		case <-deadline:
			t.Fatalf("timeout waiting for synthetic Final on read error")
		}
	}
}

// Review #11: unknown/unmarshal frames bump UnknownFrames counter.
func TestOpenAIRealtimeDecoder_UnknownFrames_Counted(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn([][]byte{
		[]byte("not json"),
		[]byte(`{"type":"session.created"}`),
		finalFrame(t, "done"),
	})
	r := NewOpenAIRealtimeDecoder(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Dialer:      &fakeDialer{conn: conn},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := r.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout final")
	}
	if got := r.Metrics().UnknownFrames(); got < 2 {
		t.Fatalf("UnknownFrames = %d, want >=2", got)
	}
}

// Review #9: started flag must clear on readLoop exit so Start can be called again.
func TestOpenAIRealtimeDecoder_StartedFlag_ResetOnExit(t *testing.T) {
	t.Parallel()
	conn := newFakeWSConn(nil)
	dialer := &fakeDialer{conn: conn}
	r := NewOpenAIRealtimeDecoder(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		Dialer:      dialer,
	})
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := r.Start(ctx)
	if err != nil {
		t.Fatalf("Start 1: %v", err)
	}
	cancel()
	deadline := time.After(2 * time.Second)
DRAIN:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break DRAIN
			}
		case <-deadline:
			t.Fatalf("first session did not close")
		}
	}
	testutil.Eventually(t, time.Second, 10*time.Millisecond, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return !r.started
	})

	dialer.conn = newFakeWSConn(nil)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	if _, err := r.Start(ctx2); err != nil {
		t.Fatalf("Start 2 (after first session exit): %v", err)
	}
}
