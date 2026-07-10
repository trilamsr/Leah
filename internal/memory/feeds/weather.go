package feeds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"

	"github.com/trilam/leah/internal/platform/contracts"
)

// ScopeWeatherFetch is the operator-attestation scope the gate sees on a
// weather pull. Distinct scope per RPC so the audit log can attribute consent.
const ScopeWeatherFetch = "feeds:weather:fetch"

// defaultOWMBaseURL is OpenWeatherMap's current-weather endpoint. Overridable
// via WeatherConfig.BaseURL so httptest fakes stay hermetic.
const defaultOWMBaseURL = "https://api.openweathermap.org/data/2.5/weather"

// ErrAttestationDenied wraps the Attestor's rejection so callers can
// errors.Is on it. Same shape as gmail/gcal.
var ErrAttestationDenied = errors.New("feeds: attestation denied")

// Forecast is the weather payload the synth layer + HUD widget read. Fields
// chosen to feed the brief one-liner ("18C / 9C, light rain expected, bring
// umbrella").
type Forecast struct {
	TempC       float64
	Description string
	HighC       float64
	LowC        float64
	PrecipPct   int
}

// WeatherConfig wires the adapter. Attestor + HTTPClient + APIKey + Location
// are required; BaseURL defaults to OpenWeatherMap prod when blank.
type WeatherConfig struct {
	Attestor   contracts.Attestor
	HTTPClient *http.Client
	APIKey     string
	Location   string
	BaseURL    string
}

// Weather is the OpenWeatherMap adapter. Lifecycle is owned by the caller —
// the constructor MUST NOT start goroutines so daemon shutdown stays clean.
type Weather struct {
	att     contracts.Attestor
	client  *http.Client
	apiKey  string
	loc     string
	baseURL string
}

// NewWeather fails fast on a half-wired config; a missing Attestor would
// silently bypass the operator-consent gate.
func NewWeather(cfg WeatherConfig) (*Weather, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("feeds.NewWeather: Attestor required (operator-attestation gate)")
	}
	if cfg.HTTPClient == nil {
		return nil, errors.New("feeds.NewWeather: HTTPClient required")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("feeds.NewWeather: APIKey required")
	}
	if cfg.Location == "" {
		return nil, errors.New("feeds.NewWeather: Location required (no IP-geolocation fallback)")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultOWMBaseURL
	}
	return &Weather{
		att:     cfg.Attestor,
		client:  cfg.HTTPClient,
		apiKey:  cfg.APIKey,
		loc:     cfg.Location,
		baseURL: base,
	}, nil
}

// Name satisfies a future Feed-style registry; the Forecast-returning Fetch
// is distinct from the generic Feed.Fetch since the synth layer wants the
// typed payload directly.
func (w *Weather) Name() string { return "weather" }

// Fetch gates on attestation BEFORE the HTTP call so a denied operator never
// causes a network round-trip nor a key-in-query exposure to the provider.
func (w *Weather) Fetch(ctx context.Context) (Forecast, error) {
	if err := w.att.Attest(ctx, ScopeWeatherFetch); err != nil {
		return Forecast{}, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	q := url.Values{}
	q.Set("q", w.loc)
	q.Set("appid", w.apiKey)
	q.Set("units", "metric")
	reqURL := w.baseURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Forecast{}, fmt.Errorf("feeds.weather: build request: %w", err)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return Forecast{}, fmt.Errorf("feeds.weather: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Forecast{}, fmt.Errorf("feeds.weather: provider %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Forecast{}, fmt.Errorf("feeds.weather: read body: %w", err)
	}

	var raw owmResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return Forecast{}, fmt.Errorf("feeds.weather: decode: %w", err)
	}
	return Forecast{
		TempC:       raw.Main.Temp,
		HighC:       raw.Main.TempMax,
		LowC:        raw.Main.TempMin,
		Description: raw.firstDescription(),
		PrecipPct:   int(math.Round(raw.Pop * 100)),
	}, nil
}

// owmResponse trims OpenWeatherMap's payload to the fields the brief consumes.
// Field tags follow OWM's JSON; unknown fields are silently dropped.
type owmResponse struct {
	Weather []struct {
		Description string `json:"description"`
	} `json:"weather"`
	Main struct {
		Temp    float64 `json:"temp"`
		TempMin float64 `json:"temp_min"`
		TempMax float64 `json:"temp_max"`
	} `json:"main"`
	Pop float64 `json:"pop"`
}

func (r owmResponse) firstDescription() string {
	if len(r.Weather) == 0 {
		return ""
	}
	return r.Weather[0].Description
}
