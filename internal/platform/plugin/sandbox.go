package plugin

import (
	"context"
	"errors"
	"os/exec"
)

// DefaultRSSCapMB is the hard RSS ceiling enforced on every plugin subprocess per spec §7.4 / §7.9.
const DefaultRSSCapMB = 256

// ErrRSSCapInvalid is returned by Sandbox.Spawn when the caller asked for a non-positive cap.
var ErrRSSCapInvalid = errors.New("sandbox: rssCapMB must be > 0")

// Sandbox launches a plugin binary under platform-appropriate isolation.
type Sandbox interface {
	Spawn(ctx context.Context, bundlePath string, env []string, rssCapMB int) (*exec.Cmd, error)
}

// TaskPolicy is the injection point for the supervisor's hard RSS / CPU caps (T17 wires the real impl).
type TaskPolicy interface {
	ApplyRSSCap(pid int, rssCapMB int) error
}

// NewSandbox returns the platform-specific sandbox: darwin uses posix_spawn + RLIMIT_AS, others fall back to exec for tests.
func NewSandbox(policy TaskPolicy) Sandbox { return newSandbox(policy) }

// noopPolicy is the zero-value when no supervisor is wired yet — RLIMIT_AS still applies on darwin, just no extra task-policy.
type noopPolicy struct{}

func (noopPolicy) ApplyRSSCap(int, int) error { return nil }

// resolveBinary returns the plugin binary path inside the bundle per spec §7.2.
func resolveBinary(bundlePath string) string { return bundlePath + "/Contents/binary" }
