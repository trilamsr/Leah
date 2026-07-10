package a2a

import (
	"testing"

	"github.com/trilam/leah/internal/platform/budget"
)

func newSpentBudget(t *testing.T, ceiling, spend float64) *budget.Budget {
	t.Helper()
	b := &budget.Budget{Ceiling: ceiling}
	if err := b.Charge(spend); err != nil {
		t.Fatalf("budget seed: %v", err)
	}
	return b
}
