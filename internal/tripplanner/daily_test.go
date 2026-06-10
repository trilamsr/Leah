package tripplanner_test

import (
	"context"
	"testing"
	"time"

	"github.com/trilam/leah/internal/tripplanner"
)

// categoryNearby returns a different POI set per category — lets the
// DailyItinerary tests drive interest-fanout from the test fixture.
type categoryNearby struct {
	byCat map[string][]tripplanner.Place
}

func (c *categoryNearby) POINearby(_ context.Context, _ tripplanner.Place, _ int, category string) ([]tripplanner.Place, error) {
	return c.byCat[category], nil
}

func TestDailyItinerary_HappyPath(t *testing.T) {
	geo := &fakeGeocoder{results: map[string][]tripplanner.Place{
		"Paris": {{ID: "city", Name: "Paris", Lat: 48.85, Lng: 2.35}},
	}}
	near := &categoryNearby{byCat: map[string][]tripplanner.Place{
		"museum":     {{ID: "louvre", Name: "Louvre", Categories: []string{"museum"}}},
		"restaurant": {{ID: "lecinq", Name: "Le Cinq", Categories: []string{"restaurant"}}},
	}}
	planner, err := tripplanner.New(tripplanner.Config{
		Router: &fakeRouter{}, Corridor: &fakeCorridor{}, Geocoder: geo, Nearby: near,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	when := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	itin, err := planner.DailyItinerary(context.Background(), "Paris", when, tripplanner.Profile{
		Interests: []string{"museum", "restaurant"},
	})
	if err != nil {
		t.Fatalf("DailyItinerary: %v", err)
	}
	if !itin.Date.Equal(when) {
		t.Fatalf("Date = %v; want %v", itin.Date, when)
	}
	if len(itin.Places) != 2 {
		t.Fatalf("Places = %+v; want 2 (louvre + lecinq)", itin.Places)
	}
}

func TestDailyItinerary_EmptyInterests_ReturnsDefaults(t *testing.T) {
	geo := &fakeGeocoder{results: map[string][]tripplanner.Place{
		"Rome": {{ID: "city", Lat: 41.9, Lng: 12.5}},
	}}
	// Defaults are restaurant + attraction per spec.
	near := &categoryNearby{byCat: map[string][]tripplanner.Place{
		"restaurant": {{ID: "r1", Name: "Trattoria"}},
		"attraction": {{ID: "a1", Name: "Colosseum"}},
	}}
	planner, err := tripplanner.New(tripplanner.Config{
		Router: &fakeRouter{}, Corridor: &fakeCorridor{}, Geocoder: geo, Nearby: near,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	itin, err := planner.DailyItinerary(context.Background(), "Rome", time.Now(), tripplanner.Profile{})
	if err != nil {
		t.Fatalf("DailyItinerary: %v", err)
	}
	if len(itin.Places) != 2 {
		t.Fatalf("Places = %+v; want 2 defaults", itin.Places)
	}
}

func TestDailyItinerary_ClosedAtTime_ExcludesPlace(t *testing.T) {
	geo := &fakeGeocoder{results: map[string][]tripplanner.Place{
		"Berlin": {{ID: "city", Lat: 52.5, Lng: 13.4}},
	}}
	// 2am query — restaurant with 9am-10pm hours must be excluded.
	openMorning := tripplanner.Place{
		ID:   "cafe-day",
		Name: "Day Cafe",
		Hours: []tripplanner.TimeWindow{
			{OpenMinute: 9 * 60, CloseMinute: 22 * 60},
		},
	}
	open24h := tripplanner.Place{ID: "diner-24", Name: "Diner 24"} // no Hours = always open
	near := &categoryNearby{byCat: map[string][]tripplanner.Place{
		"restaurant": {openMorning, open24h},
	}}
	planner, err := tripplanner.New(tripplanner.Config{
		Router: &fakeRouter{}, Corridor: &fakeCorridor{}, Geocoder: geo, Nearby: near,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	at2am := time.Date(2026, 7, 4, 2, 0, 0, 0, time.UTC)
	itin, err := planner.DailyItinerary(context.Background(), "Berlin", at2am, tripplanner.Profile{
		Interests: []string{"restaurant"},
	})
	if err != nil {
		t.Fatalf("DailyItinerary: %v", err)
	}
	if len(itin.Places) != 1 || itin.Places[0].ID != "diner-24" {
		t.Fatalf("Places = %+v; want only diner-24", itin.Places)
	}
}

func TestDailyItinerary_GeocodeMiss_ErrNoCity(t *testing.T) {
	geo := &fakeGeocoder{results: map[string][]tripplanner.Place{}}
	near := &categoryNearby{}
	planner, err := tripplanner.New(tripplanner.Config{
		Router: &fakeRouter{}, Corridor: &fakeCorridor{}, Geocoder: geo, Nearby: near,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = planner.DailyItinerary(context.Background(), "Atlantis", time.Now(), tripplanner.Profile{})
	if err == nil {
		t.Fatal("unknown city: want error")
	}
}
