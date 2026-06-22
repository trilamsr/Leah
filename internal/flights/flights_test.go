package flights_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/trilam/leah/internal/flights"
)

// TestQuery_JSONRoundTrip ensures Query marshals and unmarshals symmetrically.
func TestQuery_JSONRoundTrip(t *testing.T) {
	q := flights.Query{
		Origin:      "SFO",
		Destination: "LIS",
		Depart:      "2026-09-08",
		Return:      "2026-09-19",
		Pax:         2,
		Cabin:       "economy",
		MaxStops:    1,
	}
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got flights.Query
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != q {
		t.Fatalf("round-trip mismatch: %+v vs %+v", got, q)
	}
}

// TestMatrix_GlobalMin asserts Matrix tags the lowest cell once.
func TestMatrix_GlobalMin(t *testing.T) {
	m := flights.Matrix{
		Origin:      "SFO",
		Destination: "LIS",
		FetchedAt:   time.Now(),
		Cells: []flights.Cell{
			{Date: "2026-09-08", PriceCents: 81200},
			{Date: "2026-09-09", PriceCents: 68900, GlobalMin: true},
			{Date: "2026-09-10", PriceCents: 76000},
		},
	}
	mins := 0
	for _, c := range m.Cells {
		if c.GlobalMin {
			mins++
		}
	}
	if mins != 1 {
		t.Fatalf("expected exactly 1 global-min cell, got %d", mins)
	}
}
