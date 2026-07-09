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

// runConnect is `leah connect`. w is stdout (test seam); stderr stays
// process-global for usage + error lines.
func runConnect(ctx context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printConnectUsage(w)
		return 0
	}
	if len(args) == 0 {
		printConnectUsage(os.Stderr)
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
	// regatta is not an OAuth provider — it's either a local Docker container
	// or a cloud bearer-token connect. Pre-empt the OAuth registry lookup so
	// `leah connect regatta [--cloud …]` reaches the right handler (without
	// this, runConnect would route through Authorize → ErrRegattaUseConnectRegatta).
	if name == "regatta" {
		return runConnectRegatta(ctx, args[1:], w, nil)
	}
	p, err := reg.Lookup(name)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah connect: %v: %q (try --list)\n", err, name)
		return 2
	}

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}
	att := newConnectAttestor()

	prompt := func(verificationURL, userCode string) {
		_, _ = fmt.Fprintf(w, "Open %s and enter code: %s\n", verificationURL, userCode)
	}

	if _, err := connect.Authorize(ctx, p, att, prompt); err != nil {
		_ = a.Append(audit.Entry{
			Kind:        "connect_" + name,
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      summarizeConnectErr(err),
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect %s: %v\n", name, err)
		return 1
	}
	_ = a.Append(audit.Entry{Kind: "connect_" + name, BlastRadius: 2, Outcome: "success"})
	_, _ = fmt.Fprintf(w, "ok: %s authorized → %s\n", name, p.TokenPath())
	return 0
}

// summarizeConnectErr scrubs nested token bytes from the audit row — only the
// sentinel name reaches disk.
func summarizeConnectErr(err error) string {
	switch {
	case errors.Is(err, connect.ErrAttestationDenied):
		return "attestation_denied"
	case errors.Is(err, connect.ErrDeviceAuth):
		return "device_auth_failed"
	case errors.Is(err, connect.ErrTokenExchange):
		return "token_exchange_failed"
	default:
		return "error"
	}
}

func printConnectUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah connect <integration>")
	_, _ = fmt.Fprintln(w, "       leah connect --list")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "authorization method depends on the integration: some use OAuth (browser), others require an API token. Run `leah connect --list` to see which applies.")
}

type connectAttestor struct{}

func newConnectAttestor() connect.Attestor { return connectAttestor{} }

func (connectAttestor) Attest(_ context.Context, scope string) error {
	if v := os.Getenv("LEAH_CONNECT_AUTO_ATTEST"); v == "1" {
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "attest %s? [y/N] ", scope)
	var resp string
	_, _ = fmt.Fscanln(os.Stdin, &resp)
	// Empty / EOF / closed stdin defaults to NO — granting consent on
	// unanswered prompts (the prior "" → accept) would let a piped-from-
	// /dev/null invocation silently attest scopes the operator never saw.
	if resp == "" {
		return errors.New("denied")
	}
	switch resp {
	case "y", "Y", "yes", "YES":
		return nil
	}
	return errors.New("denied")
}
