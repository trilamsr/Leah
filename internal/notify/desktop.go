package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
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

type Desktop struct {
	Exec Executor
}

func NewDesktop() *Desktop {
	return &Desktop{Exec: ShellExec{}}
}

func (d *Desktop) Notify(ctx context.Context, title, body string) error {
	t := strings.ReplaceAll(title, `"`, `\"`)
	b := strings.ReplaceAll(body, `"`, `\"`)
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, b, t)
	_, err := d.Exec.Run(ctx, []string{"osascript", "-e", script})
	if err != nil {
		return fmt.Errorf("osascript: %w", err)
	}
	return nil
}
