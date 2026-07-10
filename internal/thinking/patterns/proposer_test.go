package patterns

import (
	"strings"
	"testing"
	"time"
)

// TestProposeMarkdownFormat asserts the per-cluster 3-line block + header.
func TestProposeMarkdownFormat(t *testing.T) {
	clusters := []Cluster{{
		Kind:       "ask",
		HashPrefix: "deadbeef",
		Count:      7,
		Samples:    []string{"how does X work", "explain Y", "what is Z"},
	}}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	got := ProposeAt(clusters, now)

	for _, want := range []string{
		"# Skill candidates",
		"window 2026-05-10..2026-06-09",
		"- **ask** × 7 (last 30d) — hash deadbeef…",
		"samples: how does X work | explain Y | what is Z",
		"`leah skill approve ask-deadbeef`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

// TestProposeEmptyClusters asserts header-only output when nothing meets threshold.
func TestProposeEmptyClusters(t *testing.T) {
	got := ProposeAt(nil, time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(got, "# Skill candidates") {
		t.Errorf("missing header: %s", got)
	}
	if !strings.Contains(got, "_No clusters at or above threshold this window._") {
		t.Errorf("missing empty marker: %s", got)
	}
}

// TestProposeTruncatesAt200 asserts file-size bound (spec §3 lifecycle).
func TestProposeTruncatesAt200(t *testing.T) {
	clusters := make([]Cluster, MaxClustersInFile+5)
	for i := range clusters {
		clusters[i] = Cluster{Kind: "ask", HashPrefix: "aa", Count: 5}
	}
	got := ProposeAt(clusters, time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(got, "# truncated: 5 more clusters") {
		t.Errorf("missing truncation footer:\n%s", got[len(got)-200:])
	}
}
