package feeds_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/feeds"
)

// fakeAttestor mirrors gmail/gcal — Attest gates every external fetch.
type fakeAttestor struct {
	calls []string
	err   error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.calls = append(f.calls, scope)
	return f.err
}

// owmHappyBody matches the fields the adapter parses; trimmed from a real OWM
// /data/2.5/weather payload.
const owmHappyBody = `{
	"weather":[{"description":"light rain"}],
	"main":{"temp":18.2,"temp_min":9.1,"temp_max":21.4},
	"clouds":{"all":40},
	"pop":0.45
}`

// TestWeather_FetchHappyPath parses an OWM-shaped response into Forecast.
func TestWeather_FetchHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "appid=test-key") {
			t.Errorf("missing appid in query: %q", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "q=Brooklyn") {
			t.Errorf("missing q=Brooklyn in query: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(owmHappyBody))
	}))
	defer srv.Close()

	att := &fakeAttestor{}
	wx, err := feeds.NewWeather(feeds.WeatherConfig{
		Attestor:   att,
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Location:   "Brooklyn",
		BaseURL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("NewWeather: %v", err)
	}

	got, err := wx.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.TempC != 18.2 {
		t.Errorf("TempC = %v, want 18.2", got.TempC)
	}
	if got.HighC != 21.4 || got.LowC != 9.1 {
		t.Errorf("High/Low = %v/%v, want 21.4/9.1", got.HighC, got.LowC)
	}
	if got.Description != "light rain" {
		t.Errorf("Description = %q, want light rain", got.Description)
	}
	if got.PrecipPct != 45 {
		t.Errorf("PrecipPct = %d, want 45", got.PrecipPct)
	}
	if len(att.calls) != 1 || att.calls[0] != feeds.ScopeWeatherFetch {
		t.Errorf("Attest calls = %v, want [%s]", att.calls, feeds.ScopeWeatherFetch)
	}
}

// TestWeather_AttestationDenied_NoHTTPCall — denied gate aborts before network.
func TestWeather_AttestationDenied_NoHTTPCall(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit = true
	}))
	defer srv.Close()

	wx, err := feeds.NewWeather(feeds.WeatherConfig{
		Attestor:   &fakeAttestor{err: errors.New("denied")},
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		Location:   "Brooklyn",
		BaseURL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("NewWeather: %v", err)
	}
	if _, err := wx.Fetch(context.Background()); !errors.Is(err, feeds.ErrAttestationDenied) {
		t.Fatalf("Fetch err = %v, want ErrAttestationDenied wrap", err)
	}
	if hit {
		t.Fatalf("HTTP server was hit after denied attestation")
	}
}

// TestNewWeather_RejectsMissingDeps — half-wired config fails fast.
func TestNewWeather_RejectsMissingDeps(t *testing.T) {
	base := feeds.WeatherConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: http.DefaultClient,
		APIKey:     "k",
		Location:   "Brooklyn",
	}
	cases := []struct {
		name string
		mut  func(*feeds.WeatherConfig)
	}{
		{"no attestor", func(c *feeds.WeatherConfig) { c.Attestor = nil }},
		{"no http client", func(c *feeds.WeatherConfig) { c.HTTPClient = nil }},
		{"no api key", func(c *feeds.WeatherConfig) { c.APIKey = "" }},
		{"no location", func(c *feeds.WeatherConfig) { c.Location = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mut(&c)
			if _, err := feeds.NewWeather(c); err == nil {
				t.Fatalf("NewWeather accepted half-wired config")
			}
		})
	}
}
