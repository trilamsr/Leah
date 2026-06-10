// Package dispatcher is Layer 4 of the closed loop — the CLI orchestrations
// (Ask, Ship, SelfBuild, Status) that take operator intent, drive Reasoner +
// gh + regatta, and write the audit row. Every operator action lands here.
package dispatcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
)

// Reasoner is the LLM surface dispatcher uses. Defined here so Ship/SelfBuild
// can swap in a prebakedReasoner wrapper (selfbuild.go) without dragging in
// the reasoner package.
type Reasoner interface {
	Ask(ctx context.Context, user string) (string, error)
}

// Ask is the BR=0 read-only `leah ask` orchestration — one Reasoner call,
// one audit row, stdout result.
type Ask struct {
	Reasoner Reasoner
	Audit    *audit.Logger
	Budget   *budget.Budget
	Out      io.Writer
}

// Run executes the ask, writing the audit row on both success and failure
// paths so the operator's audit log never misses a charged Reasoner call.
func (a *Ask) Run(ctx context.Context, query string) error {
	text, err := a.Reasoner.Ask(ctx, query)
	entry := audit.Entry{
		Kind:        "ask",
		ArgsHash:    argsHash(query),
		BlastRadius: 0,
		CostDollars: a.Budget.Spent(),
	}
	if err != nil {
		entry.Outcome = "failed"
		entry.Detail = err.Error()
		_ = a.Audit.Append(entry)
		return err
	}
	entry.Outcome = "success"
	if writeErr := a.Audit.Append(entry); writeErr != nil {
		_, _ = fmt.Fprintf(a.Out, "warning: audit append failed: %v\n", writeErr)
	}
	_, _ = fmt.Fprintln(a.Out, text)
	return nil
}

func argsHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}
