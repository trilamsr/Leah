package maps

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeAttestor records scope so tests can assert per-RPC gate routing.
type fakeAttestor struct {
	called    int
	lastScope string
	scopes    []string
	err       error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	f.lastScope = scope
	f.scopes = append(f.scopes, scope)
	if !strings.HasPrefix(scope, "maps:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

func newTestAdapter(t *testing.T, att Attestor, srv *httptest.Server) *Adapter {
	t.Helper()
	a, err := New(Config{
		Attestor:   att,
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		BaseURL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// TestGeocode_HappyPath: stubbed Geocoding response parses into a Place.
func TestGeocode_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/geocode/json") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("missing api key")
		}
		if r.URL.Query().Get("address") != "1600 Amphitheatre Pkwy" {
			t.Errorf("address not forwarded: %q", r.URL.Query().Get("address"))
		}
		_, _ = w.Write([]byte(`{
			"status": "OK",
			"results": [{
				"place_id": "ChIJtest",
				"formatted_address": "1600 Amphitheatre Parkway, Mountain View, CA",
				"geometry": {"location": {"lat": 37.4221, "lng": -122.0841}},
				"types": ["street_address"]
			}]
		}`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, srv)
	places, err := a.Geocode(context.Background(), "1600 Amphitheatre Pkwy")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if len(places) != 1 {
		t.Fatalf("len(places) = %d, want 1", len(places))
	}
	p := places[0]
	if p.ID != "ChIJtest" || p.Lat != 37.4221 || p.Lng != -122.0841 {
		t.Fatalf("place parsed wrong: %+v", p)
	}
	if att.lastScope != ScopeGeocode {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeGeocode)
	}
}

// TestGeocode_AttestationDenied_NoHTTPCall: gate-ordering — HTTP MUST NOT fire on deny.
func TestGeocode_AttestationDenied_NoHTTPCall(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("HTTP MUST NOT be called when attestation denies")
	}))
	defer srv.Close()
	att := &fakeAttestor{err: errors.New("denied")}
	a := newTestAdapter(t, att, srv)
	if _, err := a.Geocode(context.Background(), "anywhere"); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

// TestRoute_HappyPath: Directions response parses into a Route.
func TestRoute_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/directions/json") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("mode") != "driving" {
			t.Errorf("mode not forwarded: %q", r.URL.Query().Get("mode"))
		}
		_, _ = w.Write([]byte(`{
			"status": "OK",
			"routes": [{
				"overview_polyline": {"points": "abc123"},
				"legs": [{
					"distance": {"value": 5000},
					"duration": {"value": 600},
					"steps": [{
						"html_instructions": "Head north",
						"distance": {"value": 1000},
						"duration": {"value": 120}
					}]
				}]
			}]
		}`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, srv)
	rt, err := a.Route(context.Background(), "A", "B", ModeDriving)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if rt.Polyline != "abc123" || rt.DistanceM != 5000 || rt.DurationS != 600 {
		t.Fatalf("route parsed wrong: %+v", rt)
	}
	if len(rt.Steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(rt.Steps))
	}
	if att.lastScope != ScopeRoute {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeRoute)
	}
}

// TestPOINearby_HappyPath: Places Nearby Search response parses into places.
func TestPOINearby_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/place/nearbysearch/json") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("radius") != "500" {
			t.Errorf("radius not forwarded")
		}
		if r.URL.Query().Get("type") != "cafe" {
			t.Errorf("type not forwarded")
		}
		_, _ = w.Write([]byte(`{
			"status": "OK",
			"results": [
				{"place_id": "p1", "name": "Cafe A", "geometry": {"location": {"lat": 1.0, "lng": 2.0}}, "types": ["cafe"], "rating": 4.5},
				{"place_id": "p2", "name": "Cafe B", "geometry": {"location": {"lat": 1.1, "lng": 2.1}}, "types": ["cafe"], "rating": 4.0}
			]
		}`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, srv)
	center := Place{Lat: 1.0, Lng: 2.0}
	places, err := a.POINearby(context.Background(), center, 500, "cafe")
	if err != nil {
		t.Fatalf("POINearby: %v", err)
	}
	if len(places) != 2 {
		t.Fatalf("len(places) = %d, want 2", len(places))
	}
	if places[0].Name != "Cafe A" || places[0].Rating != 4.5 {
		t.Fatalf("first place wrong: %+v", places[0])
	}
	if att.lastScope != ScopePOI {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopePOI)
	}
}

// TestNewRejectsMissingDeps: nil Attestor and empty APIKey are fail-closed.
func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
		want error
	}{
		{name: "no attestor", cfg: Config{APIKey: "k"}, want: ErrAttestorRequired},
		{name: "empty api key", cfg: Config{Attestor: &fakeAttestor{}}, want: ErrAPIKeyRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.cfg)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestAttestationScope_PerOperation: each RPC routes its own scope to the gate.
func TestAttestationScope_PerOperation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status": "OK", "results": [], "routes": []}`))
	}))
	defer srv.Close()

	cases := []struct {
		name  string
		call  func(*Adapter) error
		scope string
	}{
		{
			name:  "geocode",
			call:  func(a *Adapter) error { _, err := a.Geocode(context.Background(), "x"); return err },
			scope: ScopeGeocode,
		},
		{
			name:  "route",
			call:  func(a *Adapter) error { _, err := a.Route(context.Background(), "a", "b", ModeDriving); return err },
			scope: ScopeRoute,
		},
		{
			name: "poi",
			call: func(a *Adapter) error {
				_, err := a.POINearby(context.Background(), Place{Lat: 1, Lng: 2}, 100, "cafe")
				return err
			},
			scope: ScopePOI,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			att := &fakeAttestor{}
			a := newTestAdapter(t, att, srv)
			// Route returns ErrNoRoute when results are empty; ignore non-attestation errors.
			_ = tc.call(a)
			if att.lastScope != tc.scope {
				t.Fatalf("scope = %q, want %q", att.lastScope, tc.scope)
			}
		})
	}
}
