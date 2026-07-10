package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/testutil"
)

// shortSock returns a path under /tmp (not t.TempDir) because macOS
// sun_path is capped at 104 bytes and t.TempDir's per-test prefix
// blows the budget for the longer test names below.
func shortSock(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "ipc")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return filepath.Join(d, "s")
}

func dialReady(t *testing.T, sock string) net.Conn {
	t.Helper()
	var c net.Conn
	testutil.Eventually(t, 2*time.Second, 20*time.Millisecond, func() bool {
		var err error
		c, err = net.Dial("unix", sock)
		return err == nil
	})
	return c
}

func TestServerEcho(t *testing.T) {
	sock := shortSock(t)
	handler := func(ctx context.Context, req Frame) (<-chan Frame, error) {
		out := make(chan Frame, 1)
		out <- Frame{Kind: "prose.delta", TurnID: req.TurnID, Seq: 1, Payload: json.RawMessage(`{"text":"echo"}`)}
		close(out)
		return out, nil
	}
	s := NewServer(sock, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Serve(ctx) }()
	conn := dialReady(t, sock)
	defer func() { _ = conn.Close() }()
	if err := WriteFrame(conn, Frame{Kind: "ask", TurnID: "t1", Seq: 0, Payload: json.RawMessage(`{"text":"hi"}`)}); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp, err := ReadFrame(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.Kind != "prose.delta" || resp.TurnID != "t1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

// Handler returning a nil channel must not deadlock the per-conn goroutine.
// First request returns (nil, nil); second must still be read and answered —
// without the nil guard the server blocks on `range nil` forever.
func TestServerNilChan(t *testing.T) {
	sock := shortSock(t)
	calls := 0
	handler := func(ctx context.Context, req Frame) (<-chan Frame, error) {
		calls++
		if calls == 1 {
			return nil, nil
		}
		out := make(chan Frame, 1)
		out <- Frame{Kind: "prose.delta", TurnID: req.TurnID, Seq: 1}
		close(out)
		return out, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = NewServer(sock, handler).Serve(ctx) }()
	conn := dialReady(t, sock)
	defer func() { _ = conn.Close() }()
	if err := WriteFrame(conn, Frame{Kind: "ask", TurnID: "t1"}); err != nil {
		t.Fatalf("write1: %v", err)
	}
	if err := WriteFrame(conn, Frame{Kind: "ask", TurnID: "t2"}); err != nil {
		t.Fatalf("write2: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("deadline: %v", err)
	}
	resp, err := ReadFrame(conn)
	if err != nil {
		t.Fatalf("read (server likely deadlocked on nil channel): %v", err)
	}
	if resp.TurnID != "t2" {
		t.Fatalf("expected response to t2, got %+v", resp)
	}
}

// Handler error must surface as an error frame; subsequent peer reads
// still work (or, on write failure, the conn closes cleanly).
func TestServerHandlerErr(t *testing.T) {
	sock := shortSock(t)
	handler := func(ctx context.Context, req Frame) (<-chan Frame, error) {
		return nil, errors.New("boom")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = NewServer(sock, handler).Serve(ctx) }()
	conn := dialReady(t, sock)
	defer func() { _ = conn.Close() }()
	if err := WriteFrame(conn, Frame{Kind: "ask", TurnID: "t1"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp, err := ReadFrame(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.Kind != "error" || resp.TurnID != "t1" {
		t.Fatalf("expected error frame, got %+v", resp)
	}
}

// Stop() must unblock Serve()'s accept loop without leaking the
// ctx-watcher goroutine — a Background context never cancels, so the
// shutdown path itself must release that watcher.
func TestServer_Shutdown_StopsAcceptLoop_GracefulClose(t *testing.T) {
	sock := shortSock(t)
	handler := func(ctx context.Context, req Frame) (<-chan Frame, error) { return nil, nil }
	s := NewServer(sock, handler)
	pre := runtime.NumGoroutine()
	done := make(chan error, 1)
	go func() { done <- s.Serve(context.Background()) }()
	_ = dialReady(t, sock).Close()
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned error on graceful close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after Stop()")
	}
	testutil.Eventually(t, 1*time.Second, 20*time.Millisecond, func() bool {
		return runtime.NumGoroutine() <= pre+1
	})
}

// flakyListener returns EMFILE on the first Accept then defers to a real
// listener. Proves the accept loop survives transient FD exhaustion
// instead of crashing the daemon.
type flakyListener struct {
	real   net.Listener
	failed atomic.Bool
}

func (f *flakyListener) Accept() (net.Conn, error) {
	if f.failed.CompareAndSwap(false, true) {
		return nil, &net.OpError{Op: "accept", Net: "unix", Err: syscall.EMFILE}
	}
	return f.real.Accept()
}
func (f *flakyListener) Close() error   { return f.real.Close() }
func (f *flakyListener) Addr() net.Addr { return f.real.Addr() }

func TestServer_AcceptLoop_RetriesOnTransientFDExhaustion(t *testing.T) {
	sock := shortSock(t)
	real, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	fl := &flakyListener{real: real}
	var got sync.WaitGroup
	got.Add(1)
	handler := func(ctx context.Context, req Frame) (<-chan Frame, error) {
		got.Done()
		out := make(chan Frame, 1)
		out <- Frame{Kind: "prose.delta", TurnID: req.TurnID, Seq: 1}
		close(out)
		return out, nil
	}
	s := NewServer(sock, handler)
	done := make(chan error, 1)
	go func() { done <- s.serveListener(context.Background(), fl) }()
	conn := dialReady(t, sock)
	defer func() { _ = conn.Close() }()
	if err := WriteFrame(conn, Frame{Kind: "ask", TurnID: "t1"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	resp, err := ReadFrame(conn)
	if err != nil {
		t.Fatalf("read (loop likely died on EMFILE): %v", err)
	}
	if resp.TurnID != "t1" {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	if !fl.failed.Load() {
		t.Fatal("flakyListener never injected EMFILE")
	}
	got.Wait()
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	<-done
}

// permListener returns a non-temporary error forever. The accept loop
// must break (return the error) rather than spin or retry indefinitely.
type permListener struct{ closed atomic.Bool }

var errPermanent = errors.New("permanent accept failure")

func (p *permListener) Accept() (net.Conn, error) {
	if p.closed.Load() {
		return nil, net.ErrClosed
	}
	return nil, errPermanent
}
func (p *permListener) Close() error   { p.closed.Store(true); return nil }
func (p *permListener) Addr() net.Addr { return &net.UnixAddr{Name: "perm", Net: "unix"} }

func TestServer_AcceptLoop_BreaksOnPermanentError(t *testing.T) {
	s := NewServer("/tmp/unused.sock", func(ctx context.Context, req Frame) (<-chan Frame, error) { return nil, nil })
	done := make(chan error, 1)
	go func() { done <- s.serveListener(context.Background(), &permListener{}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error return on permanent failure, got nil")
		}
		if errors.Is(err, net.ErrClosed) {
			t.Fatalf("permanent error misclassified as graceful close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("accept loop did not break on permanent error (likely spinning)")
	}
}
