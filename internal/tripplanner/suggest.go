package tripplanner

import (
	"context"
	"fmt"
	"time"
)

// suggestDepartLeadDays is the default offset between "now" and the
// suggested depart date. Kept const because operator-tunable date
// selection lives in the brief surface, not the composer.
const suggestDepartLeadDays = 14

// SuggestTrip composes a destination Trip: optional flight offers
// (origin → dest) plus a per-day DailyItinerary at the destination.
// Flight failures degrade silently — UX > strict.
func (c *Composer) SuggestTrip(ctx context.Context, dest string, durationDays int, profile Profile) (Trip, error) {
	if durationDays <= 0 {
		return Trip{}, fmt.Errorf("%w: got %d", ErrInvalidDuration, durationDays)
	}

	depart := time.Now().AddDate(0, 0, suggestDepartLeadDays)
	ret := depart.AddDate(0, 0, durationDays)

	trip := Trip{Destination: dest}

	if c.flights != nil && profile.Origin != "" {
		// Flight errors are non-fatal: a days plan with no flights still
		// answers the operator's "what would a trip to X look like" ask.
		if offers, err := c.flights.SearchOffers(ctx, profile.Origin, dest, depart, ret); err == nil {
			trip.Flights = offers
		}
	}

	trip.Days = make([]Itinerary, 0, durationDays)
	for d := 0; d < durationDays; d++ {
		dayDate := depart.AddDate(0, 0, d)
		itin, err := c.DailyItinerary(ctx, dest, dayDate, profile)
		if err != nil {
			return Trip{}, fmt.Errorf("tripplanner: suggest trip: day %d: %w", d, err)
		}
		trip.Days = append(trip.Days, itin)
	}
	return trip, nil
}
