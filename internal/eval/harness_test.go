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

func TestJudge_HardConstraint_ReturnsFail(t *testing.T) {
	// Even if the soft score is high, missing must_contain → hard fail
	// without a judge call (saves budget per spec §4.3).
	j := &fakeJudge{pass: true, score: 0.99}
	res := EvaluateTrace(
		Trace{
			ID: "reasoner.001", Feature: "reasoner",
			Expected: Expected{MustContain: []string{"banana"}},
		},
		"this output has no fruit at all",
		j,
		context.Background(),
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
