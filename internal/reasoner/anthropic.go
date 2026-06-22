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
	return NewAnthropicClientWithModel("")
}

// NewAnthropicClientWithModel pins a specific model — used by the
// degraded leg of the Router (Haiku) so the warn-state swap doesn't
// require a second env knob.
func NewAnthropicClientWithModel(model string) (*AnthropicClient, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}
	c := anthropic.NewClient(option.WithAPIKey(key))
	if model == "" {
		model = defaultModel
		if v := os.Getenv("LEAH_MODEL"); v != "" {
			model = v
		}
	}
	return &AnthropicClient{sdk: c, model: model}, nil
}

// Model returns the model string this client targets. Exposed so the
// Router can populate `leah_cost_breaker_degrade_total{from_model,to_model}`.
func (c *AnthropicClient) Model() string { return c.model }

// Complete sends one system + user message pair and returns the joined text
// blocks plus the LLM-dim payload (cost, tokens, model, cache-hit, egress
// bytes). Sonnet 4.6 pricing as of 2026-06.
func (c *AnthropicClient) Complete(ctx context.Context, system, user string) (CompleteResult, error) {
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
	// EgressBytes is gated behind LEAH_AUDIT_EGRESS_BYTES=1 — the SDK
	// doesn't surface the wire-serialized payload, so deriving it
	// requires a parallel json.Marshal on every call (best-effort
	// approximation, not the actual TLS-frame byte count). Default OFF
	// keeps the unconditional CPU cost out of the hot path; operators
	// who want byte-level egress accounting opt in.
	var egress int64
	if os.Getenv("LEAH_AUDIT_EGRESS_BYTES") == "1" {
		if b, mErr := json.Marshal(params); mErr == nil {
			egress = int64(len(b))
		}
	}

	resp, err := c.sdk.Messages.New(ctx, params)
	if err != nil {
		return CompleteResult{Model: c.model, EgressBytes: egress}, fmt.Errorf("anthropic api: %w", err)
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

// OneShot sends a single non-streaming call with max_tokens=64. Used by the
// widget-intent classifier where budget ≤ 200 ms and output is one JSON line.
func (c *AnthropicClient) OneShot(ctx context.Context, system, user string) (string, error) {
	resp, err := c.sdk.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 64,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user))},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic oneshot: %w", err)
	}
	for _, blk := range resp.Content {
		if blk.Type == "text" {
			return blk.Text, nil
		}
	}
	return "", nil
}

// Stream issues a streaming messages call and translates SDK events into
// reasoner.Delta. Text deltas pass through; tool-use blocks surface as
// ToolUseEvent (suppressed by AskStream callers today). Token counts fire on
// the Final delta from message_delta usage. Returned channel closes when the
// SDK stream ends OR ctx is cancelled.
func (c *AnthropicClient) Stream(ctx context.Context, system, user string) (<-chan Delta, error) {
	sysBlock := buildSystemBlock(system)
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{sysBlock},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
		},
	}
	stream := c.sdk.Messages.NewStreaming(ctx, params)
	out := make(chan Delta, 16)
	go func() {
		// Close the SSE response body explicitly on every exit path —
		// http.Transport observes ctx-cancel, but the contract-correct
		// release is Stream.Close → res.Body.Close.
		defer func() { _ = stream.Close() }()
		defer close(out)
		var inTok, outTok int
		for stream.Next() {
			ev := stream.Current()
			switch v := ev.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				if tu := v.ContentBlock.AsToolUse(); tu.Name != "" {
					select {
					case <-ctx.Done():
						return
					case out <- Delta{ToolUse: &ToolUseEvent{Name: tu.Name, ID: tu.ID}}:
					}
				}
			case anthropic.ContentBlockDeltaEvent:
				if td := v.Delta.AsTextDelta(); td.Text != "" {
					select {
					case <-ctx.Done():
						return
					case out <- Delta{Text: td.Text}:
					}
				}
			case anthropic.MessageStartEvent:
				inTok = int(v.Message.Usage.InputTokens)
			case anthropic.MessageDeltaEvent:
				outTok = int(v.Usage.OutputTokens)
			}
		}
		if err := stream.Err(); err != nil {
			out <- Delta{Err: fmt.Errorf("anthropic stream: %w", err)}
			return
		}
		out <- Delta{Final: true, InputTok: inTok, OutputTok: outTok}
	}()
	return out, nil
}
