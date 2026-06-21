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
	"sort"
	"strings"
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

// newsBundles maps a curated bundle name to a verified RSS source list.
// Every URL probed live (HEAD + content-type) before commit; see PR body for
// the verification matrix. Operators who want bundle behavior plus a personal
// override can copy the chosen bundle into $LEAH_STATE_DIR/feeds-news.json.
//
// TODO(MAY): anthropic via feeds.feedburner.com/anthropic — FeedBurner is
// deprecated Google infrastructure; swap to native Anthropic RSS once they
// publish one. Tracker: pending Linear issue.
var newsBundles = map[string][]feeds.NewsSource{
	"ai": {
		{Name: "arxiv-cs.ai", URL: "https://export.arxiv.org/rss/cs.AI"},
		{Name: "arxiv-cs.lg", URL: "https://export.arxiv.org/rss/cs.LG"},
		{Name: "anthropic", URL: "https://feeds.feedburner.com/anthropic"},
		{Name: "openai", URL: "https://openai.com/news/rss.xml"},
		{Name: "deepmind", URL: "https://deepmind.google/blog/rss.xml"},
		{Name: "huggingface", URL: "https://huggingface.co/blog/feed.xml"},
		{Name: "simonwillison", URL: "https://simonwillison.net/atom/everything/"},
	},
	"tech": {
		{Name: "hn", URL: "https://hnrss.org/frontpage"},
		{Name: "lobsters", URL: "https://lobste.rs/rss"},
		{Name: "pragmatic-engineer", URL: "https://newsletter.pragmaticengineer.com/feed"},
	},
}

// bundleURLOverride is a test-only hook patched from news_test.go. Package
// variable — not env-readable — so a stray production ENV cannot redirect
// bundle fetches at runtime. Empty in every shipped binary.
var bundleURLOverride string

// bundleSources returns the curated source list for the named bundle. The
// second return is false when the name is unregistered; callers translate
// that into an exit-2 usage error rather than a soft-fail since a typo
// shouldn't silently swap to defaults.
func bundleSources(name string) ([]feeds.NewsSource, bool) {
	src, ok := newsBundles[name]
	if !ok {
		return nil, false
	}
	if bundleURLOverride != "" {
		out := make([]feeds.NewsSource, len(src))
		for i, s := range src {
			out[i] = feeds.NewsSource{Name: s.Name, URL: bundleURLOverride}
		}
		return out, true
	}
	return src, true
}

// runNews prints a synthesized DailyDigest. Flags: --bundle <name> picks a
// curated source list; otherwise operator config + defaults apply.
func runNews(parent context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printNewsHelp(w)
		return 0
	}
	bundle, rest, err := parseBundleFlag(args)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah news: %v\n", err)
		return 2
	}
	since, rest, err := parseSinceFlag(rest)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah news: %v\nusage: leah news [--bundle <name>] [--since <RFC3339>]\n", err)
		return 2
	}
	if len(rest) > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "leah news: unexpected argument %q\n", rest[0])
		return 2
	}

	var sources []feeds.NewsSource
	if bundle != "" {
		src, ok := bundleSources(bundle)
		if !ok {
			_, _ = fmt.Fprintf(os.Stderr, "leah news: unknown bundle %q (known: %s)\n", bundle, knownBundles())
			return 2
		}
		sources = src
	} else {
		sources = loadNewsSources(stateDir())
	}

	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

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
	if !since.IsZero() {
		filtered := articles[:0]
		for _, a := range articles {
			// Items with no parsable pubDate are dropped under a cursor —
			// caller asked for "since X" and we can't prove the item meets it.
			if !a.Published.IsZero() && !a.Published.Before(since) {
				filtered = append(filtered, a)
			}
		}
		articles = filtered
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

func printNewsHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah news [--bundle <name>] [--since <RFC3339>]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Fetch + synthesize a top-3 news digest from RSS sources.")
	_, _ = fmt.Fprintln(w, "Sources default to HN + Google News; override by writing")
	_, _ = fmt.Fprintln(w, "$LEAH_STATE_DIR/feeds-news.json.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "--since <RFC3339> drops items published before the cursor")
	_, _ = fmt.Fprintln(w, "(e.g. --since=2026-06-21T00:00:00Z for today only).")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintf(w, "Curated bundles (--bundle): %s\n", knownBundles())
}

// parseBundleFlag consumes a single --bundle <name> pair from args. Supports
// `--bundle ai` and `--bundle=ai`. Rejects repeats so a silent last-wins
// merge of conflicting bundles can't ship a digest the operator didn't ask
// for. Returns the remaining args so the caller can still reject unexpected
// positionals.
func parseBundleFlag(args []string) (bundle string, rest []string, err error) {
	rest = make([]string, 0, len(args))
	seen := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--bundle":
			if seen {
				return "", nil, errors.New("--bundle may only be specified once")
			}
			if i+1 >= len(args) {
				return "", nil, errors.New("--bundle requires a value")
			}
			bundle = args[i+1]
			seen = true
			i++
		case strings.HasPrefix(a, "--bundle="):
			if seen {
				return "", nil, errors.New("--bundle may only be specified once")
			}
			bundle = strings.TrimPrefix(a, "--bundle=")
			if bundle == "" {
				return "", nil, errors.New("--bundle requires a value")
			}
			seen = true
		default:
			rest = append(rest, a)
		}
	}
	return bundle, rest, nil
}

// parseSinceFlag consumes a single --since <RFC3339> pair. Mirrors parseBundleFlag
// shape so duplicate detection and = vs space forms work identically — operator
// muscle-memory from --bundle transfers.
func parseSinceFlag(args []string) (since time.Time, rest []string, err error) {
	rest = make([]string, 0, len(args))
	seen := false
	var raw string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--since":
			if seen {
				return time.Time{}, nil, errors.New("--since may only be specified once")
			}
			if i+1 >= len(args) {
				return time.Time{}, nil, errors.New("--since requires a value")
			}
			raw = args[i+1]
			seen = true
			i++
		case strings.HasPrefix(a, "--since="):
			if seen {
				return time.Time{}, nil, errors.New("--since may only be specified once")
			}
			raw = strings.TrimPrefix(a, "--since=")
			if raw == "" {
				return time.Time{}, nil, errors.New("--since requires a value")
			}
			seen = true
		default:
			rest = append(rest, a)
		}
	}
	if !seen {
		return time.Time{}, rest, nil
	}
	t, perr := time.Parse(time.RFC3339, raw)
	if perr != nil {
		return time.Time{}, nil, fmt.Errorf("--since: invalid RFC3339 %q: %w", raw, perr)
	}
	return t, rest, nil
}

func knownBundles() string {
	names := make([]string, 0, len(newsBundles))
	for k := range newsBundles {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, "|")
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
