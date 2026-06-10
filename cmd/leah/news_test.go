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
