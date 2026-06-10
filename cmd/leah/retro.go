package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/selflearn"
	"github.com/trilam/leah/internal/selflearn/rules"
)

// runRetro renders the weekly retro markdown to stdout.
func runRetro(args []string) int {
	fs := flag.NewFlagSet("retro", flag.ExitOnError)
	week := fs.String("week", "", "ISO week YYYY-WW (defaults to current)")
	_ = fs.Parse(args)

	store, err := selflearn.OpenMistakeStore(memoryPath())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah retro: open store: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()

	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	r := &selflearn.Retro{
		AuditPath:          auditPath,
		Store:              store,
		AttestationScanner: attestationScannerAdapter(rules.AttestationGate{}),
	}
	md, err := r.Generate(*week)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah retro: %v\n", err)
		return 1
	}
	a := &audit.Logger{Path: auditPath}
	_ = a.Append(audit.Entry{Kind: "retro", ArgsHash: *week, BlastRadius: 0, Outcome: "success", Detail: *week})
	fmt.Print(md)
	return 0
}

// attestationScannerAdapter bridges rules.AttestationGate.Scan (which
// returns rules.Violation) to selflearn.Retro.AttestationScanner (which
// requires selflearn.AttestationViolation). The two-type shape exists
// only to keep selflearn → rules dependency one-way; this adapter does
// the trivial field copy at the composition root.
func attestationScannerAdapter(g rules.AttestationGate) func(context.Context, string) ([]selflearn.AttestationViolation, error) {
	return func(ctx context.Context, path string) ([]selflearn.AttestationViolation, error) {
		raw, err := g.Scan(ctx, path)
		if err != nil {
			return nil, err
		}
		out := make([]selflearn.AttestationViolation, 0, len(raw))
		for _, v := range raw {
			out = append(out, selflearn.AttestationViolation{
				Repo: v.Repo, PRNumber: v.PRNumber, URL: v.URL,
			})
		}
		return out, nil
	}
}
