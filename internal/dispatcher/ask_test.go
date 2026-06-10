package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
	"github.com/trilam/leah/internal/reasoner"
)

type fakeReasoner struct {
	got string
	out string
}

func (f *fakeReasoner) Ask(ctx context.Context, user string) (string, error) {
	f.got = user
	return f.out, nil
}

func TestAskWritesToWriterAndAuditsAndReturnsCleanly(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	a := &audit.Logger{Path: auditPath}
	b := &budget.Budget{Ceiling: 5.0}
	r := &fakeReasoner{out: "the answer is 42"}
	out := &bytes.Buffer{}

	ask := &Ask{Reasoner: r, Audit: a, Budget: b, Out: out}
	if err := ask.Run(context.Background(), "what is the meaning of life"); err != nil {
		t.Fatalf("run: %v", err)
	}

	if r.got != "what is the meaning of life" {
		t.Errorf("reasoner got %q", r.got)
	}
	if !strings.Contains(out.String(), "the answer is 42") {
		t.Errorf("output: %q", out.String())
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !strings.Contains(string(data), `"kind":"ask"`) {
		t.Errorf("audit missing kind=ask: %q", data)
	}
	if !strings.Contains(string(data), `"blast_radius":0`) {
		t.Errorf("audit missing blast_radius=0: %q", data)
	}
}

// richFakeReasoner satisfies dispatcher.Reasoner AND LLMDimReporter so
// Ask.Run can stamp the LLM-dim audit fields. e2e fixture for W94.
type richFakeReasoner struct {
	out  string
	info reasoner.CallInfo
}

func (f *richFakeReasoner) Ask(ctx context.Context, user string) (string, error) {
	return f.out, nil
}
func (f *richFakeReasoner) LastCallInfo() reasoner.CallInfo { return f.info }

// TestReasoner_WritesLLMDimFields (e2e) asserts dispatcher.Ask.Run
// reads LLMDimReporter.LastCallInfo and stamps every W94 audit field
// onto the row.
func TestReasoner_WritesLLMDimFields(t *testing.T) {
	dir := t.TempDir()
	auditPath := dir + "/audit.jsonl"
	a := &audit.Logger{Path: auditPath}
	b := &budget.Budget{Ceiling: 5.0}
	r := &richFakeReasoner{
		out: "ok",
		info: reasoner.CallInfo{
			Model:        "claude-sonnet-4-6",
			PromptSHA:    "deadbeefcafef00d",
			InputTokens:  100,
			OutputTokens: 50,
			LatencyMS:    321,
			EgressBytes:  1024,
			CacheHit:     true,
		},
	}
	ask := &Ask{Reasoner: r, Audit: a, Budget: b, Out: &bytes.Buffer{}}
	if err := ask.Run(context.Background(), "hi"); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	var got audit.Entry
	if err := json.Unmarshal(bytes.TrimSpace(data), &got); err != nil {
		t.Fatalf("unmarshal audit row: %v", err)
	}
	want := audit.Entry{
		Model:        "claude-sonnet-4-6",
		PromptSHA:    "deadbeefcafef00d",
		InputTokens:  100,
		OutputTokens: 50,
		LatencyMS:    321,
		EgressBytes:  1024,
		CacheHit:     true,
	}
	if got.Model != want.Model || got.PromptSHA != want.PromptSHA ||
		got.InputTokens != want.InputTokens || got.OutputTokens != want.OutputTokens ||
		got.LatencyMS != want.LatencyMS || got.EgressBytes != want.EgressBytes ||
		got.CacheHit != want.CacheHit {
		t.Errorf("LLM-dim fields not stamped:\n got  %+v\n want %+v", got, want)
	}
}
