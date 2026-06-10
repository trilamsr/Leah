package eval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	// JudgeModel is pinned per release; spec §4.1. Changing this string
	// busts the eval cache (key includes Model) so stale BASE scores
	// can never serve a different judge.
	JudgeModel = "claude-sonnet-4-5-20251022"

	// JudgeTemperature = 0 makes the pinned judge near-deterministic.
	JudgeTemperature = 0.0

	// JudgeMaxTokens caps the judge response — the prompt forces a single
	// JSON line so 1024 is generous.
	JudgeMaxTokens = 1024
)

// JudgeCompleter is the minimal LLM interface the judge uses. Production
// wiring passes reasoner.AnthropicClient pinned to JudgeModel; tests pass a
// stub.
type JudgeCompleter interface {
	Complete(ctx context.Context, system, user string) (text string, costUSD float64, err error)
}

// LLMJudge renders the eval-judge prompt and parses the strict-JSON reply.
type LLMJudge struct {
	Completer    JudgeCompleter
	PromptTmpl   string // contents of prompts/eval-judge.md
	promptSHAHex string
}

// NewLLMJudge loads the judge prompt from path; the SHA is folded into the
// cache key so an operator edit to the prompt invalidates BASE scores.
func NewLLMJudge(c JudgeCompleter, path string) (*LLMJudge, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("eval: load judge prompt: %w", err)
	}
	sum := sha256.Sum256(b)
	return &LLMJudge{
		Completer:    c,
		PromptTmpl:   string(b),
		promptSHAHex: hex.EncodeToString(sum[:]),
	}, nil
}

// PromptSHA returns the sha256 of the loaded judge prompt template, used
// as one component of the eval cache key (spec §5.2).
func (j *LLMJudge) PromptSHA() string { return j.promptSHAHex }

// Model returns the pinned judge model string for the cache key.
func (j *LLMJudge) Model() string { return JudgeModel }

// Score renders the prompt, calls the completer, and parses the strict
// JSON response. Unparseable replies map to a non-pass result with a
// "judge_unparseable" reason — never silently dropped (spec §4.2).
func (j *LLMJudge) Score(ctx context.Context, r JudgeRequest) (JudgeResult, float64, error) {
	prompt := renderJudgePrompt(j.PromptTmpl, r)
	text, cost, err := j.Completer.Complete(ctx, "", prompt)
	if err != nil {
		return JudgeResult{}, cost, err
	}
	res, ok := parseJudgeReply(text)
	if !ok {
		return JudgeResult{Pass: false, Score: 0, Reason: "judge_unparseable"}, cost, nil
	}
	return res, cost, nil
}

func renderJudgePrompt(tmpl string, r JudgeRequest) string {
	out := tmpl
	out = strings.ReplaceAll(out, "{{.Feature}}", r.Feature)
	out = strings.ReplaceAll(out, "{{.RubricID}}", r.RubricID)
	out = strings.ReplaceAll(out, "{{.Input}}", r.Input)
	out = strings.ReplaceAll(out, "{{.Actual}}", r.Actual)
	out = strings.ReplaceAll(out, "{{.Expected}}", r.Expected)
	return out
}

// parseJudgeReply pulls the first {…}-delimited JSON object out of the
// judge response and decodes it. ok=false on any malformed input.
func parseJudgeReply(text string) (JudgeResult, bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < 0 || end <= start {
		return JudgeResult{}, false
	}
	var r JudgeResult
	if err := json.Unmarshal([]byte(text[start:end+1]), &r); err != nil {
		return JudgeResult{}, false
	}
	return r, true
}
