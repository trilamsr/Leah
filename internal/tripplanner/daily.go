package tripplanner

import (
	"context"
	"fmt"
	"time"
)

// dailyRadiusM is the search radius around the city centroid. 8km
// covers a walkable downtown plus first-ring neighborhoods; same order
// of magnitude as midpointRadiusM.
const dailyRadiusM = 8000

// defaultInterests is what an empty Profile.Interests resolves to —
// the two categories that answer "what should I do here today" for
// every destination.
var defaultInterests = []string{"restaurant", "attraction"}

// DailyItinerary geocodes the city, fans out one POINearby per
// interest, drops places closed at `on`, and returns the combined list.
// Per-interest failures fail the call — partial silence would mislead.
func (c *Composer) DailyItinerary(ctx context.Context, city string, on time.Time, profile Profile) (Itinerary, error) {
	cands, err := c.geocoder.Geocode(ctx, city)
	if err != nil {
		return Itinerary{}, fmt.Errorf("tripplanner: daily: geocode: %w", err)
	}
	if len(cands) == 0 {
		return Itinerary{}, fmt.Errorf("%w: %q", ErrUnknownCity, city)
	}
	center := cands[0]

	interests := profile.Interests
	if len(interests) == 0 {
		interests = defaultInterests
	}

	itin := Itinerary{Date: on}
	for _, cat := range interests {
		places, err := c.nearby.POINearby(ctx, center, dailyRadiusM, cat)
		if err != nil {
			return Itinerary{}, fmt.Errorf("tripplanner: daily: nearby %q: %w", cat, err)
		}
		for _, p := range places {
			if isOpenAt(p, on) {
				itin.Places = append(itin.Places, p)
			}
		}
	}
	return itin, nil
}

// isOpenAt reports whether `p` is open at `at`. A place with no Hours
// is treated as always open — matches Google Places' "no hours
// returned" semantics where the operator should not be denied a
// candidate just because the data is sparse.
func isOpenAt(p Place, at time.Time) bool {
	if len(p.Hours) == 0 {
		return true
	}
	minute := at.Hour()*60 + at.Minute()
	for _, w := range p.Hours {
		if w.OpenMinute >= w.CloseMinute { // 24h or wraparound: treat as open
			return true
		}
		if minute >= w.OpenMinute && minute < w.CloseMinute {
			return true
		}
	}
	return false
}
