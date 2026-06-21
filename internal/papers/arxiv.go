package papers

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// DefaultArxivEndpoint is the public Atom-API endpoint. https avoids the
// 301 the http variant returns, saving a redirect round-trip.
const DefaultArxivEndpoint = "https://export.arxiv.org/api/query"

// arxivIDRE matches both new-style (YYMM.NNNNN) and old-style (archive/YYMMNNN)
// arXiv identifiers. Capture group 1 is the canonical ID we persist.
var arxivIDRE = regexp.MustCompile(`([a-z\-]+/\d{7}|\d{4}\.\d{4,5})`)

// NormalizeArxivID accepts a bare ID, versioned ID, or any arxiv.org URL
// shape and returns the canonical ID with no version suffix. Versionless
// canonical form is the dedup key so saving `2406.12345v1` then
// `2406.12345v2` collapses to one row.
func NormalizeArxivID(in string) (string, error) {
	s := strings.TrimSpace(in)
	if s == "" {
		return "", errors.New("empty arxiv id")
	}
	m := arxivIDRE.FindString(strings.ToLower(s))
	if m == "" {
		return "", fmt.Errorf("not a valid arxiv id: %q", in)
	}
	return m, nil
}

// ArxivFetcher hits the Atom API to populate a Paper's metadata. Endpoint +
// Client are injectable so tests wire httptest without touching the network.
type ArxivFetcher struct {
	Endpoint string
	Client   *http.Client
}

// FetchMetadata returns a Paper with Title/Abstract/Authors/Link/ID populated.
// SavedTS + Status are caller-supplied — the fetcher is metadata-only so the
// store layer owns lifecycle state.
func (f *ArxivFetcher) FetchMetadata(ctx context.Context, id string) (Paper, error) {
	endpoint := f.Endpoint
	if endpoint == "" {
		endpoint = DefaultArxivEndpoint
	}
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return Paper{}, fmt.Errorf("papers.arxiv: parse endpoint: %w", err)
	}
	q := u.Query()
	q.Set("id_list", id)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Paper{}, fmt.Errorf("papers.arxiv: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Paper{}, fmt.Errorf("papers.arxiv: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Paper{}, fmt.Errorf("papers.arxiv: provider %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Paper{}, fmt.Errorf("papers.arxiv: read body: %w", err)
	}

	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return Paper{}, fmt.Errorf("papers.arxiv: decode: %w", err)
	}
	if len(feed.Entries) == 0 {
		return Paper{}, fmt.Errorf("papers.arxiv: no entry for id %q", id)
	}
	e := feed.Entries[0]

	authors := make([]string, 0, len(e.Authors))
	for _, a := range e.Authors {
		if name := strings.TrimSpace(a.Name); name != "" {
			authors = append(authors, name)
		}
	}
	link := ""
	for _, l := range e.Links {
		if l.Rel == "" || l.Rel == "alternate" {
			link = l.Href
			break
		}
	}
	return Paper{
		ID:       id,
		Title:    strings.TrimSpace(collapseWS(e.Title)),
		Abstract: strings.TrimSpace(collapseWS(e.Summary)),
		Authors:  authors,
		Link:     link,
	}, nil
}

// atomFeed maps just the fields the queue cares about; xml.Unmarshal silently
// ignores the opensearch/arxiv namespace tags the API also emits.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string       `xml:"title"`
	Summary string       `xml:"summary"`
	Authors []atomAuthor `xml:"author"`
	Links   []atomLink   `xml:"link"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

// collapseWS flattens the arXiv Atom feed's pretty-printed multi-line
// title/summary blocks into a single space-separated string so terminal
// output doesn't carry stray newlines from the XML indentation.
func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
