package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func dialReady(t *testing.T, sock string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c, err := net.Dial("unix", sock); err == nil {
			return c
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("socket never appeared: %s", sock)
	return nil
}

func TestServerEcho(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "leah.sock")
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
	defer conn.Close()
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
	dir := t.TempDir()
	sock := filepath.Join(dir, "leah.sock")
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
	defer conn.Close()
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
	dir := t.TempDir()
	sock := filepath.Join(dir, "leah.sock")
	handler := func(ctx context.Context, req Frame) (<-chan Frame, error) {
		return nil, errors.New("boom")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = NewServer(sock, handler).Serve(ctx) }()
	conn := dialReady(t, sock)
	defer conn.Close()
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
