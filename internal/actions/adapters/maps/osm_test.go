package maps

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/contracts"
)

func newTestOSM(t *testing.T, att contracts.Attestor, srv *httptest.Server) *OSM {
	t.Helper()
	o, err := NewOSM(OSMConfig{Attestor: att, HTTPClient: srv.Client(), BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewOSM: %v", err)
	}
	return o
}

// TestOSM_SatisfiesProvider: drop-in for Google behind the Provider surface.
func TestOSM_SatisfiesProvider(t *testing.T) {
	t.Parallel()
	var _ Provider = (*OSM)(nil)
}

// TestOSM_Geocode_HappyPath: Nominatim /search parses into a Place + sets UA.
func TestOSM_Geocode_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "jsonv2" {
			t.Errorf("format not set: %q", r.URL.Query().Get("format"))
		}
		if r.URL.Query().Get("q") != "Berlin" {
			t.Errorf("q not forwarded: %q", r.URL.Query().Get("q"))
		}
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "leah") {
			t.Errorf("missing/wrong User-Agent: %q", ua)
		}
		_, _ = w.Write([]byte(`[
			{"place_id": 123, "display_name": "Berlin, Germany", "lat": "52.52", "lon": "13.405", "type": "city", "category": "place"}
		]`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	o := newTestOSM(t, att, srv)
	places, err := o.Geocode(context.Background(), "Berlin")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if len(places) != 1 {
		t.Fatalf("len(places) = %d, want 1", len(places))
	}
	p := places[0]
	if p.ID != "123" || p.Lat != 52.52 || p.Lng != 13.405 || p.Name != "Berlin, Germany" {
		t.Fatalf("place parsed wrong: %+v", p)
	}
	if att.lastScope != ScopeGeocode {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeGeocode)
	}
}

// TestOSM_ReverseGeocode_HappyPath: Nominatim /reverse parses into a Place.
func TestOSM_ReverseGeocode_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/reverse") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("lat") != "52.52" || r.URL.Query().Get("lon") != "13.405" {
			t.Errorf("latlon not forwarded: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"place_id": 7, "display_name": "Mitte, Berlin", "lat": "52.52", "lon": "13.405", "type": "suburb", "category": "boundary"}`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	o := newTestOSM(t, att, srv)
	places, err := o.ReverseGeocode(context.Background(), 52.52, 13.405)
	if err != nil {
		t.Fatalf("ReverseGeocode: %v", err)
	}
	if len(places) != 1 || places[0].ID != "7" {
		t.Fatalf("reverse parsed wrong: %+v", places)
	}
	if att.lastScope != ScopeGeocode {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeGeocode)
	}
}

// TestOSM_Route_HappyPath: OSRM /route parses geometry + distance + duration.
func TestOSM_Route_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/route/v1/driving/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"code": "Ok",
			"routes": [{
				"geometry": "abc_poly",
				"distance": 5000.0,
				"duration": 600.0,
				"legs": [{"steps": [{"name": "Main St", "distance": 1000.0, "duration": 120.0}]}]
			}]
		}`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	o := newTestOSM(t, att, srv)
	rt, err := o.Route(context.Background(), "13.388,52.517", "13.397,52.529", ModeDriving)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if rt.Polyline != "abc_poly" || rt.DistanceM != 5000 || rt.DurationS != 600 {
		t.Fatalf("route parsed wrong: %+v", rt)
	}
	if len(rt.Steps) != 1 || rt.Steps[0].DistanceM != 1000 {
		t.Fatalf("steps parsed wrong: %+v", rt.Steps)
	}
	if att.lastScope != ScopeRoute {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeRoute)
	}
}

// TestOSM_Route_NoRoute: OSRM code != Ok maps to ErrNoRoute.
func TestOSM_Route_NoRoute(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code": "NoRoute", "routes": []}`))
	}))
	defer srv.Close()
	o := newTestOSM(t, &fakeAttestor{}, srv)
	if _, err := o.Route(context.Background(), "a", "b", ModeDriving); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("err = %v, want ErrNoRoute", err)
	}
}

// TestOSM_AttestationDenied_NoHTTP: gate-ordering holds for the OSM backend too.
func TestOSM_AttestationDenied_NoHTTP(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("HTTP MUST NOT be called when attestation denies")
	}))
	defer srv.Close()
	att := &fakeAttestor{err: errors.New("denied")}
	o := newTestOSM(t, att, srv)
	if _, err := o.Geocode(context.Background(), "x"); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

// TestOSM_Unsupported: RPCs OSM cannot serve fail with ErrUnsupported, not fake data.
func TestOSM_Unsupported(t *testing.T) {
	t.Parallel()
	o := newTestOSM(t, &fakeAttestor{}, httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})))
	if _, err := o.POINearby(context.Background(), Place{}, 100, "cafe"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("POINearby err = %v, want ErrUnsupported", err)
	}
	if _, err := o.POIAlongRoute(context.Background(), Route{}, CorridorOpts{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("POIAlongRoute err = %v, want ErrUnsupported", err)
	}
	if _, err := o.TrafficETA(context.Background(), Route{}, time.Now()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("TrafficETA err = %v, want ErrUnsupported", err)
	}
}

// TestOSM_Throttle_Spaces1ReqPerSec: back-to-back calls honor MinGap — the
// IP-ban-prevention half of Nominatim's policy.
func TestOSM_Throttle_Spaces1ReqPerSec(t *testing.T) {
	t.Parallel()
	var hits []time.Time
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits = append(hits, time.Now())
		mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	gap := 80 * time.Millisecond
	o, err := NewOSM(OSMConfig{Attestor: &fakeAttestor{}, HTTPClient: srv.Client(), BaseURL: srv.URL, MinGap: gap})
	if err != nil {
		t.Fatalf("NewOSM: %v", err)
	}
	ctx := context.Background()
	if _, err := o.Geocode(ctx, "a"); err != nil {
		t.Fatalf("first Geocode: %v", err)
	}
	if _, err := o.Geocode(ctx, "b"); err != nil {
		t.Fatalf("second Geocode: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hits) != 2 {
		t.Fatalf("server hits = %d, want 2", len(hits))
	}
	// 90% tolerates timer jitter yet still rules out the no-throttle (sub-ms) case.
	if min := gap * 9 / 10; hits[1].Sub(hits[0]) < min {
		t.Fatalf("calls spaced %v, want >= %v (throttle not enforced)", hits[1].Sub(hits[0]), min)
	}
}

// TestNewOSM_RejectsMissingAttestor: fail-closed on nil Attestor.
func TestNewOSM_RejectsMissingAttestor(t *testing.T) {
	t.Parallel()
	if _, err := NewOSM(OSMConfig{}); !errors.Is(err, ErrAttestorRequired) {
		t.Fatalf("err = %v, want ErrAttestorRequired", err)
	}
}
