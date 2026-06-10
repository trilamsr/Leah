package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// placesNearbyResponse mirrors the Google Places Nearby Search JSON; only the
// fields needed to build a Place are decoded.
type placesNearbyResponse struct {
	Status  string `json:"status"`
	Results []struct {
		PlaceID  string `json:"place_id"`
		Name     string `json:"name"`
		Geometry struct {
			Location struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"location"`
		} `json:"geometry"`
		Types        []string `json:"types"`
		Rating       float32  `json:"rating"`
		OpeningHours *struct {
			OpenNow bool `json:"open_now"`
		} `json:"opening_hours,omitempty"`
	} `json:"results"`
}

// POINearby returns places of the given category within radius meters of center.
func (a *Adapter) POINearby(ctx context.Context, center Place, radiusM int, category string) ([]Place, error) {
	if err := a.gate(ctx, ScopePOI); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("location", strconv.FormatFloat(center.Lat, 'f', -1, 64)+","+strconv.FormatFloat(center.Lng, 'f', -1, 64))
	q.Set("radius", strconv.Itoa(radiusM))
	q.Set("type", category)
	q.Set("key", a.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/place/nearbysearch/json?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("maps: build nearby request: %w", err)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("maps: nearby http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body placesNearbyResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("maps: decode nearby: %w", err)
	}
	if body.Status != "OK" && body.Status != "ZERO_RESULTS" {
		return nil, fmt.Errorf("%w: %s", ErrAPIStatus, body.Status)
	}
	out := make([]Place, 0, len(body.Results))
	for _, r := range body.Results {
		p := Place{
			ID:         r.PlaceID,
			Name:       r.Name,
			Lat:        r.Geometry.Location.Lat,
			Lng:        r.Geometry.Location.Lng,
			Categories: r.Types,
			Rating:     r.Rating,
		}
		if r.OpeningHours != nil {
			p.Hours = OpenHours{OpenNow: r.OpeningHours.OpenNow}
		}
		out = append(out, p)
	}
	return out, nil
}
