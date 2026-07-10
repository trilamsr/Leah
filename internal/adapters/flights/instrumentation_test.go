package flights

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/obs/obstest"
)

// TestRegisterMetrics_AddsSeries asserts the shared leah_connect_* series surface bound to flights.
func TestRegisterMetrics_AddsSeries(t *testing.T) {
	r := telemetry.NewRegistry()
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
		handler  http.HandlerFunc
		run      func(*testing.T, *Adapter)
	}{
		{
			name:     "search_offers success",
			endpoint: "search_offers",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
			},
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.SearchOffers(context.Background(), SearchReq{Origin: "SFO", Dest: "NRT", DepartOn: time.Now(), Pax: 1}); err != nil {
					t.Fatalf("SearchOffers: %v", err)
				}
			},
		},
		{
			name:     "search_offers failure still observed",
			endpoint: "search_offers",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.SearchOffers(context.Background(), SearchReq{Origin: "SFO", Dest: "NRT", DepartOn: time.Now(), Pax: 1}); err == nil {
					t.Fatal("SearchOffers: expected http error")
				}
			},
		},
		{
			name:     "flight_status success",
			endpoint: "flight_status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
					"flightDesignator": map[string]any{"carrierCode": "UA", "flightNumber": "837"},
					"flightStatus":     "SCHEDULED",
				}}})
			},
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.FlightStatus(context.Background(), "UA837", time.Now()); err != nil {
					t.Fatalf("FlightStatus: %v", err)
				}
			},
		},
		{
			name:     "airport_info success",
			endpoint: "airport_info",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{
					"iataCode": "SFO",
					"name":     "San Francisco Intl",
				}}})
			},
			run: func(t *testing.T, a *Adapter) {
				if _, err := a.AirportInfo(context.Background(), "SFO"); err != nil {
					t.Fatalf("AirportInfo: %v", err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)
			r := telemetry.NewRegistry()
			RegisterMetrics(r)
			a, err := New(Config{
				Attestor:    &fakeAttestor{},
				TokenSource: &fakeTokenSource{tok: "t"},
				HTTPClient:  srv.Client(),
				BaseURL:     srv.URL,
				Metrics:     Metrics(r),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			tc.run(t, a)
			keys := obstest.SnapshotKeys(t, r)
			want := "leah_connect_api_call_total|endpoint=" + tc.endpoint + ",provider=flights"
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
	a, err := New(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{tok: "t"},
		HTTPClient:  http.DefaultClient,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.WatchPrice(context.Background(), SearchReq{Origin: "SFO", Dest: "NRT", DepartOn: time.Now(), Pax: 1}, Money{Amount: 70000, Currency: "USD"}); err != nil {
		t.Fatalf("WatchPrice: %v", err)
	}
	// also exercise an HTTP-backed RPC against an erroring transport to ensure nil-safe ObserveAPI.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	a2, err := New(Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{tok: "t"},
		HTTPClient:  srv.Client(),
		BaseURL:     srv.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a2.SearchOffers(context.Background(), SearchReq{Origin: "SFO", Dest: "NRT", DepartOn: time.Now(), Pax: 1}); !errors.Is(err, ErrAPIStatus) {
		t.Fatalf("SearchOffers err = %v, want ErrAPIStatus", err)
	}
}
