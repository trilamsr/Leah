package main

import (
	"os"
	"strings"
	"testing"
)

// TestNewsBundles_AnthropicURL_StableOrTracked locks the FeedBurner URL until a
// concrete revisit signal lands. The source comment must carry a verification
// date so a future agent can mechanically decide whether to re-probe native RSS.
func TestNewsBundles_AnthropicURL_StableOrTracked(t *testing.T) {
	const wantURL = "https://feeds.feedburner.com/anthropic"
	var got string
	for _, s := range newsBundles["ai"] {
		if s.Name == "anthropic" {
			got = s.URL
		}
	}
	if got != wantURL {
		t.Fatalf("anthropic URL = %q, want %q", got, wantURL)
	}

	src, err := os.ReadFile("news.go")
	if err != nil {
		t.Fatalf("read news.go: %v", err)
	}
	body := string(src)
	if strings.Contains(body, "pending Linear issue") {
		t.Errorf("news.go still carries vague 'pending Linear issue' TODO; comment must name a concrete revisit signal")
	}
	if !strings.Contains(body, "anthropic.com/news/rss.xml") {
		t.Errorf("news.go anthropic comment must reference the native RSS path so a future agent can re-probe")
	}
	if !strings.Contains(body, "2026-06-21") {
		t.Errorf("news.go anthropic comment must carry the verification date")
	}
}
