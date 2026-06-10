package selflearn

import (
	"regexp"
	"time"
)

// dangleAfter is the grace window before a dispatched self-build with no
// paired outcome is flagged. Aligned with the resolver's default Since so
// operators see a dangling row exactly once it falls out of the active
// reconciliation window.
const dangleAfter = 7 * 24 * time.Hour

// DanglingSelfBuild is a self-build dispatch whose paired
// kind=self-build.outcome row never landed within dangleAfter. Surfaced
// as a defect candidate to the weekly retro so operator-aborts and
// regatta-watcher stalls accumulate visibly instead of silently masking
// the WHERE clause `kind='self-build' AND outcome='dispatched'`.
type DanglingSelfBuild struct {
	IssueURL    string
	ArgsHash    string
	DispatchTS  string
	AgeDuration time.Duration
}

// selfbuildURL extracts the `url=<github URL>` token from a self-build
// audit row's Detail. The dispatcher writes both
// `url=https://github.com/.../issues/N` and
// `url=https://github.com/.../pull/N` shapes (issue first, PR after the
// regatta agent opens it) so we match either path segment.
var selfbuildURL = regexp.MustCompile(`url=(https?://github\.com/[^/]+/[^/]+/(?:issues|pull)/\d+)`)

// DetectDanglingSelfBuild scans the audit log at path for
// kind=self-build, outcome=dispatched rows older than dangleAfter whose
// extracted URL has no matching kind=self-build.outcome row. now is
// injected so tests get a deterministic window.
func DetectDanglingSelfBuild(path string, now func() time.Time) ([]DanglingSelfBuild, error) {
	if now == nil {
		now = time.Now
	}
	entries, err := readAudit(path)
	if err != nil {
		return nil, err
	}

	// URL → seen-an-outcome. A single terminal answer for a URL clears every
	// dispatched row pointing at it; the operator already knows the dispatch
	// landed because the URL is the same.
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
