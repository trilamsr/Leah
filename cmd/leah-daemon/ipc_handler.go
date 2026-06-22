package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/knowledge"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/obs"
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

// classifyFn routes a user query to an Intent before the stream path fires.
type classifyFn func(ctx context.Context, text string) reasoner.Intent

// fetchFn retrieves top-k knowledge chunks for a query; nil return is safe.
type fetchFn func(ctx context.Context, query string, k int) ([]knowledge.Chunk, error)

// newIPCHandler is the production constructor wired into main.go.
func newIPCHandler(sonnet *reasoner.AnthropicClient, db *sql.DB, kg *knowledge.Graph, ring *obs.ErrorRing) ipc.Handler {
	return newIPCHandlerWithClassify(db,
		liveStreamFn(sonnet),
		liveOpusStreamFn(sonnet),
		liveClassifyFn(),
		livePingFn(),
		liveFetchFn(kg),
		time.Now(),
		ring,
	)
}

// newIPCHandlerForTest wires a caller-supplied streamFn; used by tests that
// don't need to exercise classify or verify-key.
func newIPCHandlerForTest(db *sql.DB, s streamFn) ipc.Handler {
	noClassify := func(_ context.Context, _ string) reasoner.Intent { return reasoner.Intent{Kind: "chat"} }
	return newIPCHandlerWithClassify(db, s, s, noClassify,
		func(_ context.Context, _ string) error { return nil }, nil, time.Time{}, nil)
}

// newIPCHandlerWithPingForTest is kept for existing verify-key tests.
func newIPCHandlerWithPingForTest(db *sql.DB, s streamFn, ping pingFn) ipc.Handler {
	noClassify := func(_ context.Context, _ string) reasoner.Intent { return reasoner.Intent{Kind: "chat"} }
	return newIPCHandlerWithClassify(db, s, s, noClassify, ping, nil, time.Time{}, nil)
}

// newIPCHandlerWithDiag is kept for the diag_test.go test fixture.
func newIPCHandlerWithDiag(db *sql.DB, s streamFn, ping pingFn, startTime time.Time) ipc.Handler {
	noClassify := func(_ context.Context, _ string) reasoner.Intent { return reasoner.Intent{Kind: "chat"} }
	return newIPCHandlerWithClassify(db, s, s, noClassify, ping, nil, startTime, nil)
}

// newIPCHandlerWithClassify is the full injection point — used by all tests.
func newIPCHandlerWithClassify(
	db *sql.DB,
	sonnetStream, opusStream streamFn,
	classify classifyFn,
	ping pingFn,
	fetch fetchFn,
	startTime time.Time,
	ring *obs.ErrorRing,
) ipc.Handler {
	return func(ctx context.Context, req ipc.Frame) (<-chan ipc.Frame, error) {
		switch req.Kind {
		case "verify-key":
			return handleVerifyKey(ctx, req, ping)
		case "diag.state":
			var last string
			if ring != nil {
				last = ring.Last()
			}
			return ipc.HandleState(ctx, startTime, last)
		default:
			return handleAsk(ctx, req, db, sonnetStream, opusStream, classify, fetch)
		}
	}
}

// handleAsk classifies the query (Haiku router), routes widget intents to
// widget.mount, escalates to Opus when requested, and persists the turn.
// RecordTurn failure is logged but non-fatal per spec §17.16.
func handleAsk(
	ctx context.Context,
	req ipc.Frame,
	db *sql.DB,
	sonnetStream, opusStream streamFn,
	classify classifyFn,
	fetch fetchFn,
) (<-chan ipc.Frame, error) {
	var p struct {
		Text         string `json:"text"`
		EscalateOpus bool   `json:"escalate_opus"`
	}
	_ = json.Unmarshal(req.Payload, &p)

	// Haiku classify: widget intent emits widget.mount and skips Sonnet.
	if classify != nil {
		intent := classify(ctx, p.Text)
		if intent.Kind == "widget" {
			payload, _ := json.Marshal(map[string]string{"widget_type": intent.Widget})
			out := make(chan ipc.Frame, 1)
			out <- ipc.Frame{Kind: "widget.mount", TurnID: req.TurnID, Seq: 1, Payload: payload}
			close(out)
			return out, nil
		}
	}

	// Pick stream: Opus on escalation flag, Sonnet otherwise.
	s := sonnetStream
	if p.EscalateOpus && opusStream != nil {
		s = opusStream
	}

	if s == nil {
		// No reasoner available (API key missing at boot): emit error frame.
		out := make(chan ipc.Frame, 1)
		payload, _ := json.Marshal(map[string]string{"error": "reasoner unavailable"})
		out <- ipc.Frame{Kind: "error", TurnID: req.TurnID, Seq: 1, Payload: payload}
		close(out)
		return out, nil
	}

	promptText := p.Text
	if fetch != nil {
		if chunks, err := fetch(ctx, p.Text, 5); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah-daemon: SearchRelevant: %v\n", err)
		} else if len(chunks) > 0 {
			texts := make([]string, len(chunks))
			for i, c := range chunks {
				texts[i] = c.Text
			}
			promptText = "Context:\n" + strings.Join(texts, "\n") + "\n\nQuery: " + p.Text
		}
	}

	raw, err := s(ctx, req.TurnID, promptText)
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
				// Persist original user text (not the RAG-augmented prompt).
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

// liveStreamFn builds the production Sonnet streamFn from a live AnthropicClient.
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

// liveOpusStreamFn builds the Opus streamFn for the escalate_opus path.
func liveOpusStreamFn(c *reasoner.AnthropicClient) streamFn {
	if c == nil {
		return nil
	}
	opus, err := reasoner.NewAnthropicClientWithModel("claude-opus-4-8")
	if err != nil {
		return nil
	}
	return liveStreamFn(opus)
}

// liveClassifyFn builds the Haiku-backed classify function.
// Degrades to a no-op chat classifier when ANTHROPIC_API_KEY is absent.
func liveClassifyFn() classifyFn {
	haiku, err := reasoner.NewAnthropicClientWithModel("claude-haiku-4-5")
	if err != nil {
		return func(_ context.Context, _ string) reasoner.Intent { return reasoner.Intent{Kind: "chat"} }
	}
	return func(ctx context.Context, text string) reasoner.Intent {
		intent, err := reasoner.Classify(ctx, haiku, text)
		if err != nil {
			return reasoner.Intent{Kind: "chat"}
		}
		return intent
	}
}

// livePingFn validates the wizard-supplied key by issuing a 1-token Messages
// call with a transient SDK client bound to THAT key — NOT the daemon's
// existing ANTHROPIC_API_KEY. Without this, the wizard would silently
// validate the previously-set key and false-positive bad input.
func livePingFn() pingFn {
	return reasoner.VerifyKey
}

// liveFetchFn wraps a knowledge.Graph's SearchRelevant as a fetchFn.
// Returns nil when graph is nil (no knowledge DB configured).
func liveFetchFn(kg *knowledge.Graph) fetchFn {
	if kg == nil {
		return nil
	}
	return kg.SearchRelevant
}
