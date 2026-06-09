package budget

import (
	"fmt"
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
	defer b.mu.Unlock()
	if b.spent+dollars > b.Ceiling {
		return &ExceededError{Spent: b.spent, Attempted: dollars, Ceiling: b.Ceiling}
	}
	b.spent += dollars
	return nil
}
