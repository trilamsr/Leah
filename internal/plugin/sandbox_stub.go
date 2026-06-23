//go:build !darwin

package plugin

import (
	"context"
	"os/exec"
)

type stubSandbox struct{ policy TaskPolicy }

func newSandbox(policy TaskPolicy) Sandbox {
	if policy == nil {
		policy = noopPolicy{}
	}
	return &stubSandbox{policy: policy}
}

// Spawn on non-darwin runs the binary without OS-level isolation — useful for CI tests but NOT a production sandbox.
func (s *stubSandbox) Spawn(ctx context.Context, bundlePath string, env []string, rssCapMB int) (*exec.Cmd, error) {
	if rssCapMB <= 0 {
		return nil, ErrRSSCapInvalid
	}
	cmd := exec.CommandContext(ctx, resolveBinary(bundlePath))
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if err := s.policy.ApplyRSSCap(cmd.Process.Pid, rssCapMB); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	return cmd, nil
}
