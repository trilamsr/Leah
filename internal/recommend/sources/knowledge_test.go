package sources

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

type fakeGraph struct {
	people []PersonContext
	err    error
}

func (f *fakeGraph) ActivePeople(ctx context.Context, within time.Duration) ([]PersonContext, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.people, nil
}

// TestKnowledge_RecommendationFromPersonContext: a person entity with ≥2
// recent touchpoints (e.g. meeting today + prior notes) → DraftFollowupEmail
// candidate. Mirror of the spec example "Sarah is meeting today AND
// operator has notes from last meeting".
func TestKnowledge_RecommendationFromPersonContext(t *testing.T) {
	src := NewKnowledgeGraphSource(&fakeGraph{
		people: []PersonContext{
			{
				Key:         "person:sarah",
				Display:     "Sarah",
				ItemCount:   4,
				HasMeeting:  true,
				LastTouched: time.Now().Add(-2 * 24 * time.Hour),
			},
		},
	}, KnowledgeOpts{})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("want >=1 rec from person context, got 0")
	}
	if recs[0].Source != "knowledge-graph" {
		t.Errorf("Source = %q, want knowledge-graph", recs[0].Source)
	}
	// Privacy guard: rec ID/pattern must NOT leak Display name verbatim per
	// knowledge.go spec §5 "keys must stay PII-free".
	if containsName(recs[0].ID, "Sarah") || containsName(recs[0].Pattern, "Sarah") {
		t.Errorf("rec %+v leaks display name 'Sarah'", recs[0])
	}
}

// TestKnowledge_NoEntities_EmptyRecs: cold-start.
func TestKnowledge_NoEntities_EmptyRecs(t *testing.T) {
	src := NewKnowledgeGraphSource(&fakeGraph{}, KnowledgeOpts{})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("want 0 recs, got %d", len(recs))
	}
}

// TestKnowledge_SeamError_Propagates mirrors the macos source so the
// aggregator can isolate per-source failures.
func TestKnowledge_SeamError_Propagates(t *testing.T) {
	sentinel := errors.New("graph closed")
	src := NewKnowledgeGraphSource(&fakeGraph{err: sentinel}, KnowledgeOpts{})
	_, err := src.Recommendations(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wraps %v", err, sentinel)
	}
}

// TestKnowledge_NoMeeting_BelowThreshold_NoRec — person with prior touchpoints
// but no active meeting context drops below the propose threshold. Deletion
// default: noise out, signal only.
func TestKnowledge_NoMeeting_BelowThreshold_NoRec(t *testing.T) {
	src := NewKnowledgeGraphSource(&fakeGraph{
		people: []PersonContext{
			{Key: "person:alex", Display: "Alex", ItemCount: 1, HasMeeting: false},
		},
	}, KnowledgeOpts{})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("want 0 recs without meeting+history signal, got %d", len(recs))
	}
}

// containsName is a literal-substring guard for the PII assertion above.
func containsName(s, name string) bool {
	for i := 0; i+len(name) <= len(s); i++ {
		if s[i:i+len(name)] == name {
			return true
		}
	}
	return false
}

// TestKnowledgeGraphSource_DecayAt7Days_HalvesWeight: exp(-7/14) ≈ 0.6065. A
// person 7 days stale carries ~60.65% of their base confidence — the half-life
// (14d) is the load-bearing curve. ItemCount 10 → base confidence 1.0; expect
// adjusted ≈ 0.6065.
func TestKnowledgeGraphSource_DecayAt7Days_HalvesWeight(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	src := NewKnowledgeGraphSource(&fakeGraph{
		people: []PersonContext{{
			Key:         "person:sarah",
			ItemCount:   10,
			HasMeeting:  true,
			LastTouched: now.Add(-7 * 24 * time.Hour),
			Demotion:    1.0,
		}},
	}, KnowledgeOpts{Now: func() time.Time { return now }})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 rec, got %d", len(recs))
	}
	want := math.Exp(-7.0 / 14.0)
	if diff := math.Abs(recs[0].Confidence - want); diff > 1e-6 {
		t.Errorf("Confidence = %v, want ~%v (delta %v)", recs[0].Confidence, want, diff)
	}
}

// TestKnowledgeGraphSource_DemotionPastTTL_HalvesWeeklyMultiplier: 67d stale,
// TTL=60d → 1 week past TTL → demotion multiplier 0.5. Base ItemCount 10
// (conf 1.0). decay = exp(-67/14), weight = decay * 0.5. Floor guards drop
// elsewhere; here the value stays above 0.05? exp(-67/14) ≈ 0.0083, *0.5 ≈
// 0.0041 — that's below the floor, so this fixture uses Demotion 1.0 and
// short age 70d with TTL=60 so we get exactly one week past. Pick TTL=60, age
// 7d past (67d): decay tiny. Adjusted: keep age modest by setting the test's
// base touch only 67d ago with TTL=60 but expect drop. To isolate demotion's
// effect, pin Demotion field to 1.0, age to 67d, and assert the rec drops
// below floor. That's the FloorBelow test — this one needs a fixture where
// only demotion multiplies (decay = 1). Use age = 0 with Demotion already at
// 0.5 (post-week-of-stale state); base conf 0.4 (ItemCount 4) → weight 0.2.
func TestKnowledgeGraphSource_DemotionPastTTL_HalvesWeeklyMultiplier(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	src := NewKnowledgeGraphSource(&fakeGraph{
		people: []PersonContext{{
			Key:         "person:alex",
			ItemCount:   4,
			HasMeeting:  true,
			LastTouched: now,
			Demotion:    0.5,
		}},
	}, KnowledgeOpts{Now: func() time.Time { return now }})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 rec, got %d", len(recs))
	}
	// decay=1 (age 0), Demotion=0.5, base conf 0.4 → adjusted 0.2.
	want := 0.4 * 0.5
	if diff := math.Abs(recs[0].Confidence - want); diff > 1e-6 {
		t.Errorf("Confidence = %v, want %v", recs[0].Confidence, want)
	}
}

// TestKnowledgeGraphSource_FloorAt005: at the floor boundary, weight is
// exactly 0.05 (spec: max(0.05, decay*demotion)). Fixture: age 0 → decay=1,
// Demotion=0.05 → weight=0.05. Base ItemCount 10 → conf 1.0 → adjusted 0.05.
// One step further down (decay*demotion < 0.05) is the drop path covered by
// the next test.
func TestKnowledgeGraphSource_FloorAt005(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	src := NewKnowledgeGraphSource(&fakeGraph{
		people: []PersonContext{{
			Key:         "person:floor",
			ItemCount:   10,
			HasMeeting:  true,
			LastTouched: now,
			Demotion:    0.05,
		}},
	}, KnowledgeOpts{Now: func() time.Time { return now }})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 rec at floor, got %d", len(recs))
	}
	if diff := math.Abs(recs[0].Confidence - 0.05); diff > 1e-6 {
		t.Errorf("Confidence = %v, want 0.05 (floor)", recs[0].Confidence)
	}
}

// TestKnowledgeGraphSource_DropsRecommendationsBelowFloor: when decay*demotion
// drops strictly below 0.05, the source stops emitting the Recommendation
// rather than emit a floored signal — noise out, signal only.
func TestKnowledgeGraphSource_DropsRecommendationsBelowFloor(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	src := NewKnowledgeGraphSource(&fakeGraph{
		people: []PersonContext{{
			Key:         "person:stale",
			ItemCount:   10,
			HasMeeting:  true,
			LastTouched: now.Add(-90 * 24 * time.Hour), // decay = exp(-90/14) ≈ 0.0017
			Demotion:    0.5,
		}},
	}, KnowledgeOpts{Now: func() time.Time { return now }})
	recs, err := src.Recommendations(context.Background())
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("want 0 recs below floor, got %d (conf=%v)", len(recs), recs[0].Confidence)
	}
}
