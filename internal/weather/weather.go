// Package weather adapts Apple WeatherKit REST for widget tiles. Daemon
// signs the WeatherKit JWT from a local .p8 key; SwiftUI gets parsed
// forecast data only — never raw provider responses.
package weather

import (
	"errors"
	"time"
)

// Forecast is the normalized payload returned by Adapter.Fetch.
type Forecast struct {
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	Condition string  `json:"condition"`
	TempC     float64 `json:"temp_c"`
	TempF     float64 `json:"temp_f"`
	Humidity  float64 `json:"humidity"`
	AsOf      string  `json:"as_of"`
}

// Options configures an Adapter. JWTPath points to the WeatherKit .p8 key
// the daemon owns; BaseURL/Now are test seams.
type Options struct {
	Lat     float64
	Lon     float64
	JWTPath string
	BaseURL string
	Now     func() time.Time
}

func (o Options) validate() error {
	// Reject partial init: an unset coordinate is almost always a bug
	// (Lat=37.7749, Lon=0 lands in the Atlantic off Africa, not San Francisco).
	if o.Lat == 0 || o.Lon == 0 {
		return errors.New("weather: Options.Lat and Options.Lon required (non-zero)")
	}
	if o.JWTPath == "" {
		return errors.New("weather: Options.JWTPath required")
	}
	return nil
}
