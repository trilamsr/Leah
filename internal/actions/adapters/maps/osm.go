package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/trilam/leah/internal/platform/contracts"
	"github.com/trilam/leah/internal/platform/telemetry/connectadapter"
)

// Nominatim policy (https://operations.osmfoundation.org/policies/nominatim/):
// 1 req/s + identifying User-Agent are MANDATORY — violating either IP-bans the
// source. throttle() + nominatimUA honor both. OSRM demo is keyless, best-effort.
const (
	nominatimUA     = "leah/maps-adapter (https://github.com/trilam/leah)"
	osmGeocodeBase  = "https://nominatim.openstreetmap.org"
	osmRouteBase    = "https://router.project-osrm.org"
	nominatimMinGap = time.Second
)

// OSMConfig.BaseURL overrides BOTH Nominatim and OSRM hosts (tests); empty in
// prod. MinGap overrides the 1 req/s throttle (tests); zero keeps the policy gap.
type OSMConfig struct {
	Attestor   contracts.Attestor
	HTTPClient *http.Client
	BaseURL    string
	MinGap     time.Duration
	Metrics    *connectadapter.Metrics
}

// OSM is the keyless Nominatim/OSRM-backed Provider (sovereign posture).
type OSM struct {
	att         contracts.Attestor
	http        *http.Client
	geocodeBase string
	routeBase   string
	minGap      time.Duration
	m           *connectadapter.Metrics

	mu       sync.Mutex
	lastCall time.Time
}

func NewOSM(cfg OSMConfig) (*OSM, error) {
	if cfg.Attestor == nil {
		return nil, ErrAttestorRequired
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	gb, rb := osmGeocodeBase, osmRouteBase
	if cfg.BaseURL != "" {
		gb, rb = cfg.BaseURL, cfg.BaseURL
	}
	gap := cfg.MinGap
	if gap <= 0 {
		gap = nominatimMinGap
	}
	return &OSM{att: cfg.Attestor, http: hc, geocodeBase: gb, routeBase: rb, minGap: gap, m: cfg.Metrics}, nil
}

func (o *OSM) gate(ctx context.Context, scope string) error {
	if err := o.att.Attest(ctx, scope); err != nil {
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	return nil
}

// throttle enforces the 1 req/s policy without a background goroutine.
func (o *OSM) throttle(ctx context.Context) error {
	o.mu.Lock()
	wait := o.minGap - time.Since(o.lastCall)
	o.mu.Unlock()
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	o.mu.Lock()
	o.lastCall = time.Now()
	o.mu.Unlock()
	return nil
}

func (o *OSM) observe(rpc string, start time.Time) {
	if o.m != nil {
		o.m.ObserveAPI(rpc, time.Since(start).Seconds())
	}
}

type nominatimPlace struct {
	PlaceID     int64  `json:"place_id"`
	DisplayName string `json:"display_name"`
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	Type        string `json:"type"`
	Category    string `json:"category"`
}

func (n nominatimPlace) toPlace() Place {
	lat, _ := strconv.ParseFloat(n.Lat, 64)
	lng, _ := strconv.ParseFloat(n.Lon, 64)
	var cats []string
	if n.Category != "" {
		cats = append(cats, n.Category)
	}
	if n.Type != "" {
		cats = append(cats, n.Type)
	}
	return Place{ID: strconv.FormatInt(n.PlaceID, 10), Name: n.DisplayName, Lat: lat, Lng: lng, Categories: cats}
}

func (o *OSM) Geocode(ctx context.Context, address string) ([]Place, error) {
	if err := o.gate(ctx, ScopeGeocode); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("q", address)
	q.Set("format", "jsonv2")
	start := time.Now()
	defer o.observe("geocode", start)
	var body []nominatimPlace
	if err := o.getJSON(ctx, o.geocodeBase+"/search?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	out := make([]Place, 0, len(body))
	for _, n := range body {
		out = append(out, n.toPlace())
	}
	return out, nil
}

func (o *OSM) ReverseGeocode(ctx context.Context, lat, lng float64) ([]Place, error) {
	if err := o.gate(ctx, ScopeGeocode); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("lat", strconv.FormatFloat(lat, 'f', -1, 64))
	q.Set("lon", strconv.FormatFloat(lng, 'f', -1, 64))
	q.Set("format", "jsonv2")
	start := time.Now()
	defer o.observe("reverse_geocode", start)
	var n nominatimPlace
	if err := o.getJSON(ctx, o.geocodeBase+"/reverse?"+q.Encode(), &n); err != nil {
		return nil, err
	}
	if n.PlaceID == 0 && n.DisplayName == "" {
		return nil, ErrNoResults
	}
	return []Place{n.toPlace()}, nil
}

type osrmResponse struct {
	Code   string `json:"code"`
	Routes []struct {
		Geometry string  `json:"geometry"`
		Distance float64 `json:"distance"`
		Duration float64 `json:"duration"`
		Legs     []struct {
			Steps []struct {
				Name     string  `json:"name"`
				Distance float64 `json:"distance"`
				Duration float64 `json:"duration"`
			} `json:"steps"`
		} `json:"legs"`
	} `json:"routes"`
}

// Route takes origin/dest "lng,lat" pairs. The demo server only carries the
// driving profile, so mode is intentionally ignored rather than ErrUnsupported.
func (o *OSM) Route(ctx context.Context, origin, dest string, _ TransportMode) (Route, error) {
	if err := o.gate(ctx, ScopeRoute); err != nil {
		return Route{}, err
	}
	q := url.Values{}
	q.Set("overview", "full")
	q.Set("steps", "true")
	start := time.Now()
	defer o.observe("route", start)
	u := o.routeBase + "/route/v1/driving/" + url.PathEscape(origin) + ";" + url.PathEscape(dest) + "?" + q.Encode()
	var body osrmResponse
	if err := o.getJSON(ctx, u, &body); err != nil {
		return Route{}, err
	}
	if body.Code != "Ok" || len(body.Routes) == 0 {
		return Route{}, ErrNoRoute
	}
	r := body.Routes[0]
	var steps []Step
	if len(r.Legs) > 0 {
		steps = make([]Step, 0, len(r.Legs[0].Steps))
		for _, s := range r.Legs[0].Steps {
			steps = append(steps, Step{Instructions: s.Name, DistanceM: int(s.Distance), DurationS: int(s.Duration)})
		}
	}
	return Route{Polyline: r.Geometry, DistanceM: int(r.Distance), DurationS: int(r.Duration), Steps: steps}, nil
}

// POINearby — Nominatim is a geocoder, not a ranked-POI search; use Google.
func (o *OSM) POINearby(context.Context, Place, int, string) ([]Place, error) {
	return nil, fmt.Errorf("POINearby: %w", ErrUnsupported)
}

func (o *OSM) POIAlongRoute(context.Context, Route, CorridorOpts) ([]Place, error) {
	return nil, fmt.Errorf("POIAlongRoute: %w", ErrUnsupported)
}

// TrafficETA — OSRM has no live-traffic model.
func (o *OSM) TrafficETA(context.Context, Route, time.Time) (ETA, error) {
	return ETA{}, fmt.Errorf("TrafficETA: %w", ErrUnsupported)
}

func (o *OSM) getJSON(ctx context.Context, rawURL string, out any) error {
	if err := o.throttle(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("maps/osm: build request: %w", err)
	}
	req.Header.Set("User-Agent", nominatimUA)
	resp, err := o.http.Do(req)
	if err != nil {
		return fmt.Errorf("maps/osm: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %d", ErrAPIStatus, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("maps/osm: decode: %w", err)
	}
	return nil
}

var _ Provider = (*OSM)(nil)
