package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/trilam/leah/internal/ghclient"
)

// defaultPRStateRepo is the same self-build target backlog uses — the operator
// reads pr-state most often against the Leah repo. Override with --repo.
const defaultPRStateRepo = "trilamsr/Leah"

// prSummary is the projected shape behind every one-line render. Only the
// signal an operator can act on — body, comments, reactions stay out.
type prSummary struct {
	Number           int
	Title            string
	State            string
	MergeStateStatus string
	ReviewDecision   string
	ChecksConclusion string
	Author           string
	CreatedAt        string
	IsDraft          bool
	Mergeable        string
}

// prStateFields is the allowlist for gh pr view --json. Per CLAUDE.md
// gh-minimal-fields, requesting all fields blows token budget on body/comments.
var prStateFields = []string{
	"number", "title", "state", "mergeStateStatus", "reviewDecision",
	"statusCheckRollup", "author", "createdAt", "isDraft", "mergeable",
}

// runPRState dispatches the three usage shapes: single PR, --open list,
// --queue ranked. The executor seam exists so the verb is testable without
// the gh binary; production injects ghclient.ShellExec.
func runPRState(parent context.Context, exec ghclient.Executor, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(w, "usage: leah pr-state <N> | --open | --queue [--repo <owner/name>]")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "Aggregate one-line PR readiness signal (state, CI, review, mergeability).")
		return 0
	}
	if exec == nil {
		exec = ghclient.ShellExec{}
	}
	repo := defaultPRStateRepo
	mode := "" // "", "open", "queue"
	prNum := 0
	prNumSeen := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--open":
			mode = "open"
		case a == "--queue":
			mode = "queue"
		case a == "--repo":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(os.Stderr, "leah pr-state: --repo needs a value")
				return 2
			}
			repo = args[i+1]
			i++
		case strings.HasPrefix(a, "--"):
			_, _ = fmt.Fprintf(os.Stderr, "leah pr-state: unknown flag %q\n", a)
			return 2
		default:
			n, err := strconv.Atoi(a)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "leah pr-state: %q is not a PR number\n", a)
				return 2
			}
			// Silently overwriting the prior number would let
			// `leah pr-state 1 2` look like it queried #1.
			if prNumSeen {
				_, _ = fmt.Fprintln(os.Stderr, "leah pr-state: only one PR number accepted")
				return 2
			}
			prNum = n
			prNumSeen = true
		}
	}
	// Positional PR + mode flag are mutually exclusive — the silent-drop
	// shape `leah pr-state 1 --open` would hide the operator's intent.
	if mode != "" && prNumSeen {
		_, _ = fmt.Fprintln(os.Stderr, "leah pr-state: <N> and --open/--queue are mutually exclusive")
		return 2
	}

	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	now := time.Now()
	switch mode {
	case "open":
		return runPRStateList(ctx, exec, repo, false, now, w)
	case "queue":
		return runPRStateList(ctx, exec, repo, true, now, w)
	default:
		if prNum == 0 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah pr-state <N> | --open | --queue")
			return 2
		}
		return runPRStateOne(ctx, exec, repo, prNum, now, w)
	}
}

func runPRStateOne(ctx context.Context, exec ghclient.Executor, repo string, num int, now time.Time, w io.Writer) int {
	pr, err := fetchPRDetail(ctx, exec, repo, num)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah pr-state: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(w, formatPRLine(pr, now))
	return 0
}

func runPRStateList(ctx context.Context, exec ghclient.Executor, repo string, rank bool, now time.Time, w io.Writer) int {
	prs, err := fetchOpenPRsByMe(ctx, exec, repo)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah pr-state: %v\n", err)
		return 1
	}
	if rank {
		rankQueue(prs)
	}
	for _, pr := range prs {
		_, _ = fmt.Fprintln(w, formatPRLine(pr, now))
	}
	return 0
}

// fetchPRDetail wraps gh pr view with the field allowlist and flattens the
// statusCheckRollup into a single conclusion. Rolled-up because the operator
// reads "is CI green" not "which of 7 contexts is red" at the one-line layer;
// they can `gh pr checks <N>` for the breakdown.
func fetchPRDetail(ctx context.Context, exec ghclient.Executor, repo string, num int) (prSummary, error) {
	args := []string{"gh", "pr", "view", strconv.Itoa(num)}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	args = append(args, "--json", strings.Join(prStateFields, ","))
	out, err := exec.Run(ctx, args)
	if err != nil {
		return prSummary{}, fmt.Errorf("gh pr view: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return prSummary{}, fmt.Errorf("parse pr view json: %w", err)
	}
	return projectPR(raw), nil
}

func fetchOpenPRsByMe(ctx context.Context, exec ghclient.Executor, repo string) ([]prSummary, error) {
	args := []string{"gh", "pr", "list"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	args = append(args,
		"--state", "open",
		"--author", "@me",
		"--json", strings.Join(prStateFields, ","),
		"-L", "30",
	)
	out, err := exec.Run(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		return nil, fmt.Errorf("parse pr list json: %w", err)
	}
	prs := make([]prSummary, 0, len(arr))
	for _, m := range arr {
		prs = append(prs, projectPR(m))
	}
	return prs, nil
}

// projectPR centralises the raw-map -> prSummary projection so view + list
// stay in lockstep. Defensive type assertions because gh's JSON projection
// returns null for missing fields, which Go decodes as nil interface.
func projectPR(m map[string]any) prSummary {
	pr := prSummary{}
	if v, ok := m["number"].(float64); ok {
		pr.Number = int(v)
	}
	pr.Title, _ = m["title"].(string)
	pr.State, _ = m["state"].(string)
	pr.MergeStateStatus, _ = m["mergeStateStatus"].(string)
	pr.ReviewDecision, _ = m["reviewDecision"].(string)
	pr.CreatedAt, _ = m["createdAt"].(string)
	pr.IsDraft, _ = m["isDraft"].(bool)
	pr.Mergeable, _ = m["mergeable"].(string)
	if author, ok := m["author"].(map[string]any); ok {
		pr.Author, _ = author["login"].(string)
	}
	pr.ChecksConclusion = rollupChecks(m["statusCheckRollup"])
	return pr
}

// rollupChecks collapses the per-context check array into one of
// SUCCESS / FAILURE / PENDING / "". One failure trumps; one empty conclusion
// (still running) downgrades the rest to PENDING.
func rollupChecks(v any) string {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return ""
	}
	pending := false
	for _, item := range arr {
		c, _ := item.(map[string]any)
		concl, _ := c["conclusion"].(string)
		switch strings.ToUpper(concl) {
		case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED":
			return "FAILURE"
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			// keep scanning
		case "":
			pending = true
		}
	}
	if pending {
		return "PENDING"
	}
	return "SUCCESS"
}

// formatPRLine renders one prSummary as a single line. Column order is
// load-bearing: number first (operator types it), then title (what), then
// the signal cluster (state, merge, review, CI), then author and age. The
// glyph for CI is intentional UTF-8 — three chars convey ✓/✗/⋯ unambiguously.
func formatPRLine(pr prSummary, now time.Time) string {
	glyph := "⋯"
	switch pr.ChecksConclusion {
	case "SUCCESS":
		glyph = "✓"
	case "FAILURE":
		glyph = "✗"
	}
	title := truncate(pr.Title, 50)
	state := pr.State
	if pr.IsDraft {
		state = "DRAFT"
	}
	merge := pr.MergeStateStatus
	if merge == "" {
		merge = "-"
	}
	review := pr.ReviewDecision
	if review == "" {
		review = "-"
	}
	return fmt.Sprintf("#%-4d %-50s %-7s %-8s %-8s %s  %-12s %s",
		pr.Number, title, state, merge, review, glyph, pr.Author, ageSince(pr.CreatedAt, now))
}

// rankQueue sorts in-place by readiness-to-merge: APPROVED + CLEAN + SUCCESS
// is best; any CI failure sinks; review state is the tiebreaker. Stable so
// equal-rank PRs preserve gh's age-desc ordering.
func rankQueue(prs []prSummary) {
	sort.SliceStable(prs, func(i, j int) bool {
		return readinessScore(prs[i]) > readinessScore(prs[j])
	})
}

func readinessScore(pr prSummary) int {
	score := 0
	switch pr.ReviewDecision {
	case "APPROVED":
		score += 100
	case "REVIEW_REQUIRED":
		score += 10
	case "CHANGES_REQUESTED":
		score -= 50
	}
	switch pr.MergeStateStatus {
	case "CLEAN":
		score += 50
	case "UNSTABLE":
		score += 20
	case "BLOCKED":
		score += 5
	case "DIRTY":
		score -= 30
	}
	switch pr.ChecksConclusion {
	case "SUCCESS":
		score += 30
	case "PENDING":
		score += 10
	case "FAILURE":
		score -= 300
	}
	if pr.IsDraft {
		score -= 200
	}
	return score
}
