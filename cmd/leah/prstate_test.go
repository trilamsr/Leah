package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// fakeExec is an in-memory gh stand-in. Indexed by the joined arg string so a
// single test can stub `gh pr view` + `gh pr list` independently without
// re-implementing arg-matching logic.
type fakeExec struct {
	responses map[string]string
	errs      map[string]error
}

func (f *fakeExec) Run(_ context.Context, args []string) (string, error) {
	key := strings.Join(args, " ")
	if err, ok := f.errs[key]; ok {
		return "", err
	}
	if out, ok := f.responses[key]; ok {
		return out, nil
	}
	// Fallback: best-effort prefix match so tests don't have to spell out
	// every gh flag. Stable order via the responses map's iteration is
	// fine for the small surfaces under test.
	for k, v := range f.responses {
		if strings.HasPrefix(key, k) {
			return v, nil
		}
	}
	return "", nil
}

// TestFormatPRLine pins the one-line shape the operator reads. Order matters
// (left-justified columns) — re-ordering breaks the operator's eye-scan
// muscle memory across many sessions.
func TestFormatPRLine(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	pr := prSummary{
		Number:            289,
		Title:             "feat(cli): leah news --bundle ai|tech",
		State:             "OPEN",
		MergeStateStatus:  "CLEAN",
		ReviewDecision:    "APPROVED",
		ChecksConclusion:  "SUCCESS",
		Author:            "trilam",
		CreatedAt:         now.Add(-2 * time.Hour).Format(time.RFC3339),
		IsDraft:           false,
		Mergeable:         "MERGEABLE",
	}
	got := formatPRLine(pr, now)
	// Required substrings — exact column widths can drift; what must NOT
	// drift is which signals are visible.
	for _, want := range []string{"#289", "OPEN", "CLEAN", "APPROVED", "✓", "trilam", "2h"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatPRLine missing %q in %q", want, got)
		}
	}
}

func TestFormatPRLine_FailingChecks(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	pr := prSummary{
		Number:           42,
		Title:            "fix: thing",
		State:            "OPEN",
		MergeStateStatus: "BLOCKED",
		ChecksConclusion: "FAILURE",
		Author:           "bob",
		CreatedAt:        now.Add(-30 * time.Minute).Format(time.RFC3339),
	}
	got := formatPRLine(pr, now)
	if !strings.Contains(got, "✗") {
		t.Errorf("failing checks should render ✗; got %q", got)
	}
	if !strings.Contains(got, "BLOCKED") {
		t.Errorf("BLOCKED merge state should appear; got %q", got)
	}
}

func TestFormatPRLine_PendingChecks(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	pr := prSummary{
		Number:           7,
		Title:            "wip",
		State:            "OPEN",
		ChecksConclusion: "",
		Author:           "alice",
		CreatedAt:        now.Add(-15 * time.Minute).Format(time.RFC3339),
		IsDraft:          true,
	}
	got := formatPRLine(pr, now)
	if !strings.Contains(got, "⋯") {
		t.Errorf("pending checks should render ⋯; got %q", got)
	}
	if !strings.Contains(got, "DRAFT") {
		t.Errorf("draft PR should be flagged; got %q", got)
	}
}

func TestFetchPRDetail(t *testing.T) {
	body := map[string]any{
		"number":           289,
		"title":            "feat(cli): leah news",
		"state":            "OPEN",
		"mergeStateStatus": "CLEAN",
		"reviewDecision":   "APPROVED",
		"author":           map[string]any{"login": "trilam"},
		"createdAt":        "2026-06-21T10:00:00Z",
		"isDraft":          false,
		"mergeable":        "MERGEABLE",
		"statusCheckRollup": []any{
			map[string]any{"conclusion": "SUCCESS"},
			map[string]any{"conclusion": "SUCCESS"},
		},
	}
	raw, _ := json.Marshal(body)
	fx := &fakeExec{responses: map[string]string{"gh pr view 289": string(raw)}}
	got, err := fetchPRDetail(context.Background(), fx, "trilamsr/Leah", 289)
	if err != nil {
		t.Fatalf("fetchPRDetail: %v", err)
	}
	if got.Number != 289 {
		t.Errorf("Number = %d, want 289", got.Number)
	}
	if got.Author != "trilam" {
		t.Errorf("Author = %q, want trilam", got.Author)
	}
	if got.ChecksConclusion != "SUCCESS" {
		t.Errorf("ChecksConclusion = %q, want SUCCESS", got.ChecksConclusion)
	}
}

func TestFetchPRDetail_RolledUpFailure(t *testing.T) {
	// One failure in the rollup = overall FAILURE; mirrors gh's own behavior
	// so the operator sees the same verdict the merge gate enforces.
	body := map[string]any{
		"number": 1,
		"title":  "x",
		"state":  "OPEN",
		"author": map[string]any{"login": "a"},
		"statusCheckRollup": []any{
			map[string]any{"conclusion": "SUCCESS"},
			map[string]any{"conclusion": "FAILURE"},
		},
	}
	raw, _ := json.Marshal(body)
	fx := &fakeExec{responses: map[string]string{"gh pr view 1": string(raw)}}
	got, err := fetchPRDetail(context.Background(), fx, "r/x", 1)
	if err != nil {
		t.Fatalf("fetchPRDetail: %v", err)
	}
	if got.ChecksConclusion != "FAILURE" {
		t.Errorf("ChecksConclusion = %q, want FAILURE", got.ChecksConclusion)
	}
}

func TestFetchPRDetail_PendingRollup(t *testing.T) {
	body := map[string]any{
		"number": 1,
		"title":  "x",
		"state":  "OPEN",
		"author": map[string]any{"login": "a"},
		"statusCheckRollup": []any{
			map[string]any{"conclusion": "SUCCESS"},
			map[string]any{"conclusion": ""},
		},
	}
	raw, _ := json.Marshal(body)
	fx := &fakeExec{responses: map[string]string{"gh pr view 1": string(raw)}}
	got, _ := fetchPRDetail(context.Background(), fx, "r/x", 1)
	if got.ChecksConclusion != "PENDING" {
		t.Errorf("ChecksConclusion = %q, want PENDING", got.ChecksConclusion)
	}
}

func TestRankQueue(t *testing.T) {
	// Higher readiness ranks first. APPROVED+CLEAN+SUCCESS = top; FAILURE = bottom.
	prs := []prSummary{
		{Number: 1, ReviewDecision: "REVIEW_REQUIRED", MergeStateStatus: "BLOCKED", ChecksConclusion: "SUCCESS"},
		{Number: 2, ReviewDecision: "APPROVED", MergeStateStatus: "CLEAN", ChecksConclusion: "SUCCESS"},
		{Number: 3, ReviewDecision: "APPROVED", MergeStateStatus: "CLEAN", ChecksConclusion: "FAILURE"},
		{Number: 4, ReviewDecision: "APPROVED", MergeStateStatus: "UNSTABLE", ChecksConclusion: "SUCCESS"},
	}
	rankQueue(prs)
	if prs[0].Number != 2 {
		t.Errorf("rank[0] = #%d, want #2 (the only fully-clean approved)", prs[0].Number)
	}
	if prs[len(prs)-1].Number != 3 {
		t.Errorf("rank[last] = #%d, want #3 (CI failure)", prs[len(prs)-1].Number)
	}
}

func TestRunPRStateUsage(t *testing.T) {
	var buf strings.Builder
	rc := runPRState(context.Background(), nil, []string{"--help"}, &buf)
	if rc != 0 {
		t.Errorf("--help exit = %d, want 0", rc)
	}
	if !strings.Contains(buf.String(), "usage:") {
		t.Errorf("--help output missing usage line: %q", buf.String())
	}
}

func TestRunPRStateBadArg(t *testing.T) {
	var buf strings.Builder
	rc := runPRState(context.Background(), nil, []string{"not-a-number"}, &buf)
	if rc != 2 {
		t.Errorf("bad arg exit = %d, want 2", rc)
	}
}

func TestRunPRStateMultipleNumbersRejected(t *testing.T) {
	var buf strings.Builder
	rc := runPRState(context.Background(), nil, []string{"1", "2"}, &buf)
	if rc != 2 {
		t.Errorf("two PR numbers exit = %d, want 2 (silent-overwrite bug)", rc)
	}
}

func TestRunPRStateNumberWithModeRejected(t *testing.T) {
	var buf strings.Builder
	rc := runPRState(context.Background(), nil, []string{"1", "--open"}, &buf)
	if rc != 2 {
		t.Errorf("<N> + --open exit = %d, want 2 (silent-drop bug)", rc)
	}
	var buf2 strings.Builder
	rc2 := runPRState(context.Background(), nil, []string{"--queue", "7"}, &buf2)
	if rc2 != 2 {
		t.Errorf("--queue + <N> exit = %d, want 2", rc2)
	}
}
