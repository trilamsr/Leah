package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	mcpInbound "github.com/trilam/leah/internal/platform/mcp/inbound"
	_ "modernc.org/sqlite"
)

// TestPhase4Producers_InboundMCPWired guards against the shipped-but-not-wired
// class of regression: a package can land in internal/ but never get
// instantiated at the composition root. Asserts wirePhase4Producers both
// constructs the inbound MCP server AND registers the first-party toolset —
// duplicate-register probe catches the case where the server is constructed
// but RegisterFirstParty gets skipped (a nil-check alone would miss that).
func TestPhase4Producers_InboundMCPWired(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	p := wirePhase4Producers(db, io.Discard, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if p.MCPInbound == nil {
		t.Fatal("MCPInbound nil — inbound MCP server not wired at composition root")
	}
	if p.MCPInbound.Tokens() == nil {
		t.Fatal("MCPInbound.Tokens() nil — token store not attached")
	}
	err = p.MCPInbound.Register(mcpInbound.MCPTool{
		Name:    "leah.memory.search",
		Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil },
	})
	if !errors.Is(err, mcpInbound.ErrDuplicateTool) {
		t.Fatalf("first-party tools not registered; re-register leah.memory.search = %v, want ErrDuplicateTool", err)
	}
}
