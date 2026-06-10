package hud

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client is a thin HTTP poller over the leah-daemon dashboard surface.
// Wails/gRPC arrives in W35; W34 ships the stdlib seam so the static panel
// can live-update via fetch() against /metrics + SSE /events.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 5 * time.Second},
	}
}

// PollMetrics fetches /metrics and parses Prometheus text format into a
// name{labels}→value map. Help/Type/blank lines are skipped; malformed
// rows are silently dropped (the HUD treats /metrics as advisory).
func (c *Client) PollMetrics(ctx context.Context) (map[string]float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/metrics", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hud: /metrics status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := make(map[string]float64)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Last space separates value from name{labels}.
		idx := strings.LastIndex(line, " ")
		if idx <= 0 || idx == len(line)-1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		v, err := strconv.ParseFloat(strings.TrimSpace(line[idx+1:]), 64)
		if err != nil {
			continue
		}
		out[key] = v
	}
	return out, nil
}

// StreamEvents subscribes to /events and invokes onLine per data row.
// Returns when ctx is cancelled or the server closes the body. SSE
// "data: " prefix is stripped; non-data lines are skipped.
func (c *Client) StreamEvents(ctx context.Context, onLine func(string)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	// Streaming: don't use the per-request timeout from c.HTTP.
	httpCli := &http.Client{}
	resp, err := httpCli.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hud: /events status %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		onLine(payload)
	}
	return sc.Err()
}
