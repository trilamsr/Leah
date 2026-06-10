// Package tripplanner is the composition layer over Leah's maps, flights, and
// calendar adapters. It exposes operator-meaningful trip primitives —
// "what's on the way?", "meet in the middle?" — as a single Planner surface
// so voice, HUD, and brief callers don't re-orchestrate the same adapter
// graphs three times.
//
// The package defines its own minimal Router / CorridorSearcher / Geocoder
// seams (structural typing, no concrete adapter imports) so swapping or
// faking the underlying providers is one struct away — and so the
// dependency graph stays one-way: callers → tripplanner → seam → adapter,
// never the reverse.
//
// W61 lands OnTheWay + MeetInMiddle; SuggestTrip + DailyItinerary are
// deferred to W62 per the trip-planning plan.
package tripplanner
