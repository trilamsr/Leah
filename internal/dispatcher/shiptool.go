package dispatcher

import (
	"context"
	"fmt"
	"io"

	"github.com/trilam/leah/internal/attest"
	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/contracts"
)

type ToolWriter interface {
	Write(ctx context.Context, title string) error
}

// ShipTool attests before dispatching an irreversible outward write to one tool.
type ShipTool struct {
	Tool     string
	Attestor contracts.Attestor
	Writer   ToolWriter
	Audit    *audit.Logger
	Out      io.Writer
}

func (s *ShipTool) Run(ctx context.Context, title string) error {
	kind := "ship." + s.Tool
	if err := s.Attestor.Attest(ctx, attest.ScopeShipTool); err != nil {
		s.append(kind, "declined", "")
		return fmt.Errorf("ship --%s: attestation declined: %w", s.Tool, err)
	}
	if err := s.Writer.Write(ctx, title); err != nil {
		s.append(kind, "failed", err.Error())
		return fmt.Errorf("ship --%s: %w", s.Tool, err)
	}
	s.append(kind, "ok", title)
	_, _ = fmt.Fprintf(s.Out, "shipped to %s\n", s.Tool)
	return nil
}

func (s *ShipTool) append(kind, outcome, detail string) {
	_ = s.Audit.Append(audit.Entry{
		Kind:        kind,
		ArgsHash:    argsHash(detail),
		BlastRadius: 3,
		Outcome:     outcome,
		Detail:      detail,
	})
}
