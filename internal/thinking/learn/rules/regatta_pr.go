// Package rules holds per-action-kind world probes for the selflearn
// resolver (spec §2.1).
package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/thinking/learn"
)

// RegattaPR probes `gh pr view` for a ship-kind audit row whose Detail
// is a https://github.com/<owner>/<repo>/pull/<N> URL.
type RegattaPR struct {
	Now func() time.Time
	// GH lets tests stub the gh invocation. Default shells out to gh.
	GH func(ctx context.Context, repo string, prNum int) (state string, mergedAt string, err error)
}

var prURL = regexp.MustCompile(`https?://github\.com/([^/]+/[^/]+)/pull/(\d+)`)

// Resolve returns success if PR is MERGED; failed if CLOSED w/o merge;
// pending if OPEN <7d (waiting); unknown for OPEN ≥7d or unparseable.
func (r RegattaPR) Resolve(ctx context.Context, e audit.Entry) (learn.RetroOutcome, string) {
	if r.Now == nil {
		r.Now = time.Now
	}
	m := prURL.FindStringSubmatch(e.Detail)
	if m == nil {
		return learn.RetroUnknown, "no-pr-url-in-detail"
	}
	repo := m[1]
	prNum, err := strconv.Atoi(m[2])
	if err != nil {
		return learn.RetroUnknown, "bad-pr-number"
	}

	probe := r.GH
	if probe == nil {
		probe = defaultGH
	}
	state, mergedAt, err := probe(ctx, repo, prNum)
	if err != nil {
		return learn.RetroUnknown, fmt.Sprintf("gh-error:%v", err)
	}
	switch state {
	case "MERGED":
		return learn.RetroSuccess, "state=MERGED"
	case "CLOSED":
		if mergedAt == "" {
			return learn.RetroFailed, "state=CLOSED mergedAt=nil"
		}
		return learn.RetroSuccess, "state=CLOSED mergedAt=" + mergedAt
	case "OPEN":
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			return learn.RetroPending, "state=OPEN ts-unparseable"
		}
		age := r.Now().Sub(ts)
		if age < 7*24*time.Hour {
			return learn.RetroPending, fmt.Sprintf("state=OPEN age=%s", age.Round(time.Hour))
		}
		return learn.RetroUnknown, fmt.Sprintf("state=OPEN age=%s -> unknown", age.Round(time.Hour))
	}
	return learn.RetroUnknown, "state=" + state
}

func defaultGH(ctx context.Context, repo string, prNum int) (string, string, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(prNum), "--repo", repo, "--json", "state,mergedAt")
	out, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	var v struct {
		State    string `json:"state"`
		MergedAt string `json:"mergedAt"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", "", err
	}
	return v.State, v.MergedAt, nil
}
