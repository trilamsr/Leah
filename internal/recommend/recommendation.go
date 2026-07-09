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
// function (not interface): adapters wrap their own state in a closure and
// there is no Undo path yet.
type Action func(ctx context.Context) error

// Recommendation is one suggested next action. Source is a single string
// (selflearn|operatormodel|patterns); multi-source ranking, Reason, and
// ExpiresAt are follow-ups.
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
// NewMemoryEngine is the hermetic in-memory implementation; SQLiteEngine
// swaps the backing store without changing this interface.
type Engine interface {
	Propose(ctx context.Context) ([]Recommendation, error)
	Accept(ctx context.Context, id string) error
	Reject(ctx context.Context, id string) error
	Apply(ctx context.Context, rec Recommendation) error
}

// Signal is the event-driven trigger that replaced the 60s daemon-tick poll.
// Context is operatormodel.ctxmgr.Current() at fire time; Detail carries the
// bundle ID / event UUID / per-kind payload.
type Signal struct {
	Kind    string
	Context string
	At      time.Time
	Detail  string
}

// SignalMatcher emits zero-or-more Recommendations for one Signal. The
// engine owns dedup + debounce + persistence — matchers are stateless.
type SignalMatcher interface {
	Match(ctx context.Context, sig Signal) ([]Recommendation, error)
}

// SignalEngine is Engine extended with event-driven Propose. Implementors
// debounce per-pattern (≥30s between same-Pattern fires per Wave-9 V8).
type SignalEngine interface {
	Engine
	OnSignal(ctx context.Context, sig Signal) ([]Recommendation, error)
}

// SignalDebounceWindow is the minimum gap between Propose-fires of the
// same Pattern. NSWorkspace bursts 10-30 push events/min; 30s lets one
// focus-pattern surface once a transition without saturating downstream.
const SignalDebounceWindow = 30 * time.Second
