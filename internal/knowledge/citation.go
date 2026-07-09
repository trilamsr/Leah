package knowledge

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

// CitationEnrichment attaches source-domain and KG-entity metadata to a
// citation widget tile so a rendered tile can show its origin (paper /
// repo / project) without the UI re-resolving the URL.
//
// Empty EntityKey means the URL parsed but had no KG match — the tile
// still gets Domain + (when applicable) ArxivID so the UI is useful
// before the graph catches up.
type CitationEnrichment struct {
	URL        string     `json:"url"`
	Domain     string     `json:"domain,omitempty"`
	ArxivID    string     `json:"arxiv_id,omitempty"`
	EntityKind EntityKind `json:"entity_kind,omitempty"`
	EntityKey  string     `json:"entity_key,omitempty"`
	Display    string     `json:"display,omitempty"`
}

// arxivPath captures both new-style (YYMM.NNNNN) and old-style
// (archive/YYMMNNN) IDs. Mirrors internal/papers/arxiv.go without
// importing it — citation enrichment must stay leaf-package free of
// fetchers.
var arxivPath = regexp.MustCompile(`([a-z\-]+/\d{7}|\d{4}\.\d{4,5})`)

// EnrichCitation looks up KG nodes whose aliases contain the citation
// URL and returns source-domain + entity metadata. Returns (nil, nil)
// for unknown URLs — only ctx cancellation surfaces as an error.
func EnrichCitation(ctx context.Context, g *Graph, citationURL string) (*CitationEnrichment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if citationURL == "" {
		return nil, nil
	}
	u, err := url.Parse(citationURL)
	if err != nil || u.Host == "" {
		return nil, nil
	}
	enr := &CitationEnrichment{URL: citationURL, Domain: strings.ToLower(u.Host)}
	// Exact host or subdomain only — bare HasSuffix would accept
	// hostile look-alikes like evil-arxiv.org and spoof an ID.
	if enr.Domain == "arxiv.org" || strings.HasSuffix(enr.Domain, ".arxiv.org") {
		if m := arxivPath.FindString(strings.ToLower(u.Path)); m != "" {
			enr.ArxivID = m
		}
	}
	hit, herr := lookupByAlias(ctx, g, citationURL)
	if herr != nil {
		return nil, herr
	}
	if hit != nil {
		enr.EntityKind = hit.Kind
		enr.EntityKey = hit.Key
		enr.Display = hit.Display
	}
	if enr.EntityKey == "" && enr.ArxivID == "" {
		// Domain alone isn't useful — drop to nil so the widget
		// renders the raw URL rather than a half-empty tile.
		return nil, nil
	}
	return enr, nil
}

// lookupByAlias walks KG entities for an exact alias match. Linear scan
// is fine — citation enrichment runs in the widget mount path on a
// short-lived per-message basis, and the KG is bounded.
func lookupByAlias(ctx context.Context, g *Graph, needle string) (*Entity, error) {
	if g == nil {
		return nil, nil
	}
	entities, err := g.store.matchEntities("", "", 0)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i := range entities {
		for _, a := range entities[i].Aliases {
			if a == needle {
				return &entities[i], nil
			}
		}
	}
	return nil, nil
}

