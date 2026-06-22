package flights_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/trilam/leah/internal/flights"
)

func newAdapter(t *testing.T) *flights.Adapter {
	t.Helper()
	p, err := flights.NewFixtureProvider("testdata/sfo_lis_matrix.json")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return flights.NewAdapter(p)
}

// TestAdapter_Type returns the canonical kind string.
func TestAdapter_Type(t *testing.T) {
	a := newAdapter(t)
	if got := a.Type(); got != "flights" {
		t.Fatalf("Type() = %q, want %q", got, "flights")
	}
}

// TestAdapter_ValidateIATA covers positive + negative IATA regex cases.
func TestAdapter_ValidateIATA(t *testing.T) {
	a := newAdapter(t)
	cases := []struct {
		name    string
		props   string
		wantErr bool
	}{
		{"valid 3-letter codes", `{"origin":"SFO","destination":"LIS","depart":"2026-09-08","pax":1,"cabin":"economy","max_stops":1}`, false},
		{"lowercase origin", `{"origin":"sfo","destination":"LIS","depart":"2026-09-08","pax":1}`, true},
		{"4-letter dest", `{"origin":"SFO","destination":"LIST","depart":"2026-09-08","pax":1}`, true},
		{"2-letter origin", `{"origin":"SF","destination":"LIS","depart":"2026-09-08","pax":1}`, true},
		{"empty origin", `{"origin":"","destination":"LIS","depart":"2026-09-08","pax":1}`, true},
		{"digits in code", `{"origin":"SF1","destination":"LIS","depart":"2026-09-08","pax":1}`, true},
		{"pax 0", `{"origin":"SFO","destination":"LIS","depart":"2026-09-08","pax":0}`, true},
		{"pax 10", `{"origin":"SFO","destination":"LIS","depart":"2026-09-08","pax":10}`, true},
		{"max_stops 3", `{"origin":"SFO","destination":"LIS","depart":"2026-09-08","pax":1,"max_stops":3}`, true},
		{"max_stops -1", `{"origin":"SFO","destination":"LIS","depart":"2026-09-08","pax":1,"max_stops":-1}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := a.Validate(json.RawMessage(tc.props))
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// TestAdapter_Fetch28CellsWithGlobalMin asserts the date×price matrix shape + tagged min.
func TestAdapter_Fetch28CellsWithGlobalMin(t *testing.T) {
	a := newAdapter(t)
	props := json.RawMessage(`{"origin":"SFO","destination":"LIS","depart":"2026-09-08","pax":2,"cabin":"economy","max_stops":1}`)
	payload, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if payload.Source != "flights-fixture" {
		t.Fatalf("Source = %q", payload.Source)
	}
	if payload.StaleAfter != 0 {
		t.Fatalf("StaleAfter = %v, want 0 (manual-only)", payload.StaleAfter)
	}
	var m flights.Matrix
	if err := json.Unmarshal(payload.Data, &m); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if len(m.Cells) != 28 {
		t.Fatalf("cells = %d, want 28", len(m.Cells))
	}
	mins := 0
	minIdx := -1
	for i, c := range m.Cells {
		if c.GlobalMin {
			mins++
			minIdx = i
		}
	}
	if mins != 1 {
		t.Fatalf("global-min count = %d, want 1", mins)
	}
	for i, c := range m.Cells {
		if i == minIdx {
			continue
		}
		if c.PriceCents < m.Cells[minIdx].PriceCents {
			t.Fatalf("cell[%d] price %d below tagged min %d", i, c.PriceCents, m.Cells[minIdx].PriceCents)
		}
	}
}

// TestAdapter_RefreshIsManualOnly verifies auto-refresh returns ErrManualOnly.
func TestAdapter_RefreshIsManualOnly(t *testing.T) {
	a := newAdapter(t)
	props := json.RawMessage(`{"origin":"SFO","destination":"LIS","depart":"2026-09-08","pax":1}`)
	_, err := a.Refresh(context.Background(), "leah://widget/flights/1", props, nil)
	if !errors.Is(err, flights.ErrManualOnly) {
		t.Fatalf("Refresh err = %v, want ErrManualOnly", err)
	}
}

// TestAdapter_FetchCtxCancel ensures Fetch honours context cancellation.
func TestAdapter_FetchCtxCancel(t *testing.T) {
	a := newAdapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	props := json.RawMessage(`{"origin":"SFO","destination":"LIS","depart":"2026-09-08","pax":1}`)
	_, err := a.Fetch(ctx, props)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch with cancelled ctx err = %v, want context.Canceled", err)
	}
}

// TestAdapter_FetchInvalidPropsRejected guards Fetch against bypassing Validate.
func TestAdapter_FetchInvalidPropsRejected(t *testing.T) {
	a := newAdapter(t)
	props := json.RawMessage(`{"origin":"sfo","destination":"LIS","depart":"2026-09-08","pax":1}`)
	if _, err := a.Fetch(context.Background(), props); err == nil {
		t.Fatal("expected error for lowercase IATA")
	}
}

// TestAdapter_FetchedAtNonZero ensures payload carries fetch wall time.
func TestAdapter_FetchedAtNonZero(t *testing.T) {
	a := newAdapter(t)
	props := json.RawMessage(`{"origin":"SFO","destination":"LIS","depart":"2026-09-08","pax":1}`)
	before := time.Now().Add(-time.Second)
	p, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if p.FetchedAt.Before(before) {
		t.Fatalf("FetchedAt = %v before %v", p.FetchedAt, before)
	}
}
