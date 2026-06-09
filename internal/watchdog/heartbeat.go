package watchdog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Heartbeat struct {
	URL  string
	HTTP *http.Client
}

func New() *Heartbeat {
	return &Heartbeat{
		URL:  os.Getenv("LEAH_HEALTHCHECK_URL"),
		HTTP: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *Heartbeat) Ping(ctx context.Context) error {
	if h.URL == "" {
		return nil // skip silently
	}
	req, err := http.NewRequestWithContext(ctx, "GET", h.URL, nil)
	if err != nil {
		return err
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("heartbeat status %d", resp.StatusCode)
	}
	return nil
}
