package ipc

import (
	"context"
	"encoding/json"
	"time"
)

// DiagStatePayload carries best-effort daemon state for debugging.
type DiagStatePayload struct {
	Clients       []string       `json:"clients"`
	Conversation  map[string]any `json:"conversation"`
	MemoryStats   map[string]any `json:"memory_stats"`
	PendingTTS    bool           `json:"pending_tts"`
	DaemonUptimeS int64          `json:"daemon_uptime_s"`
	LastError     string         `json:"last_error"`
}

// HandleState returns a single-frame channel containing a diag.state.response
// frame. lastError is the most recent ERROR-level log line from the daemon's
// obs.ErrorRing; pass "" when none is available. turnID is echoed back so
// HUD can correlate response to request on multiplexed conns.
func HandleState(_ context.Context, startTime time.Time, lastError string, turnID string) (<-chan Frame, error) {
	p := DiagStatePayload{
		Clients:       []string{},
		Conversation:  map[string]any{},
		MemoryStats:   map[string]any{},
		PendingTTS:    false,
		DaemonUptimeS: int64(time.Since(startTime).Seconds()),
		LastError:     lastError,
	}
	payload, _ := json.Marshal(p)
	out := make(chan Frame, 1)
	out <- Frame{Kind: "diag.state.response", TurnID: turnID, Seq: 1, Payload: payload}
	close(out)
	return out, nil
}
