package ipc

import (
	"context"
	"encoding/json"
	"time"
)

// DiagStatePayload carries best-effort daemon state for debugging.
type DiagStatePayload struct {
	Clients       []string               `json:"clients"`
	Conversation  map[string]interface{} `json:"conversation"`
	MemoryStats   map[string]interface{} `json:"memory_stats"`
	PendingTTS    bool                   `json:"pending_tts"`
	DaemonUptimeS int64                  `json:"daemon_uptime_s"`
	LastError     string                 `json:"last_error"`
}

// HandleState returns a single-frame channel containing a diag.state.response
// frame. lastError is the most recent ERROR-level log line from the daemon's
// obs.ErrorRing; pass "" when none is available.
func HandleState(_ context.Context, startTime time.Time, lastError string) (<-chan Frame, error) {
	p := DiagStatePayload{
		Clients:       []string{},
		Conversation:  map[string]interface{}{},
		MemoryStats:   map[string]interface{}{},
		PendingTTS:    false,
		DaemonUptimeS: int64(time.Since(startTime).Seconds()),
		LastError:     lastError,
	}
	payload, _ := json.Marshal(p)
	out := make(chan Frame, 1)
	out <- Frame{Kind: "diag.state.response", Seq: 1, Payload: payload}
	close(out)
	return out, nil
}
