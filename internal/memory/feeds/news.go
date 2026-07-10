package feeds

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/trilam/leah/internal/platform/contracts"
)

// ScopeNewsFetch is the operator-attestation scope for a news pull. Distinct
// per-RPC so the audit log attributes consent at source-fetch granularity.
const ScopeNewsFetch = "feeds:news:fetch"

// Article is the synthesizer-facing payload. Stays narrow — title, link,
// publication time, source attribution. Synthesizer ranks on these fields
// alone (recency + source-trust); body summarization is a follow-up.
type Article struct {
	Title     string
	URL       string
	Summary   string
	Source    string
	Published time.Time
}

// NewsSource names a single RSS endpoint. Name is the human-readable
// attribution surfaced to the operator ("hn", "google-news", "reddit").
type NewsSource struct {
	Name string
	URL  string
}

// NewsConfig wires the adapter. Attestor + HTTPClient + at least one Source
// are required; constructor refuses half-wired wiring so a missing gate never
// silently leaks a network call.
type NewsConfig struct {
	Attestor   contracts.Attestor
	HTTPClient *http.Client
	Sources    []NewsSource
}

// News is the multi-source RSS adapter. Lifecycle is caller-owned; no
// background goroutines so daemon shutdown stays clean.
type News struct {
	att     contracts.Attestor
	client  *http.Client
	sources []NewsSource
}

// NewNews fails fast on missing deps. An RSS endpoint without an Attestor
// would silently bypass the operator-consent gate — that's a security bug,
// not a soft warning.
func NewNews(config NewsConfig) (*News, error) {
	if config.Attestor == nil {
		return nil, errors.New("feeds.NewNews: Attestor required (operator-attestation gate)")
	}
	if config.HTTPClient == nil {
		return nil, errors.New("feeds.NewNews: HTTPClient required")
	}
	if len(config.Sources) == 0 {
		return nil, errors.New("feeds.NewNews: at least one Source required")
	}
	return &News{att: config.Attestor, client: config.HTTPClient, sources: config.Sources}, nil
}

// Name satisfies a future Feed-style registry; the Article-returning Fetch is
// distinct from the generic Feed.Fetch since the synth layer wants the typed
// payload.
func (n *News) Name() string { return "news" }

// Fetch gates on attestation BEFORE the HTTP call so a denied operator never
// causes a network round-trip. Aggregates across all configured sources;
// partial failure is non-fatal (one bad source shouldn't blank the brief).
func (n *News) Fetch(ctx context.Context) ([]Article, error) {
	if err := n.att.Attest(ctx, ScopeNewsFetch); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	var all []Article
	var firstErr error
	for _, src := range n.sources {
		arts, err := n.fetchOne(ctx, src)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, arts...)
	}
	// If every source failed, surface the first error so the operator sees
	// the failure rather than an empty digest.
	if len(all) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return all, nil
}

func (n *News) fetchOne(ctx context.Context, src NewsSource) ([]Article, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("feeds.news[%s]: build request: %w", src.Name, err)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("feeds.news[%s]: http: %w", src.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feeds.news[%s]: provider %s", src.Name, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("feeds.news[%s]: read body: %w", src.Name, err)
	}
	var raw rssFeed
	if err := xml.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("feeds.news[%s]: decode: %w", src.Name, err)
	}
	out := make([]Article, 0, len(raw.Channel.Items))
	for _, it := range raw.Channel.Items {
		out = append(out, Article{
			Title:     it.Title,
			URL:       it.Link,
			Summary:   it.Description,
			Source:    src.Name,
			Published: parseRSSTime(it.PubDate),
		})
	}
	return out, nil
}

// rssFeed trims RSS-2.0 to the four fields the synthesizer reads. xml.Unmarshal
// silently drops unknown tags so this stays robust to RSS dialect drift.
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}
type rssChannel struct {
	Items []rssItem `xml:"item"`
}
type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

// parseRSSTime tries the two RFC layouts RSS-2.0 actually emits in the wild.
// Returns zero time on unparseable input — synthesizer treats zero as "unknown
// age" which sorts last under recency rank.
func parseRSSTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
