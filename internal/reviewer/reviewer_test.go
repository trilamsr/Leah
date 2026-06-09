package reviewer

import (
	"context"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/budget"
)

type fakeSubagent struct {
	gotPrompt string
	gotInput  string
	resp      string
	respCost  float64
	respErr   error
}

func (f *fakeSubagent) Run(ctx context.Context, systemPrompt, input string) (string, float64, error) {
	f.gotPrompt = systemPrompt
	f.gotInput = input
	return f.resp, f.respCost, f.respErr
}

func TestReviewParsesVerdictAndAgentID(t *testing.T) {
	sa := &fakeSubagent{
		resp: `Summary: looks fine.

Findings:
- LOW: a minor nit

Reviewer-recommendation: APPROVE
Reviewer-agent-id: a1234567890abcdef
`,
	}
	r := &Reviewer{Subagent: sa, SystemPrompt: "you are reviewer"}

	v, err := r.Review(context.Background(), "diff content", "linked issue body")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if v.Recommendation != "APPROVE" {
		t.Errorf("rec: %v", v.Recommendation)
	}
	if v.AgentID != "a1234567890abcdef" {
		t.Errorf("id: %v", v.AgentID)
	}
	if !strings.Contains(v.Body, "Summary") {
		t.Errorf("body missing summary: %v", v.Body)
	}
}

func TestReviewRejectsWrongShapeAgentID(t *testing.T) {
	sa := &fakeSubagent{
		resp: `Findings: none.

Reviewer-recommendation: APPROVE
Reviewer-agent-id: leah-self
`,
	}
	r := &Reviewer{Subagent: sa, SystemPrompt: "x"}

	_, err := r.Review(context.Background(), "diff", "issue")
	if err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Errorf("want canonical-shape error, got %v", err)
	}
}

func TestReviewRejectsMissingVerdict(t *testing.T) {
	sa := &fakeSubagent{resp: "no verdict here"}
	r := &Reviewer{Subagent: sa, SystemPrompt: "x"}

	_, err := r.Review(context.Background(), "diff", "issue")
	if err == nil || !strings.Contains(err.Error(), "verdict") {
		t.Errorf("want missing-verdict error, got %v", err)
	}
}

func TestReviewRejectsMissingAgentID(t *testing.T) {
	sa := &fakeSubagent{resp: "Reviewer-recommendation: APPROVE"}
	r := &Reviewer{Subagent: sa, SystemPrompt: "x"}

	_, err := r.Review(context.Background(), "diff", "issue")
	if err == nil || !strings.Contains(err.Error(), "agent-id") {
		t.Errorf("want missing-agent-id error, got %v", err)
	}
}

func TestReviewBlocksOnBudgetExceeded(t *testing.T) {
	sa := &fakeSubagent{
		resp: `Reviewer-recommendation: APPROVE
Reviewer-agent-id: a1234567890abcdef
`,
		respCost: 10.0,
	}
	b := &budget.Budget{Ceiling: 1.0}
	r := &Reviewer{Subagent: sa, Budget: b, SystemPrompt: "x"}
	_, err := r.Review(context.Background(), "diff", "issue")
	if err == nil || !strings.Contains(err.Error(), "budget exceeded") {
		t.Errorf("want budget exceeded error, got %v", err)
	}
}
