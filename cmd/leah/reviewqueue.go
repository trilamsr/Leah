package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/trilam/leah/internal/ghclient"
)

// reviewRow is the projection we render. Repo + number + age is what the
// operator scans; URL is kept because --json consumers (`leah brief`, etc.)
// need a one-click handle back to the PR.
type reviewRow struct {
	Number    int       `json:"number"`
	Repo      string    `json:"repo"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updatedAt"`
	IsDraft   bool      `json:"isDraft"`
}

// reviewQueueGraphQL is the search query. `review-requested:@me` matches the
// operator's pending-review inbox; sorting client-side because GH search
// sort by updated-asc is unreliable across orgs.
const reviewQueueGraphQL = `query($q: String!) {
  search(query: $q, type: ISSUE, first: 50) {
    nodes {
      ... on PullRequest {
        number
        title
        url
        updatedAt
        isDraft
        repository { nameWithOwner }
      }
    }
  }
}`

func runReviewQueue(parent context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(w, "usage: leah review-queue [--org <name>] [--json]")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "List PRs awaiting your review across configured GH orgs.")
		_, _ = fmt.Fprintln(w, "Ranked oldest-first; drafts sink below ready PRs.")
		return 0
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	return runReviewQueueWith(ctx, ghclient.ShellExec{}, args, w)
}

func runReviewQueueWith(ctx context.Context, exec ghclient.Executor, args []string, w io.Writer) int {
	fs := flag.NewFlagSet("review-queue", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		org    = fs.String("org", "", "scope query to one GH org (repo:owner/* implied)")
		asJSON = fs.Bool("json", false, "machine-readable output")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	rows, err := fetchReviewQueue(ctx, exec, *org)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah review-queue: %v\n", err)
		return 1
	}
	rankReviewQueue(rows)

	if *asJSON {
		if rows == nil {
			rows = []reviewRow{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rows)
		return 0
	}
	// Empty result stays silent so the verb composes in pipelines
	// (`leah review-queue | head` doesn't need a placeholder line).
	if len(rows) == 0 {
		return 0
	}
	now := time.Now()
	for _, r := range rows {
		_, _ = fmt.Fprintln(w, formatReviewRow(r, now))
	}
	return 0
}

func fetchReviewQueue(ctx context.Context, exec ghclient.Executor, org string) ([]reviewRow, error) {
	q := "is:pr is:open review-requested:@me archived:false"
	if org != "" {
		q += " org:" + org
	}
	args := []string{"gh", "api", "graphql",
		"-f", "query=" + reviewQueueGraphQL,
		"-F", "q=" + q,
	}
	out, err := exec.Run(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("gh api graphql: %w", err)
	}
	var resp struct {
		Data struct {
			Search struct {
				Nodes []map[string]any `json:"nodes"`
			} `json:"search"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return nil, fmt.Errorf("parse graphql json: %w", err)
	}
	// GH returns HTTP 200 with {"errors":[...]} on rate-limit / auth / schema
	// failure; without surfacing the message, the operator gets an empty queue
	// and assumes a clean inbox while their token has expired. Empty-message
	// fallback because "errors[]: empty string" leaves a useless naked prefix.
	if len(resp.Errors) > 0 {
		msg := resp.Errors[0].Message
		if msg == "" {
			msg = "(unknown error)"
		}
		return nil, fmt.Errorf("gh api graphql: %s", msg)
	}
	rows := make([]reviewRow, 0, len(resp.Data.Search.Nodes))
	for _, n := range resp.Data.Search.Nodes {
		if len(n) == 0 {
			continue
		}
		// Drop malformed nodes rather than render ghost rows (#0, epoch-zero
		// "from 1970" ages). GH's search GraphQL union returns {} for non-PR
		// matches even with is:pr, and field-presence drift across API
		// versions is real.
		num, ok := n["number"].(float64)
		if !ok || num <= 0 {
			continue
		}
		ts, ok := n["updatedAt"].(string)
		if !ok {
			continue
		}
		updated, err := time.Parse(time.RFC3339, ts)
		if err != nil || updated.IsZero() {
			continue
		}
		r := reviewRow{Number: int(num), UpdatedAt: updated}
		r.Title, _ = n["title"].(string)
		r.URL, _ = n["url"].(string)
		r.IsDraft, _ = n["isDraft"].(bool)
		if repo, ok := n["repository"].(map[string]any); ok {
			r.Repo, _ = repo["nameWithOwner"].(string)
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// rankReviewQueue orders by oldest updatedAt first (FIFO inbox) with drafts
// sunk regardless of age — operator's attention belongs on ready PRs first.
func rankReviewQueue(rows []reviewRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].IsDraft != rows[j].IsDraft {
			return !rows[i].IsDraft
		}
		return rows[i].UpdatedAt.Before(rows[j].UpdatedAt)
	})
}

// formatReviewRow renders one row. Title gets the lion's share because that's
// what the operator reads to decide priority; repo + age are sidebar context.
func formatReviewRow(r reviewRow, now time.Time) string {
	repo := truncate(r.Repo, 24)
	title := truncate(r.Title, 60)
	flag := "  "
	if r.IsDraft {
		flag = "DRAFT"
	}
	age := ageSince(r.UpdatedAt.Format(time.RFC3339), now)
	return fmt.Sprintf("#%-5d %-24s %-60s %-5s %s",
		r.Number, repo, title, flag, age)
}
