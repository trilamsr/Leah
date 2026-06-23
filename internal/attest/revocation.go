package attest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type RevocationList struct {
	Pubkeys   []string  `json:"pubkeys"`
	FetchedAt time.Time `json:"fetched_at"`
}

// Stale reports whether the list has exceeded the 7d offline tolerance.
// After 7d the caller downgrades plugin load-block to load-warn (spec §6.7).
func (r RevocationList) Stale(now time.Time) bool {
	return now.Sub(r.FetchedAt) > revocationStale
}

func (r RevocationList) IsRevoked(pubkey string) bool {
	for _, k := range r.Pubkeys {
		if k == pubkey {
			return true
		}
	}
	return false
}

func FetchRevocations(ctx context.Context, url string) (RevocationList, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return RevocationList{}, err
	}
	client := &http.Client{Timeout: revocationHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return RevocationList{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return RevocationList{}, fmt.Errorf("revocation HTTP %d", resp.StatusCode)
	}
	var wire struct {
		Pubkeys []string `json:"pubkeys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return RevocationList{}, err
	}
	return RevocationList{Pubkeys: wire.Pubkeys, FetchedAt: time.Now()}, nil
}
