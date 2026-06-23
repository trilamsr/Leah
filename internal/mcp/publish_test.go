package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newPublishTest(t *testing.T) (sock string, cancel context.CancelFunc, errCh <-chan error) {
	t.Helper()
	dir := t.TempDir()
	sock = filepath.Join(dir, "leah-mcp.sock")
	ctx, c := context.WithCancel(context.Background())
	ch := make(chan error, 1)
	go func() { ch <- ServePublish(ctx, sock) }()
	waitForSocket(t, sock)
	return sock, c, ch
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			c, err := net.Dial("unix", path)
			if err == nil {
				_ = c.Close()
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("socket %s did not become ready", path)
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func roundTrip(t *testing.T, sock string, req rpcRequest) rpcResponse {
	t.Helper()
	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	enc := json.NewEncoder(c)
	if err := enc.Encode(req); err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec := json.NewDecoder(bufio.NewReader(c))
	var resp rpcResponse
	if err := dec.Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func TestPublish_GateOffByDefault(t *testing.T) {
	t.Setenv("LEAH_MCP_PUBLISH", "")
	dir := t.TempDir()
	sock := filepath.Join(dir, "leah-mcp.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := ServePublish(ctx, sock)
	if err != ErrPublishDisabled {
		t.Fatalf("want ErrPublishDisabled, got %v", err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket file should not exist when gate off: %v", err)
	}
}

func TestPublish_InitializeRoundTrip(t *testing.T) {
	t.Setenv("LEAH_MCP_PUBLISH", "1")
	sock, cancel, errCh := newPublishTest(t)
	defer func() {
		cancel()
		<-errCh
	}()
	resp := roundTrip(t, sock, rpcRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"})
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	var got struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ProtocolVersion == "" {
		t.Fatalf("missing protocolVersion: %s", resp.Result)
	}
	if got.ServerInfo.Name != "leah" {
		t.Fatalf("server name = %q, want leah", got.ServerInfo.Name)
	}
}

func TestPublish_ToolsListReturnsThreeEntries(t *testing.T) {
	t.Setenv("LEAH_MCP_PUBLISH", "1")
	sock, cancel, errCh := newPublishTest(t)
	defer func() {
		cancel()
		<-errCh
	}()
	resp := roundTrip(t, sock, rpcRequest{JSONRPC: "2.0", ID: 2, Method: "tools/list"})
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}
	var got struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Tools) != 3 {
		t.Fatalf("tools len = %d, want 3 (%+v)", len(got.Tools), got.Tools)
	}
	want := map[string]bool{"leah.search": false, "leah.recall": false, "leah.brief.summary": false}
	for _, tl := range got.Tools {
		if _, ok := want[tl.Name]; !ok {
			t.Fatalf("unexpected tool %q", tl.Name)
		}
		want[tl.Name] = true
		if tl.Description == "" {
			t.Fatalf("tool %q missing description", tl.Name)
		}
	}
	for k, v := range want {
		if !v {
			t.Fatalf("missing tool %q", k)
		}
	}
}

func TestPublish_ToolsCallReturns501(t *testing.T) {
	t.Setenv("LEAH_MCP_PUBLISH", "1")
	sock, cancel, errCh := newPublishTest(t)
	defer func() {
		cancel()
		<-errCh
	}()
	resp := roundTrip(t, sock, rpcRequest{
		JSONRPC: "2.0", ID: 3, Method: "tools/call",
		Params: map[string]any{"name": "leah.search", "arguments": map[string]any{"q": "hi"}},
	})
	if resp.Error == nil {
		t.Fatalf("want JSON-RPC error, got result %s", resp.Result)
	}
	if resp.Error.Code != codeNotImplemented {
		t.Fatalf("error code = %d, want %d", resp.Error.Code, codeNotImplemented)
	}
}

func TestPublish_CtxCancelRemovesSocket(t *testing.T) {
	t.Setenv("LEAH_MCP_PUBLISH", "1")
	sock, cancel, errCh := newPublishTest(t)
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("socket should exist before cancel: %v", err)
	}
	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("serve returned unexpected error: %v", err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket file should be removed after ctx cancel, stat err = %v", err)
	}
}

func TestPublish_UnknownMethod(t *testing.T) {
	t.Setenv("LEAH_MCP_PUBLISH", "1")
	sock, cancel, errCh := newPublishTest(t)
	defer func() {
		cancel()
		<-errCh
	}()
	resp := roundTrip(t, sock, rpcRequest{JSONRPC: "2.0", ID: 9, Method: "nope/nope"})
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("want method-not-found, got %+v / %s", resp.Error, resp.Result)
	}
}
