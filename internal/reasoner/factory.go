package reasoner

import (
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/obs"
)

// degradedModel is the Haiku snapshot Router swaps in under warn (spec §7.2).
// Override via LEAH_DEGRADED_MODEL when a newer Haiku ships.
const degradedModel = "claude-haiku-4-5"

// NewRoutedClient is the production wiring for a `kind`-bound LLM call site:
// AnthropicClient → Router(Breaker, ProcessCap) → CompleteResult. Single
// constructor so the four cmd/leah call-sites (ask, ship, recall, selfbuild)
// pick up breaker + degrade + counter wiring in one line. Breaker may be nil
// (older test paths); Router then no-ops the gate.
func NewRoutedClient(kind string, b Breaker, processCap *budget.Budget, reg *obs.Registry) (*Router, error) {
	primary, err := NewAnthropicClient()
	if err != nil {
		return nil, err
	}
	primary.Registry = reg
	degraded, err := NewAnthropicClientWithModel(degradedModel)
	if err != nil {
		return nil, err
	}
	degraded.Registry = reg
	return &Router{
		Primary:       primary,
		Degraded:      degraded,
		Breaker:       b,
		Kind:          kind,
		ProcessCap:    processCap,
		PrimaryModel:  primary.Model(),
		DegradedModel: degraded.Model(),
		Registry:      reg,
	}, nil
}
