package reviewer

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicSubagent struct {
	sdk   anthropic.Client
	model string
}

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

func (a *AnthropicSubagent) Run(ctx context.Context, system, input string) (string, error) {
	resp, err := a.sdk.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(input)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic subagent: %w", err)
	}
	text := ""
	for _, blk := range resp.Content {
		if blk.Type == "text" {
			text += blk.Text
		}
	}
	return text, nil
}
