package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// cachedPlaces is the on-disk envelope for Geocode/ReverseGeocode results.
type cachedPlaces struct {
	Places []Place `json:"places"`
}

// geocodeResponse mirrors the Google Geocoding API JSON; only fields the
// adapter projects into Place are decoded.
type geocodeResponse struct {
	Status  string `json:"status"`
	Results []struct {
		PlaceID          string `json:"place_id"`
		FormattedAddress string `json:"formatted_address"`
		Geometry         struct {
			Location struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"location"`
		} `json:"geometry"`
		Types []string `json:"types"`
	} `json:"results"`
}

// Geocode resolves a freeform address to candidate Places.
func (a *Adapter) Geocode(ctx context.Context, address string) ([]Place, error) {
	if err := a.gate(ctx, ScopeGeocode); err != nil {
		return nil, err
	}
	ck := keyHash("geocode", address)
	if p, ok := a.cacheGetPlaces(ctx, ck); ok {
		return p, nil
	}
	q := url.Values{}
	q.Set("address", address)
	q.Set("key", a.apiKey)
	out, err := a.doGeocode(ctx, q)
	if err != nil {
		return nil, err
	}
	a.cachePutPlaces(ctx, ck, out)
	return out, nil
}

// ReverseGeocode resolves a lat/lng to candidate Places.
func (a *Adapter) ReverseGeocode(ctx context.Context, lat, lng float64) ([]Place, error) {
	if err := a.gate(ctx, ScopeGeocode); err != nil {
		return nil, err
	}
	ck := keyHash("revgeocode", lat, lng)
	if p, ok := a.cacheGetPlaces(ctx, ck); ok {
		return p, nil
	}
	q := url.Values{}
	q.Set("latlng", strconv.FormatFloat(lat, 'f', -1, 64)+","+strconv.FormatFloat(lng, 'f', -1, 64))
	q.Set("key", a.apiKey)
	out, err := a.doGeocode(ctx, q)
	if err != nil {
		return nil, err
	}
	a.cachePutPlaces(ctx, ck, out)
	return out, nil
}

func (a *Adapter) cacheGetPlaces(ctx context.Context, key string) ([]Place, bool) {
	if a.cache == nil {
		return nil, false
	}
	raw, ok, err := a.cache.Get(ctx, ScopeGeocode, key)
	if err != nil || !ok {
		return nil, false
	}
	var env cachedPlaces
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, false
	}
	return env.Places, true
}

func (a *Adapter) cachePutPlaces(ctx context.Context, key string, places []Place) {
	if a.cache == nil {
		return
	}
	blob, err := json.Marshal(cachedPlaces{Places: places})
	if err != nil {
		return
	}
	_ = a.cache.Set(ctx, ScopeGeocode, key, blob, TTLFor(ScopeGeocode))
}

func (a *Adapter) doGeocode(ctx context.Context, q url.Values) ([]Place, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/geocode/json?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("maps: build geocode request: %w", err)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("maps: geocode http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body geocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("maps: decode geocode: %w", err)
	}
	if body.Status != "OK" && body.Status != "ZERO_RESULTS" {
		return nil, fmt.Errorf("%w: %s", ErrAPIStatus, body.Status)
	}
	out := make([]Place, 0, len(body.Results))
	for _, r := range body.Results {
		out = append(out, Place{
			ID:         r.PlaceID,
			Name:       r.FormattedAddress,
			Lat:        r.Geometry.Location.Lat,
			Lng:        r.Geometry.Location.Lng,
			Categories: r.Types,
		})
	}
	return out, nil
}
