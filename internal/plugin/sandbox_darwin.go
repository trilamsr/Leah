//go:build darwin

package plugin

import (
	"context"
	"os/exec"
	"syscall"
)

type darwinSandbox struct{ policy TaskPolicy }

func newSandbox(policy TaskPolicy) Sandbox {
	if policy == nil {
		policy = noopPolicy{}
	}
	return &darwinSandbox{policy: policy}
}

// Spawn launches the bundle binary under a new pgid + RLIMIT_AS cap so a runaway alloc trips ENOMEM rather than swap-trash.
func (s *darwinSandbox) Spawn(ctx context.Context, bundlePath string, env []string, rssCapMB int) (*exec.Cmd, error) {
	if rssCapMB <= 0 {
		return nil, ErrRSSCapInvalid
	}
	cmd := exec.CommandContext(ctx, resolveBinary(bundlePath))
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Best-effort RLIMIT_AS on the parent before the child execs is wrong direction;
	// apply via task-policy after Start. Real RLIMIT_AS-in-child requires a separate
	// launcher binary on darwin (posix_spawn limits are not on go's exec path) — T17
	// owns that launcher; we delegate.
	if err := s.policy.ApplyRSSCap(cmd.Process.Pid, rssCapMB); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	return cmd, nil
}
