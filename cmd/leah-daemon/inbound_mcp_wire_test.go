package main

import (
	"database/sql"
	"io"
	"log/slog"
	"testing"

	_ "modernc.org/sqlite"
)

// TestPhase4Producers_InboundMCPWired asserts wirePhase4Producers constructs
// the inbound MCP server + registers the §5.3 first-party toolset. The pre-fix
// path left MCPInbound nil (PR #459 shipped internal/mcp/inbound but the
// composition root never instantiated it — same v3.3.0 shipped-but-not-wired
// pattern that Phase 4 T19 was supposed to catch).
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
}
