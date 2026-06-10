package tripplanner_test

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"time"

	"github.com/trilam/leah/internal/tripplanner"
)

// fakeRouter / fakeCorridor / fakeGeocoder / fakeNearby implement the
// seams. Each captures its last call and returns the configured result —
// stub-not-mock so tests assert observable composition, not call shape.

type fakeRouter struct {
	gotOrigin, gotDest, gotMode string
	out                         tripplanner.Route
	err                         error
}

func (f *fakeRouter) Route(_ context.Context, origin, dest, mode string) (tripplanner.Route, error) {
	f.gotOrigin, f.gotDest, f.gotMode = origin, dest, mode
	return f.out, f.err
}

type fakeCorridor struct {
	gotRoute tripplanner.Route
	gotOpts  tripplanner.CorridorOpts
	out      []tripplanner.Place
	err      error
}

func (f *fakeCorridor) POIAlongRoute(_ context.Context, route tripplanner.Route, opts tripplanner.CorridorOpts) ([]tripplanner.Place, error) {
	f.gotRoute = route
	f.gotOpts = opts
	return f.out, f.err
}

type fakeGeocoder struct {
	results map[string][]tripplanner.Place
	err     error
}

func (f *fakeGeocoder) Geocode(_ context.Context, address string) ([]tripplanner.Place, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.results[address], nil
}

type fakeNearby struct {
	gotCenter   tripplanner.Place
	gotRadius   int
	gotCategory string
	out         []tripplanner.Place
	err         error
}

func (f *fakeNearby) POINearby(_ context.Context, center tripplanner.Place, radiusM int, category string) ([]tripplanner.Place, error) {
	f.gotCenter, f.gotRadius, f.gotCategory = center, radiusM, category
	return f.out, f.err
}

func newComposer(t *testing.T, r tripplanner.Router, c tripplanner.CorridorSearcher, g tripplanner.Geocoder, n tripplanner.NearbySearcher) *tripplanner.Composer {
	t.Helper()
	planner, err := tripplanner.New(tripplanner.Config{
		Router: r, Corridor: c, Geocoder: g, Nearby: n,
	})
	if err != nil {
		t.Fatalf("tripplanner.New: %v", err)
	}
	return planner
}

func TestOnTheWay_HappyPath(t *testing.T) {
	want := []tripplanner.Place{
		{ID: "p1", Name: "Diner", Rating: 4.5},
		{ID: "p2", Name: "Park", Rating: 4.2},
	}
	router := &fakeRouter{out: tripplanner.Route{Polyline: "abc", DistanceM: 12000, DurationS: 600}}
	corridor := &fakeCorridor{out: want}
	planner := newComposer(t, router, corridor, &fakeGeocoder{}, &fakeNearby{})

	opts := tripplanner.CorridorOpts{Categories: []string{"restaurant"}, TopK: 5}
	got, err := planner.OnTheWay(context.Background(), "Seattle", "Bellevue", opts)
	if err != nil {
		t.Fatalf("OnTheWay: %v", err)
	}

	if router.gotOrigin != "Seattle" || router.gotDest != "Bellevue" || router.gotMode != "driving" {
		t.Fatalf("router got origin=%q dest=%q mode=%q; want Seattle/Bellevue/driving",
			router.gotOrigin, router.gotDest, router.gotMode)
	}
	if corridor.gotRoute.Polyline != "abc" {
		t.Fatalf("corridor got polyline %q; want abc", corridor.gotRoute.Polyline)
	}
	if len(got) != 2 || got[0].ID != "p1" || got[1].ID != "p2" {
		t.Fatalf("OnTheWay places = %+v; want %+v", got, want)
	}
}

func TestOnTheWay_RouteFails_ErrPropagated(t *testing.T) {
	boom := errors.New("directions denied")
	router := &fakeRouter{err: boom}
	corridor := &fakeCorridor{} // must not be called
	planner := newComposer(t, router, corridor, &fakeGeocoder{}, &fakeNearby{})

	_, err := planner.OnTheWay(context.Background(), "A", "B", tripplanner.CorridorOpts{})
	if !errors.Is(err, boom) {
		t.Fatalf("OnTheWay err = %v; want errors.Is boom", err)
	}
	if corridor.gotRoute.Polyline != "" {
		t.Fatal("corridor should not be called when router fails")
	}
}

func TestOnTheWay_CorridorFails_ErrPropagated(t *testing.T) {
	boom := errors.New("nearby blew up")
	router := &fakeRouter{out: tripplanner.Route{Polyline: "abc"}}
	corridor := &fakeCorridor{err: boom}
	planner := newComposer(t, router, corridor, &fakeGeocoder{}, &fakeNearby{})

	_, err := planner.OnTheWay(context.Background(), "A", "B", tripplanner.CorridorOpts{})
	if !errors.Is(err, boom) {
		t.Fatalf("OnTheWay err = %v; want errors.Is boom", err)
	}
}

func TestMeetInMiddle_HappyPath(t *testing.T) {
	geo := &fakeGeocoder{results: map[string][]tripplanner.Place{
		"100 Main St":  {{ID: "a", Lat: 47.6, Lng: -122.3}},
		"500 Oak Ave":  {{ID: "b", Lat: 47.8, Lng: -122.1}},
	}}
	want := []tripplanner.Place{{ID: "cafe1", Name: "Mid Cafe", Rating: 4.6}}
	nearby := &fakeNearby{out: want}
	planner := newComposer(t, &fakeRouter{}, &fakeCorridor{}, geo, nearby)

	got, err := planner.MeetInMiddle(context.Background(), "100 Main St", "500 Oak Ave", "cafe")
	if err != nil {
		t.Fatalf("MeetInMiddle: %v", err)
	}
	// midpoint = (47.7, -122.2)
	const epsilon = 1e-9
	if abs(nearby.gotCenter.Lat-47.7) > epsilon || abs(nearby.gotCenter.Lng-(-122.2)) > epsilon {
		t.Fatalf("midpoint = (%v,%v); want (47.7,-122.2)", nearby.gotCenter.Lat, nearby.gotCenter.Lng)
	}
	if nearby.gotCategory != "cafe" {
		t.Fatalf("category = %q; want cafe", nearby.gotCategory)
	}
	if len(got) != 1 || got[0].ID != "cafe1" {
		t.Fatalf("MeetInMiddle = %+v; want %+v", got, want)
	}
}

func TestMeetInMiddle_GeocodeFail_ErrPropagated(t *testing.T) {
	boom := errors.New("geocode quota")
	geo := &fakeGeocoder{err: boom}
	planner := newComposer(t, &fakeRouter{}, &fakeCorridor{}, geo, &fakeNearby{})

	_, err := planner.MeetInMiddle(context.Background(), "A", "B", "cafe")
	if !errors.Is(err, boom) {
		t.Fatalf("MeetInMiddle err = %v; want errors.Is boom", err)
	}
}

func TestMeetInMiddle_ZeroGeocodeResults_ErrNoMidpoint(t *testing.T) {
	geo := &fakeGeocoder{results: map[string][]tripplanner.Place{
		"good":   {{ID: "x", Lat: 1, Lng: 1}},
		"unknown": nil,
	}}
	planner := newComposer(t, &fakeRouter{}, &fakeCorridor{}, geo, &fakeNearby{})

	_, err := planner.MeetInMiddle(context.Background(), "unknown", "good", "cafe")
	if !errors.Is(err, tripplanner.ErrNoMidpoint) {
		t.Fatalf("party-A miss: err = %v; want ErrNoMidpoint", err)
	}
	_, err = planner.MeetInMiddle(context.Background(), "good", "unknown", "cafe")
	if !errors.Is(err, tripplanner.ErrNoMidpoint) {
		t.Fatalf("party-B miss: err = %v; want ErrNoMidpoint", err)
	}
}

func TestNew_NilSeams_Rejected(t *testing.T) {
	cases := []struct {
		name string
		cfg  tripplanner.Config
	}{
		{"router", tripplanner.Config{Corridor: &fakeCorridor{}, Geocoder: &fakeGeocoder{}, Nearby: &fakeNearby{}}},
		{"corridor", tripplanner.Config{Router: &fakeRouter{}, Geocoder: &fakeGeocoder{}, Nearby: &fakeNearby{}}},
		{"geocoder", tripplanner.Config{Router: &fakeRouter{}, Corridor: &fakeCorridor{}, Nearby: &fakeNearby{}}},
		{"nearby", tripplanner.Config{Router: &fakeRouter{}, Corridor: &fakeCorridor{}, Geocoder: &fakeGeocoder{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tripplanner.New(tc.cfg); err == nil {
				t.Fatalf("New with nil %s: want error", tc.name)
			}
		})
	}
}

func TestDeferred_ReturnsNotImplemented(t *testing.T) {
	planner := newComposer(t, &fakeRouter{}, &fakeCorridor{}, &fakeGeocoder{}, &fakeNearby{})
	if _, err := planner.SuggestTrip(context.Background(), "Tokyo", 5, tripplanner.Profile{}); !errors.Is(err, tripplanner.ErrNotImplemented) {
		t.Fatalf("SuggestTrip err = %v; want ErrNotImplemented", err)
	}
	if _, err := planner.DailyItinerary(context.Background(), "Tokyo", time.Time{}, tripplanner.Profile{}); !errors.Is(err, tripplanner.ErrNotImplemented) {
		t.Fatalf("DailyItinerary err = %v; want ErrNotImplemented", err)
	}
}

// TestPlanner_StructuralDeps enforces the package's load-bearing
// invariant: tripplanner composes via structural seams, so it MUST NOT
// pull internal/adapters/maps or internal/adapters/flights into its
// dependency graph. `go list -deps` is the cheapest way to assert this —
// any concrete import would surface here and fail the gate.
func TestPlanner_StructuralDeps(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "./...").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "github.com/trilam/leah/internal/adapters/maps" ||
			line == "github.com/trilam/leah/internal/adapters/flights" {
			t.Fatalf("tripplanner must not depend on %s — use structural seam", line)
		}
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
