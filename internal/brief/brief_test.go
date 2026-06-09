package brief

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/operatormodel"
	"github.com/trilam/leah/internal/regattaclient"
)

// TestRenderIncludesRecap asserts the Yesterday section lists per-kind action
// counts and spend when audit rows are present.
func TestRenderIncludesRecap(t *testing.T) {
	d := Data{
		Now:              time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC),
		YesterdayActions: map[string]int{"ask": 4, "ship": 2, "review": 1},
		YesterdaySpend:   1.2345,
	}
	out := Render(d)
	if !strings.Contains(out, "## Yesterday") {
		t.Fatalf("missing Yesterday header:\n%s", out)
	}
	for _, want := range []string{"4 ask", "2 ship", "1 review", "$1.2345"} {
		if !strings.Contains(out, want) {
			t.Errorf("recap missing %q in output:\n%s", want, out)
		}
	}
}

// TestRenderRecapEmptyDay asserts the no-actions branch renders a placeholder
// instead of an empty section.
func TestRenderRecapEmptyDay(t *testing.T) {
	d := Data{Now: time.Now(), YesterdayActions: map[string]int{}}
	out := Render(d)
	if !strings.Contains(out, "no leah actions logged") {
		t.Errorf("expected empty-day placeholder, got:\n%s", out)
	}
}

// TestRenderIncludesBacklog asserts the Regatta backlog section lists the top-3
// active agents and indicates overflow when more exist.
func TestRenderIncludesBacklog(t *testing.T) {
	agents := []regattaclient.Agent{
		{ID: "abc123def456ghi789", Branch: "regatta/agent-1", State: "running", PR: 42},
		{ID: "bbb", Branch: "regatta/agent-2", State: "escalated", PR: 43},
		{ID: "ccc", Branch: "regatta/agent-3", State: "running", PR: 44},
		{ID: "ddd", Branch: "regatta/agent-4", State: "running", PR: 45},
	}
	d := Data{Now: time.Now(), ActiveAgents: agents}
	out := Render(d)
	if !strings.Contains(out, "## Regatta backlog") {
		t.Fatalf("missing Regatta backlog header:\n%s", out)
	}
	for _, want := range []string{"abc123def456", "PR #42", "regatta/agent-2", "regatta/agent-3"} {
		if !strings.Contains(out, want) {
			t.Errorf("backlog missing %q", want)
		}
	}
	if strings.Contains(out, "regatta/agent-4") {
		t.Errorf("backlog should not include 4th agent in top-3 list")
	}
	if !strings.Contains(out, "1 more") {
		t.Errorf("expected overflow marker, got:\n%s", out)
	}
	if strings.Contains(out, "abc123def456ghi789") {
		t.Errorf("expected ID to be truncated to 12 chars")
	}
}

// TestRenderBacklogEmpty asserts the empty-agents branch renders a placeholder.
func TestRenderBacklogEmpty(t *testing.T) {
	d := Data{Now: time.Now()}
	out := Render(d)
	if !strings.Contains(out, "no active agents") {
		t.Errorf("expected empty-agents placeholder, got:\n%s", out)
	}
}

// TestRenderRecommendationsColdStart asserts the not-Ready path explains itself
// instead of silently empty.
func TestRenderRecommendationsColdStart(t *testing.T) {
	d := Data{Now: time.Now(), ModelReady: false}
	out := Render(d)
	if !strings.Contains(out, "operator-model not ready") {
		t.Errorf("expected cold-start message, got:\n%s", out)
	}
}

// TestRenderRecommendationsListed asserts ready+nonempty recs print numbered.
func TestRenderRecommendationsListed(t *testing.T) {
	d := Data{
		Now:        time.Now(),
		ModelReady: true,
		Recommendations: []operatormodel.Recommendation{
			{Kind: "ship", Reason: "typical at 09:00", Weight: 3.2},
			{Kind: "review", Reason: "Mon cadence", Weight: 1.5},
		},
	}
	out := Render(d)
	if !strings.Contains(out, "1. ship") || !strings.Contains(out, "2. review") {
		t.Errorf("expected numbered recs, got:\n%s", out)
	}
}

// TestRenderCostOutlook asserts WTD + projected monthly are printed.
func TestRenderCostOutlook(t *testing.T) {
	d := Data{
		Now:              time.Now(),
		WeekToDateUSD:    7.0,
		ProjectedMonthly: 30.0,
	}
	out := Render(d)
	if !strings.Contains(out, "$7.0000") || !strings.Contains(out, "$30.0000") {
		t.Errorf("missing cost numbers in:\n%s", out)
	}
}

// TestRenderBugFixCount asserts the queued-count branch renders the number
// when >0 and a placeholder otherwise.
func TestRenderBugFixCount(t *testing.T) {
	out := Render(Data{Now: time.Now(), BugFixCount: 3})
	if !strings.Contains(out, "3 queued") {
		t.Errorf("expected count, got:\n%s", out)
	}
	out2 := Render(Data{Now: time.Now()})
	if !strings.Contains(out2, "none queued") {
		t.Errorf("expected none-queued, got:\n%s", out2)
	}
}

// TestYesterdayBoundsLocalMidnight asserts [start,end) is exactly the previous
// calendar day in local TZ, not a 24h sliding window.
func TestYesterdayBoundsLocalMidnight(t *testing.T) {
	now := time.Date(2026, 6, 9, 14, 30, 0, 0, time.UTC)
	start, end := YesterdayBounds(now)
	wantStart := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Errorf("got [%v, %v), want [%v, %v)", start, end, wantStart, wantEnd)
	}
}

// TestScanYesterdayFiltersByBounds asserts only rows with ts in [start, end)
// are counted, malformed lines skipped, missing file returns empty.
func TestScanYesterdayFiltersByBounds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	lines := strings.Join([]string{
		`{"ts":"2026-06-08T10:00:00Z","kind":"ask","cost_dollars":0.5}`,
		`{"ts":"2026-06-08T22:00:00Z","kind":"ship","cost_dollars":1.0}`,
		`{"ts":"2026-06-08T22:00:00Z","kind":"ask","cost_dollars":0.25}`,
		`{"ts":"2026-06-07T10:00:00Z","kind":"ask","cost_dollars":99.0}`, // before
		`{"ts":"2026-06-09T10:00:00Z","kind":"ask","cost_dollars":99.0}`, // after
		`{garbage`, // skipped
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	counts, spend := ScanYesterday(path, start, end)
	if counts["ask"] != 2 || counts["ship"] != 1 {
		t.Errorf("counts: %v", counts)
	}
	if spend < 1.74 || spend > 1.76 {
		t.Errorf("spend = %v, want ~1.75", spend)
	}
	c2, s2 := ScanYesterday(filepath.Join(dir, "nope.jsonl"), start, end)
	if len(c2) != 0 || s2 != 0 {
		t.Errorf("missing file should be empty, got %v %v", c2, s2)
	}
}

// TestFilterActiveAgentsKeepsActiveOnly asserts only running + escalated
// survive — pending/terminal states drop.
func TestFilterActiveAgentsKeepsActiveOnly(t *testing.T) {
	in := []regattaclient.Agent{
		{ID: "1", State: "running"},
		{ID: "2", State: "merged"},
		{ID: "3", State: "escalated"},
		{ID: "4", State: "pending"},
		{ID: "5", State: "failed"},
	}
	out := FilterActiveAgents(in)
	if len(out) != 2 || out[0].ID != "1" || out[1].ID != "3" {
		t.Errorf("got %v, want [1, 3]", out)
	}
}

// TestCountBugFixCandidatesH2 asserts H2 headers (## ) are counted; H1 (# )
// and H3 (### ) are not.
func TestCountBugFixCandidatesH2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bug-fix-candidates.md")
	body := strings.Join([]string{
		"# header one",
		"## candidate 2026-06-01 panic-rate",
		"some text",
		"## candidate 2026-06-08 panic-rate",
		"### sub",
		"more text",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if n := CountBugFixCandidates(path); n != 2 {
		t.Errorf("got %d, want 2", n)
	}
	if n := CountBugFixCandidates(filepath.Join(dir, "missing.md")); n != 0 {
		t.Errorf("missing file should be 0, got %d", n)
	}
}

// TestWriteFileWritesAndOverwrites asserts WriteFile writes to
// briefs/YYYY-MM-DD.md and overwrites on second call (idempotent per day).
func TestWriteFileWritesAndOverwrites(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC)
	body := "# test brief\nhello\n"
	if err := WriteFile(dir, now, body); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "briefs", "2026-06-09.md")
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != body {
		t.Errorf("file mismatch: got %q want %q", got, body)
	}
	if err := WriteFile(dir, now, "second"); err != nil {
		t.Fatal(err)
	}
	got2, _ := os.ReadFile(want)
	if string(got2) != "second" {
		t.Errorf("expected overwrite, got %q", got2)
	}
}

// TestVoiceSummary1Sentence asserts the spoken summary is a single
// sentence with the four key facts (yesterday count, agents, bugs, $).
func TestVoiceSummary1Sentence(t *testing.T) {
	d := Data{
		YesterdayActions: map[string]int{"ask": 3, "ship": 1},
		ActiveAgents:     []regattaclient.Agent{{ID: "1"}, {ID: "2"}},
		BugFixCount:      5,
		WeekToDateUSD:    12.34,
	}
	got := VoiceSummary(d)
	for _, want := range []string{"4 actions", "2 active", "5 bug-fix", "$12.34"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q in %q", want, got)
		}
	}
	if strings.Count(got, ".") < 1 {
		t.Errorf("expected at least one period: %q", got)
	}
}
