package discord

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/testutil"
)

// fakeConn replays a scripted sequence of gateway frames then returns ErrClosed.
type fakeConn struct {
	frames     [][]byte
	cursor     int
	readGate   chan struct{}
	mu         sync.Mutex
	closed     bool
	closeCalls int32
}

func newFakeConn(frames [][]byte) *fakeConn {
	return &fakeConn{frames: frames, readGate: make(chan struct{})}
}

func (c *fakeConn) ReadMessage() ([]byte, error) {
	c.mu.Lock()
	if c.cursor >= len(c.frames) {
		c.mu.Unlock()
		<-c.readGate
		return nil, errors.New("conn closed")
	}
	frame := c.frames[c.cursor]
	c.cursor++
	c.mu.Unlock()
	return frame, nil
}

func (c *fakeConn) consumed() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cursor
}

func (c *fakeConn) Close() error {
	atomic.AddInt32(&c.closeCalls, 1)
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.readGate)
	}
	c.mu.Unlock()
	return nil
}

type fakeDialer struct {
	calls int32
	conn  *fakeConn
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

func messageFrame(t *testing.T, channelID, guildID, authorID, body string) []byte {
	t.Helper()
	frame := map[string]any{
		"op": 0,
		"t":  "MESSAGE_CREATE",
		"d": map[string]any{
			"channel_id": channelID,
			"guild_id":   guildID,
			"content":    body,
			"author":     map[string]any{"id": authorID},
			"timestamp":  "2026-06-10T12:00:00Z",
		},
	}
	b, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func newSubAdapter(t *testing.T, dialer WebSocketDialer, allow []string) (*Adapter, *fakeAttestor, *fakeTokenSource, *fakeAudit) {
	t.Helper()
	att := &fakeAttestor{}
	ts := &fakeTokenSource{}
	audit := &fakeAudit{}
	a, err := New(Config{
		Attestor:        att,
		TokenSource:     ts,
		WebSocketDialer: dialer,
		GuildAllowlist:  allow,
		Audit:           audit,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, att, ts, audit
}

// TestSubscribe_HappyPath: three gateway frames in the allowlisted guild
// route to the handler with channel/author/body intact.
func TestSubscribe_HappyPath(t *testing.T) {
	t.Parallel()
	frames := [][]byte{
		messageFrame(t, "chan-1", "guild-A", "user-1", "hello"),
		messageFrame(t, "chan-1", "guild-A", "user-2", "world"),
		messageFrame(t, "chan-1", "guild-A", "user-3", "again"),
	}
	conn := newFakeConn(frames)
	dialer := &fakeDialer{conn: conn}
	a, att, ts, _ := newSubAdapter(t, dialer, []string{"guild-A"})

	var got []Message
	var mu sync.Mutex
	handler := func(m Message) {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Subscribe(ctx, []string{"chan-1"}, handler); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 3
	})
	if att.lastScope != ScopeSubscribe {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeSubscribe)
	}
	if ts.called != 1 {
		t.Fatalf("token loaded %d times, want 1", ts.called)
	}
	if dialer.tok != "bot-token-xyz" {
		t.Fatalf("dialer token = %q, want bot-token-xyz", dialer.tok)
	}
	mu.Lock()
	defer mu.Unlock()
	if got[0].Body != "hello" || got[0].ChannelID != "chan-1" || got[0].AuthorID != "user-1" {
		t.Fatalf("first msg = %+v", got[0])
	}
}

// TestSubscribe_GuildAllowlistRejects: events from a non-allowlisted guild
// MUST NOT reach the handler; a counter row IS recorded with hashed guild.
func TestSubscribe_GuildAllowlistRejects(t *testing.T) {
	t.Parallel()
	frames := [][]byte{
		messageFrame(t, "chan-1", "guild-EVIL", "user-1", "leak"),
	}
	conn := newFakeConn(frames)
	dialer := &fakeDialer{conn: conn}
	a, _, _, audit := newSubAdapter(t, dialer, []string{"guild-A"})

	var calls int32
	handler := func(_ Message) { atomic.AddInt32(&calls, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Subscribe(ctx, nil, handler); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		for _, r := range audit.snapshot() {
			if r.Kind == "discord_inbound" && r.Reason == "guild_not_allowed" {
				return true
			}
		}
		return false
	})
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("handler called %d times on disallowed guild", got)
	}
	for _, r := range audit.snapshot() {
		blob, _ := json.Marshal(r)
		if string(blob) == "" {
			continue
		}
		if contains(string(blob), "guild-EVIL") {
			t.Fatalf("plaintext guild id leaked in audit row: %s", blob)
		}
	}
}

// TestSubscribe_AttestationDenied_NoDial: dialer MUST stay untouched on deny.
func TestSubscribe_AttestationDenied_NoDial(t *testing.T) {
	t.Parallel()
	dialer := &fakeDialer{conn: newFakeConn(nil)}
	att := &fakeAttestor{err: errors.New("denied")}
	ts := &fakeTokenSource{}
	a, err := New(Config{
		Attestor:        att,
		TokenSource:     ts,
		WebSocketDialer: dialer,
		GuildAllowlist:  []string{"guild-A"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = a.Subscribe(context.Background(), []string{"chan-1"}, func(_ Message) {})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if atomic.LoadInt32(&dialer.calls) != 0 {
		t.Fatalf("dialer invoked on attestation deny — token leak risk")
	}
	if ts.called != 0 {
		t.Fatalf("token loaded %d times on deny", ts.called)
	}
}

// TestSubscribe_HandlerSlowDrops: a blocked handler must not stall the
// gateway reader. After the queue fills, surplus frames are dropped and
// audit records the handler_slow_drop reason.
func TestSubscribe_HandlerSlowDrops(t *testing.T) {
	t.Parallel()

	total := inboundQueueDepth + 8
	frames := make([][]byte, total)
	for i := range frames {
		frames[i] = messageFrame(t, "chan-1", "guild-A", "u", "x")
	}
	conn := newFakeConn(frames)
	dialer := &fakeDialer{conn: conn}
	a, _, _, audit := newSubAdapter(t, dialer, []string{"guild-A"})

	block := make(chan struct{})
	release := make(chan struct{})
	var started int32
	handler := func(_ Message) {
		if atomic.CompareAndSwapInt32(&started, 0, 1) {
			close(block)
		}
		<-release
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Subscribe(ctx, []string{"chan-1"}, handler); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	<-block

	// All frames are consumed by the read loop even though only one handler
	// invocation has progressed — the rest must overflow the queue and drop.
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		return conn.consumed() == total
	})

	// Some frames MUST have been dropped, and each drop MUST audit.
	drops := 0
	for _, r := range audit.snapshot() {
		if r.Reason == "handler_slow_drop" {
			drops++
		}
	}
	if drops == 0 {
		t.Fatalf("no handler_slow_drop audit rows; backpressure stalled the gateway")
	}
	close(release)
}

// TestSubscribe_CtxCancel_ClosesConnection: cancel MUST close the gateway.
func TestSubscribe_CtxCancel_ClosesConnection(t *testing.T) {
	t.Parallel()
	conn := newFakeConn(nil)
	dialer := &fakeDialer{conn: conn}
	a, _, _, _ := newSubAdapter(t, dialer, []string{"guild-A"})

	ctx, cancel := context.WithCancel(context.Background())
	if err := a.Subscribe(ctx, nil, func(_ Message) {}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		return atomic.LoadInt32(&conn.closeCalls) > 0
	})
}

// TestSubscribe_NilHandler: nil handler rejected before any side effects.
func TestSubscribe_NilHandler(t *testing.T) {
	t.Parallel()
	dialer := &fakeDialer{conn: newFakeConn(nil)}
	a, _, _, _ := newSubAdapter(t, dialer, []string{"guild-A"})
	if err := a.Subscribe(context.Background(), nil, nil); err == nil {
		t.Fatal("nil handler MUST be rejected")
	}
	if atomic.LoadInt32(&dialer.calls) != 0 {
		t.Fatalf("dialer invoked despite nil handler")
	}
}

// TestSubscribe_RequiresDialer: New constructs without a dialer (other RPCs
// still work) but Subscribe MUST fail with a descriptive error.
func TestSubscribe_RequiresDialer(t *testing.T) {
	t.Parallel()
	a, err := New(Config{
		Attestor:       &fakeAttestor{},
		TokenSource:    &fakeTokenSource{},
		GuildAllowlist: []string{"g"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.Subscribe(context.Background(), nil, func(_ Message) {}); err == nil {
		t.Fatal("Subscribe without dialer MUST fail")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
