package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/recommend"
)

// ErrForgetAttestationDenied fires when the operator declines the prompt.
// Surfaced via summarizeForgetErr → audit row Detail.
var ErrForgetAttestationDenied = errors.New("forget: attestation denied")

// runForget is `leah forget <pattern-id|all>`. w is the stdout seam.
//
// Operator-attested (scope `recommend:forget`). On success, wipes the
// operator_profile + operator_profile_meta tables (when `all`) or no DB
// touch for a single pattern — the engine-side quarantine + audit row
// carry the signal forward until W16 wires recommend_pattern_state.
func runForget(ctx context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printForgetUsage(w)
		return 0
	}
	if len(args) == 0 {
		printForgetUsage(os.Stderr)
		return 2
	}

	dryRun := false
	autoYes := false
	target := ""
	for _, a := range args {
		switch a {
		case "--dry-run":
			dryRun = true
		case "--yes", "-y":
			autoYes = true
		default:
			if target != "" {
				_, _ = fmt.Fprintln(os.Stderr, "leah forget: only one pattern (or 'all') allowed")
				return 2
			}
			target = a
		}
	}
	if target == "" {
		printForgetUsage(os.Stderr)
		return 2
	}

	logger := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}

	if dryRun {
		_, _ = fmt.Fprintf(w, "dry-run: would forget %q (operator_profile + recommend pending)\n", target)
		return 0
	}

	if !autoYes {
		if err := forgetAttest(target); err != nil {
			_ = logger.Append(audit.Entry{
				Kind:        "recommendation_forget",
				BlastRadius: 2,
				Outcome:     "failed",
				Detail:      "attestation_denied",
			})
			_, _ = fmt.Fprintf(os.Stderr, "leah forget: %v\n", err)
			return 1
		}
	}

	if target == recommend.ForgetAll {
		if err := wipeOperatorProfile(); err != nil {
			_ = logger.Append(audit.Entry{
				Kind:        "recommendation_forget",
				BlastRadius: 2,
				Outcome:     "failed",
				Detail:      "wipe_profile: " + err.Error(),
			})
			_, _ = fmt.Fprintf(os.Stderr, "leah forget all: %v\n", err)
			return 1
		}
	}

	_ = logger.Append(audit.Entry{
		Kind:        "recommendation_forget",
		BlastRadius: 2,
		Outcome:     "success",
		Detail:      target,
	})
	_, _ = fmt.Fprintf(w, "ok: forgot %q\n", target)
	return 0
}

// wipeOperatorProfile truncates the W4 operatormodel tables. Soft-fail on a
// missing DB file — first-run operators may call `leah forget all` before
// any observe pass has materialized the tables.
func wipeOperatorProfile() error {
	if _, err := os.Stat(memoryPath()); os.IsNotExist(err) {
		return nil
	}
	store, err := openMemoryStore()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	if _, err := store.DB().Exec(`DELETE FROM operator_profile`); err != nil {
		return fmt.Errorf("wipe operator_profile: %w", err)
	}
	if _, err := store.DB().Exec(`DELETE FROM operator_profile_meta`); err != nil {
		return fmt.Errorf("wipe operator_profile_meta: %w", err)
	}
	return nil
}

// forgetAttest is the operator-confirmation prompt. LEAH_FORGET_AUTO_ATTEST=1
// short-circuits for test + scripted invocation (sibling to LEAH_CONNECT_AUTO_ATTEST).
func forgetAttest(target string) error {
	if v := os.Getenv("LEAH_FORGET_AUTO_ATTEST"); v == "1" {
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "Forget %q from operator model? [y/N] ", target)
	var resp string
	_, _ = fmt.Fscanln(os.Stdin, &resp)
	switch resp {
	case "y", "Y", "yes", "YES":
		return nil
	}
	return ErrForgetAttestationDenied
}

func printForgetUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah forget <pattern-id>")
	_, _ = fmt.Fprintln(w, "       leah forget all")
	_, _ = fmt.Fprintln(w, "       leah forget --dry-run <pattern-id|all>")
	_, _ = fmt.Fprintln(w, "(use --yes to skip confirmation; --dry-run shows effect without changing state)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Wipe a pattern (or everything) from operator-model recommendations.")
	_, _ = fmt.Fprintln(w, "Operator-attested. Re-prompt skipped via --yes or LEAH_FORGET_AUTO_ATTEST=1.")
}
