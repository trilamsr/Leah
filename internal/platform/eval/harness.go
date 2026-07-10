// Package eval is Leah's LLM-as-judge regression harness. The Harness runs
// a feature's golden traces against both HEAD and BASE code paths, scores
// the outputs with a pinned judge model, and emits a delta table the merge
// gate consumes. Budget for judge calls is tracked separately from the
// per-process internal/budget ceiling — see Spec §8.1.
package eval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// PassScoreThreshold matches spec §4.2 — a trace passes when the judge
	// soft score crosses 0.8 AND all hard constraints are satisfied.
	PassScoreThreshold = 0.8

	// DefaultBudgetDollars is the per-run cap when LEAH_EVAL_BUDGET_DOLLARS
	// is unset; matches spec §8.1 ($3 / run).
	DefaultBudgetDollars = 3.0
)

// Trace is one row of evals/<feature>.jsonl. Field set matches spec §3.1
// envelope; DisallowUnknownFields enforces that operators cannot smuggle
// off-schema keys.
type Trace struct {
	ID       string          `json:"id"`
	Feature  string          `json:"feature"`
	AddedAt  string          `json:"added_at,omitempty"`
	AddedBy  string          `json:"added_by,omitempty"`
	Input    json.RawMessage `json:"input"`
	Expected Expected        `json:"expected"`
	Rubric   string          `json:"rubric,omitempty"`
	Tags     []string        `json:"tags,omitempty"`
	Skip     string          `json:"skip_reason,omitempty"`
}

// Expected captures the per-feature pass condition (hard + soft).
type Expected struct {
	MustContain    []string `json:"must_contain,omitempty"`
	MustNotContain []string `json:"must_not_contain,omitempty"`
	ToolCalled     string   `json:"tool_called,omitempty"`
	RubricNotes    string   `json:"rubric_notes,omitempty"`
}

// Asker is the minimal subset of *reasoner.Reasoner the harness depends on
// — exposed as an interface so tests can swap a hermetic fake without
// touching the Anthropic SDK.
type Asker interface {
	Ask(ctx context.Context, user string) (string, error)
}

// Judge scores one trace pair. Cost (USD) is returned separately so the
// harness can charge the per-run budget independently from the judge's
// internal accounting.
type Judge interface {
	Score(ctx context.Context, r JudgeRequest) (JudgeResult, float64, error)
	PromptSHA() string
	Model() string
}

// JudgeRequest is the rendered prompt-context for one trace evaluation.
type JudgeRequest struct {
	Feature  string
	TraceID  string
	RubricID string
	Input    string
	Actual   string
	Expected string
}

// JudgeResult is the parsed judge JSON line.
type JudgeResult struct {
	Pass   bool    `json:"pass"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// JudgeRubric carries the hard+soft pass criteria for one trace; the
// harness applies the hard gate in-code before calling the judge so a
// pure-code failure does not spend budget (spec §4.3).
type JudgeRubric struct {
	Expected  Expected
	Threshold float64
}

// TraceResult is one row's verdict after hard gate + (optional) judge call.
type TraceResult struct {
	TraceID         string
	Pass            bool
	Score           float64
	Reason          string
	BudgetExhausted bool
}

// DeltaRow is one feature's BASE vs HEAD aggregate. Mirrors spec §5.3.
// JudgeUnparseable counts both sides; spec §6.3 blocks merge when the
// rate exceeds 5% of total judge calls.
type DeltaRow struct {
	Feature          string
	BasePass         int
	HeadPass         int
	Total            int
	DeltaPP          float64
	BudgetExhausted  bool
	JudgeUnparseable int
	JudgeCalls       int
}

// DeltaTable is the emitter contract — one row per feature plus a derived
// overall row produced by Render.
type DeltaTable struct {
	Rows []DeltaRow
}

// Harness runs feature evals. Head + Base are pre-built reasoners pointing
// at the two code paths under comparison; Judge is the pinned scorer.
type Harness struct {
	Head     Asker
	Base     Asker
	Judge    Judge
	BaseSHA  string
	CacheDir string
}

// budgetFromEnv reads LEAH_EVAL_BUDGET_DOLLARS, falling back to the spec
// default. Kept separate from internal/budget so eval CI cannot starve the
// daemon's own per-process ceiling.
func budgetFromEnv() float64 {
	if v := os.Getenv("LEAH_EVAL_BUDGET_DOLLARS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return DefaultBudgetDollars
}

// Run evaluates a single feature file. featurePath and basePath may point
// at the same JSONL — the harness runs each trace through Head + Base
// asker and judges both sides.
func (h *Harness) Run(ctx context.Context, featurePath, basePath string) (DeltaTable, error) {
	return h.RunAll(ctx, []string{featurePath}, []string{basePath})
}

// RunAll evaluates multiple feature files in order. On a budget-exceeded
// error mid-run, remaining features land in the table with
// BudgetExhausted=true and the error is returned — partial results help
// the operator see how far the run got before the cap tripped.
func (h *Harness) RunAll(ctx context.Context, featurePaths, basePaths []string) (DeltaTable, error) {
	if len(featurePaths) != len(basePaths) {
		return DeltaTable{}, fmt.Errorf("eval: featurePaths/basePaths length mismatch")
	}
	cap := budgetFromEnv()
	spent := 0.0
	dt := DeltaTable{}
	var budgetErr error

	for i, fp := range featurePaths {
		bp := basePaths[i]
		feature := strings.TrimSuffix(filepath.Base(fp), ".jsonl")
		row := DeltaRow{Feature: feature}
		if budgetErr != nil {
			row.BudgetExhausted = true
			dt.Rows = append(dt.Rows, row)
			continue
		}
		traces, err := loadTraces(fp)
		if err != nil {
			return dt, fmt.Errorf("eval: load %s: %w", fp, err)
		}
		baseTraces, err := loadTraces(bp)
		if err != nil {
			return dt, fmt.Errorf("eval: load base %s: %w", bp, err)
		}

		for ti, tr := range traces {
			if tr.Skip != "" {
				continue
			}
			row.Total++

			// TODO: swap to a real BASE checkout; currently Base.Ask runs on
			// the same code path. The dual-asker shape is wired so the
			// checkout is a non-breaking addition.
			headActual, err := h.Head.Ask(ctx, string(tr.Input))
			if err != nil {
				return dt, fmt.Errorf("eval: head ask %s: %w", tr.ID, err)
			}
			// Hard-gate check runs before budget so a code-detectable
			// failure never burns budget.
			if reason := hardFail(tr.Expected, headActual); reason == "" {
				// 0.001 = nominal one-call reservation against the per-run cap.
				if spent+0.001 > cap {
					budgetErr = fmt.Errorf("eval: budget exceeded")
					break
				}
				res, cost, jerr := h.Judge.Score(ctx, buildJudgeRequest(tr, headActual))
				if jerr != nil {
					return dt, fmt.Errorf("eval: judge %s: %w", tr.ID, jerr)
				}
				spent += cost
				row.JudgeCalls++
				if res.Reason == "judge_unparseable" {
					row.JudgeUnparseable++
				}
				if res.Pass && res.Score >= PassScoreThreshold {
					row.HeadPass++
				}
			}

			// BASE side — same flow, against the prior code path.
			var btr Trace
			if ti < len(baseTraces) {
				btr = baseTraces[ti]
			} else {
				btr = tr
			}
			baseActual, err := h.Base.Ask(ctx, string(btr.Input))
			if err != nil {
				return dt, fmt.Errorf("eval: base ask %s: %w", tr.ID, err)
			}
			if reason := hardFail(btr.Expected, baseActual); reason == "" {
				if spent+0.001 > cap {
					budgetErr = fmt.Errorf("eval: budget exceeded")
					break
				}
				res, cost, jerr := h.Judge.Score(ctx, buildJudgeRequest(btr, baseActual))
				if jerr != nil {
					return dt, fmt.Errorf("eval: judge %s: %w", btr.ID, jerr)
				}
				spent += cost
				row.JudgeCalls++
				if res.Reason == "judge_unparseable" {
					row.JudgeUnparseable++
				}
				if res.Pass && res.Score >= PassScoreThreshold {
					row.BasePass++
				}
			}
		}
		if row.Total > 0 {
			row.DeltaPP = 100.0 * float64(row.HeadPass-row.BasePass) / float64(row.Total)
		}
		if budgetErr != nil {
			row.BudgetExhausted = true
		}
		dt.Rows = append(dt.Rows, row)
	}
	return dt, budgetErr
}

func loadTraces(path string) ([]Trace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Trace
	for ln, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var t Trace
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&t); err != nil {
			return nil, fmt.Errorf("line %d: %w", ln+1, err)
		}
		out = append(out, t)
	}
	return out, nil
}

// hardFail returns the first violated hard constraint as a human string,
// or "" when all hard constraints pass. The judge is only invoked when
// this returns "". tool_called is checked as a substring of actual —
// matches the spec §4.3 reasoner row (tool_called == expected ∨ unset)
// and the bootstrap-trace shape where the response names the tool.
func hardFail(e Expected, actual string) string {
	for _, s := range e.MustContain {
		if !strings.Contains(actual, s) {
			return fmt.Sprintf("must_contain missing: %q", s)
		}
	}
	for _, s := range e.MustNotContain {
		if strings.Contains(actual, s) {
			return fmt.Sprintf("must_not_contain hit: %q", s)
		}
	}
	if e.ToolCalled != "" && !strings.Contains(actual, e.ToolCalled) {
		return fmt.Sprintf("tool_called missing: %q", e.ToolCalled)
	}
	return ""
}

// buildJudgeRequest renders the trace's Input and Expected into the strings
// the judge prompt template substitutes for {{.Input}} / {{.Expected}}. The
// raw JSON shape is preserved — the judge then has both the user input and
// the rubric (must_contain, must_not_contain, tool_called, rubric_notes)
// in front of it when scoring.
func buildJudgeRequest(tr Trace, actual string) JudgeRequest {
	expectedBytes, _ := json.Marshal(tr.Expected)
	return JudgeRequest{
		Feature:  tr.Feature,
		TraceID:  tr.ID,
		RubricID: tr.Rubric,
		Input:    string(tr.Input),
		Actual:   actual,
		Expected: string(expectedBytes),
	}
}

// EvaluateTrace applies the hard gate, and only calls the judge when the
// hard gate passes — mirrors spec §4.3 ("no judge call needed when hard
// fails — saves budget").
func EvaluateTrace(ctx context.Context, tr Trace, actual string, j Judge) TraceResult {
	if reason := hardFail(tr.Expected, actual); reason != "" {
		return TraceResult{TraceID: tr.ID, Pass: false, Reason: reason}
	}
	res, _, err := j.Score(ctx, buildJudgeRequest(tr, actual))
	if err != nil {
		return TraceResult{TraceID: tr.ID, Pass: false, Reason: "judge_error: " + err.Error()}
	}
	pass := res.Pass && res.Score >= PassScoreThreshold
	return TraceResult{TraceID: tr.ID, Pass: pass, Score: res.Score, Reason: res.Reason}
}

// CacheKey is sha256(base_sha || trace_id || judge_prompt_sha || judge_model).
// Editing prompts/eval-judge.md OR bumping JudgeModel MUST bust the cache —
// both inputs are folded in to enforce that. On-disk read/write lookup that
// skips re-judging cached BASE results lands with the real BASE-checkout
// asker.
func CacheKey(baseSHA string, r JudgeRequest, judgePromptSHA, judgeModel string) string {
	h := sha256.New()
	h.Write([]byte(baseSHA))
	h.Write([]byte{0})
	h.Write([]byte(r.TraceID))
	h.Write([]byte{0})
	h.Write([]byte(judgePromptSHA))
	h.Write([]byte{0})
	h.Write([]byte(judgeModel))
	return hex.EncodeToString(h.Sum(nil))
}
