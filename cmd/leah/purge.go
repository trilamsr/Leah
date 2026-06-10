package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/connect"
)

// ErrPurgeAttestationDenied fires when the operator declines the BR=4 prompt.
var ErrPurgeAttestationDenied = errors.New("purge: attestation denied")

// runPurge is `leah purge --everything` (S10/M2 trust-moat). BR=4 attested.
// Stages: provider OAuth revoke → rm -rf state dir → distribution-cleanup hints.
// Failure to reach a provider does NOT block local deletion — single-blast-radius
// guarantee is the design intent, brew/PATH cleanup is hints (never auto-exec).
func runPurge(ctx context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printPurgeUsage(w)
		return 0
	}
	if len(args) == 0 || args[0] != "--everything" {
		printPurgeUsage(os.Stderr)
		return 2
	}

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}

	if err := purgeAttest(); err != nil {
		_ = a.Append(audit.Entry{
			Kind:        "purge_everything",
			BlastRadius: 4,
			Outcome:     "failed",
			Detail:      "attestation_denied",
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah purge: %v\n", err)
		return 1
	}

	reg := connect.DefaultRegistry()
	revoked := purgeRevokeAll(ctx, reg, w)

	// Pre-deletion audit row. The state dir is about to be wiped so we also
	// echo a copy to stdout for operator archival per spec §3.1.
	_ = a.Append(audit.Entry{
		Kind:        "purge_everything",
		BlastRadius: 4,
		Outcome:     "attested",
		Detail:      "providers=" + strings.Join(revoked, ","),
	})
	_, _ = fmt.Fprintf(w, "final-audit: kind=purge_everything outcome=attested providers=%s\n", strings.Join(revoked, ","))

	sd := stateDir()
	if err := os.RemoveAll(sd); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah purge: rm state dir: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(w, "deleted: %s\n", sd)

	printPurgeHints(w)
	return 0
}

// purgeRevokeAll iterates the connect.Registry and calls Disconnect on every
// authorized provider. Errors are logged per-provider and continue — the
// spec's "remote unreachable still local-deletes" guarantee.
func purgeRevokeAll(ctx context.Context, reg *connect.Registry, w io.Writer) []string {
	att := newPurgeAutoAttestor()
	var names []string
	for _, s := range reg.List() {
		names = append(names, s.Name)
		if !s.Authorized {
			_, _ = fmt.Fprintf(w, "skip: %s (not authorized)\n", s.Name)
			continue
		}
		p, err := reg.Lookup(s.Name)
		if err != nil {
			_, _ = fmt.Fprintf(w, "warn: %s lookup: %v\n", s.Name, err)
			continue
		}
		if err := connect.Disconnect(ctx, p, att); err != nil {
			_, _ = fmt.Fprintf(w, "warn: %s revoke: %v\n", s.Name, err)
			continue
		}
		_, _ = fmt.Fprintf(w, "revoked: %s\n", s.Name)
	}
	return names
}

// purgeAttest is the BR=4 operator-confirmation prompt. Operator must type
// the phrase exactly — y/N is rejected; this is the highest-blast-radius
// destructive op in the CLI. LEAH_PURGE_AUTO_ATTEST=1 short-circuits for tests.
func purgeAttest() error {
	if v := os.Getenv("LEAH_PURGE_AUTO_ATTEST"); v == "1" {
		return nil
	}
	_, _ = fmt.Fprintln(os.Stderr, "Leah purge --everything will:")
	_, _ = fmt.Fprintln(os.Stderr, "  1. Revoke OAuth at every authorized provider.")
	_, _ = fmt.Fprintln(os.Stderr, "  2. Delete ~/.leah-state/ (operator memory, audit log, mirrors).")
	_, _ = fmt.Fprintln(os.Stderr, "Type exactly: purge everything")
	var line string
	_, _ = fmt.Fscanln(os.Stdin, &line)
	// Phrase has a space; Fscanln stops at whitespace so accept the leading word
	// only if a second token follows — read again.
	if line == "purge" {
		var second string
		_, _ = fmt.Fscanln(os.Stdin, &second)
		if second == "everything" {
			return nil
		}
	}
	return ErrPurgeAttestationDenied
}

// purgeAutoAttestor satisfies connect.Attestor by always permitting — the
// BR=4 purge-everything prompt already attested the operator; the provider-side
// Disconnect's attestation gate would otherwise re-prompt per provider, which
// breaks the single-prompt UX guarantee.
type purgeAutoAttestor struct{}

func newPurgeAutoAttestor() connect.Attestor { return purgeAutoAttestor{} }

func (purgeAutoAttestor) Attest(_ context.Context, _ string) error { return nil }

func printPurgeHints(w io.Writer) {
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Leah binary still on disk. To remove:")
	_, _ = fmt.Fprintln(w, "  brew uninstall leah        # if installed via brew")
	_, _ = fmt.Fprintln(w, "  rm $(which leah)            # otherwise")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "PATH entry in ~/.zshrc / ~/.bashrc may reference leah; review and remove manually.")
}

func printPurgeUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah purge --everything")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "BR=4 destructive: OAuth revoke at every provider + delete ~/.leah-state/.")
	_, _ = fmt.Fprintln(w, "Operator-attested via the phrase \"purge everything\".")
	_, _ = fmt.Fprintln(w, "Binary + PATH cleanup are hinted, not executed (single blast radius).")
}
