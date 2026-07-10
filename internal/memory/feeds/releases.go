package feeds

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/trilam/leah/internal/contracts"
)

// ScopeReleasesFetch is the operator-attestation scope for a release-notes
// pull. Distinct per-RPC so the audit log attributes consent at source-fetch
// granularity — the news scope can't double as cover for a separate adapter
// hitting a different surface (github.com vs news aggregators).
const ScopeReleasesFetch = "feeds:releases:fetch"

// ReleaseSource names one GitHub repo's per-repo release feed. Name is the
// "<owner>/<repo>" attribution surfaced to the operator; URL is the
// `https://github.com/<owner>/<repo>/releases.atom` endpoint.
type ReleaseSource struct {
	Name string
	URL  string
}

// ReleasesConfig wires the adapter. Same fail-fast contract as news/market —
// constructor refuses half-wired wiring so a missing gate never silently
// leaks a network call.
type ReleasesConfig struct {
	Attestor   contracts.Attestor
	HTTPClient *http.Client
	Sources    []ReleaseSource
}

// Releases is the GitHub release-notes Atom adapter. GitHub publishes RSS-2.0
// only for activity feeds; per-repo release feeds are Atom 1.0, so this needs
// its own parser distinct from the news RSS path.
type Releases struct {
	att     contracts.Attestor
	client  *http.Client
	sources []ReleaseSource
}

// NewReleases fails fast on missing deps. An Atom endpoint without an
// Attestor would silently bypass the operator-consent gate — that's a
// security bug, not a soft warning.
func NewReleases(cfg ReleasesConfig) (*Releases, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("feeds.NewReleases: Attestor required (operator-attestation gate)")
	}
	if cfg.HTTPClient == nil {
		return nil, errors.New("feeds.NewReleases: HTTPClient required")
	}
	if len(cfg.Sources) == 0 {
		return nil, errors.New("feeds.NewReleases: at least one Source required")
	}
	return &Releases{att: cfg.Attestor, client: cfg.HTTPClient, sources: cfg.Sources}, nil
}

// Name satisfies the Feed-style registry shape.
func (r *Releases) Name() string { return "releases" }

// Fetch gates on attestation BEFORE the HTTP call so a denied operator never
// causes a network round-trip. Partial failure is non-fatal — one rate-limited
// or renamed repo shouldn't blank the whole release digest.
func (r *Releases) Fetch(ctx context.Context) ([]Article, error) {
	if err := r.att.Attest(ctx, ScopeReleasesFetch); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	var all []Article
	var firstErr error
	for _, src := range r.sources {
		arts, err := r.fetchOne(ctx, src)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, arts...)
	}
	if len(all) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return all, nil
}

func (r *Releases) fetchOne(ctx context.Context, src ReleaseSource) ([]Article, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("feeds.releases[%s]: build request: %w", src.Name, err)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("feeds.releases[%s]: http: %w", src.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feeds.releases[%s]: provider %s", src.Name, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("feeds.releases[%s]: read body: %w", src.Name, err)
	}
	var raw atomFeed
	if err := xml.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("feeds.releases[%s]: decode: %w", src.Name, err)
	}
	out := make([]Article, 0, len(raw.Entries))
	for _, e := range raw.Entries {
		out = append(out, Article{
			Title:     e.Title,
			URL:       pickAtomLink(e.Links),
			Summary:   e.Content,
			Source:    src.Name,
			Published: parseAtomTime(e.Updated),
		})
	}
	return out, nil
}

// atomFeed trims Atom 1.0 to the fields synthesizers read. xml.Unmarshal
// drops unknown tags silently so this stays robust to GitHub's occasional
// tag additions (media:thumbnail, author/name, etc.).
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}
type atomEntry struct {
	Title   string     `xml:"title"`
	Updated string     `xml:"updated"`
	Links   []atomLink `xml:"link"`
	Content string     `xml:"content"`
}
type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
	Href string `xml:"href,attr"`
}

// pickAtomLink prefers rel="alternate" (the human-facing release page);
// falls back to the first link so a feed that omits rel still yields a URL.
func pickAtomLink(links []atomLink) string {
	for _, l := range links {
		if l.Rel == "alternate" {
			return l.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}

// parseAtomTime accepts the RFC3339 layout GitHub emits. Returns zero on
// unparseable input — synthesizer treats zero as "unknown age" which sorts
// last under recency rank.
func parseAtomTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
