package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const notionVersion = "2022-06-28"

// HTTPTransport is the default Transport: hand-rolled api.notion.com/v1 REST.
// The jomei/notionapi SDK is deferred per spec § 2 (adopt-over-build); the
// Transport interface keeps the swap open without touching the policy layer.
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

// richText models Notion's title array; only plain_text is read for the MVP
// string surface (rich-text deltas defer per spec § 10).
type richText struct {
	PlainText string `json:"plain_text"`
}

type pageObject struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Parent struct {
		DatabaseID string `json:"database_id"`
	} `json:"parent"`
	Properties map[string]struct {
		Title []richText `json:"title"`
	} `json:"properties"`
}

func (p pageObject) toPage() Page {
	return Page{ID: p.ID, Title: p.titleText(), URL: p.URL, DB: p.Parent.DatabaseID}
}

// titleText returns the first title-typed property's plain text; Notion names
// the title property freely (Name, Title, …) so the type, not the key, selects.
func (p pageObject) titleText() string {
	for _, prop := range p.Properties {
		if len(prop.Title) > 0 {
			return prop.Title[0].PlainText
		}
	}
	return ""
}

func (h *HTTPTransport) ListDatabases(ctx context.Context, bearer string) ([]DB, error) {
	body := map[string]any{"filter": map[string]string{"property": "object", "value": "database"}}
	var out struct {
		Results []struct {
			ID    string     `json:"id"`
			URL   string     `json:"url"`
			Title []richText `json:"title"`
		} `json:"results"`
	}
	if err := h.do(ctx, bearer, http.MethodPost, "/search", body, &out); err != nil {
		return nil, err
	}
	dbs := make([]DB, len(out.Results))
	for i, r := range out.Results {
		title := ""
		if len(r.Title) > 0 {
			title = r.Title[0].PlainText
		}
		dbs[i] = DB{ID: r.ID, Title: title, URL: r.URL}
	}
	return dbs, nil
}

func (h *HTTPTransport) QueryDatabase(ctx context.Context, bearer, dbID string, f Filter) ([]Page, error) {
	var body map[string]any
	if f.PropertyName != "" {
		body = map[string]any{"filter": map[string]any{
			"property":  f.PropertyName,
			"rich_text": map[string]string{"equals": f.PropertyValue},
		}}
	}
	var out struct {
		Results []pageObject `json:"results"`
	}
	if err := h.do(ctx, bearer, http.MethodPost, "/databases/"+url.PathEscape(dbID)+"/query", body, &out); err != nil {
		return nil, err
	}
	pages := make([]Page, len(out.Results))
	for i, r := range out.Results {
		pages[i] = r.toPage()
	}
	return pages, nil
}

func (h *HTTPTransport) GetPage(ctx context.Context, bearer, id string) (Page, error) {
	var out pageObject
	if err := h.do(ctx, bearer, http.MethodGet, "/pages/"+url.PathEscape(id), nil, &out); err != nil {
		return Page{}, err
	}
	return out.toPage(), nil
}

func (h *HTTPTransport) CreatePage(ctx context.Context, bearer, dbID string, p Props) (Page, error) {
	props := map[string]any{
		"Name": map[string]any{"title": []map[string]any{{"text": map[string]string{"content": p.Title}}}},
	}
	for k, v := range p.Fields {
		props[k] = map[string]any{"rich_text": []map[string]any{{"text": map[string]string{"content": v}}}}
	}
	body := map[string]any{
		"parent":     map[string]string{"database_id": dbID},
		"properties": props,
	}
	var out pageObject
	if err := h.do(ctx, bearer, http.MethodPost, "/pages", body, &out); err != nil {
		return Page{}, err
	}
	return out.toPage(), nil
}

func (h *HTTPTransport) do(ctx context.Context, bearer, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("notion: marshal: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("notion: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Notion-Version", notionVersion)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.hc.Do(req)
	if err != nil {
		return fmt.Errorf("notion: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := mapStatus(resp.StatusCode); err != nil {
		return err
	}
	dec, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("notion: read body: %w", err)
	}
	if err := json.Unmarshal(dec, out); err != nil {
		return fmt.Errorf("notion: decode: %w", err)
	}
	return nil
}

// mapStatus turns HTTP status into the package's typed sentinels so callers
// switch on failure mode without inspecting wrapped strings.
func mapStatus(code int) error {
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == 401 || code == 403:
		return ErrAuthFailed
	case code == 404:
		return ErrNotFound
	case code == 429:
		return ErrRateLimited
	default:
		return fmt.Errorf("notion: upstream status %d", code)
	}
}
