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

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/budget/view"
	"github.com/trilam/leah/internal/memory/store"
	"github.com/trilam/leah/internal/thinking/operatormodel"
	"github.com/trilam/leah/internal/actions/regattaclient"
	"golang.org/x/sync/errgroup"
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

// workCap bounds each work-tool list to a scannable glance, matching gcalCap.
const workCap = 10

// GmailLister is the gmail-adapter subset the brief consumes; defining
// it here keeps the dependency one-way (brief → adapters) and lets tests
// inject a fake without dragging in the real attestation gate.
type GmailLister interface {
	ListUnread(ctx context.Context) ([]string, error)
}

// Event is the brief-local calendar shape; defining it here keeps the brief
// package free of any concrete adapter import (callers map gcal.Event →
// brief.Event at the wire-up site). Only the fields Render actually uses
// live here — adding a field is cheaper than removing one.
type Event struct {
	Start   time.Time
	Summary string
}

// GcalLister mirrors GmailLister for calendar events.
type GcalLister interface {
	ListToday(ctx context.Context) ([]Event, error)
}

// WorkItem is the brief-local row for every work-tool section. Title is the
// primary label, Detail an optional status/space suffix; callers map each
// adapter's own type onto these two fields at the wire-up site so the brief
// package stays free of any adapter import.
type WorkItem struct {
	Title  string
	Detail string
}

// WorkLister is the one-method shape every work-tool lister satisfies. The
// wire site adapts the adapter's real read RPC (jira ListMyIssues,
// confluence ListRecentPages) onto it.
type WorkLister interface {
	List(ctx context.Context) ([]WorkItem, error)
}

// GatherOpts carries the optional adapter listers. Zero value = adapters
// not configured → brief omits the corresponding sections entirely
// (silent absence beats noisy "unavailable" for unconfigured features).
type GatherOpts struct {
	Gmail      GmailLister
	Gcal       GcalLister
	Weather    WeatherReporter
	News       NewsReporter
	Market     MarketReporter
	Jira       WorkLister
	Confluence WorkLister
	Watchlist  WatchlistQuoter
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
	TodayEvents         []Event
	CalendarUnavailable bool

	// Weather / News / Market are nil when the operator hasn't wired the
	// feed; *Unavailable flips on runtime failure (mirrors mail/calendar).
	Weather            *Forecast
	WeatherUnavailable bool
	News               *Article
	NewsUnavailable    bool
	Market             *Pulse
	MarketUnavailable  bool

	// Work-tool sections — same silent-absence vs runtime-failure split as
	// mail/calendar: nil items + Unavailable=false → tool not connected,
	// section omitted; Unavailable=true → connected but the read failed.
	JiraItems             []WorkItem
	JiraUnavailable       bool
	ConfluenceItems       []WorkItem
	ConfluenceUnavailable bool

	// Watchlist — silent absence when ~/.leah-state/watchlist.json is missing
	// or its symbol list is empty; Unavailable flips on quoter failure.
	Watchlist            []WatchlistQuote
	WatchlistUnavailable bool
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

	// Each section writes a disjoint set of Data fields, so the fan-out needs
	// no lock — only its own error is captured into its own Unavailable flag.
	// Sequential network fetches would otherwise stack latency on the brief.
	var g errgroup.Group

	if rc != nil {
		g.Go(func() error {
			if agents, err := rc.List(ctx); err == nil {
				d.ActiveAgents = FilterActiveAgents(agents)
			}
			return nil
		})
	}

	g.Go(func() error {
		memPath := filepath.Join(sd, "memory.db")
		store, err := memory.NewStore(memPath)
		if err != nil {
			return nil // cold start common — degrade silently
		}
		defer func() { _ = store.Close() }()
		if profile, err := operatormodel.Load(ctx, store.DB()); err == nil {
			d.ModelReady = profile.Ready
			if recs, err := operatormodel.Recommend(profile, "", now); err == nil {
				d.Recommendations = recs
			}
		}
		return nil
	})

	g.Go(func() error {
		d.BugFixCount = CountBugFixCandidates(filepath.Join(sd, "bug-fix-candidates.md"))
		return nil
	})

	g.Go(func() error {
		weekStart := now.AddDate(0, 0, -7)
		if summary, err := view.Aggregate(auditPath, weekStart); err == nil {
			d.WeekToDateUSD = summary.TotalUSD
			// Projected monthly = week × (30 / 7). Coarse — operator-use only.
			d.ProjectedMonthly = summary.TotalUSD * (30.0 / 7.0)
		}
		return nil
	})

	if o.Gmail != nil {
		g.Go(func() error {
			// nil err with empty list means inbox-zero, not unavailable.
			if subs, err := o.Gmail.ListUnread(ctx); err != nil {
				d.MailUnavailable = true
			} else {
				d.UnreadMailTotal = len(subs)
				d.UnreadMail = truncateSubjects(subs, gmailCap, subjectMaxLen)
			}
			return nil
		})
	}

	if o.Gcal != nil {
		g.Go(func() error {
			if evs, err := o.Gcal.ListToday(ctx); err != nil {
				d.CalendarUnavailable = true
			} else {
				d.TodayEvents = evs
			}
			return nil
		})
	}

	gatherWork(ctx, &g, o.Jira, &d.JiraItems, &d.JiraUnavailable)
	gatherWork(ctx, &g, o.Confluence, &d.ConfluenceItems, &d.ConfluenceUnavailable)

	// Read watchlist.json synchronously — the spawn decision needs the symbol
	// list. Missing file or empty list → no quoter fire (silent absence,
	// matches the worktool paste-token gating).
	if o.Watchlist != nil {
		symbols := LoadWatchlistSymbols(filepath.Join(sd, "watchlist.json"))
		if len(symbols) > 0 {
			g.Go(func() error {
				if qs, err := o.Watchlist.FetchAll(ctx, symbols); err != nil {
					d.WatchlistUnavailable = true
				} else {
					d.Watchlist = qs
				}
				return nil
			})
		}
	}

	g.Go(func() error {
		gatherFeeds(ctx, &d, o)
		return nil
	})

	_ = g.Wait()
	return d
}

// gatherWork fans one work-tool lister into its own goroutine, writing a
// disjoint pair of Data fields so the errgroup needs no lock and one tool's
// failure flips only its own Unavailable flag.
func gatherWork(ctx context.Context, g *errgroup.Group, l WorkLister, items *[]WorkItem, unavail *bool) {
	if l == nil {
		return
	}
	g.Go(func() error {
		if got, err := l.List(ctx); err != nil {
			*unavail = true
		} else {
			*items = got
		}
		return nil
	})
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
