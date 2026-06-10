package maps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to maps.
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := obs.NewRegistry()
	RegisterMetrics(r)
	keys := obstest.SnapshotKeys(t, r)
	for _, want := range []string{
		"leah_connect_api_call_total",
		"leah_connect_exchange_total",
		"leah_connect_refresh_total",
		"leah_connect_token_age_seconds",
		"leah_connect_api_latency_seconds",
	} {
		if !obstest.ContainsPrefix(keys, want) {
			t.Fatalf("series %q missing from %v", want, keys)
		}
	}
}

// TestObserveAPI_OnRPC asserts each HTTP-backed RPC bumps api_call_total on success and failure.
func TestObserveAPI_OnRPC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		body     string
		run      func(*testing.T, *Adapter)
	}{
		{
			name:     "geocode success",
			endpoint: "geocode",
			body:     `{"status":"OK","results":[{"place_id":"p","formatted_address":"a","geometry":{"location":{"lat":1,"lng":2}}}]}`,
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.Geocode(context.Background(), "x"); err != nil {
					t.Fatalf("Geocode: %v", err)
				}
			},
		},
		{
			name:     "geocode failure still observed",
			endpoint: "geocode",
			body:     `{"status":"REQUEST_DENIED"}`,
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.Geocode(context.Background(), "x"); err == nil {
					t.Fatal("Geocode: expected api status error")
				}
			},
		},
		{
			name:     "reverse_geocode success",
			endpoint: "reverse_geocode",
			body:     `{"status":"OK","results":[]}`,
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.ReverseGeocode(context.Background(), 1, 2); err != nil {
					t.Fatalf("ReverseGeocode: %v", err)
				}
			},
		},
		{
			name:     "route success",
			endpoint: "route",
			body:     `{"status":"OK","routes":[{"overview_polyline":{"points":"p"},"legs":[{"distance":{"value":1},"duration":{"value":1}}]}]}`,
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.Route(context.Background(), "a", "b", ModeDriving); err != nil {
					t.Fatalf("Route: %v", err)
				}
			},
		},
		{
			name:     "route failure still observed",
			endpoint: "route",
			body:     `{"status":"REQUEST_DENIED"}`,
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.Route(context.Background(), "a", "b", ModeDriving); err == nil {
					t.Fatal("Route: expected api status error")
				}
			},
		},
		{
			name:     "poi_nearby success",
			endpoint: "poi_nearby",
			body:     `{"status":"OK","results":[]}`,
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.POINearby(context.Background(), Place{Lat: 1, Lng: 2}, 100, "cafe"); err != nil {
					t.Fatalf("POINearby: %v", err)
				}
			},
		},
		{
			name:     "traffic_eta success",
			endpoint: "traffic_eta",
			body:     `{"status":"OK","routes":[{"legs":[{"distance":{"value":1},"duration":{"value":10},"duration_in_traffic":{"value":15}}]}]}`,
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.TrafficETA(context.Background(), Route{Polyline: "abc"}, time.Unix(1700000000, 0)); err != nil {
					t.Fatalf("TrafficETA: %v", err)
				}
			},
		},
		{
			name:     "traffic_eta failure still observed",
			endpoint: "traffic_eta",
			body:     `{"status":"REQUEST_DENIED"}`,
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.TrafficETA(context.Background(), Route{Polyline: "abc"}, time.Unix(1700000000, 0)); err == nil {
					t.Fatal("TrafficETA: expected api status error")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			r := obs.NewRegistry()
			RegisterMetrics(r)
			a, err := New(Config{
				Attestor:   &fakeAttestor{},
				HTTPClient: srv.Client(),
				APIKey:     "k",
				BaseURL:    srv.URL,
				Metrics:    Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			tc.run(t, a)
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=maps"
			if !obstest.ContainsExact(keys, want) {
				t.Fatalf("missing %q in %v", want, keys)
			}
			if !obstest.ContainsPrefix(keys, "leah_connect_api_latency_seconds") {
				t.Fatalf("latency histogram not observed: %v", keys)
			}
		})
	}
}

// TestObserveAPI_NilMetricsNoop proves the legacy Config caller shape still works when Metrics is omitted.
func TestObserveAPI_NilMetricsNoop(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"OK","results":[]}`))
	}))
	defer srv.Close()
	a, err := New(Config{
		Attestor:   &fakeAttestor{},
		HTTPClient: srv.Client(),
		APIKey:     "k",
		BaseURL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Geocode(context.Background(), "x"); err != nil {
		t.Fatalf("Geocode: %v", err)
	}
}
