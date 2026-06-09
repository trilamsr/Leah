package reviewer

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/trilam/leah/internal/budget"
)

type Subagent interface {
	Run(ctx context.Context, systemPrompt, input string) (text string, costUSD float64, err error)
}

type Reviewer struct {
	Subagent     Subagent
	Budget       *budget.Budget // optional; if nil, cost not charged
	SystemPrompt string
}

type Verdict struct {
	Recommendation string // APPROVE | REVISE | BLOCK
	AgentID        string
	Body           string
}

var (
	recRegex = regexp.MustCompile(`(?mi)^Reviewer-recommendation:\s*(APPROVE|REVISE|BLOCK)\s*$`)
	idRegex  = regexp.MustCompile(`(?mi)^Reviewer-agent-id:\s*(\S+)\s*$`)
)

func (r *Reviewer) Review(ctx context.Context, diff, linkedIssue string) (Verdict, error) {
	input := "Linked issue body:\n" + linkedIssue + "\n\n---\n\nDiff:\n" + diff
	resp, cost, err := r.Subagent.Run(ctx, r.SystemPrompt, input)
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
