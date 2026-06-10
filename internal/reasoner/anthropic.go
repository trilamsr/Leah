package reasoner

import (
	"context"
	"encoding/json"
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
// blocks plus the LLM-dim payload (cost, tokens, model, cache-hit, egress
// bytes). Sonnet 4.6 pricing as of 2026-06.
func (c *AnthropicClient) Complete(ctx context.Context, system, user string) (CompleteResult, error) {
	return c.complete(ctx, system, user, nil)
}

// CompleteStream is Complete with an onChunk callback that fires for each
// decoded text delta in order. Same return semantics as Complete — final
// joined text + LLM-dim payload still come back at the end.
func (c *AnthropicClient) CompleteStream(ctx context.Context, system, user string, onChunk func(string)) (CompleteResult, error) {
	return c.complete(ctx, system, user, onChunk)
}

func (c *AnthropicClient) complete(ctx context.Context, system, user string, onChunk func(string)) (CompleteResult, error) {
	sysBlock := buildSystemBlock(system)
	cacheEnabled := string(sysBlock.CacheControl.Type) != ""

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{sysBlock},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
		},
	}
	// EgressBytes gated behind LEAH_AUDIT_EGRESS_BYTES=1 — the SDK doesn't
	// surface the wire-serialized payload, so deriving it requires a
	// parallel json.Marshal on every call (best-effort, not the actual
	// TLS-frame byte count). Default OFF keeps the unconditional CPU cost
	// out of the hot path.
	var egress int64
	if os.Getenv("LEAH_AUDIT_EGRESS_BYTES") == "1" {
		if b, mErr := json.Marshal(params); mErr == nil {
			egress = int64(len(b))
		}
	}

	var resp anthropic.Message
	if onChunk != nil {
		stream := c.sdk.Messages.NewStreaming(ctx, params)
		acc := anthropic.Message{}
		for stream.Next() {
			event := stream.Current()
			if err := acc.Accumulate(event); err != nil {
				return CompleteResult{Model: c.model, EgressBytes: egress}, fmt.Errorf("anthropic stream accumulate: %w", err)
			}
			// ContentBlockDeltaEvent + TextDelta is the only chunk shape the
			// operator wants surfaced — other deltas (thinking, tool input)
			// stay internal to the accumulator.
			if d, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
				if td, ok := d.Delta.AsAny().(anthropic.TextDelta); ok {
					onChunk(td.Text)
				}
			}
		}
		if err := stream.Err(); err != nil {
			return CompleteResult{Model: c.model, EgressBytes: egress}, fmt.Errorf("anthropic stream: %w", err)
		}
		resp = acc
	} else {
		r, err := c.sdk.Messages.New(ctx, params)
		if err != nil {
			return CompleteResult{Model: c.model, EgressBytes: egress}, fmt.Errorf("anthropic api: %w", err)
		}
		resp = *r
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

	cacheHit := false
	switch {
	case !cacheEnabled:
		RecordCacheOutcome(c.Registry, OutcomeDisabled, 0)
	case resp.Usage.CacheReadInputTokens > 0:
		RecordCacheOutcome(c.Registry, OutcomeHit, resp.Usage.CacheReadInputTokens)
		cacheHit = true
	default:
		RecordCacheOutcome(c.Registry, OutcomeMiss, 0)
	}
	return CompleteResult{
		Text:         text,
		CostUSD:      cost,
		Model:        c.model,
		InputTokens:  int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
		EgressBytes:  egress,
		CacheHit:     cacheHit,
	}, nil
}
