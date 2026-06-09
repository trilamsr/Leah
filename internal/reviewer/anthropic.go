package reviewer

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const (
	inputCostPerToken  = 3.0 / 1_000_000
	outputCostPerToken = 15.0 / 1_000_000
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

// Run sends one system + user message pair and returns the joined text
// blocks plus the computed dollar cost.
func (a *AnthropicSubagent) Run(ctx context.Context, system, input string) (string, float64, error) {
	resp, err := a.sdk.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(input)),
		},
	})
	if err != nil {
		return "", 0, fmt.Errorf("anthropic subagent: %w", err)
	}
	text := ""
	for _, blk := range resp.Content {
		if blk.Type == "text" {
			text += blk.Text
		}
	}
	cost := float64(resp.Usage.InputTokens)*inputCostPerToken +
		float64(resp.Usage.OutputTokens)*outputCostPerToken
	return text, cost, nil
}
