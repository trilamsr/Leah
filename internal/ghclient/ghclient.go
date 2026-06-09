package ghclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Executor interface {
	Run(ctx context.Context, args []string) (stdout string, err error)
}

type ShellExec struct{}

func (ShellExec) Run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %s", args[0], string(ee.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

type Client struct {
	Exec Executor
}

func New() *Client {
	return &Client{Exec: ShellExec{}}
}

type CreateIssueArgs struct {
	Repo     string
	Title    string
	BodyFile string
	Labels   []string
}

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

func (c *Client) ViewPR(ctx context.Context, repo string, num int, fields []string) (map[string]any, error) {
	args := []string{"gh", "pr", "view", fmt.Sprintf("%d", num),
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
