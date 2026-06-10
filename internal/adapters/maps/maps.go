package maps

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors are exported so callers switch on failure mode instead of
// string-matching wrapped messages.
var (
	ErrAttestationDenied = errors.New("maps: attestation denied")
	ErrAttestorRequired  = errors.New("maps: Config.Attestor required (operator-attestation gate)")
	ErrAPIKeyRequired    = errors.New("maps: Config.APIKey required")
	ErrNoResults         = errors.New("maps: no results")
	ErrNoRoute           = errors.New("maps: no route")
	ErrAPIStatus         = errors.New("maps: api non-OK status")
)

// Scopes the Attestor sees. Distinct per RPC so the operator-attestation log
// attributes consent at the per-action grain (geocode vs. route vs. poi).
const (
	ScopeGeocode       = "maps:geocode"
	ScopeRoute         = "maps:route"
	ScopePOI           = "maps:poi"
	ScopePOIAlongRoute = "maps:poi_along_route"
)

// TransportMode is the Directions API mode parameter; constants below cover
// the four MVP-supported modes.
type TransportMode string

const (
	ModeDriving TransportMode = "driving"
	ModeWalking TransportMode = "walking"
	ModeTransit TransportMode = "transit"
	ModeBiking  TransportMode = "bicycling"
)

// OpenHours is a placeholder for Places opening_hours; the wiring wave will
// expand this when the morning brief consumes hours.
type OpenHours struct {
	OpenNow bool
}

// Place is the minimal place envelope the MVP cares about.
type Place struct {
	ID         string
	Name       string
	Lat        float64
	Lng        float64
	Categories []string
	Hours      OpenHours
	Rating     float32
}

// Step is one leg-step in a Route; HTML stripping deferred to wiring wave.
type Step struct {
	Instructions string
	DistanceM    int
	DurationS    int
}

// Toll is reserved for the Routes-API v2 toll surface; left empty in W56.
type Toll struct {
	Currency string
	Amount   float64
}

// Route is the directions envelope returned by Route().
type Route struct {
	Polyline  string
	DistanceM int
	DurationS int
	Steps     []Step
	Tolls     []Toll
}

// Attestor is Leah's operator-attestation gate; every Maps RPC calls
// Attest(scope) before issuing the HTTP request.
type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

// Config carries the adapter's collaborators. HTTPClient is injectable so
// tests use httptest without hitting the real Google endpoints. BaseURL is
// also injectable for tests; production callers leave it empty to use the
// real Google Maps endpoints.
type Config struct {
	Attestor   Attestor
	HTTPClient *http.Client
	APIKey     string
	BaseURL    string
}

// Adapter is the Maps adapter the rest of Leah depends on. No background
// goroutines; lifecycle is owned by the caller.
type Adapter struct {
	att     Attestor
	http    *http.Client
	apiKey  string
	baseURL string
}

// defaultBaseURL points at Google's Maps REST endpoints; tests override via
// Config.BaseURL so httptest can stand in.
const defaultBaseURL = "https://maps.googleapis.com/maps/api"

// New validates the wiring contract and returns a ready Adapter; no I/O.
func New(cfg Config) (*Adapter, error) {
	if cfg.Attestor == nil {
		return nil, ErrAttestorRequired
	}
	if cfg.APIKey == "" {
		return nil, ErrAPIKeyRequired
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	return &Adapter{att: cfg.Attestor, http: hc, apiKey: cfg.APIKey, baseURL: base}, nil
}

// gate runs the attestation gate; only on consent does the caller issue HTTP.
// Ordering matters — issuing the request before attestation would leak the
// API key and operator intent into a wire trace on denial.
func (a *Adapter) gate(ctx context.Context, scope string) error {
	if err := a.att.Attest(ctx, scope); err != nil {
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	return nil
}
