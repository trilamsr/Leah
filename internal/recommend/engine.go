package recommend

import (
	"context"
	"sync"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/obs"
)

// MemoryEngine is the W15 hermetic implementation: pending recs and
// acceptance state live in maps, audit rows go to the injected Logger.
// W16 replaces this with a SQLite-backed engine — same Engine surface.
type MemoryEngine struct {
	audit *audit.Logger

	mu       sync.Mutex
	pending  map[string]Recommendation // id → rec, awaiting decision
	accepted map[string]bool           // id → true once Accept lands
}

// NewMemoryEngine constructs an empty engine. The audit Logger may be nil
// for callers that haven't wired audit yet, in which case Accept/Reject/Apply
// silently skip the row — keeps unit-test setup minimal.
func NewMemoryEngine(logger *audit.Logger) *MemoryEngine {
	return &MemoryEngine{
		audit:    logger,
		pending:  make(map[string]Recommendation),
		accepted: make(map[string]bool),
	}
}

// Seed inserts a Recommendation into the pending pool. Provided for tests
// and for surface-layer callers that compose Engine with an external
// pattern-matcher; W18 replaces hand-seeding with the propose-loop. Publishes
// a recommendation.propose event so live HUD widgets (V2/W87) update without
// the 15s XHR poll.
func (e *MemoryEngine) Seed(rec Recommendation) {
	e.mu.Lock()
	e.pending[rec.ID] = rec
	e.mu.Unlock()
	obs.Publish(obs.Event{
		TS:      time.Now().UTC(),
		Kind:    "recommendation.propose",
		Actor:   "recommend.engine",
		Outcome: "ok",
		RefID:   rec.ID,
		Detail:  obs.SafeDetail(rec.Pattern),
		Payload: rec,
	})
}

// Propose returns the queued pending recommendations. Empty in W15 because
// the pattern-matcher landing in W18 has not been wired yet.
func (e *MemoryEngine) Propose(ctx context.Context) ([]Recommendation, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Recommendation, 0, len(e.pending))
	for _, r := range e.pending {
		out = append(out, r)
	}
	return out, nil
}

// Accept marks the rec ID approved for Apply and audits the decision.
func (e *MemoryEngine) Accept(ctx context.Context, id string) error {
	e.mu.Lock()
	rec, ok := e.pending[id]
	if !ok {
		e.mu.Unlock()
		return ErrNotFound
	}
	e.accepted[id] = true
	e.mu.Unlock()
	return e.logDecision("recommendation_accept", rec, "")
}

// Reject removes the rec from pending and audits the decision. The reason
// detail field stays empty in W15 — surfaces gain a reason argument in W19.
func (e *MemoryEngine) Reject(ctx context.Context, id string) error {
	e.mu.Lock()
	rec, ok := e.pending[id]
	if !ok {
		e.mu.Unlock()
		return ErrNotFound
	}
	delete(e.pending, id)
	delete(e.accepted, id)
	e.mu.Unlock()
	return e.logDecision("recommendation_reject", rec, "")
}

// Apply runs the rec's Action subject to the tier rubric.
//
// Tier semantics (spec §6):
//   - auto    → fire immediately.
//   - confirm → require prior Accept; otherwise ErrConfirmRequired.
//   - silent  → no-op success; the audit row is the whole payload.
//   - blocked → never fire; ErrBlocked.
func (e *MemoryEngine) Apply(ctx context.Context, rec Recommendation) error {
	switch rec.Tier {
	case TierBlocked:
		_ = e.logDecision("recommendation_apply", rec, "blocked")
		return ErrBlocked
	case TierSilent:
		return e.logDecision("recommendation_apply", rec, "silent")
	case TierConfirm:
		e.mu.Lock()
		ok := e.accepted[rec.ID]
		e.mu.Unlock()
		if !ok {
			return ErrConfirmRequired
		}
	case TierAuto:
		// fallthrough to invoke Action below.
	}

	if rec.Action != nil {
		if err := rec.Action(ctx); err != nil {
			_ = e.logDecision("recommendation_apply", rec, "failed")
			return err
		}
	}
	return e.logDecision("recommendation_apply", rec, "success")
}

// logDecision appends one audit row keyed by Kind. Nil Logger is a no-op
// so unit tests that don't care about audit can skip wiring it.
func (e *MemoryEngine) logDecision(kind string, rec Recommendation, outcome string) error {
	if e.audit == nil {
		return nil
	}
	return e.audit.Append(audit.Entry{
		Kind:     kind,
		ArgsHash: rec.ID,
		Outcome:  outcome,
		Detail:   rec.Pattern,
	})
}
