package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/costview"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/notify"
	"github.com/trilam/leah/internal/operatormodel"
	"github.com/trilam/leah/internal/regattaclient"
)

// briefData is the pure-data input to renderBrief — all IO happens upstream
// so the formatter is testable without a real audit log / regatta binary.
type briefData struct {
	Now              time.Time
	YesterdayActions map[string]int // kind → count (ask|ship|review|status)
	YesterdaySpend   float64
	ActiveAgents     []regattaclient.Agent // already filtered to running/escalated
	Recommendations  []operatormodel.Recommendation
	ModelReady       bool
	BugFixCount      int
	WeekToDateUSD    float64
	ProjectedMonthly float64
}

// runBrief implements `leah brief [--voice] [--silent]`. Composes a terse
// morning brief from existing data sources (audit log + memory.db +
// regatta CLI + bug-fix-candidates.md) — no LLM call by default.
func runBrief(args []string) {
	voiceMode := false
	silentMode := false
	for _, a := range args {
		switch a {
		case "--voice":
			voiceMode = true
		case "--silent":
			silentMode = true
		default:
			_, _ = fmt.Fprintf(os.Stderr, "leah brief: unknown flag %q\n", a)
			os.Exit(2)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now()
	data := gatherBrief(ctx, now, stateDir())
	body := renderBrief(data)

	// --silent precedence: file only, no stdout, no voice (cron/launchd path
	// where TTS would be a regression). --voice on its own → stdout + speak.
	if silentMode {
		if err := writeBriefFile(stateDir(), now, body); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah brief: write file: %v\n", err)
			os.Exit(1)
		}
		return
	}

	_, _ = fmt.Print(body)

	if voiceMode {
		v := notify.NewVoice()
		// Speak a 1-line summary, not the full markdown — TTS on long
		// markdown is painful and the operator already sees stdout.
		summary := briefVoiceSummary(data)
		if err := v.Notify(ctx, "Morning brief", summary); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah brief: voice: %v\n", err)
			// Not fatal — text already printed.
		}
	}
}

// gatherBrief collects all IO into one place so renderBrief stays pure.
// Soft-fails per source: a missing regatta binary or empty audit log degrades
// the brief gracefully rather than aborting.
func gatherBrief(ctx context.Context, now time.Time, sd string) briefData {
	auditPath := filepath.Join(sd, "audit.jsonl")

	yStart, yEnd := yesterdayBounds(now)
	yActions, ySpend := scanYesterday(auditPath, yStart, yEnd)

	data := briefData{
		Now:              now,
		YesterdayActions: yActions,
		YesterdaySpend:   ySpend,
	}

	// Active regatta agents (soft-fail).
	if agents, err := regattaclient.New().List(ctx); err == nil {
		data.ActiveAgents = filterBriefAgents(agents)
	}

	// Operator-model recommendations (soft-fail; cold start common).
	memPath := filepath.Join(sd, "memory.db")
	if store, err := memory.NewStore(memPath); err == nil {
		defer func() { _ = store.Close() }()
		if profile, err := operatormodel.Load(ctx, store.DB()); err == nil {
			data.ModelReady = profile.Ready
			if recs, err := operatormodel.Recommend(profile, "", now); err == nil {
				data.Recommendations = recs
			}
		}
	}

	// Bug-fix-candidates count (read tail section if present).
	data.BugFixCount = countBugFixCandidates(filepath.Join(sd, "bug-fix-candidates.md"))

	// Cost outlook.
	weekStart := now.AddDate(0, 0, -7)
	if summary, err := costview.Aggregate(auditPath, weekStart); err == nil {
		data.WeekToDateUSD = summary.TotalUSD
		// Projected monthly = week × (30 / 7). Coarse — operator-use only.
		data.ProjectedMonthly = summary.TotalUSD * (30.0 / 7.0)
	}

	return data
}

// renderBrief turns briefData into a markdown brief. Pure function — drives
// the test suite.
func renderBrief(d briefData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# leah brief — %s\n\n", d.Now.Format("Mon Jan 2 2006"))

	// 1. Yesterday recap.
	fmt.Fprintln(&b, "## Yesterday")
	total := 0
	for _, n := range d.YesterdayActions {
		total += n
	}
	if total == 0 {
		fmt.Fprintln(&b, "  (no leah actions logged)")
	} else {
		parts := []string{}
		for _, k := range []string{"ask", "ship", "review", "status"} {
			if n := d.YesterdayActions[k]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, k))
			}
		}
		fmt.Fprintf(&b, "  %s — $%.4f spent\n", strings.Join(parts, ", "), d.YesterdaySpend)
	}
	fmt.Fprintln(&b)

	// 2. Today's regatta backlog (top 3 active agents).
	fmt.Fprintln(&b, "## Regatta backlog")
	if len(d.ActiveAgents) == 0 {
		fmt.Fprintln(&b, "  (no active agents)")
	} else {
		max := 3
		if len(d.ActiveAgents) < max {
			max = len(d.ActiveAgents)
		}
		for _, a := range d.ActiveAgents[:max] {
			id := a.ID
			if len(id) > 12 {
				id = id[:12]
			}
			pr := "-"
			if a.PR > 0 {
				pr = fmt.Sprintf("PR #%d", a.PR)
			}
			fmt.Fprintf(&b, "  - %s %s (%s) %s\n", id, a.Branch, a.State, pr)
		}
		if len(d.ActiveAgents) > 3 {
			fmt.Fprintf(&b, "  …and %d more\n", len(d.ActiveAgents)-3)
		}
	}
	fmt.Fprintln(&b)

	// 3. Operator recommendations.
	fmt.Fprintln(&b, "## Recommendations")
	if !d.ModelReady {
		fmt.Fprintln(&b, "  (operator-model not ready yet — need more audit history)")
	} else if len(d.Recommendations) == 0 {
		fmt.Fprintln(&b, "  (no recommendations for this hour)")
	} else {
		for i, r := range d.Recommendations {
			fmt.Fprintf(&b, "  %d. %s — %s\n", i+1, r.Kind, r.Reason)
		}
	}
	fmt.Fprintln(&b)

	// 4. Bug-fix candidates.
	fmt.Fprintln(&b, "## Bug-fix candidates")
	if d.BugFixCount == 0 {
		fmt.Fprintln(&b, "  (none queued)")
	} else {
		fmt.Fprintf(&b, "  %d queued — see ~/.leah-state/bug-fix-candidates.md\n", d.BugFixCount)
	}
	fmt.Fprintln(&b)

	// 5. Cost outlook.
	fmt.Fprintln(&b, "## Cost")
	fmt.Fprintf(&b, "  week-to-date  $%.4f\n", d.WeekToDateUSD)
	fmt.Fprintf(&b, "  projected mo  $%.4f\n", d.ProjectedMonthly)

	return b.String()
}

// briefVoiceSummary is the 1-sentence form spoken when --voice is set.
// TTS on the full markdown is painful.
func briefVoiceSummary(d briefData) string {
	yTotal := 0
	for _, n := range d.YesterdayActions {
		yTotal += n
	}
	return fmt.Sprintf("%d actions yesterday, %d active agents, %d bug-fix candidates, $%.2f week to date.",
		yTotal, len(d.ActiveAgents), d.BugFixCount, d.WeekToDateUSD)
}

// yesterdayBounds returns [start, end) of the previous calendar day in the
// operator's local timezone. "Yesterday" = 00:00 yesterday → 00:00 today.
func yesterdayBounds(now time.Time) (time.Time, time.Time) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	return yesterday, today
}

// scanYesterday streams audit.jsonl, returning per-kind counts and total
// spend for rows with ts in [start, end). Missing file is not an error.
func scanYesterday(path string, start, end time.Time) (map[string]int, float64) {
	counts := map[string]int{}
	spend := 0.0
	f, err := os.Open(path)
	if err != nil {
		return counts, spend
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var e audit.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		if ts.Before(start) || !ts.Before(end) {
			continue
		}
		counts[e.Kind]++
		spend += e.CostDollars
	}
	return counts, spend
}

// filterBriefAgents keeps only running + escalated agents — the brief
// surfaces what the operator can still influence today.
func filterBriefAgents(in []regattaclient.Agent) []regattaclient.Agent {
	out := make([]regattaclient.Agent, 0, len(in))
	for _, a := range in {
		if a.State == "running" || a.State == "escalated" {
			out = append(out, a)
		}
	}
	return out
}

// countBugFixCandidates counts H2 sections (lines starting "## ") in the
// candidates file. Each weekly daemon tick appends one section; count maps
// 1:1 to queued candidates.
func countBugFixCandidates(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "## ") {
			n++
		}
	}
	return n
}

// writeBriefFile saves the brief under ~/.leah-state/briefs/YYYY-MM-DD.md.
// --silent invokes this; main flow does not (stdout is the primary surface).
func writeBriefFile(sd string, now time.Time, body string) error {
	dir := filepath.Join(sd, "briefs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir briefs: %w", err)
	}
	path := filepath.Join(dir, now.Format("2006-01-02")+".md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write brief: %w", err)
	}
	return nil
}
