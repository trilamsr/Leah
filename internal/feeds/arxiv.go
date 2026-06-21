package feeds

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ScopeArxivFetch is the operator-attestation scope per arXiv pull. Distinct
// from ScopeNewsFetch so the audit log can attribute paper-feed consent
// independently of generic RSS.
const ScopeArxivFetch = "feeds:arxiv:fetch"

// Paper is the synthesizer-facing payload for an arXiv item. Carries the
// arxiv-specific fields (PaperID, Authors, Category) the generic Article
// shape drops on the floor — the dedup pass and downstream ranking key on
// PaperID, not URL, so cross-listings across cs.AI/cs.LG/stat.ML collapse.
type Paper struct {
	Title     string
	URL       string
	Abstract  string
	Authors   []string
	PaperID   string
	Category  string
	Published time.Time
}

// ArxivCategory names a single arXiv RSS endpoint. Name is the canonical
// arxiv category id ("cs.AI", "cs.LG", "stat.ML"); URL points at the
// category's RSS feed (typically http://export.arxiv.org/rss/<name>).
type ArxivCategory struct {
	Name string
	URL  string
}

// ArxivConfig wires the adapter. Attestor + HTTPClient + at least one
// Category are required. AbstractKeyword is optional — when set, items whose
// abstract does not contain it (case-insensitive substring) are dropped
// before dedup so the operator-facing list stays topical.
type ArxivConfig struct {
	Attestor        Attestor
	HTTPClient      *http.Client
	Categories      []ArxivCategory
	AbstractKeyword string
}

// Arxiv is the multi-category arXiv RSS adapter. Lifecycle is caller-owned;
// no background goroutines so daemon shutdown stays clean.
type Arxiv struct {
	att        Attestor
	client     *http.Client
	categories []ArxivCategory
	keyword    string
}

// NewArxiv fails fast on missing deps — a half-wired arXiv adapter could
// silently bypass the operator-consent gate, which is a security bug not a
// soft warning. Same shape as NewNews so wiring-code muscle-memory carries.
func NewArxiv(cfg ArxivConfig) (*Arxiv, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("feeds.NewArxiv: Attestor required (operator-attestation gate)")
	}
	if cfg.HTTPClient == nil {
		return nil, errors.New("feeds.NewArxiv: HTTPClient required")
	}
	if len(cfg.Categories) == 0 {
		return nil, errors.New("feeds.NewArxiv: at least one Category required")
	}
	return &Arxiv{
		att:        cfg.Attestor,
		client:     cfg.HTTPClient,
		categories: cfg.Categories,
		keyword:    strings.ToLower(cfg.AbstractKeyword),
	}, nil
}

// Name satisfies a future Feed-style registry; the Paper-returning Fetch is
// distinct from the generic Feed.Fetch since the synth layer wants the typed
// payload.
func (a *Arxiv) Name() string { return "arxiv" }

// Fetch gates on attestation BEFORE the HTTP round-trip so a denied operator
// causes zero network calls. Iterates categories in declared order so the
// first-seen wins under dedup — operator-controlled attribution stability.
// Partial failure is non-fatal: one bad category shouldn't blank the digest.
func (a *Arxiv) Fetch(ctx context.Context) ([]Paper, error) {
	if err := a.att.Attest(ctx, ScopeArxivFetch); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	seen := make(map[string]struct{})
	var out []Paper
	var firstErr error
	for _, cat := range a.categories {
		papers, err := a.fetchOne(ctx, cat)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, p := range papers {
			if a.keyword != "" && !strings.Contains(strings.ToLower(p.Abstract), a.keyword) {
				continue
			}
			key := p.PaperID
			if key == "" {
				key = p.URL // fall back so a malformed item with no id still dedups by link
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, p)
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func (a *Arxiv) fetchOne(ctx context.Context, cat ArxivCategory) ([]Paper, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cat.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("feeds.arxiv[%s]: build request: %w", cat.Name, err)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("feeds.arxiv[%s]: http: %w", cat.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feeds.arxiv[%s]: provider %s", cat.Name, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("feeds.arxiv[%s]: read body: %w", cat.Name, err)
	}
	var raw arxivFeed
	if err := xml.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("feeds.arxiv[%s]: decode: %w", cat.Name, err)
	}
	out := make([]Paper, 0, len(raw.Channel.Items))
	for _, it := range raw.Channel.Items {
		out = append(out, Paper{
			Title:     strings.TrimSpace(it.Title),
			URL:       strings.TrimSpace(it.Link),
			Abstract:  cleanAbstract(it.Description),
			Authors:   splitAuthors(it.Creator),
			PaperID:   extractPaperID(it.Identifier, it.Link),
			Category:  cat.Name,
			Published: parseRSSTime(it.PubDate),
		})
	}
	return out, nil
}

// arxivFeed maps the arxiv: + dc: namespaces the generic rssFeed drops.
// xml.Unmarshal silently ignores unknown tags so the parser stays robust to
// arxiv's occasional schema tweaks.
type arxivFeed struct {
	XMLName xml.Name     `xml:"rss"`
	Channel arxivChannel `xml:"channel"`
}
type arxivChannel struct {
	Items []arxivItem `xml:"item"`
}
type arxivItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Identifier  string `xml:"http://purl.org/dc/elements/1.1/ identifier"`
}

// cleanAbstract strips arxiv's "Abstract: " prefix that prefixes every
// description field — synthesizer prompts already say "abstract" so leaving
// it in just wastes tokens.
func cleanAbstract(s string) string {
	s = strings.TrimSpace(s)
	return strings.TrimSpace(strings.TrimPrefix(s, "Abstract:"))
}

// splitAuthors parses dc:creator which arxiv emits as comma-separated. Empty
// or whitespace-only entries are dropped so the slice never carries "".
func splitAuthors(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if name := strings.TrimSpace(p); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// extractPaperID pulls the canonical arxiv id (e.g. "2606.00001") from
// dc:identifier ("oai:arXiv.org:2606.00001v1") or, as a fallback, the abs URL
// ("https://arxiv.org/abs/2606.00001"). Version suffix (vN) is stripped so
// cross-category dedup matches v1 and v2 of the same paper.
func extractPaperID(identifier, link string) string {
	if identifier != "" {
		if i := strings.LastIndex(identifier, ":"); i >= 0 && i+1 < len(identifier) {
			return trimVersion(identifier[i+1:])
		}
		// Trailing-colon or colon-less identifier is malformed; fall through to
		// link extraction so a salvageable abs URL still yields a real id.
	}
	if link != "" {
		if i := strings.LastIndex(link, "/abs/"); i >= 0 {
			return trimVersion(link[i+len("/abs/"):])
		}
		if i := strings.LastIndex(link, "/"); i >= 0 && i+1 < len(link) {
			return trimVersion(link[i+1:])
		}
	}
	return ""
}

// trimVersion strips a trailing "vN" suffix arxiv appends to identifiers so
// resubmissions don't escape the dedup pass.
func trimVersion(id string) string {
	id = strings.TrimSpace(id)
	if i := strings.LastIndexByte(id, 'v'); i > 0 {
		rest := id[i+1:]
		if rest != "" && allDigits(rest) {
			return id[:i]
		}
	}
	return id
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
