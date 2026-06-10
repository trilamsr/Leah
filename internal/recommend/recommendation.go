package recommend

import (
	"context"
	"errors"
	"time"
)

// Tier governs how an accepted Recommendation reaches the world.
// blocked is the tier that surfaces never propose; included so the Engine
// can reject incoming Apply calls for blast-radius rubric violations even
// when callers hand-construct a Recommendation.
type Tier string

const (
	TierAuto    Tier = "auto"    // applied immediately, no operator gate
	TierConfirm Tier = "confirm" // queued; operator must Accept first
	TierSilent  Tier = "silent"  // logged only — visible in audit / retro
	TierBlocked Tier = "blocked" // never applied; sentinel for rubric guard
)

// Action is the callable the Engine fires on Apply. Kept as a plain
// function (not interface) for W15 — adapters wrap their own state in a
// closure and we avoid an Undo path until W17 introduces the rate-limiter.
type Action func(ctx context.Context) error

// Recommendation is one suggested next action.
//
// W15 surface — Source is a single string (selflearn|operatormodel|patterns);
// the multi-source ranking from spec §5 lands in W18 once feedback writes are
// wired. Reason/ExpiresAt likewise wait for the surface layer (W19).
type Recommendation struct {
	ID         string
	Pattern    string
	Action     Action
	Tier       Tier
	Source     string
	Confidence float64
	CreatedAt  time.Time
}

// Sentinel errors. Tests assert via errors.Is.
var (
	// ErrConfirmRequired fires when Apply receives a confirm-tier rec that
	// has not yet been Accept-ed. Surfaces are expected to call Accept first.
	ErrConfirmRequired = errors.New("recommend: confirm tier requires prior Accept")

	// ErrBlocked fires when Apply receives a blocked-tier rec. Blocked
	// actions never reach the operator surface but the Engine still
	// guards Apply in case a caller constructs one by hand.
	ErrBlocked = errors.New("recommend: blocked tier never applied")

	// ErrNotFound fires when Accept/Reject reference an unknown ID.
	ErrNotFound = errors.New("recommend: unknown recommendation id")
)

// Engine is the loop entrypoint. One singleton per daemon process.
//
// W15 ships a hermetic in-memory implementation (see NewMemoryEngine);
// W16 swaps the backing store for SQLite without changing this interface.
type Engine interface {
	Propose(ctx context.Context) ([]Recommendation, error)
	Accept(ctx context.Context, id string) error
	Reject(ctx context.Context, id string) error
	Apply(ctx context.Context, rec Recommendation) error
}
