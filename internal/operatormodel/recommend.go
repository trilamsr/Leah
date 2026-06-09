package operatormodel

import (
	"fmt"
	"sort"
	"time"
)

// MaxRecommendations caps Recommend output (spec §4.1).
const MaxRecommendations = 3

// Recommendation is one suggested next action surfaced to the operator.
// Reason is a 1-line human-readable explanation; the CLI prints it
// verbatim under the default (no --llm) terse template.
type Recommendation struct {
	Kind   string
	Class  string  // observation class that produced this rec
	Weight float64 // decay-adjusted weight; higher == stronger signal
	Reason string
}

// Recommend ranks ProfileRows that match (currentCtx, currentTime) and
// returns up to MaxRecommendations. Pure function — no IO, no LLM.
//
// Cold-start gate: returns nil (no error) when !profile.Ready.
//
// Ranking:
//   - time_of_day: row.Slot must match HH(currentTime) in profile.tz()
//   - context_transition: row.Key must equal currentCtx
//   - cadence: row.Slot must match weekday-abbrev(currentTime) in profile.tz()
//
// The hour + weekday lookup honors profile.TZ (defaults to time.Local) to
// stay symmetric with Profile.Update which records slots in operator-local
// time. UTC-rounded lookup silently missed every slot for any operator
// outside UTC (audit L9).
//
// Within the matched set, sort by Weight desc, ties broken by Kind asc.
// Same Kind across classes is de-duplicated — first (highest-weight)
// occurrence wins.
func Recommend(profile Profile, currentCtx string, currentTime time.Time) ([]Recommendation, error) {
	if !profile.Ready {
		return nil, nil
	}
	local := currentTime.In(profile.tz())
	hour := fmt.Sprintf("%02d", local.Hour())
	day := local.Weekday().String()[:3]

	var candidates []Recommendation
	for _, r := range profile.Rows {
		switch r.Class {
		case "time_of_day":
			if r.Slot != hour {
				continue
			}
			candidates = append(candidates, Recommendation{
				Kind: r.Key, Class: r.Class, Weight: r.Weight,
				Reason: fmt.Sprintf("typical at %s:00 (weight=%.1f over %d events)", hour, r.Weight, r.Count),
			})
		case "context_transition":
			if r.Key != currentCtx {
				continue
			}
			candidates = append(candidates, Recommendation{
				Kind: r.Slot, Class: r.Class, Weight: r.Weight,
				Reason: fmt.Sprintf("you usually do this after entering '%s' (weight=%.1f over %d events)", currentCtx, r.Weight, r.Count),
			})
		case "cadence":
			if r.Slot != day {
				continue
			}
			candidates = append(candidates, Recommendation{
				Kind: r.Key, Class: r.Class, Weight: r.Weight,
				Reason: fmt.Sprintf("%s cadence (weight=%.1f over %d events)", day, r.Weight, r.Count),
			})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Weight != candidates[j].Weight {
			return candidates[i].Weight > candidates[j].Weight
		}
		return candidates[i].Kind < candidates[j].Kind
	})

	// De-dup by Kind — first occurrence (highest weight) wins.
	seen := map[string]struct{}{}
	out := make([]Recommendation, 0, MaxRecommendations)
	for _, c := range candidates {
		if _, dup := seen[c.Kind]; dup {
			continue
		}
		seen[c.Kind] = struct{}{}
		out = append(out, c)
		if len(out) >= MaxRecommendations {
			break
		}
	}
	return out, nil
}
