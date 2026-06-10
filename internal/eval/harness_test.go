package eval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeReasoner satisfies Reasoner without an API key — returns the canned
// text + cost on every Ask call so tests stay hermetic.
type fakeReasoner struct {
	text string
	cost float64
	err  error
	n    int
}

func (f *fakeReasoner) Ask(_ context.Context, _ string) (string, error) {
	f.n++
	return f.text, f.err
}
func (f *fakeReasoner) Cost() float64 { return f.cost * float64(f.n) }

// fakeJudge returns a canned verdict so we exercise the harness wiring
// without hitting the actual judge model.
type fakeJudge struct {
	pass   bool
	score  float64
	reason string
	cost   float64
	calls  int
}

func (f *fakeJudge) Score(_ context.Context, _ JudgeRequest) (JudgeResult, float64, error) {
	f.calls++
	return JudgeResult{Pass: f.pass, Score: f.score, Reason: f.reason}, f.cost, nil
}
func (f *fakeJudge) PromptSHA() string { return "judge-prompt-sha-abc" }
func (f *fakeJudge) Model() string     { return "claude-sonnet-4-5-20251022" }

func writeFixtureJSONL(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

const oneReasonerTrace = `{"id":"reasoner.001","feature":"reasoner","input":{"user":"hi"},"expected":{"must_contain":["hello"]}}` + "\n"

func TestHarness_Run_EmitsDeltaTable(t *testing.T) {
	dir := t.TempDir()
	feat := writeFixtureJSONL(t, dir, "reasoner.jsonl", oneReasonerTrace)

	h := &Harness{
		Head:     &fakeReasoner{text: "hello world", cost: 0.001},
		Base:     &fakeReasoner{text: "hello world", cost: 0.001},
		Judge:    &fakeJudge{pass: true, score: 0.9},
		BaseSHA:  "abc123",
		CacheDir: t.TempDir(),
	}
	dt, err := h.Run(context.Background(), feat, feat)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(dt.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(dt.Rows))
	}
	row := dt.Rows[0]
	if row.Feature != "reasoner" {
		t.Errorf("feature=%q", row.Feature)
	}
	if row.HeadPass != 1 || row.BasePass != 1 || row.Total != 1 {
		t.Errorf("expected 1/1/1, got head=%d base=%d total=%d", row.HeadPass, row.BasePass, row.Total)
	}
	if row.DeltaPP != 0 {
		t.Errorf("delta_pp=%v want 0", row.DeltaPP)
	}
}

func TestHarness_CacheKey_IncludesJudgePromptSHA(t *testing.T) {
	r := JudgeRequest{TraceID: "reasoner.001"}
	k1 := CacheKey("base-sha-1", r, "judge-sha-a", "model-x")
	k2 := CacheKey("base-sha-1", r, "judge-sha-b", "model-x")
	k3 := CacheKey("base-sha-1", r, "judge-sha-a", "model-y")
	k4 := CacheKey("base-sha-2", r, "judge-sha-a", "model-x")
	if k1 == k2 {
		t.Errorf("judge prompt sha must change key: %q == %q", k1, k2)
	}
	if k1 == k3 {
		t.Errorf("judge model must change key: %q == %q", k1, k3)
	}
	if k1 == k4 {
		t.Errorf("base sha must change key: %q == %q", k1, k4)
	}
	if k1 == "" || len(k1) != 64 {
		t.Errorf("want 64-char hex sha256, got %q", k1)
	}
}

func TestHarness_BudgetExceeded_BlocksRemainingFeatures(t *testing.T) {
	dir := t.TempDir()
	// Two single-trace features; budget allows the first to score and
	// then refuses the second so we see one feature with results and
	// the second marked budget_exhausted.
	feat1 := writeFixtureJSONL(t, dir, "reasoner.jsonl", oneReasonerTrace)
	feat2 := writeFixtureJSONL(t, dir, "recommend.jsonl",
		`{"id":"recommend.001","feature":"recommend","input":{"q":"x"},"expected":{"must_contain":["y"]}}`+"\n")

	t.Setenv("LEAH_EVAL_BUDGET_DOLLARS", "0.0005") // one judge call costs $0.001
	h := &Harness{
		Head:     &fakeReasoner{text: "y hello", cost: 0},
		Base:     &fakeReasoner{text: "y hello", cost: 0},
		Judge:    &fakeJudge{pass: true, score: 0.9, cost: 0.001},
		BaseSHA:  "deadbeef",
		CacheDir: t.TempDir(),
	}
	dt, err := h.RunAll(context.Background(), []string{feat1, feat2}, []string{feat1, feat2})
	if err == nil {
		t.Fatalf("want budget-exceeded error, got nil")
	}
	if !strings.Contains(err.Error(), "budget") {
		t.Errorf("unexpected err: %v", err)
	}
	// Second feature must surface as budget-exhausted in the table.
	if len(dt.Rows) < 2 {
		t.Fatalf("want 2 rows even on partial exhaustion, got %d", len(dt.Rows))
	}
	if !dt.Rows[1].BudgetExhausted {
		t.Errorf("second feature should be marked BudgetExhausted")
	}
}

// TestHarness_JudgeVerdictGatesHeadPass — regression for 🔴-3: previously
// HeadPass was incremented whenever the hard gate passed; the judge soft
// score was thrown away. A score below PassScoreThreshold must NOT count
// as a HEAD pass.
func TestHarness_JudgeVerdictGatesHeadPass(t *testing.T) {
	dir := t.TempDir()
	feat := writeFixtureJSONL(t, dir, "reasoner.jsonl", oneReasonerTrace)
	h := &Harness{
		Head:    &fakeReasoner{text: "hello world", cost: 0},
		Base:    &fakeReasoner{text: "hello world", cost: 0},
		Judge:   &fakeJudge{pass: false, score: 0.2, cost: 0.001},
		BaseSHA: "abc",
	}
	dt, err := h.Run(context.Background(), feat, feat)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if dt.Rows[0].HeadPass != 0 || dt.Rows[0].BasePass != 0 {
		t.Errorf("low-score judge must not count as pass; got head=%d base=%d",
			dt.Rows[0].HeadPass, dt.Rows[0].BasePass)
	}
}

// TestHarness_JudgeRequest_CarriesInputAndExpected — regression for 🔴-2:
// the JudgeRequest must carry tr.Input and tr.Expected so the prompt
// template has something to score against.
func TestHarness_JudgeRequest_CarriesInputAndExpected(t *testing.T) {
	dir := t.TempDir()
	feat := writeFixtureJSONL(t, dir, "reasoner.jsonl", oneReasonerTrace)
	cap := &captureJudge{}
	h := &Harness{
		Head:    &fakeReasoner{text: "hello world"},
		Base:    &fakeReasoner{text: "hello world"},
		Judge:   cap,
		BaseSHA: "abc",
	}
	if _, err := h.Run(context.Background(), feat, feat); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if cap.last.Input == "" {
		t.Errorf("JudgeRequest.Input empty; want trace input wired through")
	}
	if cap.last.Expected == "" {
		t.Errorf("JudgeRequest.Expected empty; want trace expected wired through")
	}
	if !strings.Contains(cap.last.Input, "hi") {
		t.Errorf("Input should contain the trace input payload; got %q", cap.last.Input)
	}
	if !strings.Contains(cap.last.Expected, "hello") {
		t.Errorf("Expected should contain must_contain entries; got %q", cap.last.Expected)
	}
}

// captureJudge records the last JudgeRequest so tests can assert wiring.
type captureJudge struct {
	last JudgeRequest
}

func (c *captureJudge) Score(_ context.Context, r JudgeRequest) (JudgeResult, float64, error) {
	c.last = r
	return JudgeResult{Pass: true, Score: 0.95}, 0, nil
}
func (c *captureJudge) PromptSHA() string { return "x" }
func (c *captureJudge) Model() string     { return "x" }

// TestHardFail_EnforcesToolCalled — regression for 🟡-7: the tool_called
// hard constraint must block a pass when the actual output does not name
// the expected tool. Per spec §4.3 reasoner row, tool_called is one of
// the hard gates.
func TestHardFail_EnforcesToolCalled(t *testing.T) {
	e := Expected{ToolCalled: "dispatch"}
	if hardFail(e, "I will run the gmail cleanup wave") == "" {
		t.Errorf("missing tool_called must hard-fail")
	}
	if got := hardFail(e, "calling dispatch tool for gmail"); got != "" {
		t.Errorf("tool_called present should pass; got %q", got)
	}
	// Unset → unconstrained.
	if hardFail(Expected{}, "anything goes") != "" {
		t.Errorf("empty tool_called must not gate")
	}
}

// TestHarness_JudgeUnparseable_AggregatedAndBlocks — regression for 🟡-6:
// the judge_unparseable signal must be counted per row and trigger a
// FAIL verdict when the rate crosses 5% (spec §6.3).
func TestHarness_JudgeUnparseable_AggregatedAndBlocks(t *testing.T) {
	dir := t.TempDir()
	feat := writeFixtureJSONL(t, dir, "reasoner.jsonl", oneReasonerTrace)
	h := &Harness{
		Head:    &fakeReasoner{text: "hello world"},
		Base:    &fakeReasoner{text: "hello world"},
		Judge:   &fakeJudge{pass: false, score: 0, reason: "judge_unparseable"},
		BaseSHA: "abc",
	}
	dt, err := h.Run(context.Background(), feat, feat)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if dt.Rows[0].JudgeUnparseable < 1 {
		t.Errorf("want JudgeUnparseable counted; got %d", dt.Rows[0].JudgeUnparseable)
	}
}

func TestJudge_HardConstraint_ReturnsFail(t *testing.T) {
	// Even if the soft score is high, missing must_contain → hard fail
	// without a judge call (saves budget per spec §4.3).
	j := &fakeJudge{pass: true, score: 0.99}
	res := EvaluateTrace(
		context.Background(),
		Trace{
			ID: "reasoner.001", Feature: "reasoner",
			Expected: Expected{MustContain: []string{"banana"}},
		},
		"this output has no fruit at all",
		j,
	)
	if res.Pass {
		t.Errorf("hard constraint violated; want fail")
	}
	if j.calls != 0 {
		t.Errorf("judge should NOT be called on hard fail; calls=%d", j.calls)
	}
	if res.Reason == "" || !strings.Contains(res.Reason, "must_contain") {
		t.Errorf("reason should cite must_contain; got %q", res.Reason)
	}
}
