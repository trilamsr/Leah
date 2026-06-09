// Package budget enforces a per-process dollar ceiling on Reasoner +
// reviewer API spend. Charge serializes on a mutex so concurrent goroutines
// (daemon weekly tick + a CLI invocation) cannot race past the ceiling.
package budget

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
)

// DefaultCeiling is the fallback per-process $ ceiling when
// LEAH_BUDGET_DOLLARS is unset or invalid.
const DefaultCeiling = 5.0

// Budget tracks dollars spent against a fixed Ceiling. Zero value is NOT
// usable — callers MUST construct via New so Ceiling is populated from env.
type Budget struct {
	Ceiling float64
	spent   float64
	mu      sync.Mutex
}

// Spent returns the dollars charged so far, taking the mutex so the read
// observes a consistent value across racing Charge calls.
func (b *Budget) Spent() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}

// New builds a Budget with Ceiling sourced from LEAH_BUDGET_DOLLARS, falling
// back to DefaultCeiling on missing / unparseable / non-positive input.
func New() *Budget {
	c := DefaultCeiling
	if v := os.Getenv("LEAH_BUDGET_DOLLARS"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed > 0 {
			c = parsed
		}
	}
	return &Budget{Ceiling: c}
}

// ExceededError is returned by Charge when the new charge would push
// total spend past Ceiling. Carries all three numbers so the operator can
// see exactly which call tripped the gate.
type ExceededError struct {
	Spent, Attempted, Ceiling float64
}

// Error formats the three dollar amounts at 4-decimal precision so cent-level
// SDK costs are visible.
func (e *ExceededError) Error() string {
	return fmt.Sprintf("budget exceeded: spent=$%.4f attempted=$%.4f ceiling=$%.4f",
		e.Spent, e.Attempted, e.Ceiling)
}

// Charge adds dollars to spend, returning *ExceededError when the new total
// would exceed Ceiling. Mutates spend only on success — failed charges leave
// the budget unchanged so the caller can choose to retry with a smaller ask.
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
