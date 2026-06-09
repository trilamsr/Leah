package patterns

import (
	"fmt"
	"strings"
	"time"
)

// MaxClustersInFile caps proposer output so an unread file can't grow without
// bound (spec §3 lifecycle).
const MaxClustersInFile = 200

// Propose renders clusters as the operator-facing markdown emitted to
// ~/.leah-state/skill-candidates.md. Format spec §3.
func Propose(clusters []Cluster) string {
	return ProposeAt(clusters, time.Now().UTC())
}

// ProposeAt is Propose with an injectable clock for deterministic tests.
func ProposeAt(clusters []Cluster, now time.Time) string {
	var sb strings.Builder
	since := now.Add(-30 * 24 * time.Hour)
	fmt.Fprintf(&sb, "# Skill candidates — generated %s, window %s..%s\n\n",
		now.Format(time.RFC3339),
		since.Format("2006-01-02"),
		now.Format("2006-01-02"),
	)
	sb.WriteString("Cherry-pick entries below. Approval CLI lands in V2 (spec §6 deferred).\n")
	sb.WriteString("Stale entries (no event in window) drop automatically on the next run.\n\n")

	if len(clusters) == 0 {
		sb.WriteString("_No clusters at or above threshold this window._\n")
		return sb.String()
	}

	truncated := 0
	if len(clusters) > MaxClustersInFile {
		truncated = len(clusters) - MaxClustersInFile
		clusters = clusters[:MaxClustersInFile]
	}

	for _, c := range clusters {
		samples := "(no detail samples)"
		if len(c.Samples) > 0 {
			samples = strings.Join(c.Samples, " | ")
		}
		fmt.Fprintf(&sb, "- **%s** × %d (last 30d) — hash %s…\n", c.Kind, c.Count, c.HashPrefix)
		fmt.Fprintf(&sb, "  samples: %s\n", samples)
		fmt.Fprintf(&sb, "  proposal: template this? `leah skill approve %s-%s` to accept, ignore to dismiss.\n",
			c.Kind, c.HashPrefix)
	}

	if truncated > 0 {
		fmt.Fprintf(&sb, "\n# truncated: %d more clusters below threshold-%d\n", truncated, MaxClustersInFile)
	}
	return sb.String()
}
