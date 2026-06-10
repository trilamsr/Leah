package recommend

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestFeedback_Accept_BoostsConfidence — Accept on a pattern lifts it above
// a peer with equal seed-confidence on the next RankedPropose call.
func TestFeedback_Accept_BoostsConfidence(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)

	low := Recommendation{ID: "low", Pattern: "p_low", Tier: TierConfirm, Source: "selflearn", Confidence: 0.5, CreatedAt: time.Now()}
	high := Recommendation{ID: "high", Pattern: "p_high", Tier: TierConfirm, Source: "selflearn", Confidence: 0.5, CreatedAt: time.Now()}
	eng.Seed(low)
	eng.Seed(high)

	if err := eng.Feedback(context.Background(), "high", FeedbackAccept); err != nil {
		t.Fatalf("feedback accept: %v", err)
	}
	// Re-seed (Accept doesn't remove from pending in this hermetic engine).
	eng.Seed(Recommendation{ID: "low2", Pattern: "p_low", Tier: TierConfirm, Source: "selflearn", Confidence: 0.5, CreatedAt: time.Now()})
	eng.Seed(Recommendation{ID: "high2", Pattern: "p_high", Tier: TierConfirm, Source: "selflearn", Confidence: 0.5, CreatedAt: time.Now()})

	ranked, err := eng.RankedPropose(context.Background())
	if err != nil {
		t.Fatalf("ranked propose: %v", err)
	}
	if len(ranked) == 0 || ranked[0].Pattern != "p_high" {
		t.Fatalf("want p_high first after Accept, got %v", patternsOf(ranked))
	}
}

// TestFeedback_Reject_DegradesConfidence — Reject demotes a pattern below
// a peer with equal seed-confidence.
func TestFeedback_Reject_DegradesConfidence(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)

	a := Recommendation{ID: "a", Pattern: "p_a", Tier: TierConfirm, Source: "selflearn", Confidence: 0.5, CreatedAt: time.Now()}
	b := Recommendation{ID: "b", Pattern: "p_b", Tier: TierConfirm, Source: "selflearn", Confidence: 0.5, CreatedAt: time.Now()}
	eng.Seed(a)
	eng.Seed(b)

	if err := eng.Feedback(context.Background(), "a", FeedbackReject); err != nil {
		t.Fatalf("feedback reject: %v", err)
	}

	ranked, err := eng.RankedPropose(context.Background())
	if err != nil {
		t.Fatalf("ranked propose: %v", err)
	}
	if len(ranked) == 0 || ranked[0].Pattern != "p_b" {
		t.Fatalf("want p_b first after Reject of p_a, got %v", patternsOf(ranked))
	}
}

// TestFeedback_DecayHalfLife30d — a 30-day-old behavioral Ignore signal
// decays to ~50% of its initial magnitude (spec §9 behavioral half-life).
func TestFeedback_DecayHalfLife30d(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	thirtyDaysAgo := now.Add(-30 * 24 * time.Hour)

	full := decayedMagnitude(1.0, now, now, behavioralHalflifeDays)
	if !approxEq(full, 1.0, 1e-9) {
		t.Fatalf("zero-age signal should be 1.0, got %f", full)
	}
	half := decayedMagnitude(1.0, thirtyDaysAgo, now, behavioralHalflifeDays)
	if !approxEq(half, 0.5, 0.01) {
		t.Fatalf("30d-old behavioral signal should decay to ~0.5, got %f", half)
	}

	// Explicit override (7d half-life) at 7 days should also be ~0.5.
	sevenDaysAgo := now.Add(-7 * 24 * time.Hour)
	overrideHalf := decayedMagnitude(1.0, sevenDaysAgo, now, feedbackHalflifeDays)
	if !approxEq(overrideHalf, 0.5, 0.01) {
		t.Fatalf("7d-old override should decay to ~0.5, got %f", overrideHalf)
	}
}

// TestFeedback_Ignore_LightDampen — Ignore registers as a small dampener,
// less than a Reject would apply.
func TestFeedback_Ignore_LightDampen(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)

	rec := Recommendation{ID: "x", Pattern: "p_x", Tier: TierConfirm, Source: "selflearn", Confidence: 0.5, CreatedAt: time.Now()}
	eng.Seed(rec)

	rejectDelta := signalDelta(FeedbackReject)
	ignoreDelta := signalDelta(FeedbackIgnore)
	if ignoreDelta >= 0 || rejectDelta >= 0 {
		t.Fatalf("both Reject and Ignore must be negative; got reject=%f ignore=%f", rejectDelta, ignoreDelta)
	}
	if -ignoreDelta >= -rejectDelta {
		t.Fatalf("Ignore must be lighter than Reject; got reject=%f ignore=%f", rejectDelta, ignoreDelta)
	}

	if err := eng.Feedback(context.Background(), "x", FeedbackIgnore); err != nil {
		t.Fatalf("feedback ignore: %v", err)
	}
}

// TestFeedback_LogsAuditRow — Feedback writes a recommendation_feedback row.
func TestFeedback_LogsAuditRow(t *testing.T) {
	logger, path := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	eng.Seed(Recommendation{ID: "y", Pattern: "p_y", Tier: TierConfirm, Source: "selflearn", Confidence: 0.6, CreatedAt: time.Now()})

	if err := eng.Feedback(context.Background(), "y", FeedbackAccept); err != nil {
		t.Fatalf("feedback: %v", err)
	}
	if !auditContainsKind(t, path, "recommendation_feedback") {
		t.Fatalf("audit missing recommendation_feedback")
	}
}

// TestFeedback_UnknownID_Errors — Feedback on an unknown rec returns ErrNotFound.
func TestFeedback_UnknownID_Errors(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	err := eng.Feedback(context.Background(), "nope", FeedbackAccept)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// TestForget_PatternID_Removes — Forget drops the pattern from the pending
// pool; subsequent Propose excludes it.
func TestForget_PatternID_Removes(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	eng.Seed(Recommendation{ID: "k1", Pattern: "doomed", Tier: TierConfirm, Source: "selflearn", Confidence: 0.7, CreatedAt: time.Now()})
	eng.Seed(Recommendation{ID: "k2", Pattern: "kept", Tier: TierConfirm, Source: "selflearn", Confidence: 0.7, CreatedAt: time.Now()})

	if err := eng.Forget(context.Background(), "doomed"); err != nil {
		t.Fatalf("forget: %v", err)
	}
	recs, err := eng.Propose(context.Background())
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	for _, r := range recs {
		if r.Pattern == "doomed" {
			t.Fatalf("doomed pattern still present after Forget")
		}
	}
	if !containsPattern(recs, "kept") {
		t.Fatalf("kept pattern unexpectedly dropped: %v", patternsOf(recs))
	}
}

// TestForget_All_RemovesAll — Forget("all") wipes every pending rec.
func TestForget_All_RemovesAll(t *testing.T) {
	logger, path := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	eng.Seed(Recommendation{ID: "k1", Pattern: "p1", Tier: TierConfirm, Source: "selflearn", Confidence: 0.7, CreatedAt: time.Now()})
	eng.Seed(Recommendation{ID: "k2", Pattern: "p2", Tier: TierConfirm, Source: "selflearn", Confidence: 0.5, CreatedAt: time.Now()})

	if err := eng.Forget(context.Background(), ForgetAll); err != nil {
		t.Fatalf("forget all: %v", err)
	}
	recs, err := eng.Propose(context.Background())
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("want empty after Forget(all), got %v", patternsOf(recs))
	}
	if !auditContainsKind(t, path, "recommendation_forget") {
		t.Fatalf("audit missing recommendation_forget")
	}
}

// TestForget_PostForget_PatternStaysForgotten — a re-Seed with the same
// pattern after Forget is dropped (30d quarantine per spec §11).
func TestForget_PostForget_PatternStaysForgotten(t *testing.T) {
	logger, _ := newAuditLogger(t)
	eng := NewMemoryEngine(logger)
	eng.Seed(Recommendation{ID: "k", Pattern: "ghost", Tier: TierConfirm, Source: "selflearn", Confidence: 0.7, CreatedAt: time.Now()})
	if err := eng.Forget(context.Background(), "ghost"); err != nil {
		t.Fatalf("forget: %v", err)
	}

	// Re-seed the same pattern.
	eng.Seed(Recommendation{ID: "k_again", Pattern: "ghost", Tier: TierConfirm, Source: "selflearn", Confidence: 0.9, CreatedAt: time.Now()})

	if !eng.Quarantined("ghost") {
		t.Fatalf("Quarantined(ghost) want true after Forget")
	}
	recs, err := eng.RankedPropose(context.Background())
	if err != nil {
		t.Fatalf("ranked propose: %v", err)
	}
	for _, r := range recs {
		if r.Pattern == "ghost" {
			t.Fatalf("forgotten pattern resurfaced via RankedPropose: %+v", r)
		}
	}
}

func patternsOf(recs []Recommendation) []string {
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.Pattern)
	}
	return out
}

func containsPattern(recs []Recommendation, p string) bool {
	for _, r := range recs {
		if r.Pattern == p {
			return true
		}
	}
	return false
}

func approxEq(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}
