package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ETA carries the live-traffic answer for a given route + departure time.
type ETA struct {
	DurationS       int
	TrafficDelaySec int
	DepartAt        time.Time
}

type trafficResponse struct {
	Status string `json:"status"`
	Routes []struct {
		Legs []struct {
			Distance struct {
				Value int `json:"value"`
			} `json:"distance"`
			Duration struct {
				Value int `json:"value"`
			} `json:"duration"`
			DurationInTraffic struct {
				Value int `json:"value"`
			} `json:"duration_in_traffic"`
		} `json:"legs"`
	} `json:"routes"`
}

type cachedETA struct {
	DurationS       int   `json:"d"`
	TrafficDelaySec int   `json:"t"`
	DepartAtUnix    int64 `json:"a"`
}

// TrafficETA returns the live-traffic ETA for departing at the given time.
// Uses the route's polyline as identity input + departAt-bucketed cache key.
func (a *Adapter) TrafficETA(ctx context.Context, route Route, departAt time.Time) (ETA, error) {
	if err := a.gate(ctx, ScopeTraffic); err != nil {
		return ETA{}, err
	}
	ck := keyHash("traffic", route.Polyline, departAt.Unix())
	if eta, ok := a.cacheGetETA(ctx, ck); ok {
		return eta, nil
	}
	q := url.Values{}
	q.Set("origin", "polyline:"+route.Polyline)
	q.Set("destination", "polyline:"+route.Polyline)
	q.Set("departure_time", strconv.FormatInt(departAt.Unix(), 10))
	q.Set("traffic_model", "best_guess")
	q.Set("key", a.apiKey)
	start := time.Now()
	defer func() { a.m.ObserveAPI("traffic_eta", time.Since(start).Seconds()) }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/directions/json?"+q.Encode(), nil)
	if err != nil {
		return ETA{}, fmt.Errorf("maps: build traffic request: %w", err)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return ETA{}, fmt.Errorf("maps: traffic http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body trafficResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return ETA{}, fmt.Errorf("maps: decode traffic: %w", err)
	}
	if body.Status != "OK" && body.Status != "ZERO_RESULTS" {
		return ETA{}, fmt.Errorf("%w: %s", ErrAPIStatus, body.Status)
	}
	if len(body.Routes) == 0 || len(body.Routes[0].Legs) == 0 {
		return ETA{}, ErrNoRoute
	}
	leg := body.Routes[0].Legs[0]
	live := leg.DurationInTraffic.Value
	if live == 0 {
		live = leg.Duration.Value
	}
	eta := ETA{
		DurationS:       live,
		TrafficDelaySec: live - leg.Duration.Value,
		DepartAt:        departAt,
	}
	a.cachePutETA(ctx, ck, eta)
	return eta, nil
}

func (a *Adapter) cacheGetETA(ctx context.Context, key string) (ETA, bool) {
	if a.cache == nil {
		return ETA{}, false
	}
	raw, ok, err := a.cache.Get(ctx, ScopeTraffic, key)
	if err != nil || !ok {
		return ETA{}, false
	}
	var env cachedETA
	if err := json.Unmarshal(raw, &env); err != nil {
		return ETA{}, false
	}
	return ETA{
		DurationS:       env.DurationS,
		TrafficDelaySec: env.TrafficDelaySec,
		DepartAt:        time.Unix(env.DepartAtUnix, 0).UTC(),
	}, true
}

func (a *Adapter) cachePutETA(ctx context.Context, key string, eta ETA) {
	if a.cache == nil {
		return
	}
	blob, err := json.Marshal(cachedETA{
		DurationS:       eta.DurationS,
		TrafficDelaySec: eta.TrafficDelaySec,
		DepartAtUnix:    eta.DepartAt.Unix(),
	})
	if err != nil {
		return
	}
	_ = a.cache.Set(ctx, ScopeTraffic, key, blob, TTLFor(ScopeTraffic))
}
