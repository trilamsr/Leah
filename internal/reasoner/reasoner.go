package reasoner

import (
	"context"
	"fmt"

	"github.com/trilam/leah/internal/budget"
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
	text, cost, err := r.Client.Complete(ctx, r.SystemPrompt, user)
	if err != nil {
		return "", fmt.Errorf("reasoner: %w", err)
	}
	if chargeErr := r.Budget.Charge(cost); chargeErr != nil {
		return "", chargeErr
	}
	return text, nil
}
