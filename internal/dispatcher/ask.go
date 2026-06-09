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

type Reasoner interface {
	Ask(ctx context.Context, user string) (string, error)
}

type Ask struct {
	Reasoner Reasoner
	Audit    *audit.Logger
	Budget   *budget.Budget
	Out      io.Writer
}

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
