// Package reviewer dispatches an independent Anthropic subagent against a
// PR diff. The structural defense against self-modification drift in the
// closed loop — Leah's main Reasoner cannot rubber-stamp its own self-build
// PRs because review runs in a separate model call with a separate prompt.
package reviewer

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/trilam/leah/internal/budget"
)

// Subagent is the LLM completion surface used for review. sink, when non-nil,
// receives token-sized text chunks as they arrive — that's what gives
// `leah review` its progressive feel instead of a 4-8s blank-terminal wait.
type Subagent interface {
	Run(ctx context.Context, systemPrompt, input string, sink io.Writer) (text string, costUSD float64, err error)
}

// Reviewer pairs a Subagent with the reviewer system prompt + optional
// budget gate. TokenSink, when non-nil, receives verdict text as it streams.
type Reviewer struct {
	Subagent     Subagent
	Budget       *budget.Budget // optional; if nil, cost not charged
	SystemPrompt string
	TokenSink    io.Writer // optional; nil disables streaming
}

// Verdict is the parsed reviewer response — the recommendation token
// regatta's gate enforces, plus the agent-id for the canonical allowlist.
type Verdict struct {
	Recommendation string // APPROVE | REVISE | BLOCK
	AgentID        string
	Body           string
}

var (
	recRegex = regexp.MustCompile(`(?mi)^Reviewer-recommendation:\s*(APPROVE|REVISE|BLOCK)\s*$`)
	idRegex  = regexp.MustCompile(`(?mi)^Reviewer-agent-id:\s*(\S+)\s*$`)
)

// Review runs the subagent over (linkedIssue, diff), parses the verdict +
// agent-id lines, and validates the agent-id against the canonical allowlist.
// Missing either header or a non-canonical id is fail-closed — regatta's
// gate would reject the verdict anyway, better to surface it here.
func (r *Reviewer) Review(ctx context.Context, diff, linkedIssue string) (Verdict, error) {
	input := "Linked issue body:\n" + linkedIssue + "\n\n---\n\nDiff:\n" + diff
	resp, cost, err := r.Subagent.Run(ctx, r.SystemPrompt, input, r.TokenSink)
	if err != nil {
		return Verdict{}, fmt.Errorf("reviewer subagent: %w", err)
	}
	if r.Budget != nil {
		if chargeErr := r.Budget.Charge(cost); chargeErr != nil {
			return Verdict{}, chargeErr
		}
	}
	v := Verdict{Body: strings.TrimSpace(resp)}

	if m := recRegex.FindStringSubmatch(resp); m != nil {
		v.Recommendation = strings.ToUpper(m[1])
	} else {
		return v, fmt.Errorf("reviewer response missing verdict line (Reviewer-recommendation: APPROVE|REVISE|BLOCK)")
	}

	if m := idRegex.FindStringSubmatch(resp); m != nil {
		v.AgentID = m[1]
	} else {
		return v, fmt.Errorf("reviewer response missing Reviewer-agent-id line")
	}

	if err := ValidateAgentID(v.AgentID); err != nil {
		return v, err
	}

	return v, nil
}
