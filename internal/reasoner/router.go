package reasoner

import (
	"context"
	"errors"
	"fmt"

	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/obs"
)

// BreakerState is the spec §7.1 ok/warn/deny tri-state. Published on the
// `leah_cost_breaker_state` gauge as 0/1/2.
type BreakerState int

const (
	StateOK BreakerState = iota
	StateWarn
	StateDeny
)

// Breaker is the runtime gate the Router consults pre-call. The interface
// keeps the Router unit-testable without the file-backed cost store.
type Breaker interface {
	State() BreakerState
	Spent() float64
	Cap() float64
	Charge(kind string, dollars float64) error
}

// ErrCostMonthExceeded is the sentinel for `breaker_denied` audit-row
// outcomes; errors.Is against *BreakerDeniedError returns true.
var ErrCostMonthExceeded = errors.New("cost-month: monthly cap exceeded")

// BreakerDeniedError carries the dollar trio so the operator-facing
// message names the exact cap that fired.
type BreakerDeniedError struct {
	Kind  string
	Spent float64
	Cap   float64
}

func (e *BreakerDeniedError) Error() string {
	return fmt.Sprintf("cost-month breaker denied %s: spent=$%.2f cap=$%.2f", e.Kind, e.Spent, e.Cap)
}

// Is wires errors.Is into the typed error so callers can route on the
// sentinel without depending on the struct.
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
// Degraded per breaker state. ProcessCap, when non-nil, fires FIRST so the
// innermost-wins precedence from spec §7.4 holds.
type Router struct {
	Primary    Client
	Degraded   Client
	Breaker    Breaker
	Kind       string
	ProcessCap *budget.Budget

	// PrimaryModel + DegradedModel surface the model strings on the
	// `leah_cost_breaker_degrade_total{from_model,to_model}` counter
	// (spec §3) — Client has no Model() method, so the factory threads
	// them in at construction.
	PrimaryModel  string
	DegradedModel string

	// Registry, when non-nil, receives the degrade counter increment.
	Registry *telemetry.Registry
}

// Complete is the Router's analogue of Client.Complete; same signature
// so a Router is itself a Client.
func (r *Router) Complete(ctx context.Context, system, user string) (CompleteResult, error) {
	// Innermost cap (per-process) fires first so even a breaker-deny gets
	// shadowed by an in-flight budget overrun.
	if r.ProcessCap != nil && r.ProcessCap.Spent() >= r.ProcessCap.Ceiling {
		return CompleteResult{}, &budget.ExceededError{
			Spent:     r.ProcessCap.Spent(),
			Ceiling:   r.ProcessCap.Ceiling,
			Attempted: 0,
		}
	}
	// Cache State() — Breaker.State() takes the store mutex, so reading
	// twice (deny branch + warn branch) is a needless lock hop.
	var state BreakerState
	if r.Breaker != nil {
		state = r.Breaker.State()
	}
	if state == StateDeny {
		return CompleteResult{}, &BreakerDeniedError{
			Kind:  r.Kind,
			Spent: r.Breaker.Spent(),
			Cap:   r.Breaker.Cap(),
		}
	}
	c := r.Primary
	if state == StateWarn && !mergeKind(r.Kind) && r.Degraded != nil {
		c = r.Degraded
		r.emitDegrade()
	}
	res, err := c.Complete(ctx, system, user)
	if err != nil {
		return res, err
	}
	if r.Breaker != nil {
		// Charge errors are non-fatal — the LLM already executed and the
		// operator owes the dollars regardless of persist success.
		_ = r.Breaker.Charge(r.Kind, res.CostUSD)
	}
	return res, nil
}

// emitDegrade increments the §3 `leah_cost_breaker_degrade_total` counter
// on every warn-state swap. Bare counter (not gauge transition) is what
// the spec mandates — each non-merge call under warn is one degrade event.
func (r *Router) emitDegrade() {
	if r.Registry == nil {
		return
	}
	r.Registry.Counter("leah_cost_breaker_degrade_total").Inc(map[string]string{
		"from_model": r.PrimaryModel,
		"to_model":   r.DegradedModel,
	})
}
