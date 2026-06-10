package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/trilam/leah/internal/ghclient"
	"github.com/trilam/leah/internal/regattaclient"
)

// defaultBacklogRepo is the self-build target — operator runs `leah backlog`
// most often against the Leah repo itself.
const defaultBacklogRepo = "trilamsr/Leah"

// backlogView is the JSON shape printed under --json.
type backlogView struct {
	Repo   string                   `json:"repo"`
	Agents []regattaclient.Agent    `json:"agents"`
	Issues []map[string]any         `json:"issues"`
	PRs    []map[string]any         `json:"prs"`
}

// runBacklog aggregates active regatta agents, open ready-for-agent issues,
// and the last 10 PRs into one operator screen — replaces the three-tab
// dashboard (regatta agents list + gh issue list + gh pr list).
func runBacklog(args []string) {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah backlog [repo] [--json]")
		return
	}
	repo := defaultBacklogRepo
	jsonMode := false
	for _, a := range args {
		switch {
		case a == "--json":
			jsonMode = true
		case strings.HasPrefix(a, "--"):
			_, _ = fmt.Fprintf(os.Stderr, "leah backlog: unknown flag %q\n", a)
			os.Exit(2)
		default:
			repo = a
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	view := backlogView{Repo: repo}

	// 1. Active regatta agents. Soft-fail: regatta may not be on PATH or daemon
	// may be down; print empty section + stderr warning rather than aborting.
	if agents, err := regattaclient.New().List(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warn: regatta agents list: %v\n", err)
	} else {
		view.Agents = filterActive(agents)
	}

	sh := ghclient.ShellExec{}

	// 2. Open ready-for-agent issues.
	if issues, err := listReadyIssues(ctx, sh, repo); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warn: gh issue list: %v\n", err)
	} else {
		view.Issues = issues
	}

	// 3. Recent PRs (last 10, any state).
	if prs, err := listRecentPRs(ctx, sh, repo); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warn: gh pr list: %v\n", err)
	} else {
		view.PRs = prs
	}

	if jsonMode {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(view); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah backlog: encode json: %v\n", err)
			os.Exit(1)
		}
		return
	}
	printBacklog(os.Stdout, view)
}

// filterActive drops terminal-state agents so the human view only shows what
// the operator can still influence. Terminal set matches daemonloop.isTerminal.
func filterActive(in []regattaclient.Agent) []regattaclient.Agent {
	out := make([]regattaclient.Agent, 0, len(in))
	for _, a := range in {
		switch a.State {
		case "merged", "failed", "stuck", "killed":
			continue
		}
		out = append(out, a)
	}
	return out
}

func listReadyIssues(ctx context.Context, sh ghclient.ShellExec, repo string) ([]map[string]any, error) {
	out, err := sh.Run(ctx, []string{
		"gh", "issue", "list",
		"--repo", repo,
		"--label", "ready-for-agent",
		"--json", "number,title,labels,createdAt",
		"-L", "20",
	})
	if err != nil {
		return nil, err
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		return nil, fmt.Errorf("parse issue list: %w", err)
	}
	return arr, nil
}

func listRecentPRs(ctx context.Context, sh ghclient.ShellExec, repo string) ([]map[string]any, error) {
	out, err := sh.Run(ctx, []string{
		"gh", "pr", "list",
		"--repo", repo,
		"--state", "all",
		"--json", "number,title,state,mergedAt,createdAt",
		"-L", "10",
	})
	if err != nil {
		return nil, err
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		return nil, fmt.Errorf("parse pr list: %w", err)
	}
	return arr, nil
}

func printBacklog(w *os.File, v backlogView) {
	_, _ = fmt.Fprintln(w, "== REGATTA AGENTS (active) ==")
	if len(v.Agents) == 0 {
		_, _ = fmt.Fprintln(w, "(none)")
	}
	for _, a := range v.Agents {
		pr := "-"
		if a.PR > 0 {
			pr = fmt.Sprintf("PR #%d", a.PR)
		}
		id := a.ID
		if len(id) > 12 {
			id = id[:12]
		}
		_, _ = fmt.Fprintf(w, "%-12s %-20s %-10s %s\n", id, a.Branch, a.State, pr)
	}

	_, _ = fmt.Fprintf(w, "\n== OPEN ISSUES (ready-for-agent in %s) ==\n", v.Repo)
	if len(v.Issues) == 0 {
		_, _ = fmt.Fprintln(w, "(none)")
	}
	now := time.Now()
	for _, is := range v.Issues {
		num, _ := is["number"].(float64)
		title, _ := is["title"].(string)
		created, _ := is["createdAt"].(string)
		_, _ = fmt.Fprintf(w, "#%-5d %-60s %s\n", int(num), truncate(title, 60), ageSince(created, now))
	}

	_, _ = fmt.Fprintf(w, "\n== RECENT PRs (last 10 in %s) ==\n", v.Repo)
	if len(v.PRs) == 0 {
		_, _ = fmt.Fprintln(w, "(none)")
	}
	for _, pr := range v.PRs {
		num, _ := pr["number"].(float64)
		title, _ := pr["title"].(string)
		state, _ := pr["state"].(string)
		ts, _ := pr["mergedAt"].(string)
		if ts == "" {
			ts, _ = pr["createdAt"].(string)
		}
		_, _ = fmt.Fprintf(w, "#%-5d %-50s %-8s %s\n", int(num), truncate(title, 50), state, ageSince(ts, now))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return s[:n-1] + "…"
}

// ageSince renders a coarse "Nh ago" / "Nd ago" — accuracy beyond hour granularity
// adds clutter without informing the operator's next decision.
func ageSince(ts string, now time.Time) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
