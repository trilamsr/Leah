// Package reasoner is Leah's main LLM surface (Anthropic-backed). One Reasoner
// per CLI invocation; each Ask charges the per-process budget before returning
// so the cap is honored even mid-conversation.
package reasoner

import (
	"context"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/obs"
)

// CompleteResult is the LLM-dim payload returned by Client.Complete.
// Tokens / Egress / CacheHit / Model are best-effort; clients that
// cannot surface a value MUST leave it zero (audit Entry has omitempty
// on the matching fields).
type CompleteResult struct {
	Text         string
	CostUSD      float64
	Model        string
	InputTokens  int
	OutputTokens int
	EgressBytes  int64
	CacheHit     bool
}

// CallInfo is the LLM-dim slice of the most recent Ask, stamped onto
// the audit row by dispatcher.Ask.Run via type-assertion. Mirrors the
// omitempty fields on audit.Entry (spec §2).
type CallInfo struct {
	Model        string
	PromptSHA    string
	InputTokens  int
	OutputTokens int
	LatencyMS    int64
	EgressBytes  int64
	CacheHit     bool
}

// Client is the LLM completion surface. Implemented by AnthropicClient in
// production and by test doubles elsewhere — the Reasoner itself is
// provider-agnostic.
type Client interface {
	Complete(ctx context.Context, system, user string) (CompleteResult, error)
}

// ToolUseEvent surfaces an SDK content_block_start with type=tool_use during
// streaming. Reserved for W17+ skill wiring; AskStream suppresses it from the
// text channel so callers consuming `<-chan string` see text-only deltas.
type ToolUseEvent struct {
	Name string
	ID   string
}

// Delta is one streaming event. Text != "" → text delta; ToolUse != nil →
// tool-use block; Final == true → terminal delta carrying cumulative token
// counts; Err != nil → stream aborted (channel still drains to close).
type Delta struct {
	Text      string
	ToolUse   *ToolUseEvent
	Final     bool
	InputTok  int
	OutputTok int
	Err       error
}

// StreamingClient extends Client with delta streaming. Anthropic implements
// this in production; non-streaming Clients keep Complete only.
type StreamingClient interface {
	Client
	Stream(ctx context.Context, system, user string) (<-chan Delta, error)
}

// Reasoner pairs a Client with the budget gate and the system prompt loaded
// from prompts/system.md (or prompts/regatta-issue.md for Ship).
//
// PersonaPrefix, when non-empty, is woven in front of SystemPrompt at Ask
// time so per-workspace tone/signature/voice settings dominate the framing
// the model sees. Empty prefix preserves the legacy behavior (SystemPrompt
// unchanged) — see internal/persona.Persona.SystemPromptPrefix for the
// canonical producer.
type Reasoner struct {
	Client        Client
	Budget        *budget.Budget
	SystemPrompt  string
	PersonaPrefix string

	// OnStreamToolUseSuppressed fires once per tool-use delta dropped by
	// AskStream. Voice instrumentation binds this to
	// leah_voice_stream_tool_use_suppressed_total so suppressed-vs-not-invoked
	// is visible. nil-safe.
	OnStreamToolUseSuppressed func()

	lastCall CallInfo
}

// LastCallInfo returns the LLM-dim slice of the most recent Ask. Not
// safe for concurrent Asks on the same Reasoner — by contract one
// Reasoner per CLI invocation (see package doc).
func (r *Reasoner) LastCallInfo() CallInfo { return r.lastCall }

// AskStream returns a text-only delta channel for the streaming voice path
// (W109). The underlying Client must implement StreamingClient; non-streaming
// Clients return an error so callers can fall back to Ask.
//
// Tool-use deltas are suppressed from the returned channel — they belong to
// the skill surface (W17+) which has not landed; for now they're a no-op.
// Budget charging is deferred until Stream surfaces the Final delta because
// token counts aren't known until the stream completes.
func (r *Reasoner) AskStream(ctx context.Context, user string) (<-chan string, error) {
	sc, ok := r.Client.(StreamingClient)
	if !ok {
		return nil, fmt.Errorf("reasoner: client does not support streaming")
	}
	system := r.SystemPrompt
	if r.PersonaPrefix != "" {
		system = r.PersonaPrefix + "\n\n" + r.SystemPrompt
	}
	deltas, err := sc.Stream(ctx, system, user)
	if err != nil {
		return nil, fmt.Errorf("reasoner: %w", err)
	}
	out := make(chan string, 8)
	go func() {
		defer close(out)
		for d := range deltas {
			if d.ToolUse != nil && r.OnStreamToolUseSuppressed != nil {
				r.OnStreamToolUseSuppressed()
			}
			if d.Text == "" {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case out <- d.Text:
			}
		}
	}()
	return out, nil
}

// Ask sends user to Client.Complete + charges the returned cost. Budget
// exceeded → returns *budget.ExceededError without surfacing partial text.
func (r *Reasoner) Ask(ctx context.Context, user string) (string, error) {
	lg := obs.LoggerFromCtx(ctx).With("package", "reasoner", "func", "Ask")
	lg.Debug("reasoner.call.start")

	start := time.Now()
	system := r.SystemPrompt
	if r.PersonaPrefix != "" {
		system = r.PersonaPrefix + "\n\n" + r.SystemPrompt
	}
	res, err := r.Client.Complete(ctx, system, user)
	durMs := time.Since(start).Milliseconds()
	// PromptSHA hashes SystemPrompt only — registry-keyed audit replay
	// (spec §4.1/§4.2) needs SHA → SystemPrompt to resolve; PersonaPrefix
	// is a per-workspace overlay tracked elsewhere.
	r.lastCall = CallInfo{
		Model:        res.Model,
		PromptSHA:    PromptSHA(r.SystemPrompt),
		InputTokens:  res.InputTokens,
		OutputTokens: res.OutputTokens,
		LatencyMS:    durMs,
		EgressBytes:  res.EgressBytes,
		CacheHit:     res.CacheHit,
	}
	if err != nil {
		lg.Error("reasoner.call.error", "duration_ms", durMs, "err", err.Error())
		return "", fmt.Errorf("reasoner: %w", err)
	}
	if chargeErr := r.Budget.Charge(res.CostUSD); chargeErr != nil {
		lg.Error("reasoner.call.budget_blocked", "duration_ms", durMs, "cost_dollars", res.CostUSD, "err", chargeErr.Error())
		return "", chargeErr
	}
	lg.Info("reasoner.call.complete", "duration_ms", durMs, "cost_dollars", res.CostUSD)
	return res.Text, nil
}
