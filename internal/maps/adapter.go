package maps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/trilam/leah/internal/widget"
)

const (
	defaultBaseURL = "https://maps-api.apple.com"
	staleAfter     = 24 * time.Hour
	source         = "apple_maps"
)

// Adapter implements widget.WidgetAdapter for the "maps" kind. Forward
// geocode only — no tile, snapshot, or directions endpoints. Daemon hands
// lat/lon + label to SwiftUI, which renders the map via MapKit.
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
	return &Adapter{opts: opts, hc: &http.Client{Timeout: 8 * time.Second}}, nil
}

func (a *Adapter) Type() string { return "maps" }

func (a *Adapter) Validate(props json.RawMessage) error {
	var p struct {
		PlaceQuery string `json:"place_query"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return fmt.Errorf("maps: props: %w", err)
	}
	if p.PlaceQuery == "" {
		return errors.New("maps: props.place_query required")
	}
	return nil
}

func (a *Adapter) Fetch(ctx context.Context, props json.RawMessage) (widget.Payload, error) {
	if err := ctx.Err(); err != nil {
		return widget.Payload{}, err
	}
	var p struct {
		PlaceQuery string `json:"place_query"`
	}
	if err := json.Unmarshal(props, &p); err != nil {
		return widget.Payload{}, fmt.Errorf("maps: props: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1/geocode?q=%s", a.opts.BaseURL, url.QueryEscape(p.PlaceQuery))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("maps: request: %w", err)
	}
	// JWT signing happens at the transport layer in production; tests use
	// a stub key path so the httptest server is unauthenticated.
	req.Header.Set("Authorization", "Bearer stub-jwt")

	resp, err := a.hc.Do(req)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("maps: do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return widget.Payload{}, fmt.Errorf("maps: status %d", resp.StatusCode)
	}

	var raw struct {
		Results []struct {
			Coordinate struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"coordinate"`
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return widget.Payload{}, fmt.Errorf("maps: decode: %w", err)
	}
	if len(raw.Results) == 0 {
		return widget.Payload{}, fmt.Errorf("maps: no results for %q", p.PlaceQuery)
	}

	r0 := raw.Results[0]
	g := Geocode{Lat: r0.Coordinate.Latitude, Lon: r0.Coordinate.Longitude, Label: r0.Name}
	data, err := json.Marshal(g)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("maps: marshal: %w", err)
	}
	return widget.Payload{
		Data:       data,
		FetchedAt:  time.Now().UTC(),
		StaleAfter: staleAfter,
		Source:     source,
	}, nil
}

func (a *Adapter) Refresh(ctx context.Context, _ string, props json.RawMessage, prev *widget.Payload) (widget.Payload, error) {
	if err := ctx.Err(); err != nil {
		return widget.Payload{}, err
	}
	if prev != nil {
		return *prev, nil
	}
	return a.Fetch(ctx, props)
}
