package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/ipc"
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

	// Map-decode first so missing keys are detectable — a typed struct
	// would silently zero-fill bool/int/string fields and miss deletions.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(frames[0].Payload, &m); err != nil {
		t.Fatalf("unmarshal payload map: %v", err)
	}
	for _, k := range []string{"clients", "conversation", "memory_stats", "pending_tts", "daemon_uptime_s", "last_error"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("diag.state: payload missing required field %q", k)
		}
	}
}
