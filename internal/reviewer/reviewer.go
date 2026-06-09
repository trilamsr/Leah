package reviewer

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type Subagent interface {
	Run(ctx context.Context, systemPrompt, input string) (string, error)
}

type Reviewer struct {
	Subagent     Subagent
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
	resp, err := r.Subagent.Run(ctx, r.SystemPrompt, input)
	if err != nil {
		return Verdict{}, fmt.Errorf("reviewer subagent: %w", err)
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
