// Package flights binds the `flights` widget kind to a date×price matrix
// provider. Spec §10.1 entry 2 — manual-refresh only.
package flights

import (
	"context"
	"errors"
	"time"
)

// ErrManualOnly is returned by Adapter.Refresh — flights fares are polled
// only on operator action (spec §10.7 refresh-cadence table).
var ErrManualOnly = errors.New("flights: refresh is manual-only")

// Query is the operator-facing search shape. Field tags mirror the JSON
// Schema in spec §10.1 entry 2.
type Query struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	Depart      string `json:"depart"`
	Return      string `json:"return,omitempty"`
	Pax         int    `json:"pax"`
	Cabin       string `json:"cabin,omitempty"`
	MaxStops    int    `json:"max_stops,omitempty"`
}

// Cell is one date×price point in the matrix.
type Cell struct {
	Date       string `json:"date"`
	PriceCents int    `json:"price_cents"`
	Carrier    string `json:"carrier,omitempty"`
	Stops      int    `json:"stops"`
	GlobalMin  bool   `json:"global_min,omitempty"`
}

// Matrix is the rendered widget payload.
type Matrix struct {
	Origin      string    `json:"origin"`
	Destination string    `json:"destination"`
	Currency    string    `json:"currency"`
	FetchedAt   time.Time `json:"fetched_at"`
	Cells       []Cell    `json:"cells"`
}

// Provider sources fare cells. The fixture provider serves v1; HTTP-backed
// providers (Kiwi, Amadeus) implement the same surface later. Source names
// the upstream so Payload.Source carries the attribution.
type Provider interface {
	Search(ctx context.Context, q Query) (Matrix, error)
	Source() string
}
