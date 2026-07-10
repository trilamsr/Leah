package ipc

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestDiagStateExposesLastError asserts that an error string passed to
// HandleState surfaces in the last_error payload field.
func TestDiagStateExposesLastError(t *testing.T) {
	const want = "disk: no space left"
	out, err := HandleState(context.Background(), time.Now(), want, "test-turn")
	if err != nil {
		t.Fatalf("HandleState: %v", err)
	}
	var frames []Frame
	for f := range out {
		frames = append(frames, f)
	}
	if len(frames) != 1 {
		t.Fatalf("want 1 frame, got %d", len(frames))
	}
	var p DiagStatePayload
	if err := json.Unmarshal(frames[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.LastError != want {
		t.Fatalf("last_error: got %q, want %q", p.LastError, want)
	}
}

// TestIPCDiagStateResponds asserts HandleState returns exactly one
// diag.state.response frame with all six payload fields present.
func TestIPCDiagStateResponds(t *testing.T) {
	out, err := HandleState(context.Background(), time.Now().Add(-5*time.Second), "", "t")
	if err != nil {
		t.Fatalf("HandleState: %v", err)
	}
	var frames []Frame
	for f := range out {
		frames = append(frames, f)
	}
	if len(frames) != 1 {
		t.Fatalf("want 1 frame, got %d", len(frames))
	}
	if frames[0].Kind != "diag.state.response" {
		t.Fatalf("got kind %q, want diag.state.response", frames[0].Kind)
	}

	// Decode into a map so missing keys are detectable — a typed struct
	// would silently fill zero values for absent fields and bypass the check.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(frames[0].Payload, &m); err != nil {
		t.Fatalf("unmarshal payload map: %v", err)
	}
	for _, k := range []string{"clients", "conversation", "memory_stats", "pending_tts", "daemon_uptime_s", "last_error"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("payload missing required field %q", k)
		}
	}

	var p DiagStatePayload
	if err := json.Unmarshal(frames[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal payload struct: %v", err)
	}
	if p.Clients == nil {
		t.Fatal("clients must not be nil")
	}
	if p.Conversation == nil {
		t.Fatal("conversation must not be nil")
	}
	if p.MemoryStats == nil {
		t.Fatal("memory_stats must not be nil")
	}
	if p.DaemonUptimeS < 1 {
		t.Fatalf("daemon_uptime_s: got %d, want >=1 from 5s-ago start", p.DaemonUptimeS)
	}
}
