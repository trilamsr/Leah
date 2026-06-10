package feeds

import (
	"context"
	"time"
)

// Item is the cross-domain payload every Feed yields. Domain-specific shape
// (Forecast, Quote, Article) hangs off downstream synthesizers in later waves;
// MVP keeps the surface narrow so adding a source doesn't ripple through callers.
type Item struct {
	ID        string
	Title     string
	Body      string
	Source    string
	Timestamp time.Time
	Tags      []string
}

// Feed is the contract every adapter satisfies. Narrow on purpose — operator-
// facing synthesis (W32+) consumes []Item uniformly.
type Feed interface {
	Name() string
	Fetch(ctx context.Context) ([]Item, error)
}
