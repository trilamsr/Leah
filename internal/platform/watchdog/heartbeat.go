// Package watchdog posts a per-tick liveness ping to healthchecks.io so
// operator gets paged when leah-daemon stalls > 5 minutes. Unset URL is a
// silent no-op — the integration is opt-in.
package watchdog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Heartbeat holds the ping URL (from LEAH_HEALTHCHECK_URL) plus a 10s
// HTTP client. Zero-value is unsafe — construct via New.
type Heartbeat struct {
	URL  string
	HTTP *http.Client
}

// New returns a Heartbeat sourced from LEAH_HEALTHCHECK_URL.
func New() *Heartbeat {
	return &Heartbeat{
		URL:  os.Getenv("LEAH_HEALTHCHECK_URL"),
		HTTP: &http.Client{Timeout: 10 * time.Second},
	}
}

// Ping issues a GET to URL. Returns nil silently when URL is unset so the
// daemon tick path never errors on a missing healthcheck integration.
// 5xx responses are treated as failure; 4xx (badge URL revoked) is success
// to avoid masking real liveness with an unrelated config rot.
func (h *Heartbeat) Ping(ctx context.Context) error {
	if h.URL == "" {
		return nil // skip silently
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL, nil)
	if err != nil {
		return fmt.Errorf("new heartbeat request: %w", err)
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("heartbeat status %d", resp.StatusCode)
	}
	return nil
}
