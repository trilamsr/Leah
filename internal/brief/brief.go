// Package brief composes a terse morning brief from existing data sources
// (audit log + memory.db + regatta CLI + bug-fix-candidates.md). Used by
// both the `leah brief` CLI and the daemon's 6th task (LEAH_BRIEF_DAILY env
// flips weekly→daily cadence). Pure data + render split keeps the formatter
// testable without spawning the regatta binary.
package brief

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/adapters/gcal"
	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/costview"
	"github.com/trilam/leah/internal/memory"
	"github.com/trilam/leah/internal/operatormodel"
	"github.com/trilam/leah/internal/regattaclient"
)

// gmailCap bounds the unread-subject list so the brief stays scannable
// when the operator's inbox is in triple digits.
const gmailCap = 5

// gcalCap bounds the calendar list; conferences can hit 20+ events/day,
// 10 is the operator's comfortable glance.
const gcalCap = 10

// subjectMaxLen truncates noisy mail subjects so a single long thread
// title cannot wrap and break the section layout.
const subjectMaxLen = 80

// GmailLister is the gmail-adapter subset the brief consumes; defining
// it here keeps the dependency one-way (brief → adapters) and lets tests
// inject a fake without dragging in the real attestation gate.
type GmailLister interface {
	ListUnread(ctx context.Context) ([]string, error)
}

// GcalLister mirrors GmailLister for calendar events.
type GcalLister interface {
	ListToday(ctx context.Context) ([]gcal.Event, error)
}

// GatherOpts carries the optional adapter listers. Zero value = adapters
// not configured → brief omits the corresponding sections entirely
// (silent absence beats noisy "unavailable" for unconfigured features).
type GatherOpts struct {
	Gmail GmailLister
	Gcal  GcalLister
}

// Data is the pure-data input to Render — all IO happens upstream so the
// formatter is testable without a real audit log / regatta binary.
type Data struct {
	Now              time.Time
	YesterdayActions map[string]int // kind → count (ask|ship|review|status)
	YesterdaySpend   float64
	ActiveAgents     []regattaclient.Agent // already filtered to running/escalated
	Recommendations  []operatormodel.Recommendation
	ModelReady       bool
	BugFixCount      int
	WeekToDateUSD    float64
	ProjectedMonthly float64

	// UnreadMail holds top-K gmail subjects (already truncated). Empty +
	// MailUnavailable=false → gmail not configured, section is omitted.
	UnreadMail          []string
	UnreadMailTotal     int
	MailUnavailable     bool
	TodayEvents         []gcal.Event
	CalendarUnavailable bool
}

// RegattaLister is the subset of regattaclient.Client Gather needs.
// Abstracted so the daemon task can inject the shared client w/o re-spawning.
type RegattaLister interface {
	List(ctx context.Context) ([]regattaclient.Agent, error)
}

// Gather collects all IO into one place so Render stays pure. Soft-fails
// per source: a missing regatta binary or empty audit log degrades the
// brief gracefully rather than aborting.
func Gather(ctx context.Context, now time.Time, sd string, rc RegattaLister, opts ...GatherOpts) Data {
	var o GatherOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	auditPath := filepath.Join(sd, "audit.jsonl")

	yStart, yEnd := YesterdayBounds(now)
	yActions, ySpend := ScanYesterday(auditPath, yStart, yEnd)

	d := Data{
		Now:              now,
		YesterdayActions: yActions,
		YesterdaySpend:   ySpend,
	}

	// Active regatta agents (soft-fail).
	if rc != nil {
		if agents, err := rc.List(ctx); err == nil {
			d.ActiveAgents = FilterActiveAgents(agents)
		}
	}

	// Operator-model recommendations (soft-fail; cold start common).
	memPath := filepath.Join(sd, "memory.db")
	if store, err := memory.NewStore(memPath); err == nil {
		defer func() { _ = store.Close() }()
		if profile, err := operatormodel.Load(ctx, store.DB()); err == nil {
			d.ModelReady = profile.Ready
			if recs, err := operatormodel.Recommend(profile, "", now); err == nil {
				d.Recommendations = recs
			}
		}
	}

	// Bug-fix-candidates count (read tail section if present).
	d.BugFixCount = CountBugFixCandidates(filepath.Join(sd, "bug-fix-candidates.md"))

	// Cost outlook.
	weekStart := now.AddDate(0, 0, -7)
	if summary, err := costview.Aggregate(auditPath, weekStart); err == nil {
		d.WeekToDateUSD = summary.TotalUSD
		// Projected monthly = week × (30 / 7). Coarse — operator-use only.
		d.ProjectedMonthly = summary.TotalUSD * (30.0 / 7.0)
	}

	// Gmail unread (soft-fail; nil err with empty list means inbox-zero).
	if o.Gmail != nil {
		if subs, err := o.Gmail.ListUnread(ctx); err != nil {
			d.MailUnavailable = true
		} else {
			d.UnreadMailTotal = len(subs)
			d.UnreadMail = truncateSubjects(subs, gmailCap, subjectMaxLen)
		}
	}

	// Gcal today (soft-fail mirrors gmail).
	if o.Gcal != nil {
		if evs, err := o.Gcal.ListToday(ctx); err != nil {
			d.CalendarUnavailable = true
		} else {
			d.TodayEvents = evs
		}
	}

	return d
}

// truncateSubjects caps the slice and ellipsizes any subject over max.
func truncateSubjects(in []string, capN, max int) []string {
	n := len(in)
	if n > capN {
		n = capN
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		s := in[i]
		if len(s) > max {
			s = s[:max-1] + "…"
		}
		out[i] = s
	}
	return out
}

// Render turns Data into a markdown brief. Pure function — drives the test
// suite.
func Render(d Data) string {
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

	// 5. Mail + Calendar render only when the operator has wired the
	// integration — silent absence beats noisy "unavailable" for
	// unconfigured features (UnavailableX is for runtime failure).
	if d.MailUnavailable || d.UnreadMailTotal > 0 || len(d.UnreadMail) > 0 {
		fmt.Fprintln(&b, "## Mail")
		if d.MailUnavailable {
			fmt.Fprintln(&b, "  (unavailable)")
		} else {
			total := d.UnreadMailTotal
			if total == 0 {
				total = len(d.UnreadMail)
			}
			shown := len(d.UnreadMail)
			if shown > gmailCap {
				shown = gmailCap
			}
			fmt.Fprintf(&b, "  %d unread\n", total)
			for _, s := range d.UnreadMail[:shown] {
				fmt.Fprintf(&b, "  - %s\n", s)
			}
			if total > shown {
				fmt.Fprintf(&b, "  …and %d more\n", total-shown)
			}
		}
		fmt.Fprintln(&b)
	}

	if d.CalendarUnavailable || len(d.TodayEvents) > 0 {
		fmt.Fprintln(&b, "## Calendar")
		if d.CalendarUnavailable {
			fmt.Fprintln(&b, "  (unavailable)")
		} else {
			max := gcalCap
			if len(d.TodayEvents) < max {
				max = len(d.TodayEvents)
			}
			fmt.Fprintf(&b, "  %d events\n", len(d.TodayEvents))
			for _, ev := range d.TodayEvents[:max] {
				fmt.Fprintf(&b, "  - %s %s\n", ev.Start.Format("15:04"), ev.Summary)
			}
			if len(d.TodayEvents) > max {
				fmt.Fprintf(&b, "  …and %d more\n", len(d.TodayEvents)-max)
			}
		}
		fmt.Fprintln(&b)
	}

	// 6. Cost outlook.
	fmt.Fprintln(&b, "## Cost")
	fmt.Fprintf(&b, "  week-to-date  $%.4f\n", d.WeekToDateUSD)
	fmt.Fprintf(&b, "  projected mo  $%.4f\n", d.ProjectedMonthly)

	return b.String()
}

// VoiceSummary is the 1-sentence form spoken when TTS notify is wired. TTS
// on the full markdown is painful.
func VoiceSummary(d Data) string {
	yTotal := 0
	for _, n := range d.YesterdayActions {
		yTotal += n
	}
	return fmt.Sprintf("%d actions yesterday, %d active agents, %d bug-fix candidates, $%.2f week to date.",
		yTotal, len(d.ActiveAgents), d.BugFixCount, d.WeekToDateUSD)
}

// YesterdayBounds returns [start, end) of the previous calendar day in the
// operator's local timezone. "Yesterday" = 00:00 yesterday → 00:00 today.
func YesterdayBounds(now time.Time) (time.Time, time.Time) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	return yesterday, today
}

// ScanYesterday streams audit.jsonl, returning per-kind counts and total
// spend for rows with ts in [start, end). Missing file is not an error.
func ScanYesterday(path string, start, end time.Time) (map[string]int, float64) {
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

// FilterActiveAgents keeps only running + escalated agents — the brief
// surfaces what the operator can still influence today.
func FilterActiveAgents(in []regattaclient.Agent) []regattaclient.Agent {
	out := make([]regattaclient.Agent, 0, len(in))
	for _, a := range in {
		if a.State == "running" || a.State == "escalated" {
			out = append(out, a)
		}
	}
	return out
}

// CountBugFixCandidates counts H2 sections (lines starting "## ") in the
// candidates file. Each weekly daemon tick appends one section; count maps
// 1:1 to queued candidates.
func CountBugFixCandidates(path string) int {
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

// WriteFile saves the brief under <sd>/briefs/YYYY-MM-DD.md. Idempotent
// per-day overwrite — daily re-fire on the same calendar day overwrites
// the prior file rather than appending.
func WriteFile(sd string, now time.Time, body string) error {
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
