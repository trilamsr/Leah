package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/trilam/leah/internal/feeds"
)

// defaultNewsSources is the MVP source list per spec §2 — RSS-only, no keys.
// HN frontpage + Google News top-stories. Operator-overridable by writing
// $LEAH_STATE_DIR/feeds-news.json with the same shape.
var defaultNewsSources = []feeds.NewsSource{
	{Name: "hn", URL: "https://hnrss.org/frontpage"},
	{Name: "google-news", URL: "https://news.google.com/rss"},
}

// runNews prints a synthesized DailyDigest. No flags in MVP — `leah news`
// just runs. Operator-config (--source filter, --json) defers to W37.
func runNews(parent context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(w, "usage: leah news")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "Fetch + synthesize a top-3 news digest from RSS sources.")
		_, _ = fmt.Fprintln(w, "Sources default to HN + Google News; override by writing")
		_, _ = fmt.Fprintln(w, "$LEAH_STATE_DIR/feeds-news.json.")
		return 0
	}
	if len(args) > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "leah news: unexpected argument %q\n", args[0])
		return 2
	}

	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

	sources := loadNewsSources(stateDir())
	n, err := feeds.NewNews(feeds.NewsConfig{
		Attestor:   newFeedsAttestor(),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		Sources:    sources,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah news: %v\n", err)
		return 1
	}
	articles, err := n.Fetch(ctx)
	if err != nil {
		if errors.Is(err, feeds.ErrAttestationDenied) {
			_, _ = fmt.Fprintln(os.Stderr, "leah news: attestation denied")
			return 1
		}
		_, _ = fmt.Fprintf(os.Stderr, "leah news: fetch: %v\n", err)
		return 1
	}
	digest := feeds.SummarizeNews(articles)
	if len(digest.Headlines) == 0 {
		_, _ = fmt.Fprintln(w, "(no headlines)")
		return 0
	}
	for i, h := range digest.Headlines {
		_, _ = fmt.Fprintf(w, "%d. %s  (%s)\n   %s\n", i+1, h.Title, h.Source, h.URL)
	}
	return 0
}

// loadNewsSources returns the operator's configured RSS sources, falling back
// to the default HN+Google list when the config file is absent or malformed.
// Soft-fail by design: a corrupt feeds-news.json shouldn't make `leah news`
// unrunnable, just non-personalized.
func loadNewsSources(dir string) []feeds.NewsSource {
	path := filepath.Join(dir, "feeds-news.json")
	raw, err := os.ReadFile(path) // #nosec G304 — path under operator's state dir
	if err != nil {
		return defaultNewsSources
	}
	var sources []feeds.NewsSource
	if err := json.Unmarshal(raw, &sources); err != nil || len(sources) == 0 {
		return defaultNewsSources
	}
	return sources
}
