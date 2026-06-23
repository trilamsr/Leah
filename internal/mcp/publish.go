package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// Outbound MCP publish surface (Phase 3 Wave 4 Task 14). Gated by
// LEAH_MCP_PUBLISH=1; Phase 4 wires tools/call into the daemon IPC. Until
// then, tools/call returns -32601-adjacent "not implemented" so peer agents
// can discover the surface without hitting unsafe execution paths.

const (
	envPublishGate     = "LEAH_MCP_PUBLISH"
	publishProtocol    = "2024-11-05"
	publishServerName  = "leah"
	publishServerVer   = "0.1.0"
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeNotImplemented = -32001
)

// ErrPublishDisabled — LEAH_MCP_PUBLISH not set to "1". Returned without
// binding the socket so peer agents see a closed door rather than a stub.
var ErrPublishDisabled = errors.New("mcp: publish disabled by LEAH_MCP_PUBLISH")

type publishTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

var publishedTools = []publishTool{
	{
		Name:        "leah.search",
		Description: "Search the operator's local knowledge store.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`),
	},
	{
		Name:        "leah.recall",
		Description: "Recall a previously remembered fact by topic.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"topic":{"type":"string"}},"required":["topic"]}`),
	},
	{
		Name:        "leah.brief.summary",
		Description: "Return the latest morning-brief summary.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	},
}

// DefaultPublishSocketPath resolves ~/Library/Caches/Leah/leah-mcp.sock.
// Falls back to $TMPDIR when HOME is unset (e.g. CI sandboxes).
func DefaultPublishSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "leah-mcp.sock")
	}
	return filepath.Join(home, "Library", "Caches", "Leah", "leah-mcp.sock")
}

// ServePublish binds the outbound MCP Unix socket. Blocks until ctx is
// cancelled or the listener errors. Returns ErrPublishDisabled when the
// LEAH_MCP_PUBLISH gate is off — no socket is created in that case.
func ServePublish(ctx context.Context, sockPath string) error {
	if os.Getenv(envPublishGate) != "1" {
		return ErrPublishDisabled
	}
	// 0o700 dir + 0o600 socket — multi-user mac box protection. Matches
	// internal/ipc/server.go convention; world-connectable MCP would
	// expose `tools/list` to any local user.
	if err := os.MkdirAll(filepath.Dir(sockPath), 0o700); err != nil {
		return err
	}
	// Stale socket from a prior crash blocks bind; remove only if it's a
	// socket file (don't clobber a regular file by accident).
	if info, err := os.Lstat(sockPath); err == nil && info.Mode()&os.ModeSocket != 0 {
		_ = os.Remove(sockPath)
	}
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(sockPath, 0o600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(sockPath)
	}()
	var wg sync.WaitGroup
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			wg.Wait()
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			servePublishConn(ctx, c)
		}(conn)
	}
}

func servePublishConn(ctx context.Context, c net.Conn) {
	defer func() { _ = c.Close() }()
	dec := json.NewDecoder(bufio.NewReader(c))
	enc := json.NewEncoder(c)
	for {
		if ctx.Err() != nil {
			return
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if !errors.Is(err, io.EOF) {
				writeParseError(enc)
			}
			return
		}
		resp := handlePublishRPC(raw)
		if err := enc.Encode(resp); err != nil {
			return
		}
	}
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcReply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func handlePublishRPC(raw json.RawMessage) rpcReply {
	var env rpcEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return rpcReply{JSONRPC: "2.0", Error: &rpcError{Code: codeParseError, Message: "parse error"}}
	}
	reply := rpcReply{JSONRPC: "2.0", ID: env.ID}
	switch env.Method {
	case "initialize":
		reply.Result = map[string]any{
			"protocolVersion": publishProtocol,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": publishServerName, "version": publishServerVer},
		}
	case "tools/list":
		reply.Result = map[string]any{"tools": publishedTools}
	case "tools/call":
		reply.Error = &rpcError{Code: codeNotImplemented, Message: "tools/call dispatch deferred to Phase 4"}
	default:
		reply.Error = &rpcError{Code: codeMethodNotFound, Message: "method not found: " + env.Method}
	}
	return reply
}

func writeParseError(enc *json.Encoder) {
	_ = enc.Encode(rpcReply{JSONRPC: "2.0", Error: &rpcError{Code: codeParseError, Message: "parse error"}})
}
