package confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/trilam/leah/internal/adapters/atlassian"
)

// HTTPTransport is the default Transport: hand-rolled Confluence Cloud REST.
// The go-atlassian SDK is deferred to the W50 combined tidy (adopt-over-build).
type HTTPTransport struct {
	hc      *http.Client
	baseURL string
}

func NewHTTPTransport(hc *http.Client, baseURL string) *HTTPTransport {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &HTTPTransport{hc: hc, baseURL: strings.TrimRight(baseURL, "/")}
}

type content struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Space struct {
		Key string `json:"key"`
	} `json:"space"`
	Body struct {
		Storage struct {
			Value string `json:"value"`
		} `json:"storage"`
	} `json:"body"`
	Links struct {
		WebUI string `json:"webui"`
		Base  string `json:"base"`
	} `json:"_links"`
	Version struct {
		When time.Time `json:"when"`
	} `json:"version"`
}

func (c content) toPage(base string) Page {
	if c.Links.Base != "" {
		base = c.Links.Base
	}
	url := c.Links.WebUI
	if url != "" {
		url = strings.TrimRight(base, "/") + url
	}
	return Page{
		ID:       c.ID,
		Title:    c.Title,
		SpaceKey: c.Space.Key,
		Body:     c.Body.Storage.Value,
		URL:      url,
		Updated:  c.Version.When,
	}
}

func (h *HTTPTransport) GetPage(ctx context.Context, bearer, id string) (Page, error) {
	var out content
	if err := h.get(ctx, bearer, "/rest/api/content/"+url.PathEscape(id), url.Values{
		"expand": {"body.storage,space,version"},
	}, &out); err != nil {
		return Page{}, err
	}
	return out.toPage(h.baseURL), nil
}

func (h *HTTPTransport) ListRecent(ctx context.Context, bearer, space string) ([]Page, error) {
	var out struct {
		Results []content `json:"results"`
	}
	if err := h.get(ctx, bearer, "/rest/api/content", url.Values{
		"spaceKey": {space},
		"orderby":  {"-history.lastUpdated"},
		"expand":   {"space,version"},
	}, &out); err != nil {
		return nil, err
	}
	return h.pages(out.Results), nil
}

func (h *HTTPTransport) SearchCQL(ctx context.Context, bearer, query string) ([]Page, error) {
	var out struct {
		Results []struct {
			Content content `json:"content"`
		} `json:"results"`
	}
	if err := h.get(ctx, bearer, "/rest/api/search", url.Values{
		"cql":    {query},
		"expand": {"content.space,content.version"},
	}, &out); err != nil {
		return nil, err
	}
	res := make([]content, len(out.Results))
	for i, r := range out.Results {
		res[i] = r.Content
	}
	return h.pages(res), nil
}

func (h *HTTPTransport) pages(cs []content) []Page {
	out := make([]Page, len(cs))
	for i, c := range cs {
		out[i] = c.toPage(h.baseURL)
	}
	return out
}

func (h *HTTPTransport) get(ctx context.Context, bearer, path string, q url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+path+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("confluence: build request: %w", err)
	}
	req.Header.Set("Authorization", bearer)
	req.Header.Set("Accept", "application/json")
	resp, err := h.hc.Do(req)
	if err != nil {
		return fmt.Errorf("confluence: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := mapStatus(resp.StatusCode); err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("confluence: read body: %w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("confluence: decode: %w", err)
	}
	return nil
}

// mapStatus re-labels shared atlassian sentinels as confluence.Err* so callers
// switch on this package's errors without importing atlassian.
func mapStatus(code int) error {
	switch atlassian.MapStatus(code) {
	case nil:
		return nil
	case atlassian.ErrAuthFailed:
		return ErrAuthFailed
	case atlassian.ErrNotFound:
		return ErrNotFound
	case atlassian.ErrRateLimited:
		return ErrRateLimited
	default:
		return fmt.Errorf("confluence: upstream status %d", code)
	}
}

// AuditDetail builds the audit-row Detail: page_id + space_key only, never
// Body, so page text cannot leak into the log.
func AuditDetail(p Page) string {
	return fmt.Sprintf("page_id=%s space_key=%s", p.ID, p.SpaceKey)
}
