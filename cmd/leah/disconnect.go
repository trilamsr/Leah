package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/connect"
)

// runDisconnect is `leah disconnect`. Mirrors runConnect's shape (writer seam +
// int-return) so SIGINT-flush via writeInterruptedAudit still fires.
func runDisconnect(ctx context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printDisconnectUsage(w)
		return 0
	}
	if len(args) == 0 {
		printDisconnectUsage(os.Stderr)
		return 2
	}

	reg := connect.DefaultRegistry()

	if args[0] == "--list" {
		for _, s := range reg.List() {
			marker := "not authorized"
			if s.Authorized {
				marker = "authorized"
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t(%s)\n", s.Name, marker, s.TokenPath)
		}
		return 0
	}

	name := args[0]
	p, err := reg.Lookup(name)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah disconnect: %v: %q (try --list)\n", err, name)
		return 2
	}

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}
	att := newConnectAttestor()

	if err := connect.Disconnect(ctx, p, att); err != nil {
		_ = a.Append(audit.Entry{
			Kind:        "disconnect_" + name,
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      summarizeDisconnectErr(err),
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah disconnect %s: %v\n", name, err)
		return 1
	}
	_ = a.Append(audit.Entry{Kind: "disconnect_" + name, BlastRadius: 2, Outcome: "success"})
	_, _ = fmt.Fprintf(w, "ok: %s disconnected (token removed: %s)\n", name, p.TokenPath())
	return 0
}

func summarizeDisconnectErr(err error) string {
	switch {
	case errors.Is(err, connect.ErrAttestationDenied):
		return "attestation_denied"
	case errors.Is(err, connect.ErrTokenFileNotFound):
		return "token_file_not_found"
	default:
		return "error"
	}
}

func printDisconnectUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah disconnect <integration>")
	_, _ = fmt.Fprintln(w, "       leah disconnect --list")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Revoke + remove the on-disk token for a shipped integration (gmail, gcal).")
}
