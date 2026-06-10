package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

const toolsToken = "secret-token"

func newToolsServer(t *testing.T) (*Server, *Tools, *audit.Logger, string) {
	t.Helper()
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	if err := os.MkdirAll(memDir, 0o700); err != nil {
		t.Fatal(err)
	}
	logger := &audit.Logger{Path: filepath.Join(dir, "audit.jsonl")}
	s := NewServer("127.0.0.1:0", toolsToken, logger.Path, logger)
	s.Now = time.Now
	tools := &Tools{Server: s, MemoryDir: memDir, AuditPath: logger.Path, Now: time.Now}
	tools.Register()
	return s, tools, logger, dir
}

func startToolsListener(t *testing.T, s *Server) func() {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.Addr = ln.Addr().String()
	_ = ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Serve(ctx) }()
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		c, err := net.DialTimeout("tcp", s.Addr, 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	})
	return cancel
}

func toolPost(t *testing.T, addr, path string, body any) (*http.Response, []byte) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "http://"+addr+path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+toolsToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, out
}

func TestGetMemoryRule_ReturnsMarkdown(t *testing.T) {
	s, _, _, dir := newToolsServer(t)
	mdBody := "# rule body\n\nhello world"
	if err := os.WriteFile(filepath.Join(dir, "memory", "myrule.md"), []byte(mdBody), 0o600); err != nil {
		t.Fatal(err)
	}
	cancel := startToolsListener(t, s)
	defer cancel()

	resp, body := toolPost(t, s.Addr, "/tools/leah_get_memory_rule", map[string]string{"name": "myrule"})
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d body=%s", resp.StatusCode, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["body_md"] != mdBody {
		t.Fatalf("body_md mismatch: %q", got["body_md"])
	}
}

func TestGetMemoryRule_PathTraversalRejected(t *testing.T) {
	s, _, _, _ := newToolsServer(t)
	cancel := startToolsListener(t, s)
	defer cancel()
	for _, bad := range []string{"../etc/passwd", "foo/bar", "/abs", ".hidden", ""} {
		resp, _ := toolPost(t, s.Addr, "/tools/leah_get_memory_rule", map[string]string{"name": bad})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("name=%q want 400, got %d", bad, resp.StatusCode)
		}
	}
}

func TestGetMemoryRule_UnknownReturns404(t *testing.T) {
	s, _, _, _ := newToolsServer(t)
	cancel := startToolsListener(t, s)
	defer cancel()
	resp, _ := toolPost(t, s.Addr, "/tools/leah_get_memory_rule", map[string]string{"name": "nosuch"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}

func TestSearchAudit_RedactDropDropsAndAudits(t *testing.T) {
	s, _, logger, _ := newToolsServer(t)
	if err := logger.Append(audit.Entry{Kind: "ship", Outcome: "ok", Detail: "user email is alice@example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Append(audit.Entry{Kind: "ship", Outcome: "ok", Detail: "clean row"}); err != nil {
		t.Fatal(err)
	}
	cancel := startToolsListener(t, s)
	defer cancel()

	resp, body := toolPost(t, s.Addr, "/tools/leah_search_audit", map[string]string{"query": "ship"})
	if resp.StatusCode != 200 {
		t.Fatalf("want 200, got %d body=%s", resp.StatusCode, body)
	}
	var got searchAuditOut
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("want 1 row (PII dropped), got %d", len(got.Rows))
	}
	if strings.Contains(got.Rows[0].Detail, "@example.com") {
		t.Fatalf("PII leaked: %q", got.Rows[0].Detail)
	}
	raw, _ := os.ReadFile(logger.Path)
	if !strings.Contains(string(raw), "mcp_redact_drop") {
		t.Fatalf("want mcp_redact_drop audit row, got %s", raw)
	}
}

func TestSearchAudit_InvalidSinceRejected(t *testing.T) {
	// Y4: malformed `since` returns 400 + mcp_request_invalid audit row
	// (previously swallowed silently → undefined query semantics).
	s, _, logger, _ := newToolsServer(t)
	cancel := startToolsListener(t, s)
	defer cancel()

	resp, _ := toolPost(t, s.Addr, "/tools/leah_search_audit", map[string]string{"since": "not-a-timestamp"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	raw, _ := os.ReadFile(logger.Path)
	if !strings.Contains(string(raw), `"kind":"mcp_request_invalid"`) {
		t.Fatalf("want mcp_request_invalid audit row, got %s", raw)
	}
	if !strings.Contains(string(raw), "field=since") {
		t.Fatalf("want field=since detail, got %s", raw)
	}
}

func TestSearchAudit_TruncationFlag(t *testing.T) {
	_, tools, logger, _ := newToolsServer(t)
	for i := 0; i < maxSearchRows+5; i++ {
		if err := logger.Append(audit.Entry{Kind: "ship", Outcome: "ok", Detail: "clean row"}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := tools.searchAudit("ship", "")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Truncated {
		t.Fatal("want truncated=true")
	}
	if len(out.Rows) != maxSearchRows {
		t.Fatalf("want %d rows, got %d", maxSearchRows, len(out.Rows))
	}
}

func TestDispatchStatus_InFlightAndCompleted(t *testing.T) {
	_, tools, logger, _ := newToolsServer(t)
	for _, e := range []audit.Entry{
		{Kind: "ship", Outcome: "ok", ArgsHash: "a1"},
		{Kind: "ask", Outcome: "ok", ArgsHash: "a2"},
		{Kind: "self-build", Outcome: "dispatched", ArgsHash: "a3"},
		{Kind: "patterns", Outcome: "ok", ArgsHash: "a4"},
	} {
		if err := logger.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	out, err := tools.dispatchStatus()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Recent) != 3 {
		t.Fatalf("want 3 dispatcher rows, got %d", len(out.Recent))
	}
	if len(out.InFlight) != 1 || out.InFlight[0].Kind != "self-build" {
		t.Fatalf("want 1 in-flight self-build row, got %+v", out.InFlight)
	}
}

func TestSelfBuildStatus_DanglingIncluded(t *testing.T) {
	dir := t.TempDir()
	logger := &audit.Logger{Path: filepath.Join(dir, "audit.jsonl"), Now: func() time.Time { return time.Now().Add(-30 * 24 * time.Hour) }}
	if err := logger.Append(audit.Entry{Kind: "self-build", Outcome: "dispatched", Detail: "url=https://github.com/x/y/issues/1"}); err != nil {
		t.Fatal(err)
	}
	tools := &Tools{AuditPath: logger.Path, Now: time.Now}
	out, err := tools.selfBuildStatus()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Dangling) != 1 {
		t.Fatalf("want 1 dangling, got %d", len(out.Dangling))
	}
}
