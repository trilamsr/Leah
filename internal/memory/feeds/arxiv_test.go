package feeds_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/trilam/leah/internal/memory/feeds"
)

// arxivCSAIBody mirrors arxiv.org's RSS-2.0 emission for a category feed —
// arxiv: + dc: namespaces carry the paper-id, authors, and announce_type the
// generic news adapter throws away. Trimmed from a real
// http://export.arxiv.org/rss/cs.AI response.
const arxivCSAIBody = `<?xml version='1.0' encoding='UTF-8'?>
<rss xmlns:arxiv="http://arxiv.org/schemas/atom" xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0">
<channel>
<title>cs.AI updates on arXiv.org</title>
<link>http://rss.arxiv.org/rss/cs.AI</link>
<description>cs.AI updates on the arXiv.org e-print archive.</description>
<item>
<title>Scaling laws for transformer attention</title>
<link>https://arxiv.org/abs/2606.00001</link>
<description>Abstract: We study the scaling behavior of attention heads under transformer pretraining.</description>
<dc:creator>Alice Researcher, Bob Scientist</dc:creator>
<dc:identifier>oai:arXiv.org:2606.00001v1</dc:identifier>
<arxiv:announce_type>new</arxiv:announce_type>
<pubDate>Wed, 10 Jun 2026 00:00:00 +0000</pubDate>
</item>
<item>
<title>A new benchmark for code agents</title>
<link>https://arxiv.org/abs/2606.00002</link>
<description>Abstract: We introduce a benchmark for measuring code-agent task completion rates.</description>
<dc:creator>Carol Coder</dc:creator>
<dc:identifier>oai:arXiv.org:2606.00002v1</dc:identifier>
<arxiv:announce_type>new</arxiv:announce_type>
<pubDate>Wed, 10 Jun 2026 00:00:00 +0000</pubDate>
</item>
</channel>
</rss>`

// arxivCSLGBody overlaps with cs.AI on paper 2606.00002 — cross-listing is
// the dedup case the adapter must collapse.
const arxivCSLGBody = `<?xml version='1.0' encoding='UTF-8'?>
<rss xmlns:arxiv="http://arxiv.org/schemas/atom" xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0">
<channel>
<title>cs.LG updates on arXiv.org</title>
<link>http://rss.arxiv.org/rss/cs.LG</link>
<description>cs.LG updates on the arXiv.org e-print archive.</description>
<item>
<title>A new benchmark for code agents</title>
<link>https://arxiv.org/abs/2606.00002</link>
<description>Abstract: We introduce a benchmark for measuring code-agent task completion rates.</description>
<dc:creator>Carol Coder</dc:creator>
<dc:identifier>oai:arXiv.org:2606.00002v1</dc:identifier>
<arxiv:announce_type>cross</arxiv:announce_type>
<pubDate>Wed, 10 Jun 2026 00:00:00 +0000</pubDate>
</item>
<item>
<title>Gradient flow in deep residual networks</title>
<link>https://arxiv.org/abs/2606.00003</link>
<description>Abstract: We analyze gradient propagation in residual architectures via stochastic differential equations.</description>
<dc:creator>Dan Theoretician</dc:creator>
<dc:identifier>oai:arXiv.org:2606.00003v1</dc:identifier>
<arxiv:announce_type>new</arxiv:announce_type>
<pubDate>Wed, 10 Jun 2026 00:00:00 +0000</pubDate>
</item>
</channel>
</rss>`

// TestArxiv_Fetch_ParsesArxivFields parses arxiv-specific fields (paper-id,
// authors) the generic RSS adapter would otherwise drop.
func TestArxiv_Fetch_ParsesArxivFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(arxivCSAIBody))
	}))
	defer srv.Close()

	att := &fakeAttestor{}
	a, err := feeds.NewArxiv(feeds.ArxivConfig{
		Attestor:   att,
		HTTPClient: srv.Client(),
		Categories: []feeds.ArxivCategory{{Name: "cs.AI", URL: srv.URL}},
	})
	if err != nil {
		t.Fatalf("NewArxiv: %v", err)
	}

	papers, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("got %d papers, want 2", len(papers))
	}
	p := papers[0]
	if p.Title != "Scaling laws for transformer attention" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.PaperID != "2606.00001" {
		t.Errorf("PaperID = %q, want 2606.00001", p.PaperID)
	}
	if len(p.Authors) != 2 || p.Authors[0] != "Alice Researcher" || p.Authors[1] != "Bob Scientist" {
		t.Errorf("Authors = %v, want [Alice Researcher, Bob Scientist]", p.Authors)
	}
	if !strings.Contains(p.Abstract, "scaling behavior of attention heads") {
		t.Errorf("Abstract missing expected text: %q", p.Abstract)
	}
	if p.Category != "cs.AI" {
		t.Errorf("Category = %q, want cs.AI", p.Category)
	}
	if len(att.calls) != 1 || att.calls[0] != feeds.ScopeArxivFetch {
		t.Errorf("Attest calls = %v, want [%s]", att.calls, feeds.ScopeArxivFetch)
	}
}

// TestArxiv_Fetch_DedupesByPaperID — cross-listing across cs.AI + cs.LG must
// not surface the same paper twice; the first-seen category wins so digest
// attribution stays stable.
func TestArxiv_Fetch_DedupesByPaperID(t *testing.T) {
	aiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(arxivCSAIBody))
	}))
	defer aiSrv.Close()
	lgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(arxivCSLGBody))
	}))
	defer lgSrv.Close()

	a, err := feeds.NewArxiv(feeds.ArxivConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: aiSrv.Client(),
		Categories: []feeds.ArxivCategory{
			{Name: "cs.AI", URL: aiSrv.URL},
			{Name: "cs.LG", URL: lgSrv.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewArxiv: %v", err)
	}
	papers, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Expect 3 unique papers: cs.AI [2606.00001, 2606.00002] + cs.LG adds [2606.00003]
	if len(papers) != 3 {
		t.Fatalf("got %d papers, want 3 (dedup failed)", len(papers))
	}
	for _, p := range papers {
		if p.PaperID == "2606.00002" && p.Category != "cs.AI" {
			t.Errorf("dedup kept wrong category %q for shared paper; first-seen cs.AI must win", p.Category)
		}
	}
}

// TestArxiv_Fetch_KeywordFilter restricts to papers whose abstract matches.
// Case-insensitive substring is the documented contract.
func TestArxiv_Fetch_KeywordFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(arxivCSAIBody))
	}))
	defer srv.Close()

	a, err := feeds.NewArxiv(feeds.ArxivConfig{
		Attestor:        &fakeAttestor{},
		HTTPClient:      srv.Client(),
		Categories:      []feeds.ArxivCategory{{Name: "cs.AI", URL: srv.URL}},
		AbstractKeyword: "BENCHMARK",
	})
	if err != nil {
		t.Fatalf("NewArxiv: %v", err)
	}
	papers, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("got %d papers, want 1 (keyword filter)", len(papers))
	}
	if papers[0].PaperID != "2606.00002" {
		t.Errorf("PaperID = %q, want 2606.00002", papers[0].PaperID)
	}
}

// TestArxiv_AttestationDenied_NoHTTPCall — gate stops network when denied.
func TestArxiv_AttestationDenied_NoHTTPCall(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	a, err := feeds.NewArxiv(feeds.ArxivConfig{
		Attestor:   &fakeAttestor{err: errors.New("denied")},
		HTTPClient: srv.Client(),
		Categories: []feeds.ArxivCategory{{Name: "cs.AI", URL: srv.URL}},
	})
	if err != nil {
		t.Fatalf("NewArxiv: %v", err)
	}
	if _, err := a.Fetch(context.Background()); !errors.Is(err, feeds.ErrAttestationDenied) {
		t.Fatalf("Fetch err = %v, want ErrAttestationDenied wrap", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("HTTP server hit %d times after denied attestation", hits)
	}
}

// TestNewArxiv_RejectsHalfWiredConfig — fail-fast on missing deps. Same shape
// as News so wiring code can't accidentally ship a gateless adapter.
func TestNewArxiv_RejectsHalfWiredConfig(t *testing.T) {
	base := feeds.ArxivConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: http.DefaultClient,
		Categories: []feeds.ArxivCategory{{Name: "cs.AI", URL: "http://example.com"}},
	}
	cases := []struct {
		name string
		mut  func(*feeds.ArxivConfig)
	}{
		{"no attestor", func(c *feeds.ArxivConfig) { c.Attestor = nil }},
		{"no http client", func(c *feeds.ArxivConfig) { c.HTTPClient = nil }},
		{"no categories", func(c *feeds.ArxivConfig) { c.Categories = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mut(&c)
			if _, err := feeds.NewArxiv(c); err == nil {
				t.Fatalf("NewArxiv accepted half-wired config")
			}
		})
	}
}

// TestArxiv_FetchMalformedXML_ReturnsError — corrupt feed surfaces as error.
func TestArxiv_FetchMalformedXML_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not xml <<>>"))
	}))
	defer srv.Close()
	a, err := feeds.NewArxiv(feeds.ArxivConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: srv.Client(),
		Categories: []feeds.ArxivCategory{{Name: "cs.AI", URL: srv.URL}},
	})
	if err != nil {
		t.Fatalf("NewArxiv: %v", err)
	}
	if _, err := a.Fetch(context.Background()); err == nil {
		t.Fatalf("Fetch malformed XML returned nil err")
	} else if !strings.Contains(err.Error(), "decode") {
		t.Errorf("err = %v, want contains 'decode'", err)
	}
}

// arxivMalformedIdentifierBody — dc:identifier ends mid-colon (corrupt
// upstream). Adapter must fall through to the abs-URL fallback so dedup
// still works instead of keying on the malformed prefix.
const arxivMalformedIdentifierBody = `<?xml version='1.0' encoding='UTF-8'?>
<rss xmlns:arxiv="http://arxiv.org/schemas/atom" xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0">
<channel>
<title>cs.AI updates on arXiv.org</title>
<item>
<title>Malformed identifier paper</title>
<link>https://arxiv.org/abs/2606.00099</link>
<description>Abstract: A paper whose dc:identifier is corrupted upstream.</description>
<dc:creator>Eve Edge</dc:creator>
<dc:identifier>oai:arXiv.org:</dc:identifier>
<pubDate>Wed, 10 Jun 2026 00:00:00 +0000</pubDate>
</item>
</channel>
</rss>`

// TestArxiv_Fetch_MalformedIdentifier_FallsBackToLink — corrupt dc:identifier
// (trailing colon) must NOT poison the dedup key; adapter recovers the id
// from the abs URL.
func TestArxiv_Fetch_MalformedIdentifier_FallsBackToLink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(arxivMalformedIdentifierBody))
	}))
	defer srv.Close()

	a, err := feeds.NewArxiv(feeds.ArxivConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: srv.Client(),
		Categories: []feeds.ArxivCategory{{Name: "cs.AI", URL: srv.URL}},
	})
	if err != nil {
		t.Fatalf("NewArxiv: %v", err)
	}
	papers, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("got %d papers, want 1", len(papers))
	}
	if papers[0].PaperID != "2606.00099" {
		t.Errorf("PaperID = %q, want 2606.00099 (link fallback)", papers[0].PaperID)
	}
}

// TestArxiv_Fetch_PartialFailure_OneCategoryDown — one 500 must not blank the
// digest when the other category succeeds.
func TestArxiv_Fetch_PartialFailure_OneCategoryDown(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(arxivCSAIBody))
	}))
	defer good.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad.Close()

	a, err := feeds.NewArxiv(feeds.ArxivConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: good.Client(),
		Categories: []feeds.ArxivCategory{
			{Name: "cs.AI", URL: good.URL},
			{Name: "cs.LG", URL: bad.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewArxiv: %v", err)
	}
	papers, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch with one bad category returned err = %v, want nil (partial OK)", err)
	}
	if len(papers) != 2 {
		t.Fatalf("got %d papers, want 2 (from healthy cs.AI)", len(papers))
	}
}
