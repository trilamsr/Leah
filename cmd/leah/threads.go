package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/trilam/leah/internal/adapters/confluence"
	"github.com/trilam/leah/internal/adapters/jira"
	"github.com/trilam/leah/internal/adapters/linear"
	"github.com/trilam/leah/internal/recommend/sources"
)

// threadRow is the unified JSON projection — one work item, one row.
type threadRow struct {
	Tool    string    `json:"tool"`
	Key     string    `json:"key"`
	Title   string    `json:"title"`
	Updated time.Time `json:"updated"`
}

type threadsOpts struct {
	now    time.Time
	seams  map[string]sources.WorkItemSeam
	stderr io.Writer
}

// runThreads is the CLI entry. Production wiring builds adapter-backed seams
// from the operator's connected tools; runThreadsWith takes injected seams so
// tests stay hermetic without touching token files or HTTP.
func runThreads(parent context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(w, "usage: leah threads [--tool <name>] [--since <dur>] [--json]")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "Unified async-context inbox across connected work tools.")
		_, _ = fmt.Fprintln(w, "Tools: jira, linear, confluence (read-only adapters).")
		return 0
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	return runThreadsWith(ctx, threadsOpts{now: time.Now().UTC(), seams: liveSeams()}, args, w)
}

func runThreadsWith(ctx context.Context, opts threadsOpts, args []string, w io.Writer) int {
	if opts.stderr == nil {
		opts.stderr = os.Stderr
	}
	fs := flag.NewFlagSet("threads", flag.ContinueOnError)
	fs.SetOutput(opts.stderr)
	var (
		tool    = fs.String("tool", "", "filter to one adapter (jira|linear|confluence)")
		sinceS  = fs.String("since", "", "drop items older than dur (e.g. 24h, 72h)")
		asJSON  = fs.Bool("json", false, "machine-readable output")
		windowS = fs.String("within", "720h", "adapter scan window (default 30d)")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *tool != "" {
		if _, ok := opts.seams[*tool]; !ok {
			_, _ = fmt.Fprintf(opts.stderr, "leah threads: unknown tool %q\n", *tool)
			return 2
		}
	}
	var since time.Duration
	if *sinceS != "" {
		d, err := time.ParseDuration(*sinceS)
		if err != nil {
			_, _ = fmt.Fprintf(opts.stderr, "leah threads: --since: %v\n", err)
			return 2
		}
		since = d
	}
	window, err := time.ParseDuration(*windowS)
	if err != nil {
		_, _ = fmt.Fprintf(opts.stderr, "leah threads: --within: %v\n", err)
		return 2
	}

	var rows []threadRow
	for name, seam := range opts.seams {
		if *tool != "" && name != *tool {
			continue
		}
		items, err := seam.StaleItems(ctx, window)
		if err != nil {
			// One adapter's failure must not blank the whole inbox.
			_, _ = fmt.Fprintf(opts.stderr, "leah threads: %s: %v\n", name, err)
			continue
		}
		for _, it := range items {
			if since > 0 && opts.now.Sub(it.Updated) > since {
				continue
			}
			rows = append(rows, threadRow{Tool: name, Key: it.Key, Title: it.Title, Updated: it.Updated})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Updated.After(rows[j].Updated) })

	if *asJSON {
		if rows == nil {
			rows = []threadRow{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rows)
		return 0
	}
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(w, "(no items)")
		return 0
	}
	for _, r := range rows {
		age := opts.now.Sub(r.Updated).Round(time.Minute)
		_, _ = fmt.Fprintf(w, "[%s] %s  %s  (%s ago)\n", r.Tool, r.Key, r.Title, age)
	}
	return 0
}

// liveSeams builds seam-satisfying shims for every adapter the operator has
// connected; absent tokens silently drop that tool so `leah threads` degrades
// to the subset the operator can actually reach.
func liveSeams() map[string]sources.WorkItemSeam {
	out := map[string]sources.WorkItemSeam{}
	hc := &http.Client{Timeout: 15 * time.Second}
	if ts := staticTokenForName("jira"); ts != "" {
		base := os.Getenv("LEAH_SHIP_JIRA_BASE_URL")
		if ad, err := jira.New(jira.Config{Attestor: noopAttestor{}, TokenSource: ts, Transport: jira.NewHTTPTransport(hc, base), BaseURL: base}); err == nil {
			out["jira"] = jiraSeam{ad}
		}
	}
	if ts := staticTokenForName("linear"); ts != "" {
		if ad, err := linear.New(linear.Config{Attestor: noopAttestor{}, TokenSource: ts, Transport: linear.NewHTTPTransport(hc, "https://api.linear.app/graphql")}); err == nil {
			out["linear"] = linearSeam{ad}
		}
	}
	if ts := staticTokenForName("confluence"); ts != "" {
		base := os.Getenv("LEAH_SHIP_CONFLUENCE_BASE_URL")
		if ad, err := confluence.New(confluence.Config{Attestor: noopAttestor{}, TokenSource: ts, Transport: confluence.NewHTTPTransport(hc, base), BaseURL: base}); err == nil {
			space := os.Getenv("LEAH_THREADS_CONFLUENCE_SPACE")
			out["confluence"] = confluenceSeam{ad: ad, space: space}
		}
	}
	return out
}

type jiraSeam struct{ ad *jira.Client }

func (s jiraSeam) StaleItems(ctx context.Context, _ time.Duration) ([]sources.WorkItem, error) {
	iss, err := s.ad.ListMyIssues(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sources.WorkItem, 0, len(iss))
	for _, i := range iss {
		out = append(out, sources.WorkItem{Key: i.Key, Title: i.Summary, Updated: i.Updated})
	}
	return out, nil
}

type linearSeam struct{ ad *linear.Client }

func (s linearSeam) StaleItems(ctx context.Context, _ time.Duration) ([]sources.WorkItem, error) {
	iss, err := s.ad.ListMyIssues(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sources.WorkItem, 0, len(iss))
	for _, i := range iss {
		out = append(out, sources.WorkItem{Key: i.Identifier, Title: i.Title, Updated: i.Updated})
	}
	return out, nil
}

type confluenceSeam struct {
	ad    *confluence.Client
	space string
}

func (s confluenceSeam) StaleItems(ctx context.Context, _ time.Duration) ([]sources.WorkItem, error) {
	pages, err := s.ad.ListRecentPages(ctx, s.space)
	if err != nil {
		return nil, err
	}
	out := make([]sources.WorkItem, 0, len(pages))
	for _, p := range pages {
		out = append(out, sources.WorkItem{Key: p.ID, Title: p.Title, Updated: p.Updated})
	}
	return out, nil
}
