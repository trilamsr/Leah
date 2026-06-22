package reasoner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/trilam/leah/internal/ipc"
)

const opusModel = "claude-opus-4-8"

// streamer is the narrow interface streamToIPCWith needs. *AnthropicClient
// satisfies it via StreamChunks; tests inject a fake.
type streamer interface {
	StreamChunks(ctx context.Context, system string, history []anthropic.MessageParam, userText string, cache bool) (<-chan string, error)
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
		totalOut := 0
		for c := range chunks {
			seq++
			totalOut += len(c) / charsPerToken
			payload, _ := json.Marshal(map[string]string{"text": c})
			select {
			case <-ctx.Done():
				return
			case out <- ipc.Frame{Kind: "prose.delta", TurnID: turnID, Seq: seq, Payload: payload}:
			}
		}
		totalIn := MeasurePromptTokens(system) + MeasurePromptTokens(userText)
		cost := map[string]any{
			"input_tokens":  totalIn,
			"output_tokens": totalOut,
			"cached":        cache,
		}
		cp, _ := json.Marshal(cost)
		seq++
		select {
		case <-ctx.Done():
		case out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: seq, Payload: cp}:
		}
	}()
	return out, nil
}
