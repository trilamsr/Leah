// Context-fetcher helpers for leah ship: PR / issue / shell-history blocks
// pre-pended to the Reasoner draft prompt when the operator passes
// --from-pr / --from-issue / --from-thread. Kept package-local + executor-
// abstracted so tests never shell out to gh.
package dispatcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CmdExecutor matches ghclient.Executor without an import cycle — same shape:
// run a command, get stdout (or wrapped error). Allows shipctx_test to
// substitute a fixture without touching the real gh binary.
type CmdExecutor interface {
	Run(ctx context.Context, args []string) (string, error)
}

// prDiffMaxLines caps the diff slice we prepend. 500 lines covers most
// review-ready diffs without blowing the Reasoner prompt budget.
const prDiffMaxLines = 500

// FetchPRContext returns a markdown block describing PR #n (title, body,
// headRefName, first prDiffMaxLines lines of diff). Empty string + nil on
// graceful skip (gh failure) so a missing PR doesn't fail the whole ship.
func FetchPRContext(ctx context.Context, exec CmdExecutor, repo string, n int) (string, error) {
	viewOut, err := exec.Run(ctx, []string{"gh", "pr", "view", strconv.Itoa(n),
		"--repo", repo,
		"--json", "title,body,headRefName",
	})
	if err != nil {
		return "", fmt.Errorf("gh pr view #%d: %w", n, err)
	}
	var pr struct {
		Title       string `json:"title"`
		Body        string `json:"body"`
		HeadRefName string `json:"headRefName"`
	}
	if err := json.Unmarshal([]byte(viewOut), &pr); err != nil {
		return "", fmt.Errorf("parse pr json: %w", err)
	}

	diffOut, err := exec.Run(ctx, []string{"gh", "pr", "diff", strconv.Itoa(n), "--repo", repo})
	if err != nil {
		// Non-fatal: title/body/branch still useful.
		diffOut = "(diff fetch failed: " + err.Error() + ")"
	}
	diff := truncateLines(diffOut, prDiffMaxLines)

	var b strings.Builder
	fmt.Fprintf(&b, "## Context from PR #%d (%s)\n\n", n, repo)
	fmt.Fprintf(&b, "**Title:** %s\n\n", pr.Title)
	fmt.Fprintf(&b, "**Branch:** `%s`\n\n", pr.HeadRefName)
	fmt.Fprintf(&b, "**Body:**\n\n%s\n\n", pr.Body)
	fmt.Fprintf(&b, "**Diff (first %d lines):**\n\n```diff\n%s\n```\n", prDiffMaxLines, diff)
	return b.String(), nil
}

// FetchIssueContext returns a markdown block: title, body, comments.
func FetchIssueContext(ctx context.Context, exec CmdExecutor, repo string, n int) (string, error) {
	out, err := exec.Run(ctx, []string{"gh", "issue", "view", strconv.Itoa(n),
		"--repo", repo,
		"--json", "title,body,comments",
	})
	if err != nil {
		return "", fmt.Errorf("gh issue view #%d: %w", n, err)
	}
	var iss struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Comments []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body string `json:"body"`
		} `json:"comments"`
	}
	if err := json.Unmarshal([]byte(out), &iss); err != nil {
		return "", fmt.Errorf("parse issue json: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Context from issue #%d (%s)\n\n", n, repo)
	fmt.Fprintf(&b, "**Title:** %s\n\n", iss.Title)
	fmt.Fprintf(&b, "**Body:**\n\n%s\n\n", iss.Body)
	if len(iss.Comments) > 0 {
		fmt.Fprintf(&b, "**Comments (%d):**\n\n", len(iss.Comments))
		for _, c := range iss.Comments {
			fmt.Fprintf(&b, "- @%s: %s\n", c.Author.Login, strings.ReplaceAll(c.Body, "\n", " "))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// FetchThreadContext reads the last-N entries of the operator's shell
// history file. `window` accepts either `<N>c` (last N commands) or
// `<N>m` (last N minutes — requires HISTTIMEFORMAT/extended_history; falls
// back to last-N lines when timestamps are missing). histPath should point
// at ~/.zsh_history or ~/.bash_history; an empty path skips.
func FetchThreadContext(window string, histPath string) (string, error) {
	if histPath == "" {
		return "", nil
	}
	f, err := os.Open(histPath)
	if err != nil {
		return "", fmt.Errorf("open history %s: %w", histPath, err)
	}
	defer func() { _ = f.Close() }()

	var lines []string
	sc := bufio.NewScanner(f)
	// Some shells store huge multi-line commands; bump buffer.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("scan history: %w", err)
	}

	n, suffix, ok := splitWindow(window)
	if !ok || n <= 0 {
		return "", fmt.Errorf("invalid --from-thread window %q (want e.g. 30m or 100c)", window)
	}
	// Minute-mode falls back to last-N-line slice when timestamps absent;
	// keeps the surface simple — the caller wanted "some recent thread".
	// Heuristic: treat 30m ≈ last 60 commands; 60m ≈ last 120.
	if suffix == "m" {
		n = n * 2
	}
	if n > len(lines) {
		n = len(lines)
	}
	tail := lines[len(lines)-n:]

	var b strings.Builder
	fmt.Fprintf(&b, "## Context from terminal thread (last %d %s)\n\n", n, longSuffix(suffix))
	b.WriteString("```\n")
	for _, ln := range tail {
		b.WriteString(stripZshHistTimestamp(ln))
		b.WriteString("\n")
	}
	b.WriteString("```\n")
	return b.String(), nil
}

// ComposeContext concatenates non-empty context blocks in PR → issue →
// thread order with a blank line separator, returning the leading text to
// prepend before the operator's `Intent:` line.
func ComposeContext(prCtx, issueCtx, threadCtx string) string {
	parts := []string{}
	for _, c := range []string{prCtx, issueCtx, threadCtx} {
		if strings.TrimSpace(c) != "" {
			parts = append(parts, strings.TrimRight(c, "\n"))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n\n"
}

func truncateLines(s string, max int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[:max], "\n") + "\n... (truncated)"
}

func splitWindow(w string) (n int, suffix string, ok bool) {
	w = strings.TrimSpace(w)
	if len(w) < 2 {
		return 0, "", false
	}
	suf := w[len(w)-1:]
	if suf != "c" && suf != "m" {
		return 0, "", false
	}
	v, err := strconv.Atoi(w[:len(w)-1])
	if err != nil {
		return 0, "", false
	}
	return v, suf, true
}

func longSuffix(s string) string {
	switch s {
	case "c":
		return "commands"
	case "m":
		return "minutes (line-count approx)"
	}
	return s
}

// stripZshHistTimestamp drops the `: <ts>:0;` prefix zsh's extended_history
// option adds. Leaves bash history lines untouched.
func stripZshHistTimestamp(line string) string {
	if !strings.HasPrefix(line, ": ") {
		return line
	}
	idx := strings.Index(line, ";")
	if idx < 0 {
		return line
	}
	return line[idx+1:]
}
