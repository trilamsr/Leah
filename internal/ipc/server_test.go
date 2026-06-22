package ipc

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

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
	// wait for socket to appear
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := net.Dial("unix", sock); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
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
