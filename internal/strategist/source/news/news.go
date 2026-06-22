// Package news wraps the existing feeds.News reader as a strategist
// source.Source. The Attestor-gated *feeds.News is constructed by the
// CLI (one consent point) and injected here; this package owns only
// the fact-shape adaptation and since-filtering.
package news

import (
	"context"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/feeds"
	"github.com/trilam/leah/internal/strategist/source"
)

// Fetcher mirrors the *feeds.News surface the strategist needs. Kept
// as an interface so tests don't need a live HTTP client or attestor.
type Fetcher interface {
	Fetch(ctx context.Context) ([]feeds.Article, error)
}

type Config struct {
	Fetcher Fetcher
}

type Source struct{ cfg Config }

func New(cfg Config) *Source { return &Source{cfg: cfg} }

func (*Source) Name() string { return "news" }

// Candidates fetches via the wrapped feeds reader, drops articles
// older than `since`, and maps each surviving article to a Fact. The
// URL is the stable origin — the operator can revisit the source.
func (s *Source) Candidates(ctx context.Context, since time.Time) ([]source.Fact, error) {
	arts, err := s.cfg.Fetcher.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("news source: %w", err)
	}
	var facts []source.Fact
	for _, a := range arts {
		if !since.IsZero() && a.Published.Before(since) {
			continue
		}
		summary := a.Title
		if a.Summary != "" {
			summary = a.Title + "\n\n" + a.Summary
		}
		facts = append(facts, source.Fact{
			Slot:    "news",
			Summary: summary,
			Origin:  a.URL,
		})
	}
	return facts, nil
}
