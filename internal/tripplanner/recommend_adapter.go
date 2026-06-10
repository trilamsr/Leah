package tripplanner

import (
	"context"
	"sort"
	"strings"
)

// Recommendation mirrors internal/recommend.Recommendation's pattern+confidence
// shape via a structural seam so tripplanner stays free of an import edge
// into internal/recommend (the StructuralDeps gate enforces this for maps
// and flights — recommend follows the same contract).
type Recommendation struct {
	Pattern    string
	Confidence float64
}

// RecommendSeam is the structural seam onto the recommendation engine.
// Propose returns the operator's accumulated accept/reject signals as
// pattern-keyed confidence scores; Accept/Reject feed observed planner
// outcomes back so the next Propose reflects them.
type RecommendSeam interface {
	Propose(ctx context.Context) ([]Recommendation, error)
	Accept(ctx context.Context, id string) error
	Reject(ctx context.Context, id string) error
}

// placePatternPrefix is the engine-side namespace for place-level signals.
// Recommendation.Pattern of "place:<id>" is the operator's confidence in a
// specific place; the planner owns this convention because the engine
// stores opaque pattern strings.
const placePatternPrefix = "place:"

// RecommendAdapter bridges the planner to the recommendation engine. It
// owns the pattern-key convention (placePatternPrefix) so the engine stays
// place-agnostic.
type RecommendAdapter struct {
	engine RecommendSeam
}

// NewRecommendAdapter panics on a nil seam — the adapter has one
// dependency and constructing it without one is a wiring bug, not a
// runtime condition worth a sentinel error.
func NewRecommendAdapter(engine RecommendSeam) *RecommendAdapter {
	if engine == nil {
		panic("tripplanner: RecommendAdapter requires non-nil engine seam")
	}
	return &RecommendAdapter{engine: engine}
}

// RankPlaces reorders places by operator confidence. Engine errors degrade
// to "return input unchanged" — personalization is a nice-to-have, not a
// hard dependency; the planner must not fail a SuggestTrip because the
// recommendation store hiccupped.
func (a *RecommendAdapter) RankPlaces(ctx context.Context, places []Place) ([]Place, error) {
	recs, err := a.engine.Propose(ctx)
	if err != nil {
		return places, nil
	}
	score := make(map[string]float64, len(recs))
	for _, r := range recs {
		id := strings.TrimPrefix(r.Pattern, placePatternPrefix)
		if id == r.Pattern {
			continue
		}
		score[id] += r.Confidence
	}

	out := make([]Place, len(places))
	copy(out, places)
	sort.SliceStable(out, func(i, j int) bool {
		return score[out[i].ID] > score[out[j].ID]
	})
	return out, nil
}

// RecordAccept propagates an accept signal to the engine, keyed by place
// ID. The engine owns idempotency and dedupe.
func (a *RecommendAdapter) RecordAccept(ctx context.Context, placeID string) error {
	return a.engine.Accept(ctx, placeID)
}

// RecordReject is the symmetric counterpart to RecordAccept.
func (a *RecommendAdapter) RecordReject(ctx context.Context, placeID string) error {
	return a.engine.Reject(ctx, placeID)
}
