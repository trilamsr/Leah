package maps

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTrafficETA_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/directions/json") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("departure_time") == "" {
			t.Errorf("departure_time missing")
		}
		_, _ = w.Write([]byte(`{
			"status": "OK",
			"routes": [{
				"legs": [{
					"distance": {"value": 5000},
					"duration": {"value": 600},
					"duration_in_traffic": {"value": 780}
				}]
			}]
		}`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, srv)
	depart := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	eta, err := a.TrafficETA(context.Background(), Route{Polyline: "abc"}, depart)
	if err != nil {
		t.Fatalf("TrafficETA: %v", err)
	}
	if eta.DurationS != 780 {
		t.Fatalf("DurationS = %d, want 780", eta.DurationS)
	}
	if eta.TrafficDelaySec != 180 {
		t.Fatalf("TrafficDelaySec = %d, want 180", eta.TrafficDelaySec)
	}
	if !eta.DepartAt.Equal(depart) {
		t.Fatalf("DepartAt = %v, want %v", eta.DepartAt, depart)
	}
	if att.lastScope != ScopeTraffic {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeTraffic)
	}
}

func TestTrafficETA_AttestationDenied_NoHTTPCall(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("HTTP MUST NOT fire on attestation deny")
	}))
	defer srv.Close()
	att := &fakeAttestor{err: errors.New("denied")}
	a := newTestAdapter(t, att, srv)
	if _, err := a.TrafficETA(context.Background(), Route{}, time.Now()); !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

func TestTrafficETA_NoRoute(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ZERO_RESULTS","routes":[]}`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, srv)
	if _, err := a.TrafficETA(context.Background(), Route{}, time.Now()); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("err = %v, want ErrNoRoute", err)
	}
}

func TestTrafficETA_CacheHitSkipsHTTP(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{
			"status":"OK",
			"routes":[{"legs":[{"distance":{"value":1},"duration":{"value":100},"duration_in_traffic":{"value":120}}]}]
		}`))
	}))
	defer srv.Close()

	c, err := OpenCache(tempCachePath(t))
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	defer func() { _ = c.Close() }()
	a, err := New(Config{
		Attestor:   &fakeAttestor{},
		HTTPClient: srv.Client(),
		APIKey:     "k",
		BaseURL:    srv.URL,
		Cache:      c,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	depart := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	rt := Route{Polyline: "p"}
	if _, err := a.TrafficETA(context.Background(), rt, depart); err != nil {
		t.Fatalf("first TrafficETA: %v", err)
	}
	if _, err := a.TrafficETA(context.Background(), rt, depart); err != nil {
		t.Fatalf("second TrafficETA: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1 (cached second hit)", got)
	}
}

func TestGeocode_CacheHitSkipsHTTP(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{
			"status":"OK",
			"results":[{
				"place_id":"x","formatted_address":"y",
				"geometry":{"location":{"lat":1,"lng":2}},
				"types":["street_address"]
			}]
		}`))
	}))
	defer srv.Close()
	c, err := OpenCache(tempCachePath(t))
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	defer func() { _ = c.Close() }()
	a, err := New(Config{
		Attestor:   &fakeAttestor{},
		HTTPClient: srv.Client(),
		APIKey:     "k",
		BaseURL:    srv.URL,
		Cache:      c,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Geocode(context.Background(), "addr"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := a.Geocode(context.Background(), "addr"); err != nil {
		t.Fatalf("second: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("HTTP calls = %d, want 1 (geocode 30d TTL)", got)
	}
}
