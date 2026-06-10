package obs

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/testutil"
)

// fakeSubscriber feeds the handler a controllable event stream so the SSE
// transport can be exercised without the W75 SQLite EventStore.
type fakeSubscriber struct {
	mu        sync.Mutex
	ch        chan Event
	closed    bool
	closeCalls int
	kinds     []string
}

func newFakeSubscriber(kinds []string) *fakeSubscriber {
	return &fakeSubscriber{ch: make(chan Event, 16), kinds: kinds}
}

func (f *fakeSubscriber) Events() <-chan Event { return f.ch }

func (f *fakeSubscriber) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls++
	if !f.closed {
		close(f.ch)
		f.closed = true
	}
	return nil
}

func (f *fakeSubscriber) send(e Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.ch <- e
	}
}

func newSSETestServer(t *testing.T, h *SSEHandler) (*httptest.Server, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	return srv, func() { srv.Close() }
}

// TestSSE_StreamsEventsAsTheyArrive: events sent into the subscriber surface
// as SSE frames on the wire in order with kind + data payload.
func TestSSE_StreamsEventsAsTheyArrive(t *testing.T) {
	sub := newFakeSubscriber(nil)
	h := &SSEHandler{
		Subscribe: func(ctx context.Context, kinds []string) (SSESubscriber, error) {
			return sub, nil
		},
	}
	srv, cleanup := newSSETestServer(t, h)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type: got %q, want text/event-stream", ct)
	}

	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	sub.send(Event{TS: now, Kind: "dispatch.ship", Actor: "daemon", Outcome: "ok"})
	sub.send(Event{TS: now.Add(time.Millisecond), Kind: "audit.append", Actor: "daemon", Outcome: "ok"})

	rd := bufio.NewReader(resp.Body)
	got := readSSEFrames(t, rd, 2, 2*time.Second)
	if len(got) < 2 {
		t.Fatalf("frames: got %d, want >=2 (%v)", len(got), got)
	}
	if !strings.HasPrefix(got[0], "event: dispatch.ship") {
		t.Errorf("frame[0]: %q", got[0])
	}
	if !strings.HasPrefix(got[1], "event: audit.append") {
		t.Errorf("frame[1]: %q", got[1])
	}
	if !strings.Contains(got[0], `"kind":"dispatch.ship"`) {
		t.Errorf("frame[0] missing kind payload: %q", got[0])
	}
}

// TestSSE_KeepAlivePingEvery15s: with zero events arriving, the handler
// emits a `: keepalive` SSE comment line on the configured cadence.
func TestSSE_KeepAlivePingEvery15s(t *testing.T) {
	sub := newFakeSubscriber(nil)
	h := &SSEHandler{
		Subscribe: func(ctx context.Context, kinds []string) (SSESubscriber, error) {
			return sub, nil
		},
		KeepAlive: 20 * time.Millisecond,
	}
	srv, cleanup := newSSETestServer(t, h)
	defer cleanup()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rd := bufio.NewReader(resp.Body)
	var pings int
	testutil.Eventually(t, 2*time.Second, 10*time.Millisecond, func() bool {
		line, err := rd.ReadString('\n')
		if err != nil {
			return false
		}
		if strings.HasPrefix(line, ": keepalive") {
			pings++
		}
		return pings >= 2
	})
	if pings < 2 {
		t.Fatalf("keepalive pings: got %d, want >=2", pings)
	}
}

// TestSSE_ClientDisconnect_StopsStream: closing the client connection
// triggers Close on the subscriber (no leaked subscription goroutine).
func TestSSE_ClientDisconnect_StopsStream(t *testing.T) {
	sub := newFakeSubscriber(nil)
	h := &SSEHandler{
		Subscribe: func(ctx context.Context, kinds []string) (SSESubscriber, error) {
			return sub, nil
		},
	}
	srv, cleanup := newSSETestServer(t, h)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	// Force the handler to enter its select loop.
	sub.send(Event{TS: time.Now(), Kind: "audit.append", Actor: "daemon", Outcome: "ok"})
	_ = readSSEFrames(t, bufio.NewReader(resp.Body), 1, time.Second)

	cancel()
	_ = resp.Body.Close()

	testutil.Eventually(t, time.Second, 10*time.Millisecond, func() bool {
		sub.mu.Lock()
		defer sub.mu.Unlock()
		return sub.closeCalls > 0
	})
}

// TestSSE_FilterByKind_OnlyMatching: ?kind=a,b passes those kinds to the
// subscribe function; the handler is transport-only and trusts the
// subscriber to do the filtering.
func TestSSE_FilterByKind_OnlyMatching(t *testing.T) {
	var gotKinds []string
	sub := newFakeSubscriber(nil)
	h := &SSEHandler{
		Subscribe: func(ctx context.Context, kinds []string) (SSESubscriber, error) {
			gotKinds = kinds
			return sub, nil
		},
	}
	srv, cleanup := newSSETestServer(t, h)
	defer cleanup()

	resp, err := http.Get(srv.URL + "?kind=dispatch.ship,audit.append")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain one frame so we know the handler ran past Subscribe.
	sub.send(Event{TS: time.Now(), Kind: "dispatch.ship", Actor: "daemon", Outcome: "ok"})
	_ = readSSEFrames(t, bufio.NewReader(resp.Body), 1, time.Second)

	want := []string{"dispatch.ship", "audit.append"}
	if len(gotKinds) != len(want) {
		t.Fatalf("kinds: got %v, want %v", gotKinds, want)
	}
	for i, w := range want {
		if gotKinds[i] != w {
			t.Errorf("kinds[%d]: got %q, want %q", i, gotKinds[i], w)
		}
	}
}

// readSSEFrames reads until n complete event frames (terminated by blank
// line) are observed or deadline expires. Keepalive comments are skipped.
func readSSEFrames(t *testing.T, rd *bufio.Reader, n int, deadline time.Duration) []string {
	t.Helper()
	frames := make([]string, 0, n)
	var cur strings.Builder
	end := time.Now().Add(deadline)
	for time.Now().Before(end) && len(frames) < n {
		line, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if line == "\n" {
			if cur.Len() > 0 {
				frames = append(frames, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteString(line)
	}
	return frames
}
