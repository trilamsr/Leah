package reasoner

import (
	"context"
	"errors"
	"testing"

	"github.com/trilam/leah/internal/budget"
)

// routerFake is a Client double that returns a pre-canned CompleteResult and
// records the system+user pair it was called with.
type routerFake struct {
	model string
	cost  float64
	text  string
	calls int
}

func (f *routerFake) Complete(_ context.Context, _, _ string) (CompleteResult, error) {
	f.calls++
	return CompleteResult{Text: f.text, CostUSD: f.cost, Model: f.model}, nil
}

// stubBreaker is a hand-rolled Breaker double so tests pin the state without
// touching the file-backed cost store.
type stubBreaker struct {
	state BreakerState
	spent float64
	cap   float64
}

func (b *stubBreaker) State() BreakerState { return b.state }
func (b *stubBreaker) Spent() float64      { return b.spent }
func (b *stubBreaker) Cap() float64        { return b.cap }
func (b *stubBreaker) Charge(_ string, _ float64) error {
	return nil
}

// Warn state on a non-merge kind swaps to Degraded.
func TestRouter_DegradesAt80pct(t *testing.T) {
	primary := &routerFake{model: "claude-sonnet-4-6", text: "P"}
	degraded := &routerFake{model: "claude-haiku-4-5", text: "D"}
	r := &Router{
		Primary:  primary,
		Degraded: degraded,
		Breaker:  &stubBreaker{state: StateWarn, spent: 42.0, cap: 50.0},
		Kind:     "intent",
	}
	got, err := r.Complete(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Model != "claude-haiku-4-5" {
		t.Errorf("Model = %q, want claude-haiku-4-5", got.Model)
	}
	if primary.calls != 0 {
		t.Errorf("primary calls = %d, want 0", primary.calls)
	}
	if degraded.calls != 1 {
		t.Errorf("degraded calls = %d, want 1", degraded.calls)
	}
}

// Warn state on a merge-kind (reasoner / self-build) keeps Sonnet.
func TestRouter_KeepsSonnetForMergeKinds(t *testing.T) {
	primary := &routerFake{model: "claude-sonnet-4-6", text: "P"}
	degraded := &routerFake{model: "claude-haiku-4-5", text: "D"}
	for _, kind := range []string{"reasoner", "self-build"} {
		t.Run(kind, func(t *testing.T) {
			primary.calls, degraded.calls = 0, 0
			r := &Router{
				Primary:  primary,
				Degraded: degraded,
				Breaker:  &stubBreaker{state: StateWarn, spent: 42.0, cap: 50.0},
				Kind:     kind,
			}
			if _, err := r.Complete(context.Background(), "s", "u"); err != nil {
				t.Fatalf("Complete: %v", err)
			}
			if primary.calls != 1 {
				t.Errorf("primary calls = %d, want 1", primary.calls)
			}
			if degraded.calls != 0 {
				t.Errorf("degraded calls = %d, want 0", degraded.calls)
			}
		})
	}
}

// Deny state surfaces *BreakerDeniedError wrapping ErrCostMonthExceeded
// and never invokes the underlying client.
func TestRouter_DeniesAt100pct(t *testing.T) {
	primary := &routerFake{model: "claude-sonnet-4-6"}
	degraded := &routerFake{model: "claude-haiku-4-5"}
	r := &Router{
		Primary:  primary,
		Degraded: degraded,
		Breaker:  &stubBreaker{state: StateDeny, spent: 50.0, cap: 50.0},
		Kind:     "intent",
	}
	_, err := r.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("Complete deny: want error, got nil")
	}
	if !errors.Is(err, ErrCostMonthExceeded) {
		t.Errorf("err = %v, want errors.Is(ErrCostMonthExceeded)", err)
	}
	var be *BreakerDeniedError
	if !errors.As(err, &be) {
		t.Errorf("err = %v, want errors.As(*BreakerDeniedError)", err)
	}
	if primary.calls != 0 || degraded.calls != 0 {
		t.Errorf("client called under deny: primary=%d degraded=%d", primary.calls, degraded.calls)
	}
}

// Per-process budget (innermost cap) fires before the monthly breaker — the
// per-process ExceededError is returned even when the breaker is in deny.
func TestBudgetPrecedence_EvalCapFiresBeforeBreaker(t *testing.T) {
	primary := &routerFake{model: "claude-sonnet-4-6"}
	degraded := &routerFake{model: "claude-haiku-4-5"}
	b := &budget.Budget{Ceiling: 1.0}
	// Drain to ceiling so any non-zero attempt would trip the per-process gate.
	if err := b.Charge(1.0); err != nil {
		t.Fatalf("seed budget: %v", err)
	}
	r := &Router{
		Primary:    primary,
		Degraded:   degraded,
		Breaker:    &stubBreaker{state: StateDeny, spent: 50.0, cap: 50.0},
		Kind:       "judge",
		ProcessCap: b,
	}
	_, err := r.Complete(context.Background(), "s", "u")
	if err == nil {
		t.Fatal("Complete: want error, got nil")
	}
	var ex *budget.ExceededError
	if !errors.As(err, &ex) {
		t.Errorf("err = %v, want errors.As(*budget.ExceededError) — inner cap should fire first", err)
	}
}
