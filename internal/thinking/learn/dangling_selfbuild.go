package learn

import (
	"regexp"
	"time"
)

// dangleAfter mirrors the resolver's default Since so a row surfaces exactly when it leaves the active reconciliation window.
const dangleAfter = 7 * 24 * time.Hour

// DanglingSelfBuild surfaces operator-aborts and regatta-watcher stalls that would otherwise silently fail the dispatched→outcome pairing.
type DanglingSelfBuild struct {
	IssueURL    string
	ArgsHash    string
	DispatchTS  string
	AgeDuration time.Duration
}

// selfbuildURL matches both `issues/N` and `pull/N` because dispatch writes the issue URL and the regatta agent later writes the PR URL.
var selfbuildURL = regexp.MustCompile(`url=(https?://github\.com/[^/]+/[^/]+/(?:issues|pull)/\d+)`)

// DetectDanglingSelfBuild takes injectable now so tests get a deterministic cutoff.
func DetectDanglingSelfBuild(path string, now func() time.Time) ([]DanglingSelfBuild, error) {
	if now == nil {
		now = time.Now
	}
	entries, err := readAudit(path)
	if err != nil {
		return nil, err
	}

	outcomeURL := map[string]bool{}
	for _, e := range entries {
		if e.Kind != "self-build.outcome" {
			continue
		}
		if url := extractSelfBuildURL(e.Detail); url != "" {
			outcomeURL[url] = true
		}
	}

	cutoff := now().Add(-dangleAfter)
	var out []DanglingSelfBuild
	for _, e := range entries {
		if e.Kind != "self-build" || e.Outcome != "dispatched" {
			continue
		}
		url := extractSelfBuildURL(e.Detail)
		if url == "" {
			continue
		}
		if outcomeURL[url] {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil || ts.After(cutoff) {
			continue
		}
		out = append(out, DanglingSelfBuild{
			IssueURL:    url,
			ArgsHash:    e.ArgsHash,
			DispatchTS:  e.Timestamp,
			AgeDuration: now().Sub(ts),
		})
	}
	return out, nil
}

func extractSelfBuildURL(detail string) string {
	m := selfbuildURL.FindStringSubmatch(detail)
	if m == nil {
		return ""
	}
	return m[1]
}
