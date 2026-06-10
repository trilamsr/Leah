package tripplanner_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trilam/leah/internal/tripplanner"
)

// fakeFlightFinder mirrors flights.Adapter.SearchOffers across the
// structural seam — same stub-not-mock posture as the other fakes.
type fakeFlightFinder struct {
	gotOrigin, gotDest string
	out                []tripplanner.FlightOffer
	err                error
}

func (f *fakeFlightFinder) SearchOffers(_ context.Context, origin, dest string, depart, ret time.Time) ([]tripplanner.FlightOffer, error) {
	f.gotOrigin, f.gotDest = origin, dest
	return f.out, f.err
}

func newComposerWithFlights(t *testing.T, ff tripplanner.FlightFinder) *tripplanner.Composer {
	t.Helper()
	planner, err := tripplanner.New(tripplanner.Config{
		Router: &fakeRouter{}, Corridor: &fakeCorridor{}, Geocoder: &fakeGeocoder{}, Nearby: &fakeNearby{},
		Flights: ff,
	})
	if err != nil {
		t.Fatalf("tripplanner.New: %v", err)
	}
	return planner
}

func TestSuggestTrip_HappyPath(t *testing.T) {
	offers := []tripplanner.FlightOffer{{ID: "of1", Price: 250_00, Currency: "USD"}}
	ff := &fakeFlightFinder{out: offers}
	nearby := &fakeNearby{out: []tripplanner.Place{
		{ID: "p1", Name: "Sushi Bar", Categories: []string{"restaurant"}},
	}}
	geo := &fakeGeocoder{results: map[string][]tripplanner.Place{
		"Tokyo": {{ID: "city", Name: "Tokyo", Lat: 35.68, Lng: 139.76}},
	}}
	planner, err := tripplanner.New(tripplanner.Config{
		Router: &fakeRouter{}, Corridor: &fakeCorridor{}, Geocoder: geo, Nearby: nearby, Flights: ff,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	prof := tripplanner.Profile{Origin: "SEA", Interests: []string{"restaurant"}}
	trip, err := planner.SuggestTrip(context.Background(), "Tokyo", 3, prof)
	if err != nil {
		t.Fatalf("SuggestTrip: %v", err)
	}
	if trip.Destination != "Tokyo" {
		t.Fatalf("Destination = %q; want Tokyo", trip.Destination)
	}
	if len(trip.Days) != 3 {
		t.Fatalf("Days = %d; want 3", len(trip.Days))
	}
	if len(trip.Flights) != 1 || trip.Flights[0].ID != "of1" {
		t.Fatalf("Flights = %+v; want one offer of1", trip.Flights)
	}
	if ff.gotOrigin != "SEA" || ff.gotDest != "Tokyo" {
		t.Fatalf("FlightFinder got origin=%q dest=%q; want SEA/Tokyo", ff.gotOrigin, ff.gotDest)
	}
	for i, day := range trip.Days {
		if len(day.Places) == 0 {
			t.Fatalf("day %d: no places", i)
		}
	}
}

func TestSuggestTrip_NoFlights_GracefulDegrade(t *testing.T) {
	// Flights seam absent → Trip still has days populated.
	nearby := &fakeNearby{out: []tripplanner.Place{{ID: "p1", Name: "Museum"}}}
	geo := &fakeGeocoder{results: map[string][]tripplanner.Place{
		"Kyoto": {{ID: "city", Name: "Kyoto", Lat: 35.0, Lng: 135.7}},
	}}
	planner, err := tripplanner.New(tripplanner.Config{
		Router: &fakeRouter{}, Corridor: &fakeCorridor{}, Geocoder: geo, Nearby: nearby,
		// Flights: nil — graceful degrade
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	trip, err := planner.SuggestTrip(context.Background(), "Kyoto", 2, tripplanner.Profile{Interests: []string{"museum"}})
	if err != nil {
		t.Fatalf("SuggestTrip: %v", err)
	}
	if len(trip.Flights) != 0 {
		t.Fatalf("Flights = %+v; want empty", trip.Flights)
	}
	if len(trip.Days) != 2 {
		t.Fatalf("Days = %d; want 2", len(trip.Days))
	}
}

func TestSuggestTrip_FlightFinderError_DegradesNotFails(t *testing.T) {
	// A flight-API failure must not block the days plan — UX > strict.
	ff := &fakeFlightFinder{err: errors.New("amadeus down")}
	nearby := &fakeNearby{out: []tripplanner.Place{{ID: "p1"}}}
	geo := &fakeGeocoder{results: map[string][]tripplanner.Place{
		"Lisbon": {{ID: "city", Lat: 38.7, Lng: -9.1}},
	}}
	pl, err := tripplanner.New(tripplanner.Config{
		Router: &fakeRouter{}, Corridor: &fakeCorridor{}, Geocoder: geo, Nearby: nearby, Flights: ff,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	trip, err := pl.SuggestTrip(context.Background(), "Lisbon", 1, tripplanner.Profile{Origin: "JFK"})
	if err != nil {
		t.Fatalf("SuggestTrip should degrade on flights err; got %v", err)
	}
	if len(trip.Flights) != 0 || len(trip.Days) != 1 {
		t.Fatalf("trip = %+v; want 0 flights, 1 day", trip)
	}
}

func TestSuggestTrip_InvalidDuration_Errors(t *testing.T) {
	planner := newComposerWithFlights(t, &fakeFlightFinder{})
	_, err := planner.SuggestTrip(context.Background(), "Paris", 0, tripplanner.Profile{})
	if err == nil {
		t.Fatal("durationDays=0: want error")
	}
	_, err = planner.SuggestTrip(context.Background(), "Paris", -1, tripplanner.Profile{})
	if err == nil {
		t.Fatal("durationDays=-1: want error")
	}
}
