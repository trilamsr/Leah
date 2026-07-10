package flights

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeAttestor struct {
	called int
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	if !strings.HasPrefix(scope, "flights:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

type fakeTokenSource struct {
	tok string
	err error
}

func (f *fakeTokenSource) Token(_ context.Context) (string, error) {
	return f.tok, f.err
}

// failingTokenSource proves attestation runs before credentials materialize.
type failingTokenSource struct{ t *testing.T }

func (f *failingTokenSource) Token(_ context.Context) (string, error) {
	f.t.Fatal("TokenSource.Token() must NOT be called when attestation denies")
	return "", nil
}

func newServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestSearchOffers_HappyPath(t *testing.T) {
	t.Parallel()
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/shopping/flight-offers" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"id":    "off-1",
					"price": map[string]any{"total": "123.45", "currency": "USD"},
				},
			},
		})
	})
	att := &fakeAttestor{}
	a, err := New(Config{Attestor: att, TokenSource: &fakeTokenSource{tok: "tok"}, HTTPClient: server.Client(), BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	offers, err := a.SearchOffers(context.Background(), SearchReq{Origin: "SFO", Dest: "NRT", DepartOn: time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC), Pax: 1, Cabin: "ECONOMY"})
	if err != nil {
		t.Fatalf("SearchOffers: %v", err)
	}
	if len(offers) != 1 || offers[0].ID != "off-1" || offers[0].Price.Amount != 12345 || offers[0].Price.Currency != "USD" {
		t.Fatalf("offers = %+v", offers)
	}
	if att.called == 0 {
		t.Fatal("attestor not called — gate bypassed")
	}
}

func TestSearchOffers_AttestationDenied(t *testing.T) {
	t.Parallel()
	a, err := New(Config{
		Attestor:    &fakeAttestor{err: errors.New("denied")},
		TokenSource: &failingTokenSource{t: t},
		HTTPClient:  http.DefaultClient,
		BaseURL:     "http://unused",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = a.SearchOffers(context.Background(), SearchReq{Origin: "SFO", Dest: "NRT", DepartOn: time.Now(), Pax: 1})
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
}

func TestFlightStatus_HappyPath(t *testing.T) {
	t.Parallel()
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/schedule/flights" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("flightNumber") != "837" || q.Get("scheduledDepartureDate") != "2026-12-15" {
			t.Errorf("query = %v", q)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"flightDesignator": map[string]any{"carrierCode": "UA", "flightNumber": "837"},
				"flightStatus":     "SCHEDULED",
				"flightPoints": []map[string]any{
					{"departure": map[string]any{"timings": []map[string]any{{"value": "2026-12-15T10:00:00Z"}}}},
					{"arrival": map[string]any{"timings": []map[string]any{{"value": "2026-12-16T14:00:00Z"}}}},
				},
			}},
		})
	})
	a, err := New(Config{Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{tok: "tok"}, HTTPClient: server.Client(), BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	st, err := a.FlightStatus(context.Background(), "UA837", time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FlightStatus: %v", err)
	}
	if st.Carrier != "UA" || st.FlightNumber != "837" || st.State != "SCHEDULED" {
		t.Fatalf("status = %+v", st)
	}
}

func TestAirportInfo_HappyPath(t *testing.T) {
	t.Parallel()
	server := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reference-data/locations" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"iataCode":       "SFO",
				"name":           "San Francisco Intl",
				"address":        map[string]any{"cityName": "SAN FRANCISCO", "countryName": "UNITED STATES"},
				"timeZoneOffset": "-08:00",
			}},
		})
	})
	a, err := New(Config{Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{tok: "tok"}, HTTPClient: server.Client(), BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ap, err := a.AirportInfo(context.Background(), "SFO")
	if err != nil {
		t.Fatalf("AirportInfo: %v", err)
	}
	if ap.IATA != "SFO" || ap.City != "SAN FRANCISCO" {
		t.Fatalf("airport = %+v", ap)
	}
}

func TestWatchPrice_StoresWatchID(t *testing.T) {
	t.Parallel()
	a, err := New(Config{Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{tok: "tok"}, HTTPClient: http.DefaultClient, BaseURL: "http://unused"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	id, err := a.WatchPrice(context.Background(), SearchReq{Origin: "SFO", Dest: "NRT", DepartOn: time.Now(), Pax: 1}, Money{Amount: 70000, Currency: "USD"})
	if err != nil {
		t.Fatalf("WatchPrice: %v", err)
	}
	if id == "" {
		t.Fatal("watchID empty")
	}
	id2, err := a.WatchPrice(context.Background(), SearchReq{Origin: "SFO", Dest: "LHR", DepartOn: time.Now(), Pax: 1}, Money{Amount: 50000, Currency: "USD"})
	if err != nil {
		t.Fatalf("WatchPrice #2: %v", err)
	}
	if id == id2 {
		t.Fatalf("watchIDs not unique: %q", id)
	}
}

func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "no attestor", cfg: Config{TokenSource: &fakeTokenSource{}, HTTPClient: http.DefaultClient}},
		{name: "no token source", cfg: Config{Attestor: &fakeAttestor{}, HTTPClient: http.DefaultClient}},
		{name: "no http client", cfg: Config{Attestor: &fakeAttestor{}, TokenSource: &fakeTokenSource{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("New: expected error for missing dep")
			}
		})
	}
}
