package reasoner

import (
	"context"
	"errors"
	"fmt"

	"github.com/trilam/leah/internal/budget"
)

// BreakerState is the spec §7.1 ok/warn/deny tri-state. Published on the
// `leah_cost_breaker_state` gauge as 0/1/2.
type BreakerState int

const (
	// StateOK keeps every call on Sonnet.
	StateOK BreakerState = iota
	// StateWarn (≥80% of cap) swaps non-merge kinds to Haiku.
	StateWarn
	// StateDeny (≥100% of cap) refuses every kind absent an attested override.
	StateDeny
)

// Breaker is the runtime gate the Router consults pre-call. Implemented
// in production by a struct wrapping the costmonth.Store; the interface
// keeps the Router unit-testable without the file-backed store.
type Breaker interface {
	State() BreakerState
	Spent() float64
	Cap() float64
	Charge(kind string, dollars float64) error
}

// ErrCostMonthExceeded is the sentinel for `breaker_denied` audit-row
// outcomes. errors.Is against BreakerDeniedError returns true so callers
// distinguish breaker-denial from per-process budget overruns.
var ErrCostMonthExceeded = errors.New("cost-month: monthly cap exceeded")

// BreakerDeniedError carries the dollar trio so the operator-facing
// error message names the exact cap that fired.
type BreakerDeniedError struct {
	Kind  string
	Spent float64
	Cap   float64
}

func (e *BreakerDeniedError) Error() string {
	return fmt.Sprintf("cost-month breaker denied %s: spent=$%.2f cap=$%.2f", e.Kind, e.Spent, e.Cap)
}

// Is wires errors.Is(err, ErrCostMonthExceeded) into the typed error so
// callers can route on the sentinel without depending on the struct.
func (e *BreakerDeniedError) Is(target error) bool { return target == ErrCostMonthExceeded }

// mergeKind reports whether kind is operator-facing or merge-gating per
// spec §7.2 — those kinds stay on Sonnet even in warn state.
func mergeKind(kind string) bool {
	switch kind {
	case "reasoner", "self-build":
		return true
	}
	return false
}

// Router gates LLM calls through the cost breaker, choosing Primary vs
// Degraded per breaker state and short-circuiting on deny. ProcessCap,
// when non-nil, is consulted FIRST so the innermost-wins precedence
// from spec §7.4 holds.
type Router struct {
	Primary    Client
	Degraded   Client
	Breaker    Breaker
	Kind       string
	ProcessCap *budget.Budget
}

// Complete is the Router's analogue of Client.Complete; same signature
// as Client so a Router is a Client.
func (r *Router) Complete(ctx context.Context, system, user string) (CompleteResult, error) {
	// Innermost cap (per-process) fires first; even a breaker-deny gets
	// shadowed by an in-flight budget overrun so the operator sees the
	// tighter wall in their error.
	if r.ProcessCap != nil && r.ProcessCap.Spent() >= r.ProcessCap.Ceiling {
		return CompleteResult{}, &budget.ExceededError{
			Spent:    r.ProcessCap.Spent(),
			Ceiling:  r.ProcessCap.Ceiling,
			Attempted: 0,
		}
	}
	if r.Breaker != nil && r.Breaker.State() == StateDeny {
		return CompleteResult{}, &BreakerDeniedError{
			Kind:  r.Kind,
			Spent: r.Breaker.Spent(),
			Cap:   r.Breaker.Cap(),
		}
	}
	c := r.Primary
	if r.Breaker != nil && r.Breaker.State() == StateWarn && !mergeKind(r.Kind) && r.Degraded != nil {
		c = r.Degraded
	}
	res, err := c.Complete(ctx, system, user)
	if err != nil {
		return res, err
	}
	if r.Breaker != nil {
		// Charge errors are non-fatal for the call — the LLM has
		// already executed and the operator owes the dollars
		// regardless of whether the persist succeeded.
		_ = r.Breaker.Charge(r.Kind, res.CostUSD)
	}
	return res, nil
}
