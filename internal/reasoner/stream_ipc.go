package reasoner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/trilam/leah/internal/ipc"
)

const opusModel = "claude-opus-4-8"

// StreamChunk carries a text delta OR (when Final=true) a real-token-count
// summary derived from MessageDeltaEvent.usage. Final chunks have Text="".
// Using a struct (not a bare string channel) so cost accounting reports the
// SDK's actual InputTokens/OutputTokens instead of the int-divided estimate
// that truncated short bursts to zero.
type StreamChunk struct {
	Text         string
	Final        bool
	InputTokens  int
	OutputTokens int
}

// streamer is the narrow interface streamToIPCWith needs. *AnthropicClient
// satisfies it via StreamChunks; tests inject a fake.
type streamer interface {
	StreamChunks(ctx context.Context, system string, history []anthropic.MessageParam, userText string, cache bool) (<-chan StreamChunk, error)
}

// StreamToIPC wraps an AnthropicClient and emits ipc.Frame events: one
// prose.delta per text chunk, then a turn.end. The caller forwards frames to
// the Unix socket; this function does not touch the socket.
//
// When escalateOpus is true the model is swapped to claude-opus-4-8 for this
// request only (F4 operator decision). Default is claude-sonnet-4-6.
func StreamToIPC(ctx context.Context, client *AnthropicClient, turnID string, systemPrompt string, history []anthropic.MessageParam, userText string, escalateOpus bool) (<-chan ipc.Frame, error) {
	c := client
	if escalateOpus {
		c = &AnthropicClient{sdk: client.sdk, model: opusModel, Registry: client.Registry}
	}
	return streamToIPCWith(ctx, c, turnID, systemPrompt, history, userText)
}

// proseFrameOverhead is the JSON-envelope cost of a prose.delta Frame minus
// the inner text bytes — Kind/TurnID/Seq/Payload keys + the {"text":""} wrap.
// Kept conservative (1 KiB) so a near-cap chunk still fits after marshaling.
const proseFrameOverhead = 1024

// streamToIPCWith is the testable core; accepts any streamer so unit tests
// inject fakeStreamer instead of hitting the network.
func streamToIPCWith(ctx context.Context, s streamer, turnID, system string, history []anthropic.MessageParam, userText string) (<-chan ipc.Frame, error) {
	cache := ShouldCachePrompt(system)
	chunks, err := s.StreamChunks(ctx, system, history, userText, cache)
	if err != nil {
		return nil, fmt.Errorf("stream: %w", err)
	}
	out := make(chan ipc.Frame, 8)
	go func() {
		defer close(out)
		var seq uint64
		var inTokFinal, outTokFinal int
		var haveFinal bool
		maxText := ipc.MaxFrameBytes - proseFrameOverhead

		emit := func(kind string, payload json.RawMessage) bool {
			seq++
			select {
			case <-ctx.Done():
				return false
			case out <- ipc.Frame{Kind: kind, TurnID: turnID, Seq: seq, Payload: payload}:
				return true
			}
		}
		abort := func(reason string) {
			cp, _ := json.Marshal(map[string]any{"error": reason})
			seq++
			select {
			case <-ctx.Done():
			case out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: seq, Payload: cp}:
			}
		}

		for ch := range chunks {
			if ch.Final {
				inTokFinal = ch.InputTokens
				outTokFinal = ch.OutputTokens
				haveFinal = true
				continue
			}
			// Split text so each prose.delta payload stays under the
			// 256 KiB ipc.Frame cap; consumer concatenates deltas in
			// order so splits are transparent.
			text := ch.Text
			for len(text) > 0 {
				n := len(text)
				if n > maxText {
					n = maxText
				}
				part := text[:n]
				payload, err := json.Marshal(map[string]string{"text": part})
				if err != nil {
					abort(fmt.Sprintf("marshal prose.delta: %v", err))
					return
				}
				if len(payload) > ipc.MaxFrameBytes {
					// Pathological — overhead estimate was wrong.
					abort(fmt.Sprintf("prose.delta payload %d exceeds cap %d", len(payload), ipc.MaxFrameBytes))
					return
				}
				if !emit("prose.delta", payload) {
					return
				}
				text = text[n:]
			}
		}

		var totalIn, totalOut int
		if haveFinal {
			totalIn = inTokFinal
			totalOut = outTokFinal
		} else {
			// Streamer closed without a Final chunk (older fakes or a
			// partial cancel). Fall back to the char-based estimate so
			// the cost frame still ships rather than dropping silently.
			totalIn = MeasurePromptTokens(system) + MeasurePromptTokens(userText)
		}
		cost := map[string]any{
			"input_tokens":  totalIn,
			"output_tokens": totalOut,
			"cached":        cache,
		}
		cp, err := json.Marshal(cost)
		if err != nil {
			abort(fmt.Sprintf("marshal turn.end: %v", err))
			return
		}
		seq++
		select {
		case <-ctx.Done():
		case out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: seq, Payload: cp}:
		}
	}()
	return out, nil
}
