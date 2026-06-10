package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/testutil"
)

const testToken = "secret-token"

func newTestServer(t *testing.T) (*Server, *audit.Logger, string) {
	t.Helper()
	dir := t.TempDir()
	logger := &audit.Logger{Path: filepath.Join(dir, "audit.jsonl")}
	s := NewServer("127.0.0.1:0", testToken, logger.Path, logger)
	s.Now = time.Now
	s.Register("ping", func(_ []byte) (any, int, error) {
		return map[string]string{"pong": "ok"}, 200, nil
	})
	return s, logger, dir
}

func startListener(t *testing.T, s *Server) (cancel func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.Addr = ln.Addr().String()
	_ = ln.Close()
	ctx, c := context.WithCancel(context.Background())
	go func() { _ = s.Serve(ctx) }()
	waitListen(t, s.Addr)
	return c
}

func callPing(t *testing.T, addr, token string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]any{})
	req, _ := http.NewRequest("POST", "http://"+addr+"/tools/ping", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func TestMCPServer_RefusesNonLoopback(t *testing.T) {
	s, _, _ := newTestServer(t)
	s.Addr = "0.0.0.0:9876"
	err := s.Serve(context.Background())
	if !errors.Is(err, ErrNonLoopback) {
		t.Fatalf("want ErrNonLoopback, got %v", err)
	}
}

func TestMCPServer_KillSwitch_LEAH_MCP_SERVER_0(t *testing.T) {
	t.Setenv(envKillSwitch, "0")
	s, _, _ := newTestServer(t)
	s.Addr = "127.0.0.1:0"
	err := s.Serve(context.Background())
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("want ErrDisabled, got %v", err)
	}
}

func TestMCPServer_RejectsMissingBearer(t *testing.T) {
	s, _, _ := newTestServer(t)
	cancel := startListener(t, s)
	defer cancel()
	if got := callPing(t, s.Addr, ""); got != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", got)
	}
}

func TestMCPServer_BearerWrong_401_AuditRow(t *testing.T) {
	s, logger, _ := newTestServer(t)
	cancel := startListener(t, s)
	defer cancel()
	if got := callPing(t, s.Addr, "wrong-token"); got != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", got)
	}
	raw, _ := os.ReadFile(logger.Path)
	if !strings.Contains(string(raw), `"kind":"mcp_auth_fail"`) {
		t.Fatalf("want mcp_auth_fail audit row, got %s", raw)
	}
	if !strings.Contains(string(raw), `"outcome":"denied"`) {
		t.Fatalf("want outcome=denied, got %s", raw)
	}
}

func TestMCPServer_RateLimit_DropsBeyondCap(t *testing.T) {
	s, _, _ := newTestServer(t)
	s.RatePerMin = 1
	cancel := startListener(t, s)
	defer cancel()
	if got := callPing(t, s.Addr, testToken); got != 200 {
		t.Fatalf("call 1 want 200, got %d", got)
	}
	if got := callPing(t, s.Addr, testToken); got != http.StatusTooManyRequests {
		t.Fatalf("call 2 want 429, got %d", got)
	}
}

func TestMCPServer_AuditRowOnReadCall(t *testing.T) {
	s, logger, _ := newTestServer(t)
	cancel := startListener(t, s)
	defer cancel()
	if got := callPing(t, s.Addr, testToken); got != 200 {
		t.Fatalf("want 200, got %d", got)
	}
	raw, _ := os.ReadFile(logger.Path)
	if !strings.Contains(string(raw), `"kind":"mcp_call"`) {
		t.Fatalf("want mcp_call audit row, got %s", raw)
	}
	if !strings.Contains(string(raw), "tool=ping") {
		t.Fatalf("want tool name in audit detail, got %s", raw)
	}
}

func TestMCPServer_ServerStartAuditRow(t *testing.T) {
	s, logger, _ := newTestServer(t)
	cancel := startListener(t, s)
	defer cancel()
	testutil.Eventually(t, time.Second, 5*time.Millisecond, func() bool {
		raw, _ := os.ReadFile(logger.Path)
		return strings.Contains(string(raw), `"kind":"mcp_server_start"`) &&
			strings.Contains(string(raw), "addr=127.0.0.1:")
	})
}

func TestMCPServer_TokenRotate_RevokesOldBearer(t *testing.T) {
	s, logger, _ := newTestServer(t)
	cancel := startListener(t, s)
	defer cancel()
	if got := callPing(t, s.Addr, testToken); got != 200 {
		t.Fatalf("pre-rotate want 200, got %d", got)
	}
	s.SetToken("new-token")
	if got := callPing(t, s.Addr, testToken); got != http.StatusUnauthorized {
		t.Fatalf("old token want 401, got %d", got)
	}
	if got := callPing(t, s.Addr, "new-token"); got != 200 {
		t.Fatalf("new token want 200, got %d", got)
	}
	raw, _ := os.ReadFile(logger.Path)
	if !strings.Contains(string(raw), `"kind":"mcp_token_rotated"`) {
		t.Fatalf("want mcp_token_rotated audit row, got %s", raw)
	}
}

func TestMCPServer_PendingTaskCap_RejectsExcess(t *testing.T) {
	// Pending-task semaphore underpins W139 attestation queue (§2.3).
	// Per-peer cap = 2; 3rd reservation for the same peer returns capScope=peer.
	s, _, _ := newTestServer(t)
	s.PerPeerPendingCap = 2
	s.GlobalPendingCap = 5
	rel1, ok, _ := s.EnqueuePending("peerA")
	if !ok {
		t.Fatal("reservation 1 should succeed")
	}
	rel2, ok, _ := s.EnqueuePending("peerA")
	if !ok {
		t.Fatal("reservation 2 should succeed")
	}
	_, ok, scope := s.EnqueuePending("peerA")
	if ok {
		t.Fatal("reservation 3 should fail at per-peer cap")
	}
	if scope != "peer" {
		t.Fatalf("want capScope=peer, got %q", scope)
	}
	// release frees a slot:
	rel1()
	rel3, ok, _ := s.EnqueuePending("peerA")
	if !ok {
		t.Fatal("post-release reservation should succeed")
	}
	rel3()
	rel2()

	// Global cap: fresh server, fill 5 slots across distinct peers, 6th fails
	// with capScope=global (per-peer cap unchanged at 2 — each peer gets 1 here).
	s2, _, _ := newTestServer(t)
	s2.PerPeerPendingCap = 2
	s2.GlobalPendingCap = 5
	for i := 0; i < 5; i++ {
		if _, ok, _ := s2.EnqueuePending(string(rune('a' + i))); !ok {
			t.Fatalf("global fill %d should succeed", i)
		}
	}
	_, ok, scope = s2.EnqueuePending("x")
	if ok {
		t.Fatal("global 6th should fail")
	}
	if scope != "global" {
		t.Fatalf("want capScope=global, got %q", scope)
	}
}

func waitListen(t *testing.T, addr string) {
	t.Helper()
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	})
}
