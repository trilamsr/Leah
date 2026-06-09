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

// Client is the LLM completion surface. Implemented by AnthropicClient in
// production and by test doubles elsewhere — the Reasoner itself is
// provider-agnostic.
type Client interface {
	Complete(ctx context.Context, system, user string) (text string, costUSD float64, err error)
}

// Reasoner pairs a Client with the budget gate and the system prompt loaded
// from prompts/system.md (or prompts/regatta-issue.md for Ship).
type Reasoner struct {
	Client       Client
	Budget       *budget.Budget
	SystemPrompt string
}

// Ask sends user to Client.Complete + charges the returned cost. Budget
// exceeded → returns *budget.ExceededError without surfacing partial text.
func (r *Reasoner) Ask(ctx context.Context, user string) (string, error) {
	lg := obs.LoggerFromCtx(ctx).With("package", "reasoner", "func", "Ask")
	lg.Debug("reasoner.call.start")

	start := time.Now()
	text, cost, err := r.Client.Complete(ctx, r.SystemPrompt, user)
	durMs := time.Since(start).Milliseconds()
	if err != nil {
		lg.Error("reasoner.call.error", "duration_ms", durMs, "err", err.Error())
		return "", fmt.Errorf("reasoner: %w", err)
	}
	if chargeErr := r.Budget.Charge(cost); chargeErr != nil {
		lg.Error("reasoner.call.budget_blocked", "duration_ms", durMs, "cost_dollars", cost, "err", chargeErr.Error())
		return "", chargeErr
	}
	lg.Info("reasoner.call.complete", "duration_ms", durMs, "cost_dollars", cost)
	return text, nil
}
