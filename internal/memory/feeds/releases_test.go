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

// atomReleasesBody mirrors GitHub's per-repo releases.atom shape — entry tags
// with title, link.href, updated, content. Trimmed from a real
// https://github.com/anthropics/claude-code/releases.atom response.
const atomReleasesBody = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <id>tag:github.com,2008:https://github.com/anthropics/claude-code/releases</id>
  <title>Release notes from claude-code</title>
  <updated>2026-06-20T18:00:00Z</updated>
  <entry>
    <id>tag:github.com,2008:Repository/1/v2.0.0</id>
    <updated>2026-06-20T18:00:00Z</updated>
    <link rel="alternate" type="text/html" href="https://github.com/anthropics/claude-code/releases/tag/v2.0.0"/>
    <title>v2.0.0</title>
    <content type="html">&lt;p&gt;Major release.&lt;/p&gt;</content>
  </entry>
  <entry>
    <id>tag:github.com,2008:Repository/1/v1.9.0</id>
    <updated>2026-06-15T09:00:00Z</updated>
    <link rel="alternate" type="text/html" href="https://github.com/anthropics/claude-code/releases/tag/v1.9.0"/>
    <title>v1.9.0</title>
    <content type="html">&lt;p&gt;Minor release.&lt;/p&gt;</content>
  </entry>
</feed>`

// TestReleases_FetchAtom_Happy parses a GitHub release atom feed into Articles.
func TestReleases_FetchAtom_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(atomReleasesBody))
	}))
	defer srv.Close()

	att := &fakeAttestor{}
	r, err := feeds.NewReleases(feeds.ReleasesConfig{
		Attestor:   att,
		HTTPClient: srv.Client(),
		Sources: []feeds.ReleaseSource{
			{Name: "anthropics/claude-code", URL: srv.URL},
		},
	})
	if err != nil {
		t.Fatalf("NewReleases: %v", err)
	}

	arts, err := r.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("got %d articles, want 2", len(arts))
	}
	if arts[0].Title != "v2.0.0" {
		t.Errorf("Title[0] = %q", arts[0].Title)
	}
	if arts[0].URL != "https://github.com/anthropics/claude-code/releases/tag/v2.0.0" {
		t.Errorf("URL[0] = %q", arts[0].URL)
	}
	if arts[0].Source != "anthropics/claude-code" {
		t.Errorf("Source[0] = %q", arts[0].Source)
	}
	if arts[0].Published.IsZero() {
		t.Errorf("Published[0] zero")
	}
	if len(att.calls) != 1 || att.calls[0] != feeds.ScopeReleasesFetch {
		t.Errorf("Attest calls = %v, want [%s]", att.calls, feeds.ScopeReleasesFetch)
	}
}

// TestReleases_AttestationDenied_NoHTTPCall — gate stops network when denied.
// Mirrors news/market: a denied operator must never trigger the round-trip,
// otherwise the audit-log entry is wrong about what data left the host.
func TestReleases_AttestationDenied_NoHTTPCall(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit = true
	}))
	defer srv.Close()

	r, err := feeds.NewReleases(feeds.ReleasesConfig{
		Attestor:   &fakeAttestor{err: errors.New("denied")},
		HTTPClient: srv.Client(),
		Sources:    []feeds.ReleaseSource{{Name: "x/y", URL: srv.URL}},
	})
	if err != nil {
		t.Fatalf("NewReleases: %v", err)
	}
	if _, err := r.Fetch(context.Background()); !errors.Is(err, feeds.ErrAttestationDenied) {
		t.Fatalf("Fetch err = %v, want ErrAttestationDenied wrap", err)
	}
	if hit {
		t.Fatalf("HTTP server hit after denied attestation")
	}
}

// TestNewReleases_RejectsHalfWiredConfig — fail-fast on missing deps.
func TestNewReleases_RejectsHalfWiredConfig(t *testing.T) {
	base := feeds.ReleasesConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: http.DefaultClient,
		Sources:    []feeds.ReleaseSource{{Name: "x/y", URL: "http://example.com"}},
	}
	cases := []struct {
		name string
		mut  func(*feeds.ReleasesConfig)
	}{
		{"no attestor", func(c *feeds.ReleasesConfig) { c.Attestor = nil }},
		{"no http client", func(c *feeds.ReleasesConfig) { c.HTTPClient = nil }},
		{"no sources", func(c *feeds.ReleasesConfig) { c.Sources = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mut(&c)
			if _, err := feeds.NewReleases(c); err == nil {
				t.Fatalf("NewReleases accepted half-wired config")
			}
		})
	}
}

// TestReleases_FetchMalformedXML_ReturnsError — corrupt feed surfaces as error.
func TestReleases_FetchMalformedXML_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not xml <<>>"))
	}))
	defer srv.Close()
	r, err := feeds.NewReleases(feeds.ReleasesConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: srv.Client(),
		Sources:    []feeds.ReleaseSource{{Name: "x/y", URL: srv.URL}},
	})
	if err != nil {
		t.Fatalf("NewReleases: %v", err)
	}
	if _, err := r.Fetch(context.Background()); err == nil {
		t.Fatalf("Fetch malformed XML returned nil err")
	} else if !strings.Contains(err.Error(), "decode") {
		t.Errorf("err = %v, want contains 'decode'", err)
	}
}

// atomEdgeCasesBody hits pickAtomLink fallback + parseAtomTime zero-time path.
const atomEdgeCasesBody = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>tag:github.com,2008:Repository/1/v3.0.0</id>
    <updated>not-a-date</updated>
    <link rel="self" type="application/atom+xml" href="https://example.com/self"/>
    <link rel="edit" type="application/atom+xml" href="https://example.com/edit"/>
    <title>v3.0.0</title>
    <content type="html">edge.</content>
  </entry>
</feed>`

// TestReleases_FetchAtom_HelperFallbacks covers non-alternate link fallback + malformed updated.
func TestReleases_FetchAtom_HelperFallbacks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(atomEdgeCasesBody))
	}))
	defer srv.Close()

	r, err := feeds.NewReleases(feeds.ReleasesConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: srv.Client(),
		Sources:    []feeds.ReleaseSource{{Name: "x/y", URL: srv.URL}},
	})
	if err != nil {
		t.Fatalf("NewReleases: %v", err)
	}
	arts, err := r.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("got %d, want 1", len(arts))
	}
	if arts[0].URL != "https://example.com/self" {
		t.Errorf("URL = %q, want first-link fallback https://example.com/self", arts[0].URL)
	}
	if !arts[0].Published.IsZero() {
		t.Errorf("Published = %v, want zero on malformed updated", arts[0].Published)
	}
}

// TestDefaultReleaseSources_Shape catches empty-default or malformed-URL regressions.
func TestDefaultReleaseSources_Shape(t *testing.T) {
	srcs := feeds.DefaultReleaseSources()
	if len(srcs) < 10 {
		t.Fatalf("DefaultReleaseSources = %d, want >= 10", len(srcs))
	}
	seen := map[string]bool{}
	for _, s := range srcs {
		if s.Name == "" {
			t.Errorf("empty Name in %+v", s)
		}
		if !strings.HasPrefix(s.URL, "https://github.com/") || !strings.HasSuffix(s.URL, "/releases.atom") {
			t.Errorf("malformed URL %q for %s", s.URL, s.Name)
		}
		if seen[s.Name] {
			t.Errorf("duplicate Name %q", s.Name)
		}
		seen[s.Name] = true
	}
}
