package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rssBody is a minimal RSS-2.0 doc with two items the synthesizer trims to
// the digest output.
const rssBody = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
<title>Test</title>
<item><title>Newest item</title><link>https://example.com/new</link><pubDate>Wed, 10 Jun 2026 12:00:00 +0000</pubDate></item>
<item><title>Older item</title><link>https://example.com/old</link><pubDate>Wed, 10 Jun 2026 10:00:00 +0000</pubDate></item>
</channel>
</rss>`

// TestRunNews_Happy fetches a fake RSS server and prints the digest.
func TestRunNews_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")

	// Override default sources via feeds-news.json
	cfg := []map[string]string{{"Name": "test", "URL": srv.URL}}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "feeds-news.json"), raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var buf bytes.Buffer
	if code := runNews(context.Background(), nil, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "Newest item") {
		t.Errorf("output missing 'Newest item': %q", out)
	}
	if !strings.Contains(out, "test") {
		t.Errorf("output missing source attribution 'test': %q", out)
	}
}

// TestRunNews_UsageError rejects positional args.
func TestRunNews_UsageError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"extra"}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunNews_Help prints help on -h and returns 0.
func TestRunNews_Help(t *testing.T) {
	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--help"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(buf.String(), "usage: leah news") {
		t.Errorf("help output missing usage line: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "--bundle") {
		t.Errorf("help output missing --bundle flag doc: %q", buf.String())
	}
}

// TestLoadNewsSources_Fallback returns defaults when file is missing.
func TestLoadNewsSources_Fallback(t *testing.T) {
	dir := t.TempDir()
	got := loadNewsSources(dir)
	if len(got) == 0 {
		t.Fatalf("expected default sources, got 0")
	}
}

// TestLoadNewsSources_CorruptJSON_FallsBackToDefault — soft-fail on bad config.
func TestLoadNewsSources_CorruptJSON_FallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "feeds-news.json"), []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := loadNewsSources(dir)
	if len(got) == 0 {
		t.Fatalf("expected default sources on corrupt config, got 0")
	}
}

// TestBundleSources_AI returns the curated AI/ML feed list.
func TestBundleSources_AI(t *testing.T) {
	got, ok := bundleSources("ai")
	if !ok {
		t.Fatalf("bundle 'ai' not registered")
	}
	if len(got) < 5 {
		t.Fatalf("ai bundle has %d sources, want at least 5", len(got))
	}
	wantNames := map[string]bool{
		"arxiv-cs.ai":   false,
		"arxiv-cs.lg":   false,
		"anthropic":     false,
		"openai":        false,
		"deepmind":      false,
		"huggingface":   false,
		"simonwillison": false,
	}
	for _, s := range got {
		if _, ok := wantNames[s.Name]; ok {
			wantNames[s.Name] = true
		}
		if s.URL == "" {
			t.Errorf("source %q has empty URL", s.Name)
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Errorf("ai bundle missing source %q", name)
		}
	}
}

// TestBundleSources_Tech returns the curated tech feed list.
func TestBundleSources_Tech(t *testing.T) {
	got, ok := bundleSources("tech")
	if !ok {
		t.Fatalf("bundle 'tech' not registered")
	}
	if len(got) < 2 {
		t.Fatalf("tech bundle has %d sources, want at least 2", len(got))
	}
}

// TestBundleSources_Unknown returns ok=false for unregistered bundle.
func TestBundleSources_Unknown(t *testing.T) {
	if _, ok := bundleSources("nope"); ok {
		t.Fatalf("bundle 'nope' should not be registered")
	}
}

// TestRunNews_BundleAI swaps the bundle URLs to a hermetic httptest server via
// the test-only package hook, then asserts the bundle short-circuits the
// feeds-news.json path.
func TestRunNews_BundleAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	prev := bundleURLOverride
	bundleURLOverride = srv.URL
	t.Cleanup(func() { bundleURLOverride = prev })

	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--bundle", "ai"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "Newest item") {
		t.Errorf("output missing 'Newest item': %q", buf.String())
	}
}

// TestRunNews_BundleDuplicate rejects repeat --bundle flags — last-wins would
// silently swap the source list.
func TestRunNews_BundleDuplicate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--bundle", "ai", "--bundle", "tech"}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunNews_BundleUnknown errors on an unknown bundle name.
func TestRunNews_BundleUnknown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--bundle", "nope"}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunNews_BundleMissingValue errors when --bundle has no argument.
func TestRunNews_BundleMissingValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--bundle"}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunNews_SinceFiltersOlder drops items with pubDate < cursor.
func TestRunNews_SinceFiltersOlder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	cfg := []map[string]string{{"Name": "test", "URL": srv.URL}}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "feeds-news.json"), raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Cursor between the two fixture items: 11:00 UTC keeps "Newest" (12:00),
	// drops "Older" (10:00).
	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--since=2026-06-10T11:00:00Z"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "Newest item") {
		t.Errorf("output missing 'Newest item': %q", out)
	}
	if strings.Contains(out, "Older item") {
		t.Errorf("output should have filtered 'Older item': %q", out)
	}
}

// TestRunNews_SinceEmptyResult — cursor past every item silently returns 0.
func TestRunNews_SinceEmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	cfg := []map[string]string{{"Name": "test", "URL": srv.URL}}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "feeds-news.json"), raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--since=2099-01-01T00:00:00Z"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "(no headlines)") {
		t.Errorf("expected '(no headlines)' empty marker, got %q", buf.String())
	}
}

// TestRunNews_SinceInvalidRFC3339 rejects malformed cursor with exit 2.
func TestRunNews_SinceInvalidRFC3339(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--since=yesterday"}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunNews_SinceMissingValue errors when --since has no argument.
func TestRunNews_SinceMissingValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--since"}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunNews_SinceDuplicate rejects repeat --since flags.
func TestRunNews_SinceDuplicate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	args := []string{"--since=2026-06-10T00:00:00Z", "--since=2026-06-11T00:00:00Z"}
	if code := runNews(context.Background(), args, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunNews_SinceSpaceForm verifies `--since <value>` parses identically to
// `--since=<value>` — operators conditioned by Unix conventions reach for both.
func TestRunNews_SinceSpaceForm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	cfg := []map[string]string{{"Name": "test", "URL": srv.URL}}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "feeds-news.json"), raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--since", "2026-06-10T11:00:00Z"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "Newest item") {
		t.Errorf("space-form --since failed to filter correctly: %q", buf.String())
	}
	if strings.Contains(buf.String(), "Older item") {
		t.Errorf("space-form --since left older item: %q", buf.String())
	}
}

// TestRunNews_SinceWithBundle pins the --since + --bundle interaction so
// composed cursor+source-list usage doesn't silently regress.
func TestRunNews_SinceWithBundle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	prev := bundleURLOverride
	bundleURLOverride = srv.URL
	t.Cleanup(func() { bundleURLOverride = prev })

	var buf bytes.Buffer
	args := []string{"--bundle", "ai", "--since=2026-06-10T11:00:00Z"}
	if code := runNews(context.Background(), args, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "Newest item") {
		t.Errorf("bundle+since combo missing newest item: %q", buf.String())
	}
	if strings.Contains(buf.String(), "Older item") {
		t.Errorf("bundle+since combo left older item: %q", buf.String())
	}
}

// TestRunNews_SinceBoundaryInclusive — an item with pubDate exactly at the
// cursor is kept. Cursor semantics are "since X" not "after X"; off-by-one
// here loses items on the seam between two daily runs.
func TestRunNews_SinceBoundaryInclusive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	cfg := []map[string]string{{"Name": "test", "URL": srv.URL}}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "feeds-news.json"), raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var buf bytes.Buffer
	// Newest item pubDate is exactly 2026-06-10T12:00:00Z.
	if code := runNews(context.Background(), []string{"--since=2026-06-10T12:00:00Z"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "Newest item") {
		t.Errorf("boundary item dropped — cursor should be inclusive: %q", buf.String())
	}
}

// rssBodyMissingPubDate has one valid-dated item + one with no pubDate. Used to
// pin the "drop zero-pubDate when cursor set" policy: caller asked for "since X"
// and an undated item can't be proven to meet it.
const rssBodyMissingPubDate = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
<title>Test</title>
<item><title>Dated item</title><link>https://example.com/dated</link><pubDate>Wed, 10 Jun 2026 12:00:00 +0000</pubDate></item>
<item><title>Undated item</title><link>https://example.com/undated</link></item>
</channel>
</rss>`

// arxivRSSBody is a minimal arXiv RSS doc the Paper-shaped adapter parses
// into one item; SummarizeNews-via-bundle-research must surface its title.
const arxivRSSBody = `<?xml version='1.0' encoding='UTF-8'?>
<rss xmlns:arxiv="http://arxiv.org/schemas/atom" xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0">
<channel>
<title>cs.AI updates on arXiv.org</title>
<item>
<title>Scaling laws for transformer attention</title>
<link>https://arxiv.org/abs/2606.00001</link>
<description>Abstract: We study scaling behavior.</description>
<dc:creator>Alice Researcher</dc:creator>
<dc:identifier>oai:arXiv.org:2606.00001v1</dc:identifier>
<pubDate>Wed, 10 Jun 2026 00:00:00 +0000</pubDate>
</item>
</channel>
</rss>`

// atomReleaseBody is a minimal GitHub-releases Atom doc; the Releases adapter
// produces []Article so SummarizeNews-via-bundle-dev surfaces the title
// unchanged.
const atomReleaseBody = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<title>Recent releases</title>
<entry>
<id>tag:github.com,2008:Repository/1/v1.2.3</id>
<title>v1.2.3 - Bug fixes</title>
<link rel="alternate" href="https://github.com/test/repo/releases/tag/v1.2.3"/>
<updated>2026-06-10T12:00:00Z</updated>
</entry>
</feed>`

// TestRunNews_BundleResearch wires the arxiv adapter through --bundle research.
// The Paper-typed Fetch is bridged to the Article-typed digest pipeline.
func TestRunNews_BundleResearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(arxivRSSBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	prev := bundleURLOverride
	bundleURLOverride = srv.URL
	t.Cleanup(func() { bundleURLOverride = prev })

	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--bundle", "research"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "Scaling laws") {
		t.Errorf("research bundle missing arxiv title: %q", buf.String())
	}
}

// TestRunNews_BundleDev wires the releases adapter through --bundle dev.
func TestRunNews_BundleDev(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(atomReleaseBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	prev := bundleURLOverride
	bundleURLOverride = srv.URL
	t.Cleanup(func() { bundleURLOverride = prev })

	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--bundle", "dev"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "v1.2.3") {
		t.Errorf("dev bundle missing release title: %q", buf.String())
	}
}

// TestKnownBundles_ListsResearchAndDev guards the help text from drifting away
// from the registered bundle map — operator discoverability hinges on it.
func TestKnownBundles_ListsResearchAndDev(t *testing.T) {
	got := knownBundles()
	if !strings.Contains(got, "research") {
		t.Errorf("knownBundles missing 'research': %q", got)
	}
	if !strings.Contains(got, "dev") {
		t.Errorf("knownBundles missing 'dev': %q", got)
	}
}

// TestRunNews_SinceDropsZeroPubDate pins the policy that items with no parsable
// pubDate are filtered out when --since is set.
func TestRunNews_SinceDropsZeroPubDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssBodyMissingPubDate))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	cfg := []map[string]string{{"Name": "test", "URL": srv.URL}}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "feeds-news.json"), raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var buf bytes.Buffer
	if code := runNews(context.Background(), []string{"--since=2026-06-10T00:00:00Z"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	if !strings.Contains(buf.String(), "Dated item") {
		t.Errorf("dated item missing: %q", buf.String())
	}
	if strings.Contains(buf.String(), "Undated item") {
		t.Errorf("undated item should be dropped under cursor: %q", buf.String())
	}
}
