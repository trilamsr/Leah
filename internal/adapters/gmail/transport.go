package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const (
	defaultBaseURL = "https://gmail.googleapis.com/gmail/v1"
	listMaxResults = 25
)

type httpTransport struct {
	hc      *http.Client
	baseURL string
}

// NewHTTPTransport is the production Transport for gmail.New.
func NewHTTPTransport(hc *http.Client, baseURL string) Transport {
	return newHTTPTransport(hc, baseURL)
}

func newHTTPTransport(hc *http.Client, baseURL string) *httpTransport {
	if hc == nil {
		hc = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &httpTransport{hc: hc, baseURL: baseURL}
}

func (t *httpTransport) ListUnread(ctx context.Context, token string) ([]string, error) {
	q := url.Values{
		"q":          {"is:unread"},
		"maxResults": {strconv.Itoa(listMaxResults)},
	}
	u := t.baseURL + "/users/me/messages?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("gmail: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := t.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gmail: list_unread: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrAuthRequired
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gmail: list_unread: provider %s", resp.Status)
	}

	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("gmail: decode: %w", err)
	}
	ids := make([]string, 0, len(out.Messages))
	for _, m := range out.Messages {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

func (t *httpTransport) MarkRead(context.Context, string, string) error { return ErrNotImplemented }

func (t *httpTransport) Send(context.Context, string, Message) error { return ErrNotImplemented }
