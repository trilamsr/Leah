package budget

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
)

const DefaultCeiling = 5.0

type Budget struct {
	Ceiling float64
	spent   float64
	mu      sync.Mutex
}

func (b *Budget) Spent() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}

func New() *Budget {
	c := DefaultCeiling
	if v := os.Getenv("LEAH_BUDGET_DOLLARS"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 {
			c = parsed
		}
	}
	return &Budget{Ceiling: c}
}

type ExceededError struct {
	Spent, Attempted, Ceiling float64
}

func (e *ExceededError) Error() string {
	return fmt.Sprintf("budget exceeded: spent=$%.4f attempted=$%.4f ceiling=$%.4f",
		e.Spent, e.Attempted, e.Ceiling)
}

func (b *Budget) Charge(dollars float64) error {
	b.mu.Lock()
	if b.spent+dollars > b.Ceiling {
		spent, ceiling := b.spent, b.Ceiling
		b.mu.Unlock()
		slog.Error("budget.exceeded",
			"package", "budget",
			"spent_dollars", spent,
			"attempted_dollars", dollars,
			"ceiling_dollars", ceiling,
		)
		return &ExceededError{Spent: spent, Attempted: dollars, Ceiling: ceiling}
	}
	b.spent += dollars
	spent := b.spent
	b.mu.Unlock()
	slog.Info("budget.charge",
		"package", "budget",
		"charged_dollars", dollars,
		"spent_dollars", spent,
		"ceiling_dollars", b.Ceiling,
	)
	return nil
}
