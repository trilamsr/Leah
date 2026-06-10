package reviewer

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
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

// NewAnthropicSubagent constructs an AnthropicSubagent, failing fast when
// ANTHROPIC_API_KEY is unset.
func NewAnthropicSubagent() (*AnthropicSubagent, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
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

	stream := a.sdk.Messages.NewStreaming(ctx, params)
	var buf strings.Builder
	var usage anthropic.MessageDeltaUsage
	for stream.Next() {
		ev := stream.Current()
		switch v := ev.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			td := v.Delta.AsTextDelta()
			if td.Text == "" {
				continue
			}
			buf.WriteString(td.Text)
			if sink != nil {
				_, _ = sink.Write([]byte(td.Text))
			}
		case anthropic.MessageDeltaEvent:
			usage = v.Usage
		case anthropic.MessageStartEvent:
			// MessageStart carries the initial input-token count + cache
			// fields; MessageDelta overwrites OutputTokens cumulatively.
			usage.InputTokens = v.Message.Usage.InputTokens
			usage.CacheCreationInputTokens = v.Message.Usage.CacheCreationInputTokens
			usage.CacheReadInputTokens = v.Message.Usage.CacheReadInputTokens
			usage.OutputTokens = v.Message.Usage.OutputTokens
		}
	}
	if err := stream.Err(); err != nil {
		return "", 0, fmt.Errorf("anthropic subagent: %w", err)
	}

	cost := float64(usage.InputTokens)*inputCostPerToken +
		float64(usage.OutputTokens)*outputCostPerToken +
		float64(usage.CacheCreationInputTokens)*cacheWriteCostPerToken +
		float64(usage.CacheReadInputTokens)*cacheReadCostPerToken
	return buf.String(), cost, nil
}
