package feeds_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/memory/feeds"
)

// hnRSSBody mirrors Hacker News' RSS shape — title + link + pubDate per item.
// Trimmed from a real https://hnrss.org/frontpage response.
const hnRSSBody = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
<title>Hacker News: Front Page</title>
<link>https://news.ycombinator.com/</link>
<description>Hacker News RSS</description>
<item>
<title>Show HN: A new Go feeds library</title>
<link>https://example.com/a</link>
<pubDate>Wed, 10 Jun 2026 12:00:00 +0000</pubDate>
<description>A tiny RSS parser.</description>
</item>
<item>
<title>Bitcoin hits new high</title>
<link>https://example.com/b</link>
<pubDate>Wed, 10 Jun 2026 11:30:00 +0000</pubDate>
<description>Markets react.</description>
</item>
<item>
<title>Old story</title>
<link>https://example.com/c</link>
<pubDate>Tue, 09 Jun 2026 09:00:00 +0000</pubDate>
<description>Older content.</description>
</item>
</channel>
</rss>`

// TestNews_FetchHN_Happy parses an HN-shaped RSS feed into Articles.
func TestNews_FetchHN_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(hnRSSBody))
	}))
	defer srv.Close()

	att := &fakeAttestor{}
	n, err := feeds.NewNews(feeds.NewsConfig{
		Attestor:   att,
		HTTPClient: srv.Client(),
		Sources: []feeds.NewsSource{
			{Name: "hn", URL: srv.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewNews: %v", err)
	}

	arts, err := n.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(arts) != 3 {
		t.Fatalf("got %d articles, want 3", len(arts))
	}
	if arts[0].Title != "Show HN: A new Go feeds library" {
		t.Errorf("Title[0] = %q", arts[0].Title)
	}
	if arts[0].URL != "https://example.com/a" {
		t.Errorf("URL[0] = %q", arts[0].URL)
	}
	if arts[0].Source != "hn" {
		t.Errorf("Source[0] = %q, want hn", arts[0].Source)
	}
	if arts[0].Published.IsZero() {
		t.Errorf("Published[0] zero")
	}
	if len(att.calls) != 1 || att.calls[0] != feeds.ScopeNewsFetch {
		t.Errorf("Attest calls = %v, want [%s]", att.calls, feeds.ScopeNewsFetch)
	}
}

// TestNews_AttestationDenied_NoHTTPCall — gate stops network when denied.
func TestNews_AttestationDenied_NoHTTPCall(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit = true
	}))
	defer srv.Close()

	n, err := feeds.NewNews(feeds.NewsConfig{
		Attestor:   &fakeAttestor{err: errors.New("denied")},
		HTTPClient: srv.Client(),
		Sources:    []feeds.NewsSource{{Name: "hn", URL: srv.URL}},
	})
	if err != nil {
		t.Fatalf("NewNews: %v", err)
	}
	if _, err := n.Fetch(context.Background()); !errors.Is(err, feeds.ErrAttestationDenied) {
		t.Fatalf("Fetch err = %v, want ErrAttestationDenied wrap", err)
	}
	if hit {
		t.Fatalf("HTTP server hit after denied attestation")
	}
}

// TestNewNews_RejectsHalfWiredConfig — fail-fast on missing deps.
func TestNewNews_RejectsHalfWiredConfig(t *testing.T) {
	base := feeds.NewsConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: http.DefaultClient,
		Sources:    []feeds.NewsSource{{Name: "hn", URL: "http://example.com"}},
	}
	cases := []struct {
		name string
		mut  func(*feeds.NewsConfig)
	}{
		{"no attestor", func(c *feeds.NewsConfig) { c.Attestor = nil }},
		{"no http client", func(c *feeds.NewsConfig) { c.HTTPClient = nil }},
		{"no sources", func(c *feeds.NewsConfig) { c.Sources = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mut(&c)
			if _, err := feeds.NewNews(c); err == nil {
				t.Fatalf("NewNews accepted half-wired config")
			}
		})
	}
}

// TestNews_FetchMalformedXML_ReturnsError — corrupt feed surfaces as error.
func TestNews_FetchMalformedXML_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not xml <<>>"))
	}))
	defer srv.Close()
	n, err := feeds.NewNews(feeds.NewsConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: srv.Client(),
		Sources:    []feeds.NewsSource{{Name: "hn", URL: srv.URL}},
	})
	if err != nil {
		t.Fatalf("NewNews: %v", err)
	}
	if _, err := n.Fetch(context.Background()); err == nil {
		t.Fatalf("Fetch malformed XML returned nil err")
	} else if !strings.Contains(err.Error(), "decode") {
		t.Errorf("err = %v, want contains 'decode'", err)
	}
}
