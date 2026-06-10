package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// directionsResponse mirrors the Google Directions API JSON; W56 only decodes
// the overview polyline + first leg's distance/duration/steps surface.
type directionsResponse struct {
	Status string `json:"status"`
	Routes []struct {
		OverviewPolyline struct {
			Points string `json:"points"`
		} `json:"overview_polyline"`
		Legs []struct {
			Distance struct {
				Value int `json:"value"`
			} `json:"distance"`
			Duration struct {
				Value int `json:"value"`
			} `json:"duration"`
			Steps []struct {
				HTMLInstructions string `json:"html_instructions"`
				Distance         struct {
					Value int `json:"value"`
				} `json:"distance"`
				Duration struct {
					Value int `json:"value"`
				} `json:"duration"`
			} `json:"steps"`
		} `json:"legs"`
	} `json:"routes"`
}

// Route returns the primary route from origin to dest for the given mode.
func (a *Adapter) Route(ctx context.Context, origin, dest string, mode TransportMode) (Route, error) {
	if err := a.gate(ctx, ScopeRoute); err != nil {
		return Route{}, err
	}
	q := url.Values{}
	q.Set("origin", origin)
	q.Set("destination", dest)
	q.Set("mode", string(mode))
	q.Set("key", a.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/directions/json?"+q.Encode(), nil)
	if err != nil {
		return Route{}, fmt.Errorf("maps: build directions request: %w", err)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return Route{}, fmt.Errorf("maps: directions http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body directionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Route{}, fmt.Errorf("maps: decode directions: %w", err)
	}
	if body.Status != "OK" && body.Status != "ZERO_RESULTS" {
		return Route{}, fmt.Errorf("%w: %s", ErrAPIStatus, body.Status)
	}
	if len(body.Routes) == 0 || len(body.Routes[0].Legs) == 0 {
		return Route{}, ErrNoRoute
	}
	r := body.Routes[0]
	leg := r.Legs[0]
	steps := make([]Step, 0, len(leg.Steps))
	for _, s := range leg.Steps {
		steps = append(steps, Step{
			Instructions: s.HTMLInstructions,
			DistanceM:    s.Distance.Value,
			DurationS:    s.Duration.Value,
		})
	}
	return Route{
		Polyline:  r.OverviewPolyline.Points,
		DistanceM: leg.Distance.Value,
		DurationS: leg.Duration.Value,
		Steps:     steps,
	}, nil
}
