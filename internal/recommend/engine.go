package recommend

import (
	"context"
	"sync"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/obs"
)

// MemoryEngine is the hermetic implementation: pending recs and acceptance
// state live in maps, audit rows go to the injected Logger. SQLiteEngine
// replaces this for persistence — same Engine surface.
type MemoryEngine struct {
	audit *audit.Logger

	mu       sync.Mutex
	pending  map[string]Recommendation // id → rec, awaiting decision
	accepted map[string]bool           // id → true once Accept lands
	feedback *feedbackStore            // per-engine signal ledger

	matchers    []SignalMatcher
	lastFiredAt map[string]time.Time // pattern → last OnSignal fire
	now         func() time.Time     // test seam; default time.Now().UTC()
}

// NewMemoryEngine constructs an empty engine. The audit Logger may be nil
// for callers that haven't wired audit yet, in which case Accept/Reject/Apply
// silently skip the row — keeps unit-test setup minimal.
func NewMemoryEngine(logger *audit.Logger) *MemoryEngine {
	return &MemoryEngine{
		audit:       logger,
		pending:     make(map[string]Recommendation),
		accepted:    make(map[string]bool),
		feedback:    newFeedbackStore(),
		lastFiredAt: make(map[string]time.Time),
		now:         func() time.Time { return time.Now().UTC() },
	}
}

// RegisterMatcher attaches m to the engine's OnSignal fan-out. Safe to
// call from init; engine doesn't dedup matchers — caller owns identity.
func (e *MemoryEngine) RegisterMatcher(m SignalMatcher) {
	e.mu.Lock()
	e.matchers = append(e.matchers, m)
	e.mu.Unlock()
}

// OnSignal fans sig out to every registered matcher, then Seeds + returns
// the resulting Recommendations. Per-pattern debounce: a Recommendation
// whose Pattern fired within SignalDebounceWindow is silently dropped so a
// noisy upstream (NSWorkspace push, EventKit) cannot saturate downstream.
func (e *MemoryEngine) OnSignal(ctx context.Context, sig Signal) ([]Recommendation, error) {
	e.mu.Lock()
	matchers := make([]SignalMatcher, len(e.matchers))
	copy(matchers, e.matchers)
	e.mu.Unlock()

	var out []Recommendation
	now := e.now()
	for _, m := range matchers {
		recs, err := m.Match(ctx, sig)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			e.mu.Lock()
			last, seen := e.lastFiredAt[r.Pattern]
			if seen && now.Sub(last) < SignalDebounceWindow {
				e.mu.Unlock()
				continue
			}
			e.lastFiredAt[r.Pattern] = now
			e.mu.Unlock()
			e.Seed(r)
			out = append(out, r)
		}
	}
	return out, nil
}

// Seed inserts a Recommendation into the pending pool. Provided for tests
// and for surface-layer callers that compose Engine with an external
// pattern-matcher. Publishes a recommendation.propose event so live HUD
// widgets update without the 15s XHR poll.
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

// Propose returns the queued pending recommendations.
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
// detail field is empty pending a surface-layer reason argument.
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
