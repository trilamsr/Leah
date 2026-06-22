// Package maps adapts Apple Maps Server forward-geocode for widget tiles.
// Daemon NEVER fetches tiles — SwiftUI renders the map via MapKit; the
// adapter only resolves a place_query to lat/lon + label.
package maps

import "errors"

// Geocode is the normalized payload Adapter.Fetch returns. Lat/lon let the
// SwiftUI MapKit view center; label feeds the tile caption.
type Geocode struct {
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Label string  `json:"label"`
}

// Options configures an Adapter. JWTPath holds the Apple Maps Server .p8
// auth key the daemon owns; BaseURL is a test seam.
type Options struct {
	JWTPath string
	BaseURL string
}

func (o Options) validate() error {
	if o.JWTPath == "" {
		return errors.New("maps: Options.JWTPath required")
	}
	return nil
}
