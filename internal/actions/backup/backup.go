// Package backup wraps the restic CLI (BSD-2, v0.19.0) to snapshot
// ~/.leah-state to a local repo + B2 offsite repo. Subprocess wrap (not
// CGO bindings) per docs/research/2026-06-09-adopt-vs-build-survey.md
// §12: restic owns the data plane, leah owns scheduling + dispatch.
//
// Three repos are anticipated (operator pre-configures with `restic init`):
//   - local:  /Volumes/leah-backup (USB or SMB mount; LEAH_BACKUP_LOCAL_PATH)
//   - b2:     b2:<bucket>:<path>   (B2 offsite; LEAH_BACKUP_B2_REPO)
//
// Encryption: restic mandates a password. RESTIC_PASSWORD_FILE must point
// at an operator-owned file (chmod 600). This package never reads or holds
// the password — restic reads it directly from the env-var path.
package backup

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Executor abstracts process spawn so tests can drive the package without
// requiring restic on the test host. Mirrors internal/voice.Executor.
type Executor interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	LookPath(name string) (string, error)
}

// ShellExec is the production Executor — wraps exec.CommandContext.
type ShellExec struct{}

// Run invokes name with args, folding ExitError stderr into the returned
// error so operator logs see why restic refused.
func (ShellExec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("%s: %s", name, string(ee.Stderr))
		}
		return out, err
	}
	return out, nil
}

// LookPath resolves name against $PATH.
func (ShellExec) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// Restic is the subprocess wrapper. Exec defaults to ShellExec when nil.
type Restic struct {
	Exec Executor
}

// Available reports whether `restic` is on PATH; callers gate cron tasks
// on this so a fresh install doesn't spam the operator with errors before
// `brew install restic`.
func (r *Restic) Available() bool {
	_, err := r.exec().LookPath("restic")
	return err == nil
}

// Init bootstraps a new restic repository at repo. Idempotency note:
// restic init FAILS if repo already exists — callers MUST treat
// "already initialized" as success at the script layer (scripts/restic-init.sh).
func (r *Restic) Init(ctx context.Context, repo string) error {
	if repo == "" {
		return errors.New("backup: init: repo path required")
	}
	if _, err := r.exec().Run(ctx, "restic", "-r", repo, "init"); err != nil {
		return fmt.Errorf("backup: init %s: %w", repo, err)
	}
	return nil
}

// Snapshot writes a new restic snapshot of statePath into repo. restic
// dedupes against prior snapshots — repeated invocations only transfer
// changed chunks.
func (r *Restic) Snapshot(ctx context.Context, repo, statePath string) error {
	if repo == "" {
		return errors.New("backup: snapshot: repo path required")
	}
	if statePath == "" {
		return errors.New("backup: snapshot: state path required")
	}
	if _, err := r.exec().Run(ctx, "restic", "-r", repo, "backup", statePath); err != nil {
		return fmt.Errorf("backup: snapshot %s → %s: %w", statePath, repo, err)
	}
	return nil
}

// Restore extracts the `latest` snapshot from repo into target. target
// MUST be an empty / non-existent directory — restic overlays files in
// place and the operator is expected to point at a scratch dir.
func (r *Restic) Restore(ctx context.Context, repo, target string) error {
	if repo == "" {
		return errors.New("backup: restore: repo path required")
	}
	if target == "" {
		return errors.New("backup: restore: target path required")
	}
	if _, err := r.exec().Run(ctx, "restic", "-r", repo, "restore", "latest", "--target", target); err != nil {
		return fmt.Errorf("backup: restore %s → %s: %w", repo, target, err)
	}
	return nil
}

// Verify runs `restic check` — walks the repo index + a sample of pack
// files to detect bit-rot. Quarterly cron firing this catches a corrupt
// repo BEFORE the operator needs the restore.
func (r *Restic) Verify(ctx context.Context, repo string) error {
	if repo == "" {
		return errors.New("backup: verify: repo path required")
	}
	if _, err := r.exec().Run(ctx, "restic", "-r", repo, "check"); err != nil {
		return fmt.Errorf("backup: verify %s: %w", repo, err)
	}
	return nil
}

func (r *Restic) exec() Executor {
	if r.Exec != nil {
		return r.Exec
	}
	return ShellExec{}
}
