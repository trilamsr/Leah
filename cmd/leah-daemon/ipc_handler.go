package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/reasoner"
)

// errVerifyFailed is returned by pingFn when the API key is rejected.
var errVerifyFailed = errors.New("api key rejected")

// streamFn adapts the live reasoner (or a test stub) into ipc.Frame output.
// Each frame must be either "prose.delta" (with {"text":"..."} payload) or
// "turn.end" (with {} payload). The channel is always closed by the producer.
type streamFn func(ctx context.Context, turnID, userText string) (<-chan ipc.Frame, error)

// pingFn is called by the verify-key path; returns nil on success.
type pingFn func(ctx context.Context, key string) error

// newIPCHandler is the production constructor wired into main.go.
// Haiku classify is skipped in this wave — widget routing lands in a
// subsequent task. The handler routes only "ask" and "verify-key" today.
func newIPCHandler(sonnet *reasoner.AnthropicClient, db *sql.DB) ipc.Handler {
	stream := liveStreamFn(sonnet)
	ping := livePingFn()
	return newIPCHandlerWithPingForTest(db, stream, ping)
}

// newIPCHandlerForTest wires a caller-supplied streamFn; used by tests that
// don't need to exercise the verify-key path.
func newIPCHandlerForTest(db *sql.DB, s streamFn) ipc.Handler {
	return newIPCHandlerWithPingForTest(db, s, func(_ context.Context, _ string) error { return nil })
}

// newIPCHandlerWithPingForTest is the injection-point used by all tests.
func newIPCHandlerWithPingForTest(db *sql.DB, s streamFn, ping pingFn) ipc.Handler {
	return func(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
		switch req.Kind {
		case "verify-key":
			return handleVerifyKey(ctx, req, ping)
		default:
			return handleAsk(ctx, req, db, s)
		}
	}
}

// handleAsk streams the Sonnet response and persists the turn on completion.
// RecordTurn failure is logged but non-fatal per spec §17.16.
func handleAsk(ctx context.Context, req ipc.Frame, db *sql.DB, s streamFn) (<-chan ipc.Frame, error) {
	var p struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(req.Payload, &p)

	if s == nil {
		// No reasoner available (API key missing at boot): emit error frame.
		out := make(chan ipc.Frame, 1)
		payload, _ := json.Marshal(map[string]string{"error": "reasoner unavailable"})
		out <- ipc.Frame{Kind: "error", TurnID: req.TurnID, Seq: 1, Payload: payload}
		close(out)
		return out, nil
	}

	raw, err := s(ctx, req.TurnID, p.Text)
	if err != nil {
		return nil, fmt.Errorf("ipc handler stream: %w", err)
	}

	out := make(chan ipc.Frame, 8)
	go func() {
		defer close(out)
		var assembled string
		for f := range raw {
			if f.Kind == "prose.delta" {
				var pp struct {
					Text string `json:"text"`
				}
				_ = json.Unmarshal(f.Payload, &pp)
				assembled += pp.Text
			}
			select {
			case <-ctx.Done():
				return
			case out <- f:
			}
			if f.Kind == "turn.end" {
				if err := memory.RecordTurn(db, req.TurnID, p.Text, assembled); err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: RecordTurn: %v\n", err)
				}
			}
		}
	}()
	return out, nil
}

// handleVerifyKey runs a 1-token ping against the Anthropic API with the
// supplied key. Returns a single "verify-key.result" frame.
func handleVerifyKey(ctx context.Context, req ipc.Frame, ping pingFn) (<-chan ipc.Frame, error) {
	var p struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(req.Payload, &p)

	err := ping(ctx, p.Key)
	ok := err == nil

	payload, _ := json.Marshal(map[string]bool{"ok": ok})
	out := make(chan ipc.Frame, 1)
	out <- ipc.Frame{Kind: "verify-key.result", TurnID: req.TurnID, Seq: 1, Payload: payload}
	close(out)
	return out, nil
}

// liveStreamFn builds the production streamFn from a live AnthropicClient.
// It calls client.Stream directly (not Reasoner.AskStream) to avoid the
// budget-charging path — the daemon HUD chat does not route through
// the per-process budget cap used by CLI commands.
func liveStreamFn(c *reasoner.AnthropicClient) streamFn {
	if c == nil {
		return nil
	}
	return func(ctx context.Context, turnID, userText string) (<-chan ipc.Frame, error) {
		deltas, err := c.Stream(ctx, "", userText)
		if err != nil {
			return nil, fmt.Errorf("anthropic stream: %w", err)
		}
		out := make(chan ipc.Frame, 16)
		go func() {
			defer close(out)
			var seq uint64
			for d := range deltas {
				if d.Text == "" {
					continue
				}
				seq++
				payload, _ := json.Marshal(map[string]string{"text": d.Text})
				out <- ipc.Frame{Kind: "prose.delta", TurnID: turnID, Seq: seq, Payload: payload}
			}
			seq++
			out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: seq, Payload: json.RawMessage(`{}`)}
		}()
		return out, nil
	}
}

// livePingFn validates the wizard-supplied key by issuing a 1-token Messages
// call with a transient SDK client bound to THAT key — NOT the daemon's
// existing ANTHROPIC_API_KEY. Without this, the wizard would silently
// validate the previously-set key and false-positive bad input.
func livePingFn() pingFn {
	return reasoner.VerifyKey
}
