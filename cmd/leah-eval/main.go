// leah-eval is the make-target driver for the eval harness. Phase-1 scope
// (W82): renders a delta table for one or all features against the same
// JSONL on both sides — BASE-checkout wiring lands in a later wave.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/trilam/leah/internal/eval"
)

func main() {
	var feature, base, evalsDir string
	flag.StringVar(&feature, "feature", "", "single feature name, or empty for all")
	flag.StringVar(&base, "base", "origin/main", "base ref (reserved; phase-1 reads same JSONL on both sides)")
	flag.StringVar(&evalsDir, "evals-dir", "evals", "directory holding <feature>.jsonl")
	flag.Parse()

	paths, err := resolveFeatures(evalsDir, feature)
	if err != nil {
		fmt.Fprintln(os.Stderr, "leah-eval:", err)
		os.Exit(2)
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "leah-eval: no feature JSONL files matched")
		os.Exit(2)
	}

	// Phase-1: stub asker + stub judge so `make eval` exits cleanly without
	// a live API key. The wiring point for the real reasoner + judge lands
	// in W83 when the first non-trivial evaluation runs in CI.
	h := &eval.Harness{
		Head:    stubAsker{},
		Base:    stubAsker{},
		Judge:   stubJudge{},
		BaseSHA: base,
	}
	dt, err := h.RunAll(context.Background(), paths, paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "leah-eval:", err)
		os.Exit(1)
	}
	fmt.Println(renderTable(dt))
}

func resolveFeatures(dir, feature string) ([]string, error) {
	if feature != "" {
		p := filepath.Join(dir, feature+".jsonl")
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("feature %q not found at %s", feature, p)
		}
		return []string{p}, nil
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// renderTable is the markdown delta table from spec §5.3 (header row +
// per-feature row); overall row is appended.
func renderTable(dt eval.DeltaTable) string {
	var b strings.Builder
	b.WriteString("| Feature | Base pass | HEAD pass | Δ pp | Verdict |\n")
	b.WriteString("|---------|-----------|-----------|------|---------|\n")
	var totBase, totHead, totN int
	for _, r := range dt.Rows {
		verdict := "PASS"
		if r.BudgetExhausted {
			verdict = "FAIL"
		}
		fmt.Fprintf(&b, "| %s | %d/%d | %d/%d | %+.1f | %s |\n",
			r.Feature, r.BasePass, r.Total, r.HeadPass, r.Total, r.DeltaPP, verdict)
		totBase += r.BasePass
		totHead += r.HeadPass
		totN += r.Total
	}
	if totN > 0 {
		delta := 100.0 * float64(totHead-totBase) / float64(totN)
		fmt.Fprintf(&b, "| **overall** | %d/%d | %d/%d | %+.1f | PASS |\n",
			totBase, totN, totHead, totN, delta)
	}
	return b.String()
}

type stubAsker struct{}

func (stubAsker) Ask(_ context.Context, _ string) (string, error) {
	return "stub-actual", nil
}

type stubJudge struct{}

func (stubJudge) Score(_ context.Context, _ eval.JudgeRequest) (eval.JudgeResult, float64, error) {
	return eval.JudgeResult{Pass: true, Score: 0.9, Reason: "stub"}, 0, nil
}
func (stubJudge) PromptSHA() string { return "stub" }
func (stubJudge) Model() string     { return eval.JudgeModel }
