package main

import (
	"context"

	"github.com/trilam/leah/internal/learn"
	"github.com/trilam/leah/internal/learn/rules"
)

// daemonAttestationScanner adapts rules.AttestationGate.Scan to the
// learn.Retro.AttestationScanner shape. Field-copy adapter mirrors
// cmd/leah/retro.go::attestationScannerAdapter; the duplication is
// intentional (composition root per binary).
func daemonAttestationScanner() func(context.Context, string) ([]learn.AttestationViolation, error) {
	g := rules.AttestationGate{}
	return func(ctx context.Context, path string) ([]learn.AttestationViolation, error) {
		raw, err := g.Scan(ctx, path)
		if err != nil {
			return nil, err
		}
		out := make([]learn.AttestationViolation, 0, len(raw))
		for _, v := range raw {
			out = append(out, learn.AttestationViolation{
				Repo: v.Repo, PRNumber: v.PRNumber, URL: v.URL,
			})
		}
		return out, nil
	}
}
