package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const apiBase = "https://api.themoviedb.org/3"

type HTTPTransport struct {
	hc   *http.Client
	base string
}

func NewHTTPTransport(hc *http.Client, base string) *HTTPTransport {
	if hc == nil {
		hc = http.DefaultClient
	}
	if base == "" {
		base = apiBase
	}
	return &HTTPTransport{hc: hc, base: strings.TrimRight(base, "/")}
}

type rawSearchHit struct {
	ID           int    `json:"id"`
	MediaType    string `json:"media_type"`
	Name         string `json:"name"`
	Title        string `json:"title"`
	FirstAirDate string `json:"first_air_date"`
	ReleaseDate  string `json:"release_date"`
}

func (h *HTTPTransport) Search(ctx context.Context, key, query string) ([]SearchHit, error) {
	q := url.Values{"query": {query}, "include_adult": {"false"}, "page": {"1"}}
	var resp struct {
		Results []rawSearchHit `json:"results"`
	}
	if err := h.call(ctx, key, "/search/multi?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	out := make([]SearchHit, 0, len(resp.Results))
	for _, r := range resp.Results {
		title := r.Title
		if title == "" {
			title = r.Name
		}
		out = append(out, SearchHit{ID: r.ID, MediaType: r.MediaType, Title: title, Year: parseYear(r.FirstAirDate, r.ReleaseDate)})
	}
	return out, nil
}

type rawProvider struct {
	Name     string `json:"provider_name"`
	LogoPath string `json:"logo_path"`
}

func (h *HTTPTransport) WatchProviders(ctx context.Context, key, mediaType string, id int, region string) (ProviderSet, error) {
	if mediaType != "movie" && mediaType != "tv" {
		return ProviderSet{}, fmt.Errorf("tmdb: unsupported media type %q", mediaType)
	}
	path := "/" + mediaType + "/" + strconv.Itoa(id) + "/watch/providers"
	var resp struct {
		Results map[string]struct {
			Flatrate []rawProvider `json:"flatrate"`
			Rent     []rawProvider `json:"rent"`
			Buy      []rawProvider `json:"buy"`
		} `json:"results"`
	}
	if err := h.call(ctx, key, path, &resp); err != nil {
		return ProviderSet{}, err
	}
	r, ok := resp.Results[region]
	if !ok {
		return ProviderSet{}, nil
	}
	return ProviderSet{
		Flatrate: convertProviders(r.Flatrate),
		Rent:     convertProviders(r.Rent),
		Buy:      convertProviders(r.Buy),
	}, nil
}

func convertProviders(in []rawProvider) []Provider {
	if len(in) == 0 {
		return nil
	}
	out := make([]Provider, len(in))
	for i, p := range in {
		out[i] = Provider(p)
	}
	return out
}

// call NEVER echoes the bearer token in error strings — leaked tokens in stderr
// are a known incident pattern with paste-flow credentials.
func (h *HTTPTransport) call(ctx context.Context, bearer, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.base+path, nil)
	if err != nil {
		return fmt.Errorf("tmdb: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")

	resp, err := h.hc.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAuthFailed
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusTooManyRequests:
		return ErrRateLimited
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("tmdb: server error %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("tmdb: http %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("tmdb: read body: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("tmdb: decode: %w", err)
	}
	return nil
}

// parseYear pulls YYYY from either /search/multi date field; 0 when both absent.
func parseYear(a, b string) int {
	for _, s := range []string{a, b} {
		if len(s) >= 4 {
			if y, err := strconv.Atoi(s[:4]); err == nil {
				return y
			}
		}
	}
	return 0
}
