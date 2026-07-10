package reviewer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/platform/budget"
)

var errSubagent = errors.New("synthetic subagent failure")

type fakeSubagent struct {
	gotPrompt string
	gotInput  string
	resp      string
	respCost  float64
	respErr   error
}

func (f *fakeSubagent) Run(ctx context.Context, systemPrompt, input string, _ io.Writer) (string, float64, error) {
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

// TestReview_VerdictAtTopOfBody asserts the verdict regex matches when the
// Reviewer-recommendation line is the first line of the response (multiline
// anchor + (?m) flag — guards against future refactor that drops (?m)).
func TestReview_VerdictAtTopOfBody(t *testing.T) {
	sa := &fakeSubagent{
		resp: `Reviewer-recommendation: REVISE
Reviewer-agent-id: cavecrew-reviewer-top-position

Findings: needs work.
`,
	}
	r := &Reviewer{Subagent: sa, SystemPrompt: "x"}
	v, err := r.Review(context.Background(), "diff", "issue")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if v.Recommendation != "REVISE" {
		t.Errorf("recommendation: %q", v.Recommendation)
	}
}

// TestReview_VerdictCaseInsensitive asserts the (?mi) flag lets the parser
// accept "reviewer-recommendation:" + lowercase verdict tokens (canonical
// is uppercase but operator drafts mix case; regex Capture maps to upper).
func TestReview_VerdictCaseInsensitive(t *testing.T) {
	sa := &fakeSubagent{
		resp: `reviewer-recommendation: approve
reviewer-agent-id: a1234567890abcdef
`,
	}
	r := &Reviewer{Subagent: sa, SystemPrompt: "x"}
	v, err := r.Review(context.Background(), "diff", "issue")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if v.Recommendation != "APPROVE" {
		t.Errorf("expected normalized APPROVE, got %q", v.Recommendation)
	}
}

// TestReview_MultipleAgentIDLinesUsesFirst asserts the parser binds to the
// first Reviewer-agent-id line if the response (incorrectly) emits two.
// Authors copy-paste templates; we want a deterministic verdict not a panic.
func TestReview_MultipleAgentIDLinesUsesFirst(t *testing.T) {
	sa := &fakeSubagent{
		resp: `Reviewer-recommendation: APPROVE
Reviewer-agent-id: a1234567890abcdef
Reviewer-agent-id: cavecrew-reviewer-secondary
`,
	}
	r := &Reviewer{Subagent: sa, SystemPrompt: "x"}
	v, err := r.Review(context.Background(), "diff", "issue")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if v.AgentID != "a1234567890abcdef" {
		t.Errorf("expected first id, got %q", v.AgentID)
	}
}

// TestReview_SubagentErrorPropagates asserts a non-nil subagent error returns
// a wrapped error and skips both budget and parse paths.
func TestReview_SubagentErrorPropagates(t *testing.T) {
	sa := &fakeSubagent{respErr: errSubagent}
	b := &budget.Budget{Ceiling: 1.0}
	r := &Reviewer{Subagent: sa, Budget: b, SystemPrompt: "x"}
	_, err := r.Review(context.Background(), "diff", "issue")
	if err == nil || !strings.Contains(err.Error(), "reviewer subagent") {
		t.Errorf("want wrapped subagent error, got %v", err)
	}
	if b.Spent() != 0 {
		t.Errorf("budget should not be charged on subagent error, got %v", b.Spent())
	}
}

// TestReview_BodyTrimsWhitespace asserts the parsed body has leading/trailing
// whitespace stripped — log + downstream readers don't want noise.
func TestReview_BodyTrimsWhitespace(t *testing.T) {
	sa := &fakeSubagent{
		resp: "\n\n\nFindings: ok.\n\nReviewer-recommendation: APPROVE\nReviewer-agent-id: a1234567890abcdef\n\n\n",
	}
	r := &Reviewer{Subagent: sa, SystemPrompt: "x"}
	v, err := r.Review(context.Background(), "diff", "issue")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if strings.HasPrefix(v.Body, "\n") || strings.HasSuffix(v.Body, "\n") {
		t.Errorf("body not trimmed: %q", v.Body)
	}
}

// TestReview_StreamsTokensToSink asserts Review wires its TokenSink down to
// the Subagent so partial verdict text reaches the user as it arrives —
// without this the operator stares at a blank terminal for 4-8s.
func TestReview_StreamsTokensToSink(t *testing.T) {
	sink := &bytes.Buffer{}
	sa := &streamingFakeSubagent{
		chunks: []string{"Summary: ", "looks ", "fine.\n"},
		resp: `Summary: looks fine.

Reviewer-recommendation: APPROVE
Reviewer-agent-id: a1234567890abcdef
`,
	}
	r := &Reviewer{Subagent: sa, SystemPrompt: "x", TokenSink: sink}
	if _, err := r.Review(context.Background(), "diff", "issue"); err != nil {
		t.Fatalf("review: %v", err)
	}
	if !sa.gotSink {
		t.Fatalf("subagent did not receive a sink — streaming wiring broken")
	}
	if got := sink.String(); got != "Summary: looks fine.\n" {
		t.Errorf("sink contents: %q", got)
	}
}

type streamingFakeSubagent struct {
	gotSink bool
	resp    string
	chunks  []string
}

func (f *streamingFakeSubagent) Run(ctx context.Context, systemPrompt, input string, sink io.Writer) (string, float64, error) {
	if sink != nil {
		f.gotSink = true
		for _, c := range f.chunks {
			_, _ = sink.Write([]byte(c))
		}
	}
	return f.resp, 0, nil
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
