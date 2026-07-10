package reviewer

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"github.com/trilam/leah/internal/platform/keychain"
)

// Anthropic sonnet-4 pricing. Cache writes cost 1.25x base input, cache reads
// 0.1x — the reviewer system prompt is large + stable across runs, so once
// it's been written to cache the per-call input bill collapses ~10x.
const (
	inputCostPerToken      = 3.0 / 1_000_000
	outputCostPerToken     = 15.0 / 1_000_000
	cacheWriteCostPerToken = 3.75 / 1_000_000
	cacheReadCostPerToken  = 0.3 / 1_000_000
)

// AnthropicSubagent is the production Subagent for the reviewer. Model
// defaults to claude-sonnet-4-6 with LEAH_REVIEWER_MODEL override —
// independent from Reasoner's LEAH_MODEL so reviewer can run a stricter
// model than the drafter.
type AnthropicSubagent struct {
	sdk   anthropic.Client
	model string
}

// NewAnthropicSubagent constructs an AnthropicSubagent, failing fast when no
// Anthropic key is available in env or Keychain.
func NewAnthropicSubagent() (*AnthropicSubagent, error) {
	key, err := keychain.LoadAnthropicKey()
	if err != nil {
		return nil, fmt.Errorf("load anthropic key: %w", err)
	}
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set (env or Keychain slot %s/%s)", keychain.AnthropicService, keychain.DefaultAccount)
	}
	model := "claude-sonnet-4-6"
	if v := os.Getenv("LEAH_REVIEWER_MODEL"); v != "" {
		model = v
	}
	c := anthropic.NewClient(option.WithAPIKey(key))
	return &AnthropicSubagent{sdk: c, model: model}, nil
}

// Run streams one system + user message pair to sink (when non-nil) and
// returns the joined text plus the computed dollar cost. The System block
// carries an ephemeral cache breakpoint — the reviewer prompt is large +
// stable, so repeat invocations within the 5-min cache window collapse the
// input-token bill ~10x.
func (a *AnthropicSubagent) Run(ctx context.Context, system, input string, sink io.Writer) (string, float64, error) {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{{
			Text:         system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(input)),
		},
	}

	text, msg, err := processStream(a.sdk.Messages.NewStreaming(ctx, params), sink)
	if err != nil {
		return "", 0, fmt.Errorf("anthropic subagent: %w", err)
	}

	cost := float64(msg.Usage.InputTokens)*inputCostPerToken +
		float64(msg.Usage.OutputTokens)*outputCostPerToken +
		float64(msg.Usage.CacheCreationInputTokens)*cacheWriteCostPerToken +
		float64(msg.Usage.CacheReadInputTokens)*cacheReadCostPerToken
	return text, cost, nil
}

// streamIter is the minimal slice of *ssestream.Stream[MessageStreamEventUnion]
// processStream needs — extracted so tests can drive the event loop without an
// HTTP server. Close MUST be called regardless of how iteration ends; see
// finding #1 on PR #231.
type streamIter interface {
	Next() bool
	Current() anthropic.MessageStreamEventUnion
	Err() error
	Close() error
}

// Compile-time check: *ssestream.Stream satisfies streamIter.
var _ streamIter = (*ssestream.Stream[anthropic.MessageStreamEventUnion])(nil)

// processStream walks events, writes text deltas to sink, accumulates Usage
// per-field via Message.Accumulate (preserves cache fields when MessageDelta
// omits them), and always closes the underlying stream. Returns the joined
// text + accumulated Message. Reviewer expects text deltas only; thinking-
// enabled models would emit ThinkingDelta events that this loop ignores —
// guard via LEAH_REVIEWER_MODEL.
func processStream(stream streamIter, sink io.Writer) (string, anthropic.Message, error) {
	defer func() { _ = stream.Close() }()
	var buf strings.Builder
	var msg anthropic.Message
	for stream.Next() {
		ev := stream.Current()
		if err := msg.Accumulate(ev); err != nil {
			return "", msg, err
		}
		if cb, ok := ev.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
			td := cb.Delta.AsTextDelta()
			if td.Text == "" {
				continue
			}
			buf.WriteString(td.Text)
			if sink != nil {
				_, _ = sink.Write([]byte(td.Text))
			}
		}
	}
	if err := stream.Err(); err != nil {
		return "", msg, err
	}
	return buf.String(), msg, nil
}
