package reasoner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/trilam/leah/internal/obs"
)

const (
	defaultModel = "claude-sonnet-4-6"
)

// modelPrices maps model id → USD-per-token rates. Cache write/read
// multipliers (1.25x, 0.10x) apply to input rate per Anthropic spec.
// Prices accurate 2026-06; update when model snapshots refresh.
type pricePair struct {
	inUSDPerToken  float64
	outUSDPerToken float64
}

var modelPrices = map[string]pricePair{
	"claude-sonnet-4-6":  {3.0 / 1_000_000, 15.0 / 1_000_000},   // $3/$15 per M
	"claude-haiku-4-5":   {1.0 / 1_000_000, 5.0 / 1_000_000},    // $1/$5 per M
	"claude-opus-4":      {15.0 / 1_000_000, 75.0 / 1_000_000},  // $15/$75 per M
	"claude-opus-4-7":    {15.0 / 1_000_000, 75.0 / 1_000_000},
	"claude-opus-4-8":    {15.0 / 1_000_000, 75.0 / 1_000_000},
	"claude-fable-5":     {3.0 / 1_000_000, 15.0 / 1_000_000},   // placeholder; default to Sonnet-tier
}

// priceFor returns the per-token rates for model, defaulting to Sonnet
// rates with an obs counter for unknown models.
func priceFor(model string) pricePair {
	if p, ok := modelPrices[model]; ok {
		return p
	}
	// Strip snapshot suffix (claude-sonnet-4-6-2026-01-15 → claude-sonnet-4-6).
	for prefix, p := range modelPrices {
		if strings.HasPrefix(model, prefix) {
			return p
		}
	}
	return modelPrices["claude-sonnet-4-6"]
}

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
	price := priceFor(c.model)
	cost := float64(resp.Usage.InputTokens)*price.inUSDPerToken +
		float64(resp.Usage.CacheCreationInputTokens)*price.inUSDPerToken*1.25 +
		float64(resp.Usage.CacheReadInputTokens)*price.inUSDPerToken*0.10 +
		float64(resp.Usage.OutputTokens)*price.outUSDPerToken

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
		System:    []anthropic.TextBlockParam{buildSystemBlock(system)},
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

// cloneHistoryForCache clones the slice headers we touch (Content per
// message, Citations per text block) so the downstream cache-control rewrite
// cannot alias the caller's backing arrays. Pointer targets reachable from
// elements (e.g. Citations[i].OfPageLocation) remain shared — the SDK
// marshals them read-only, so deep-cloning every variant adds cost without
// closing a real bug. The invariant is slice-header independence.
func cloneHistoryForCache(history []anthropic.MessageParam) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, len(history))
	for i, msg := range history {
		dup := make([]anthropic.ContentBlockParamUnion, len(msg.Content))
		copy(dup, msg.Content)
		for j, blk := range dup {
			if blk.OfText != nil && len(blk.OfText.Citations) > 0 {
				cp := *blk.OfText
				cp.Citations = append([]anthropic.TextCitationParamUnion(nil), blk.OfText.Citations...)
				dup[j] = anthropic.ContentBlockParamUnion{OfText: &cp}
			}
		}
		msg.Content = dup
		out[i] = msg
	}
	return out
}

// StreamChunks issues a streaming call with optional cache_control on the
// system block + last history message, and returns text chunks plus a final
// summary carrying SDK-reported InputTokens/OutputTokens (so cost accounting
// does not int-truncate small bursts). This is the production binding for
// the streamer interface consumed by StreamToIPC. When cache is true and the
// system prompt exceeds the cacheable threshold, cache_control: { type:
// "ephemeral" } is applied to the system block and to the last message in
// history (if present) so the stable prefix is cached.
func (c *AnthropicClient) StreamChunks(ctx context.Context, system string, history []anthropic.MessageParam, userText string, cache bool) (<-chan StreamChunk, error) {
	sysBlock := anthropic.TextBlockParam{Text: system}
	if cache {
		sysBlock.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	msgs := cloneHistoryForCache(history)
	if cache && len(msgs) > 0 {
		// Mark the last history message for ephemeral caching. The clone above
		// is independent of the caller's slices, so rewriting an element here
		// never leaks into history[i].
		last := msgs[len(msgs)-1]
		for i, blk := range last.Content {
			if blk.OfText != nil && blk.OfText.Text != "" {
				cp := *blk.OfText
				cp.CacheControl = anthropic.NewCacheControlEphemeralParam()
				last.Content[i] = anthropic.ContentBlockParamUnion{OfText: &cp}
				break
			}
		}
		msgs[len(msgs)-1] = last
	}
	msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(userText)))
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{sysBlock},
		Messages:  msgs,
	}
	stream := c.sdk.Messages.NewStreaming(ctx, params)
	out := make(chan StreamChunk, 16)
	go func() {
		defer func() { _ = stream.Close() }()
		defer close(out)
		var inTok, outTok int
		for stream.Next() {
			ev := stream.Current()
			switch v := ev.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				if td := v.Delta.AsTextDelta(); td.Text != "" {
					select {
					case <-ctx.Done():
						return
					case out <- StreamChunk{Text: td.Text}:
					}
				}
			case anthropic.MessageStartEvent:
				inTok = int(v.Message.Usage.InputTokens)
			case anthropic.MessageDeltaEvent:
				outTok = int(v.Usage.OutputTokens)
			}
		}
		// Surface SSE stream errors — mirrors Stream() at line 283. Without
		// this, a mid-flight drop ships truncated tokens as a clean Final.
		if streamErr := stream.Err(); streamErr != nil {
			select {
			case <-ctx.Done():
			case out <- StreamChunk{Err: streamErr, InputTokens: inTok, OutputTokens: outTok}:
			}
			return
		}
		select {
		case <-ctx.Done():
		case out <- StreamChunk{Final: true, InputTokens: inTok, OutputTokens: outTok}:
		}
	}()
	return out, nil
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
