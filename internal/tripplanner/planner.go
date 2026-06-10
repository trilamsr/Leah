package tripplanner

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Place is the operator-facing place envelope the planner returns. Fields
// mirror the maps adapter shape exactly so callers can route results
// straight through to HUD / voice without a second projection — but the
// type lives here so tripplanner has no compile-time edge into
// internal/adapters/maps.
type Place struct {
	ID         string
	Name       string
	Lat        float64
	Lng        float64
	Categories []string
	Rating     float32
}

// Route is the directions envelope the planner consumes from a Router. Only
// the polyline is load-bearing for W61 — the corridor algorithm decodes it
// — but distance/duration are kept on the type so future surfaces
// (SuggestTrip, HUD ETA) don't force a Router refactor.
type Route struct {
	Polyline  string
	DistanceM int
	DurationS int
}

// CorridorOpts shapes an OnTheWay query. Zero-valued fields adopt the
// underlying CorridorSearcher's defaults; this struct intentionally mirrors
// the maps adapter's CorridorOpts so the composition is a one-line
// pass-through, not a translation layer.
type CorridorOpts struct {
	MaxDetourMinutes int
	Categories       []string
	OpenAt           time.Time
	SampleEveryM     int
	TopK             int
	RadiusM          int
}

// Profile carries operator preferences a future personalised SuggestTrip /
// DailyItinerary will consult (W62+, W64). Declared here so the Planner
// interface is stable across the wave; unused by OnTheWay / MeetInMiddle.
type Profile struct {
	Categories []string
	BudgetUSD  float64
	Pace       string
}

// Trip is the SuggestTrip envelope (W62). Declared so the interface is
// stable; the W61 implementation returns ErrNotImplemented.
type Trip struct {
	Days []Itinerary
}

// Itinerary is the per-day envelope (W62). Same stability rationale as Trip.
type Itinerary struct {
	Date   time.Time
	Places []Place
}

// Router resolves a directions request between two freeform addresses. The
// seam takes a string mode (not a typed constant) so tripplanner stays
// adapter-agnostic — the underlying adapter picks the typed enum.
type Router interface {
	Route(ctx context.Context, origin, dest string, mode string) (Route, error)
}

// CorridorSearcher returns ranked POIs along a route. Implementations own
// the polyline-decode + nearby-per-anchor orchestration; tripplanner just
// hands a Route across the seam.
type CorridorSearcher interface {
	POIAlongRoute(ctx context.Context, route Route, opts CorridorOpts) ([]Place, error)
}

// Geocoder resolves a freeform address to candidate Places. Used by
// MeetInMiddle to derive lat/lng for the geographic-midpoint computation.
type Geocoder interface {
	Geocode(ctx context.Context, address string) ([]Place, error)
}

// NearbySearcher returns POIs of a category within radius of a center. Used
// by MeetInMiddle once the midpoint is geocoded.
type NearbySearcher interface {
	POINearby(ctx context.Context, center Place, radiusM int, category string) ([]Place, error)
}

// Planner is the composition surface voice / HUD / brief callers bind to.
// W61 lands OnTheWay + MeetInMiddle; SuggestTrip + DailyItinerary are
// stubbed with ErrNotImplemented so the interface is stable across the
// wave.
type Planner interface {
	OnTheWay(ctx context.Context, origin, dest string, opts CorridorOpts) ([]Place, error)
	SuggestTrip(ctx context.Context, dest string, durationDays int, profile Profile) (Trip, error)
	DailyItinerary(ctx context.Context, city string, on time.Time, profile Profile) (Itinerary, error)
	MeetInMiddle(ctx context.Context, partyA, partyB string, category string) ([]Place, error)
}

// ErrNotImplemented is returned by W61's SuggestTrip / DailyItinerary —
// the interface is stable, the bodies land in W62.
var ErrNotImplemented = errors.New("tripplanner: not implemented in W61")

// ErrNoMidpoint is returned by MeetInMiddle when either party fails to
// geocode (zero results). Distinct error so callers can prompt the
// operator to re-enter an address.
var ErrNoMidpoint = errors.New("tripplanner: no midpoint (geocode returned no results)")

// midpointRadiusM is the search radius around the computed midpoint for
// MeetInMiddle's nearby query. 5km balances "close to fair" against
// "enough candidates in a sparse area"; tunable once the surface ships.
const midpointRadiusM = 5000

// Composer is the default Planner implementation. It owns no state beyond
// its seams — adapter swaps, hermetic fakes, and per-test wiring all flow
// through the Config struct.
type Composer struct {
	router   Router
	corridor CorridorSearcher
	geocoder Geocoder
	nearby   NearbySearcher
}

// Config carries the Composer's seam wiring. All four fields are required
// for W61; New returns an error rather than panicking on a nil seam so
// callers compose without defensive checks.
type Config struct {
	Router   Router
	Corridor CorridorSearcher
	Geocoder Geocoder
	Nearby   NearbySearcher
}

// New builds a Composer. Each missing seam is a distinct config error so
// the caller gets one actionable message, not a generic nil-pointer panic
// three layers deep.
func New(cfg Config) (*Composer, error) {
	switch {
	case cfg.Router == nil:
		return nil, errors.New("tripplanner: Router required")
	case cfg.Corridor == nil:
		return nil, errors.New("tripplanner: Corridor required")
	case cfg.Geocoder == nil:
		return nil, errors.New("tripplanner: Geocoder required")
	case cfg.Nearby == nil:
		return nil, errors.New("tripplanner: Nearby required")
	}
	return &Composer{
		router:   cfg.Router,
		corridor: cfg.Corridor,
		geocoder: cfg.Geocoder,
		nearby:   cfg.Nearby,
	}, nil
}

// OnTheWay returns POIs along the driving corridor origin → dest. One
// Router call, one CorridorSearcher call — the composition is deliberately
// thin so the corridor algorithm stays the single source of ranking truth.
func (c *Composer) OnTheWay(ctx context.Context, origin, dest string, opts CorridorOpts) ([]Place, error) {
	route, err := c.router.Route(ctx, origin, dest, "driving")
	if err != nil {
		return nil, fmt.Errorf("tripplanner: on the way: route: %w", err)
	}
	places, err := c.corridor.POIAlongRoute(ctx, route, opts)
	if err != nil {
		return nil, fmt.Errorf("tripplanner: on the way: corridor: %w", err)
	}
	return places, nil
}

// MeetInMiddle resolves both parties to lat/lng, computes the geographic
// midpoint, and returns category POIs within midpointRadiusM. A true
// "drive-time fair" midpoint requires Distance Matrix iteration — deferred
// per the trip-planning plan; geographic midpoint is the
// operator-meaningful approximation that ships in W61.
func (c *Composer) MeetInMiddle(ctx context.Context, partyA, partyB string, category string) ([]Place, error) {
	a, err := c.geocoder.Geocode(ctx, partyA)
	if err != nil {
		return nil, fmt.Errorf("tripplanner: meet in middle: geocode A: %w", err)
	}
	if len(a) == 0 {
		return nil, fmt.Errorf("%w: party A %q", ErrNoMidpoint, partyA)
	}
	b, err := c.geocoder.Geocode(ctx, partyB)
	if err != nil {
		return nil, fmt.Errorf("tripplanner: meet in middle: geocode B: %w", err)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("%w: party B %q", ErrNoMidpoint, partyB)
	}
	mid := Place{
		Lat: (a[0].Lat + b[0].Lat) / 2,
		Lng: (a[0].Lng + b[0].Lng) / 2,
	}
	places, err := c.nearby.POINearby(ctx, mid, midpointRadiusM, category)
	if err != nil {
		return nil, fmt.Errorf("tripplanner: meet in middle: nearby: %w", err)
	}
	return places, nil
}

// SuggestTrip is the W62 surface — wired here for interface stability.
func (c *Composer) SuggestTrip(ctx context.Context, dest string, durationDays int, profile Profile) (Trip, error) {
	return Trip{}, ErrNotImplemented
}

// DailyItinerary is the W62 surface — wired here for interface stability.
func (c *Composer) DailyItinerary(ctx context.Context, city string, on time.Time, profile Profile) (Itinerary, error) {
	return Itinerary{}, ErrNotImplemented
}

// compile-time guarantee: Composer satisfies Planner.
var _ Planner = (*Composer)(nil)
