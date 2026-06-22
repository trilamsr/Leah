package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/trilam/leah/internal/ipc"
)

// TestIPCDiagStateResponds asserts that diag.state returns exactly one
// diag.state.response frame with all six expected payload fields populated.
func TestIPCDiagStateResponds(t *testing.T) {
	db := newTestTurnDB(t)
	noop := func(_ context.Context, _ string) error { return nil }
	h := newIPCHandlerWithDiag(db, nil, noop, time.Now())

	in := ipc.Frame{Kind: "diag.state", TurnID: "d1"}
	out, err := h(context.Background(), in)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	var frames []ipc.Frame
	for f := range out {
		frames = append(frames, f)
	}
	if len(frames) != 1 {
		t.Fatalf("diag.state: want 1 frame, got %d", len(frames))
	}
	if frames[0].Kind != "diag.state.response" {
		t.Fatalf("diag.state: got kind %q, want %q", frames[0].Kind, "diag.state.response")
	}

	var p struct {
		Clients       []string               `json:"clients"`
		Conversation  map[string]interface{} `json:"conversation"`
		MemoryStats   map[string]interface{} `json:"memory_stats"`
		PendingTTS    bool                   `json:"pending_tts"`
		DaemonUptimeS int64                  `json:"daemon_uptime_s"`
		LastError     string                 `json:"last_error"`
	}
	if err := json.Unmarshal(frames[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.Clients == nil {
		t.Fatal("diag.state: clients must not be nil")
	}
	if p.Conversation == nil {
		t.Fatal("diag.state: conversation must not be nil")
	}
	if p.MemoryStats == nil {
		t.Fatal("diag.state: memory_stats must not be nil")
	}
}
