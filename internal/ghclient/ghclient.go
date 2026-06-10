// Package ghclient wraps the `gh` CLI for the three calls Leah makes today:
// CreateIssue (ship/self-build), ViewPR (review), and ListPRsForBranch
// (future watcher work). Shells out instead of using the GitHub REST API so
// the operator's existing `gh auth login` token covers everything.
package ghclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Executor abstracts the shell hop so tests can replace gh with a fixture
// that returns canned JSON.
type Executor interface {
	Run(ctx context.Context, args []string) (stdout string, err error)
}

// ShellExec is the production Executor that runs the gh binary via os/exec.
type ShellExec struct{}

// Run executes args[0] with args[1:], surfacing ExitError.Stderr in the
// returned error so gh's "authentication required" / rate-limit messages
// reach the operator.
func (ShellExec) Run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("%s: %s", args[0], string(ee.Stderr))
		}
		return "", fmt.Errorf("%s: %w", args[0], err)
	}
	return string(out), nil
}

// Client is the gh wrapper. Exec defaults to ShellExec.
type Client struct {
	Exec Executor
}

// New returns a Client wired to the gh binary on $PATH.
func New() *Client {
	return &Client{Exec: ShellExec{}}
}

// CreateIssueArgs is the argument set for gh issue create. BodyFile is the
// preferred shape over inline body (avoids the shell-escape hazard for the
// triple-backtick release-notes fences regatta expects).
type CreateIssueArgs struct {
	Repo     string
	Title    string
	BodyFile string
	Labels   []string
}

// CreateIssue files an issue + returns the trimmed URL printed by gh.
// Callers (Ship, SelfBuild) prepend the `ready-for-agent` label so regatta
// picks the issue up on its next poll.
func (c *Client) CreateIssue(ctx context.Context, a CreateIssueArgs) (url string, err error) {
	args := []string{"gh", "issue", "create",
		"--repo", a.Repo,
		"--title", a.Title,
		"--body-file", a.BodyFile,
	}
	for _, l := range a.Labels {
		args = append(args, "--label", l)
	}
	out, err := c.Exec.Run(ctx, args)
	if err != nil {
		return "", fmt.Errorf("gh issue create: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// EnsureLabel idempotently creates `label` on `repo`. The `gh label create
// --force` flag updates the label in place when it already exists, so this
// is safe to invoke unconditionally as part of the CreateIssue retry path.
// Color #00FF00 + description match the Defect-1 fix spec; callers that
// want different cosmetics should add a new method rather than parameterize
// (the ready-for-agent label is the only one Leah dispatches today).
func (c *Client) EnsureLabel(ctx context.Context, repo, label string) error {
	args := []string{"gh", "label", "create", label,
		"--repo", repo,
		"--color", "00FF00",
		"--description", "Leah-dispatched: regatta picks up",
		"--force",
	}
	if _, err := c.Exec.Run(ctx, args); err != nil {
		// gh prints "already exists" only when --force is omitted; the
		// --force path above never surfaces it, but defend in case a future
		// gh release changes that behavior.
		if strings.Contains(err.Error(), "already exists") {
			return nil
		}
		return fmt.Errorf("gh label create: %w", err)
	}
	return nil
}

// ViewPR returns the gh-projected JSON for one PR. fields MUST be an
// explicit allowlist (per CLAUDE.md gh-minimal-fields rule) — passing all
// fields blows token budget on PR body / comments.
func (c *Client) ViewPR(ctx context.Context, repo string, num int, fields []string) (map[string]any, error) {
	args := []string{"gh", "pr", "view", strconv.Itoa(num),
		"--repo", repo,
		"--json", strings.Join(fields, ","),
	}
	out, err := c.Exec.Run(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("gh pr view: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		return nil, fmt.Errorf("parse pr json: %w", err)
	}
	return m, nil
}

// ListPRsForBranch returns up to 20 PRs whose head ref matches branch.
// Used by the watcher to correlate a regatta agent branch back to its PR.
func (c *Client) ListPRsForBranch(ctx context.Context, repo, branch string, fields []string) ([]map[string]any, error) {
	args := []string{"gh", "pr", "list",
		"--repo", repo,
		"--head", branch,
		"--json", strings.Join(fields, ","),
		"-L", "20",
	}
	out, err := c.Exec.Run(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		return nil, fmt.Errorf("parse pr list: %w", err)
	}
	return arr, nil
}
