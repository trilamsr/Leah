// Package notify pushes terminal-state alerts. Desktop = macOS osascript
// banner; Pushover = phone push via the Pushover HTTP API. Both satisfy the
// daemonloop.Notifier interface so callers can swap or fan-out.
package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Executor abstracts the shell hop so tests can stub osascript invocations.
type Executor interface {
	Run(ctx context.Context, args []string) (string, error)
}

// ShellExec is the production Executor for osascript.
type ShellExec struct{}

// Run invokes args[0] with args[1:], surfacing ExitError.Stderr in the error.
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

// Desktop is the macOS-native banner notifier (osascript). Other platforms
// will need an alternate Notifier; the daemonloop interface keeps the wiring
// pluggable.
type Desktop struct {
	Exec Executor
}

// NewDesktop returns a Desktop wired to ShellExec.
func NewDesktop() *Desktop {
	return &Desktop{Exec: ShellExec{}}
}

// Notify shells `osascript -e 'display notification ...'`. Quotes in
// title/body are escaped to prevent the AppleScript shell from breaking on
// regatta agent IDs or PR titles.
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
