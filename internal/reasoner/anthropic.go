package reasoner

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/trilam/leah/internal/obs"
)

const (
	defaultModel = "claude-sonnet-4-6"
	// Sonnet 4.6 pricing as of 2026-06; update if model changes
	inputCostPerToken  = 3.0 / 1_000_000  // $3/M input
	outputCostPerToken = 15.0 / 1_000_000 // $15/M output
)

// AnthropicClient is the production Client backed by the official
// anthropic-sdk-go package. Model defaults to claude-sonnet-4-6 but the
// LEAH_MODEL env override lets the operator pin a specific snapshot.
type AnthropicClient struct {
	sdk      anthropic.Client
	model    string
	Registry *obs.Registry // nil-safe: cache metrics no-op when unset
}

// NewAnthropicClient builds an AnthropicClient, returning an error when
// ANTHROPIC_API_KEY is unset — fail-fast in main beats a runtime 401.
func NewAnthropicClient() (*AnthropicClient, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}
	c := anthropic.NewClient(option.WithAPIKey(key))
	model := defaultModel
	if v := os.Getenv("LEAH_MODEL"); v != "" {
		model = v
	}
	return &AnthropicClient{sdk: c, model: model}, nil
}

// Complete sends one system + user message pair and returns the joined text
// blocks plus the computed dollar cost (Sonnet 4.6 pricing as of 2026-06).
func (c *AnthropicClient) Complete(ctx context.Context, system, user string) (string, float64, error) {
	sysBlock := buildSystemBlock(system)
	cacheEnabled := string(sysBlock.CacheControl.Type) != ""

	resp, err := c.sdk.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{sysBlock},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
		},
	})
	if err != nil {
		return "", 0, fmt.Errorf("anthropic api: %w", err)
	}
	text := ""
	for _, blk := range resp.Content {
		if blk.Type == "text" {
			text += blk.Text
		}
	}
	cost := float64(resp.Usage.InputTokens)*inputCostPerToken +
		float64(resp.Usage.CacheCreationInputTokens)*inputCostPerToken*1.25 +
		float64(resp.Usage.CacheReadInputTokens)*inputCostPerToken*0.10 +
		float64(resp.Usage.OutputTokens)*outputCostPerToken

	switch {
	case !cacheEnabled:
		RecordCacheOutcome(c.Registry, OutcomeDisabled, 0)
	case resp.Usage.CacheReadInputTokens > 0:
		RecordCacheOutcome(c.Registry, OutcomeHit, resp.Usage.CacheReadInputTokens)
	default:
		RecordCacheOutcome(c.Registry, OutcomeMiss, 0)
	}
	return text, cost, nil
}
