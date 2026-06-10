package sources

import (
	"context"
	"errors"
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
