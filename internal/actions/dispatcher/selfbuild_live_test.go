//go:build live_gh

// Live-gh matrix lane: exercises Ship.Run's `ready-for-agent` label-missing
// retry path against the real `gh` CLI + a throwaway sandbox repo. Skipped
// from normal CI by the build tag; the workflow lane sets `-tags=live_gh`
// only when the operator opts in (PR marker `[live-gh]` or nightly cron).
//
// Prerequisites the workflow must satisfy before the test sees a green path:
//   - `gh` on $PATH, authenticated via $GH_TOKEN
//   - Sandbox repo (default `trilamsr/leah-ci-sandbox`) writable by the token
//   - LEAH_LIVE_GH=1 (extra belt-and-suspenders gate beyond the build tag)
//
// Without those, t.Skip fires — the test never falsely fails on missing setup.
package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/budget"
	"github.com/trilam/leah/internal/actions/ghclient"
)

const (
	defaultLiveSandboxRepo = "trilamsr/leah-ci-sandbox"
	liveLabel              = "ready-for-agent"
)

// TestShipRun_LiveGH_RetryOnInvalidLabel proves Ship.Run's Defect-1 retry
// path works against the real gh CLI: pre-flight deletes the
// `ready-for-agent` label so the first CreateIssue fails with the live
// "label not found" surface; the retry calls EnsureLabel + re-creates and
// the issue lands. Created issues + the label are torn down on completion.
func TestShipRun_LiveGH_RetryOnInvalidLabel(t *testing.T) {
	if os.Getenv("LEAH_LIVE_GH") != "1" {
		t.Skip("LEAH_LIVE_GH not set — opt-in lane")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not on PATH")
	}

	repo := os.Getenv("LEAH_LIVE_GH_REPO")
	if repo == "" {
		repo = defaultLiveSandboxRepo
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Pre-flight: ensure the label does NOT exist on the sandbox repo so the
	// first CreateIssue fails with the exact surface isMissingLabelErr matches.
	// Ignore the error — "not found" is the desired post-condition either way.
	_ = exec.CommandContext(ctx, "gh", "label", "delete", liveLabel,
		"--repo", repo, "--yes").Run()

	dir := t.TempDir()
	gh := ghclient.New()
	sb := &Ship{
		Reasoner: &fakeShipReasoner{resp: "## Context\n\nlive-gh smoke\n\n## What to do\n\n- nothing\n\n## Acceptance\n\n- issue exists\n"},
		GH:       gh,
		Audit:    &audit.Logger{Path: dir + "/audit.jsonl"},
		Budget:   &budget.Budget{Ceiling: 5.0},
		Out:      &bytes.Buffer{},
		Repo:     repo,
		Title:    "[live-gh] ship.Run retry smoke",
		TmpDir:   dir,
	}

	runErr := sb.Run(ctx, "live-gh retry smoke")

	// Always attempt cleanup, regardless of test outcome.
	t.Cleanup(func() {
		closeLiveIssues(t, repo, "[live-gh] ship.Run retry smoke")
		// Leave the label installed: a successful retry path re-created it
		// and subsequent runs delete it again on entry. Deleting here too
		// would just mean the next run has nothing to delete — no functional
		// difference. Skip to keep teardown minimal.
	})

	if runErr != nil {
		t.Fatalf("Ship.Run against live gh: %v", runErr)
	}
	if sb.LastURL == "" || !strings.HasPrefix(sb.LastURL, "https://github.com/"+repo+"/issues/") {
		t.Fatalf("LastURL = %q, want gh-issue URL on %s", sb.LastURL, repo)
	}
}

// closeLiveIssues closes every open issue on repo whose title matches title.
// Best-effort: failures are logged but do not fail the test (the sandbox
// repo can accumulate orphans without affecting correctness).
func closeLiveIssues(t *testing.T, repo, title string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "gh", "issue", "list",
		"--repo", repo,
		"--state", "open",
		"--search", title,
		"--json", "number,title",
		"-L", "20",
	).Output()
	if err != nil {
		t.Logf("cleanup: gh issue list: %v", err)
		return
	}
	var issues []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		t.Logf("cleanup: parse list: %v", err)
		return
	}
	for _, is := range issues {
		if is.Title != title {
			continue
		}
		if err := exec.CommandContext(ctx, "gh", "issue", "close",
			strconv.Itoa(is.Number), "--repo", repo).Run(); err != nil {
			t.Logf("cleanup: close #%d: %v", is.Number, err)
		}
	}
}
