package regattaclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type Executor interface {
	Run(ctx context.Context, args []string) (string, error)
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

type Agent struct {
	ID     string `json:"id"`
	Branch string `json:"branch"`
	State  string `json:"state"`
	PR     int    `json:"pr"`
}

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
