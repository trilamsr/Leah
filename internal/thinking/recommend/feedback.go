package recommend

import (
	"context"
	"math"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/trilam/leah/internal/audit"
)

// FeedbackKind enumerates the operator-side signals the engine consumes
// to nudge confidence. Spec §8 — Accept/Reject are explicit overrides,
// Ignore fires when ExpiresAt elapses without a decision.
type FeedbackKind string

const (
	FeedbackAccept FeedbackKind = "accept"
	FeedbackReject FeedbackKind = "reject"
	FeedbackIgnore FeedbackKind = "ignore"
)

// ForgetAll is the sentinel argument that wipes every pending pattern +
// every feedback signal for an engine. Mirrors `leah forget all`.
const ForgetAll = "all"

// Half-life constants (spec §9). Env-overridable so an operator who finds
// 30d too lazy or 7d too jumpy can tune without a rebuild.
const (
	defaultBehavioralHalflifeDays = 30.0
	defaultFeedbackHalflifeDays   = 7.0
)

// Per-signal magnitude (spec §8). Accept reinforces strongly; Reject is
// half that weight (silence ≠ no); Ignore is the lightest dampener.
const (
	signalAccept = +1.0
	signalReject = -0.5
	signalIgnore = -0.1
)

// behavioralHalflifeDays / feedbackHalflifeDays resolve env once at process
// start. Tests don't override — the math primitives below take halflife as
// an arg so unit tests stay deterministic.
var (
	behavioralHalflifeDays = envFloat("LEAH_RECOMMEND_BEHAVIORAL_HALFLIFE_DAYS", defaultBehavioralHalflifeDays)
	feedbackHalflifeDays   = envFloat("LEAH_RECOMMEND_FEEDBACK_HALFLIFE_DAYS", defaultFeedbackHalflifeDays)
)

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return def
}

// feedbackEvent is one operator decision attached to a pattern key. We
// store kind+timestamp rather than the rec ID — pattern is the identity
// the next Propose ranks against (rec IDs are ephemeral; patterns persist).
type feedbackEvent struct {
	kind FeedbackKind
	at   time.Time
}

// feedbackStore is the per-engine signal ledger. SQLiteEngine reads the
// same shape directly out of the feedback table; MemoryEngine carries the
// store as a field.
type feedbackStore struct {
	mu        sync.Mutex
	events    map[string][]feedbackEvent // pattern → events
	forgotten map[string]time.Time       // pattern → quarantine-until
}

func newFeedbackStore() *feedbackStore {
	return &feedbackStore{
		events:    make(map[string][]feedbackEvent),
		forgotten: make(map[string]time.Time),
	}
}

// nowUTC is the engine's wall-clock seam. Overridable per call by passing
// a context-attached value isn't worth it for v1 — tests inject through
// engineNow which is package-private (no symbol surface change).
var engineNow = func() time.Time { return time.Now().UTC() }

// Feedback records an Accept|Reject|Ignore decision on the rec identified
// by id and audits the row. The pattern (not the rec ID) is the durable
// key — the next RankedPropose sees the boost/dampener regardless of
// which rec instance carries the pattern.
func (e *MemoryEngine) Feedback(ctx context.Context, id string, kind FeedbackKind) error {
	e.mu.Lock()
	rec, ok := e.pending[id]
	e.mu.Unlock()
	if !ok {
		return ErrNotFound
	}

	fs := e.feedback
	fs.mu.Lock()
	fs.events[rec.Pattern] = append(fs.events[rec.Pattern], feedbackEvent{kind: kind, at: engineNow()})
	fs.mu.Unlock()

	return e.logDecision("recommendation_feedback", rec, string(kind))
}

// RankedPropose returns the pending pool sorted by decay-adjusted
// confidence (descending). Pure read — Propose itself is untouched.
func (e *MemoryEngine) RankedPropose(ctx context.Context) ([]Recommendation, error) {
	recs, err := e.Propose(ctx)
	if err != nil {
		return nil, err
	}
	fs := e.feedback
	now := engineNow()

	adjusted := make([]Recommendation, 0, len(recs))
	for _, r := range recs {
		if e.Quarantined(r.Pattern) {
			continue
		}
		r.Confidence = r.Confidence + patternAdjustment(fs, r.Pattern, now)
		adjusted = append(adjusted, r)
	}
	sort.SliceStable(adjusted, func(i, j int) bool {
		return adjusted[i].Confidence > adjusted[j].Confidence
	})
	return adjusted, nil
}

// patternAdjustment sums the decay-weighted signals on pattern. Explicit
// overrides (Accept/Reject) use the 7d half-life; the lighter Ignore signal
// rides the 30d behavioral half-life since it's an observation, not a
// stated preference.
func patternAdjustment(fs *feedbackStore, pattern string, now time.Time) float64 {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	var sum float64
	for _, ev := range fs.events[pattern] {
		hl := feedbackHalflifeDays
		if ev.kind == FeedbackIgnore {
			hl = behavioralHalflifeDays
		}
		sum += decayedMagnitude(signalDelta(ev.kind), ev.at, now, hl)
	}
	return sum
}

// signalDelta is the unsigned-magnitude → signed-delta map for a kind.
// Exported via test helper signature only (lowercase) — package-private.
func signalDelta(k FeedbackKind) float64 {
	switch k {
	case FeedbackAccept:
		return signalAccept
	case FeedbackReject:
		return signalReject
	case FeedbackIgnore:
		return signalIgnore
	}
	return 0
}

// decayedMagnitude returns initial * 2^(-age/halflife). Negative initial
// values stay negative (Reject/Ignore); zero is a no-op.
func decayedMagnitude(initial float64, at, now time.Time, halflifeDays float64) float64 {
	if halflifeDays <= 0 {
		return initial
	}
	ageDays := now.Sub(at).Hours() / 24.0
	if ageDays < 0 {
		ageDays = 0
	}
	return initial * math.Pow(2, -ageDays/halflifeDays)
}

// Forget drops a pattern (or all patterns when patternOrAll == ForgetAll)
// from the pending pool, clears its feedback history, and quarantines the
// pattern from future Seeds for 30 days. Writes one recommendation_forget
// audit row — Detail carries the patternID or "all".
//
// Callers (cmd/leah/forget.go) are responsible for operator attestation
// before invoking this — the engine treats Forget as already-authorized
// to keep the dependency arrow one-way (recommend → audit only).
func (e *MemoryEngine) Forget(ctx context.Context, patternOrAll string) error {
	fs := e.feedback

	e.mu.Lock()
	if patternOrAll == ForgetAll {
		e.pending = make(map[string]Recommendation)
		e.accepted = make(map[string]bool)
	} else {
		for id, rec := range e.pending {
			if rec.Pattern == patternOrAll {
				delete(e.pending, id)
				delete(e.accepted, id)
			}
		}
	}
	e.mu.Unlock()

	fs.mu.Lock()
	quarantineUntil := engineNow().Add(30 * 24 * time.Hour)
	if patternOrAll == ForgetAll {
		fs.events = make(map[string][]feedbackEvent)
		// Quarantine every observed pattern — and any pattern that lands
		// next via Seed is checked via the Quarantined predicate.
		for pat := range fs.forgotten {
			fs.forgotten[pat] = quarantineUntil
		}
	} else {
		delete(fs.events, patternOrAll)
		fs.forgotten[patternOrAll] = quarantineUntil
	}
	fs.mu.Unlock()

	if e.audit != nil {
		return e.audit.Append(audit.Entry{
			Kind:    "recommendation_forget",
			Outcome: "success",
			Detail:  patternOrAll,
		})
	}
	return nil
}

// Quarantined reports whether pattern is currently inside its 30-day
// post-Forget cooldown. Surfaces (Propose callers) wrap Seed with this
// to drop resurfaced rows; tests use it to assert §11 behavior.
func (e *MemoryEngine) Quarantined(pattern string) bool {
	fs := e.feedback
	fs.mu.Lock()
	defer fs.mu.Unlock()
	until, ok := fs.forgotten[pattern]
	if !ok {
		return false
	}
	if engineNow().After(until) {
		delete(fs.forgotten, pattern)
		return false
	}
	return true
}
