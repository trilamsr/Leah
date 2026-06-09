// Package regattaclient is the read-only wrapper for `regatta agents list`.
// Used by both the per-30s daemon poll and `leah ship`'s watcher. Shells out
// because regatta is a sibling Go binary already on operator's $PATH.
package regattaclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Executor abstracts the shell hop for tests.
type Executor interface {
	Run(ctx context.Context, args []string) (string, error)
}

// ShellExec is the production Executor running the regatta binary.
type ShellExec struct{}

// Run executes args[0] with args[1:], surfacing ExitError.Stderr on failure.
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

// Client is the regatta CLI wrapper. Exec defaults to ShellExec.
type Client struct {
	Exec Executor
}

// New returns a Client wired to the regatta binary on $PATH.
func New() *Client {
	return &Client{Exec: ShellExec{}}
}

// Agent is the JSON shape regatta emits per row of `agents list --json`.
// State is the lifecycle ("pending", "running", "merged", "escalated",
// "failed", "stuck", "killed"); daemonloop.isTerminal flags the last five.
type Agent struct {
	ID     string `json:"id"`
	Branch string `json:"branch"`
	State  string `json:"state"`
	PR     int    `json:"pr"`
}

// List runs `regatta agents list --json` and parses the result. Returns the
// JSON parse error verbatim so a regatta schema bump surfaces loudly.
func (c *Client) List(ctx context.Context) ([]Agent, error) {
	out, err := c.Exec.Run(ctx, []string{"regatta", "agents", "list", "--json"})
	if err != nil {
		return nil, fmt.Errorf("regatta agents list: %w", err)
	}
	var agents []Agent
	if err := json.Unmarshal([]byte(out), &agents); err != nil {
		return nil, fmt.Errorf("parse agents json: %w", err)
	}
	return agents, nil
}
