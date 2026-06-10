package brief

import (
	"context"
	"errors"
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

// fakeGmail returns a canned subjects+err pair to Render-side tests.
type fakeGmail struct {
	subjects []string
	err      error
}

func (f *fakeGmail) ListUnread(ctx context.Context) ([]string, error) {
	return f.subjects, f.err
}

// fakeGcal returns a canned events+err pair to Render-side tests.
type fakeGcal struct {
	events []Event
	err    error
}

func (f *fakeGcal) ListToday(ctx context.Context) ([]Event, error) {
	return f.events, f.err
}

// TestBrief_GmailUnreadRendered_HappyPath asserts unread subjects appear.
func TestBrief_GmailUnreadRendered_HappyPath(t *testing.T) {
	d := Data{
		Now: time.Now(),
		UnreadMail: []string{
			"Re: regatta wave 10 review",
			"Invoice 2026-06",
			"Welcome to Leah",
		},
	}
	out := Render(d)
	if !strings.Contains(out, "## Mail") {
		t.Fatalf("missing Mail header:\n%s", out)
	}
	for _, want := range []string{"Re: regatta wave 10 review", "Invoice 2026-06", "Welcome to Leah", "3"} {
		if !strings.Contains(out, want) {
			t.Errorf("mail section missing %q in:\n%s", want, out)
		}
	}
}

// TestBrief_GcalTodayRendered_HappyPath asserts events appear with HH:MM.
func TestBrief_GcalTodayRendered_HappyPath(t *testing.T) {
	loc := time.UTC
	d := Data{
		Now: time.Date(2026, 6, 9, 9, 0, 0, 0, loc),
		TodayEvents: []Event{
			{Summary: "Standup", Start: time.Date(2026, 6, 9, 10, 30, 0, 0, loc)},
			{Summary: "Design review", Start: time.Date(2026, 6, 9, 14, 0, 0, 0, loc)},
		},
	}
	out := Render(d)
	if !strings.Contains(out, "## Calendar") {
		t.Fatalf("missing Calendar header:\n%s", out)
	}
	for _, want := range []string{"10:30 Standup", "14:00 Design review", "2"} {
		if !strings.Contains(out, want) {
			t.Errorf("calendar section missing %q in:\n%s", want, out)
		}
	}
}

// TestBrief_GmailUnavailable_RendersFallback asserts gmail error → "unavailable".
func TestBrief_GmailUnavailable_RendersFallback(t *testing.T) {
	d := Data{Now: time.Now(), MailUnavailable: true}
	out := Render(d)
	if !strings.Contains(out, "## Mail") || !strings.Contains(out, "unavailable") {
		t.Errorf("expected mail unavailable fallback, got:\n%s", out)
	}
}

// TestBrief_GcalUnavailable_RendersFallback asserts gcal error → "unavailable".
func TestBrief_GcalUnavailable_RendersFallback(t *testing.T) {
	d := Data{Now: time.Now(), CalendarUnavailable: true}
	out := Render(d)
	if !strings.Contains(out, "## Calendar") || !strings.Contains(out, "unavailable") {
		t.Errorf("expected calendar unavailable fallback, got:\n%s", out)
	}
}

// TestBrief_BothUnavailable_StillRenders asserts neither failure aborts.
func TestBrief_BothUnavailable_StillRenders(t *testing.T) {
	d := Data{Now: time.Now(), MailUnavailable: true, CalendarUnavailable: true}
	out := Render(d)
	for _, want := range []string{"## Yesterday", "## Regatta backlog", "## Cost", "unavailable"} {
		if !strings.Contains(out, want) {
			t.Errorf("brief truncated when both integrations down, missing %q:\n%s", want, out)
		}
	}
}

// TestBrief_NilListers_OmitSections asserts unset integrations stay silent.
func TestBrief_NilListers_OmitSections(t *testing.T) {
	d := Data{Now: time.Now()}
	out := Render(d)
	if strings.Contains(out, "## Mail") || strings.Contains(out, "## Calendar") {
		t.Errorf("unconfigured integrations should omit sections entirely, got:\n%s", out)
	}
}

// TestBrief_GmailCapAt5Subjects asserts only 5 shown with overflow marker.
func TestBrief_GmailCapAt5Subjects(t *testing.T) {
	subjects := make([]string, 10)
	for i := range subjects {
		subjects[i] = "subject-" + string(rune('a'+i))
	}
	d := Data{Now: time.Now(), UnreadMail: subjects}
	out := Render(d)
	if !strings.Contains(out, "subject-a") || !strings.Contains(out, "subject-e") {
		t.Errorf("expected first five subjects, got:\n%s", out)
	}
	if strings.Contains(out, "subject-f") {
		t.Errorf("expected cap at 5, sixth subject leaked:\n%s", out)
	}
	if !strings.Contains(out, "5 more") {
		t.Errorf("expected overflow marker (+N more), got:\n%s", out)
	}
}

// TestBrief_EventTime_RendersLocal pins HH:MM to the Event's own zone — a
// UTC-only formatter would silently shift the operator's day by 8 hours.
func TestBrief_EventTime_RendersLocal(t *testing.T) {
	pst := time.FixedZone("PST", -8*3600)
	d := Data{
		Now:         time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC),
		TodayEvents: []Event{{Summary: "Standup", Start: time.Date(2026, 6, 9, 10, 30, 0, 0, pst)}},
	}
	out := Render(d)
	if !strings.Contains(out, "10:30 Standup") {
		t.Errorf("expected zone-local HH:MM '10:30', got:\n%s", out)
	}
	if strings.Contains(out, "18:30") {
		t.Errorf("rendered UTC-shifted time '18:30' — formatter ignored Event zone:\n%s", out)
	}
}

// TestGatherCallsGmailLister wires Gather → fakeGmail and confirms subjects propagate.
func TestGatherCallsGmailLister(t *testing.T) {
	dir := t.TempDir()
	g := &fakeGmail{subjects: []string{"hello", "world"}}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Gmail: g})
	if len(d.UnreadMail) != 2 || d.MailUnavailable {
		t.Errorf("gmail subjects not propagated: %+v unavail=%v", d.UnreadMail, d.MailUnavailable)
	}
}

// TestGatherGmailErrorMarksUnavailable asserts a lister error sets the flag.
func TestGatherGmailErrorMarksUnavailable(t *testing.T) {
	dir := t.TempDir()
	g := &fakeGmail{err: errors.New("boom")}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Gmail: g})
	if !d.MailUnavailable || len(d.UnreadMail) != 0 {
		t.Errorf("expected unavailable flag, got %+v unavail=%v", d.UnreadMail, d.MailUnavailable)
	}
}

// TestGatherCallsGcalLister wires Gather → fakeGcal and confirms events propagate.
func TestGatherCallsGcalLister(t *testing.T) {
	dir := t.TempDir()
	c := &fakeGcal{events: []Event{{Summary: "x", Start: time.Now()}}}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Gcal: c})
	if len(d.TodayEvents) != 1 || d.CalendarUnavailable {
		t.Errorf("gcal events not propagated: %+v unavail=%v", d.TodayEvents, d.CalendarUnavailable)
	}
}

// TestGatherGcalErrorMarksUnavailable asserts a lister error sets the flag.
func TestGatherGcalErrorMarksUnavailable(t *testing.T) {
	dir := t.TempDir()
	c := &fakeGcal{err: errors.New("boom")}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Gcal: c})
	if !d.CalendarUnavailable || len(d.TodayEvents) != 0 {
		t.Errorf("expected unavailable flag, got %+v unavail=%v", d.TodayEvents, d.CalendarUnavailable)
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
