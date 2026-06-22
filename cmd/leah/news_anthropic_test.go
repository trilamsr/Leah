package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNewsBundles_AnthropicURL_StableOrTracked locks the FeedBurner URL + comment revisit signal.
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

	// Resolve news.go relative to this test file so assertions hold regardless
	// of which directory `go test` was invoked from.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	newsPath := filepath.Join(filepath.Dir(thisFile), "news.go")
	src, err := os.ReadFile(newsPath)
	if err != nil {
		t.Fatalf("read %s: %v", newsPath, err)
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
