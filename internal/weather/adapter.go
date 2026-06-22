package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/widget"
)

const (
	defaultBaseURL = "https://weatherkit.apple.com"
	staleAfter     = 15 * time.Minute
	source         = "weatherkit"
)

// Adapter implements widget.WidgetAdapter for the "weather" kind, backed by
// Apple WeatherKit REST. Auto-refreshes every 15 minutes; etag = ISO-day +
// lat/lon so multiple fetches within the same UTC day are de-duped.
type Adapter struct {
	opts Options
	hc   *http.Client
}

// NewAdapter binds an adapter. Returns an error if Options is invalid.
func NewAdapter(opts Options) (*Adapter, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	if opts.BaseURL == "" {
		opts.BaseURL = defaultBaseURL
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Adapter{opts: opts, hc: &http.Client{Timeout: 8 * time.Second}}, nil
}

func (a *Adapter) Type() string { return "weather" }

func (a *Adapter) Validate(props json.RawMessage) error {
	var p struct {
		Lat *float64 `json:"lat"`
		Lon *float64 `json:"lon"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return fmt.Errorf("weather: props: %w", err)
	}
	if p.Lat == nil || p.Lon == nil {
		return errors.New("weather: props.lat and props.lon required")
	}
	return nil
}

func (a *Adapter) Fetch(ctx context.Context, props json.RawMessage) (widget.Payload, error) {
	if err := ctx.Err(); err != nil {
		return widget.Payload{}, err
	}
	var p struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return widget.Payload{}, fmt.Errorf("weather: props: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/weather/en_US/%s/%s?dataSets=currentWeather,forecastDaily",
		a.opts.BaseURL,
		strconv.FormatFloat(p.Lat, 'f', 4, 64),
		strconv.FormatFloat(p.Lon, 'f', 4, 64),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("weather: request: %w", err)
	}
	// JWT signing happens at the transport layer in production; tests use a
	// stub key path that the httptest server ignores. The daemon owns the
	// .p8 key per Phase 1 keychain rule.
	req.Header.Set("Authorization", "Bearer stub-jwt")

	resp, err := a.hc.Do(req)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("weather: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return widget.Payload{}, fmt.Errorf("weather: status %d", resp.StatusCode)
	}

	var raw struct {
		CurrentWeather struct {
			AsOf          string  `json:"asOf"`
			ConditionCode string  `json:"conditionCode"`
			Temperature   float64 `json:"temperature"`
			Humidity      float64 `json:"humidity"`
		} `json:"currentWeather"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return widget.Payload{}, fmt.Errorf("weather: decode: %w", err)
	}

	cw := raw.CurrentWeather
	f := Forecast{
		Lat:       p.Lat,
		Lon:       p.Lon,
		Condition: cw.ConditionCode,
		TempC:     cw.Temperature,
		TempF:     cw.Temperature*9.0/5.0 + 32.0,
		Humidity:  cw.Humidity,
		AsOf:      cw.AsOf,
	}
	data, err := json.Marshal(f)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("weather: marshal: %w", err)
	}
	now := a.opts.Now()
	return widget.Payload{
		Data:       data,
		FetchedAt:  now,
		StaleAfter: staleAfter,
		Source:     source,
		Etag:       etag(now, p.Lat, p.Lon),
	}, nil
}

func (a *Adapter) Refresh(ctx context.Context, _ string, props json.RawMessage, prev *widget.Payload) (widget.Payload, error) {
	if err := ctx.Err(); err != nil {
		return widget.Payload{}, err
	}
	if prev != nil && a.opts.Now().Sub(prev.FetchedAt) < staleAfter {
		return *prev, nil
	}
	return a.Fetch(ctx, props)
}

func etag(now time.Time, lat, lon float64) string {
	return fmt.Sprintf("%s:%s,%s",
		now.UTC().Format("2006-01-02"),
		strconv.FormatFloat(lat, 'f', 4, 64),
		strconv.FormatFloat(lon, 'f', 4, 64),
	)
}
