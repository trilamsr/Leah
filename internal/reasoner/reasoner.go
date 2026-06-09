package reasoner

import (
	"context"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/obs"
)

type Client interface {
	Complete(ctx context.Context, system, user string) (text string, costUSD float64, err error)
}

type Reasoner struct {
	Client       Client
	Budget       *budget.Budget
	SystemPrompt string
}

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
