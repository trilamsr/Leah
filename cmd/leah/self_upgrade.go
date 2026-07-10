package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/trilam/leah/internal/platform/attest"
	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/contracts"
)

type upgradeDeps struct {
	Attestor contracts.Attestor
	Run      func(context.Context) error
}

// Attest must gate before Run — unattested in-place binary replacement is the threat the attestation system guards.
func runSelfUpgrade(ctx context.Context, args []string, w io.Writer, deps *upgradeDeps) int {
	if shouldShowHelp(args) {
		printSelfUpgradeUsage(w)
		return 0
	}

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}

	if deps == nil {
		deps = &upgradeDeps{Attestor: newSelfUpgradeAttestor(), Run: runUpgradeScript}
	}

	if err := deps.Attestor.Attest(ctx, attest.ScopeSelfUpgrade); err != nil {
		_ = a.Append(audit.Entry{Kind: "self_upgrade", BlastRadius: 4, Outcome: "declined"})
		_, _ = fmt.Fprintf(os.Stderr, "leah self-upgrade: attestation declined, aborting\n")
		return 1
	}

	if err := deps.Run(ctx); err != nil {
		_ = a.Append(audit.Entry{Kind: "self_upgrade", BlastRadius: 4, Outcome: "failed", Detail: err.Error()})
		_, _ = fmt.Fprintf(os.Stderr, "leah self-upgrade: %v\n", err)
		return 1
	}

	_ = a.Append(audit.Entry{Kind: "self_upgrade", BlastRadius: 4, Outcome: "ok"})
	return 0
}

// The script owns build + symlink swap + restart; Go reimplements none of it.
func runUpgradeScript(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "bash", "scripts/upgrade.sh", "upgrade")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printSelfUpgradeUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah self-upgrade")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Rebuild from the local clone and atomic-swap ~/bin/leah → the new SHA (scripts/upgrade.sh).")
	_, _ = fmt.Fprintln(w, "Gated by an operator attestation; BR=4 (mutates the running binary).")
}

type selfUpgradeAttestor struct{}

func newSelfUpgradeAttestor() contracts.Attestor { return selfUpgradeAttestor{} }

// The drawn pool question forces operator reflection; the env seam mirrors connectAttestor for non-interactive runs.
func (selfUpgradeAttestor) Attest(_ context.Context, scope string) error {
	if os.Getenv("LEAH_CONNECT_AUTO_ATTEST") == "1" {
		return nil
	}
	if q, err := pickSelfUpgradeQuestion(scope); err == nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", q)
	}
	_, _ = fmt.Fprintf(os.Stderr, "proceed with self-upgrade? [y/N] ")
	var resp string
	_, _ = fmt.Fscanln(os.Stdin, &resp)
	switch resp {
	case "y", "Y", "yes", "YES":
		return nil
	}
	return fmt.Errorf("declined")
}

func pickSelfUpgradeQuestion(scope string) (string, error) {
	pool, err := attest.Load(filepath.Join(promptDir(), "self-build-attestations.txt"), attest.ScopeSelfUpgrade)
	if err != nil {
		return "", err
	}
	return pool.Pick(scope)
}
