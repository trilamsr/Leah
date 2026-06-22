package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/trilam/leah/internal/adapters/slack"
)

// slackTransportFactory is the test seam: tests overwrite this var to swap in
// a fake Transport so runSlack stays network-free under `go test`. Production
// builds always get the HTTP transport pointed at slack.com/api.
var slackTransportFactory = func() slack.Transport {
	return slack.NewHTTPTransport(&http.Client{Timeout: 10 * time.Second}, "https://slack.com/api")
}

// runSlack dispatches `leah slack <send|list|thread|search>`. Token is read
// from the same on-disk path the `leah connect slack` paste flow writes — no
// re-auth here. Missing token is a friendly exit-2 with the connect hint
// rather than a cryptic adapter error.
func runSlack(ctx context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printSlackHelp(w)
		return 0
	}
	if len(args) == 0 {
		printSlackHelp(os.Stderr)
		return 2
	}

	tok, ok := loadSlackToken()
	if !ok {
		_, _ = fmt.Fprintln(os.Stderr, "slack not connected — run: leah connect slack")
		return 2
	}
	ad, err := slack.New(slack.Config{
		Attestor:    noopAttestor{},
		TokenSource: slackToken{staticToken(tok)},
		Transport:   slackTransportFactory(),
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah slack: %v\n", err)
		return 1
	}

	sub := args[0]
	rest := args[1:]
	switch sub {
	case "send":
		return runSlackSend(ctx, ad, rest, w)
	case "list":
		return runSlackList(ctx, ad, w)
	case "thread":
		return runSlackThread(ctx, ad, rest, w)
	case "search":
		return runSlackSearch(ctx, ad, rest, w)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "leah slack: unknown subcommand %q\n", sub)
		printSlackHelp(os.Stderr)
		return 2
	}
}

func runSlackSend(ctx context.Context, ad *slack.Adapter, args []string, _ io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah slack send <channel> <text...>")
		return 2
	}
	channel := args[0]
	text := strings.Join(args[1:], " ")
	if err := ad.PostMessage(ctx, channel, text); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah slack send: %v\n", err)
		return 1
	}
	return 0
}

func runSlackList(ctx context.Context, ad *slack.Adapter, w io.Writer) int {
	chs, err := ad.ListChannels(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah slack list: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(w, "%-12s %-30s %s\n", "ID", "NAME", "KIND")
	for _, c := range chs {
		kind := "channel"
		switch {
		case c.IsIM:
			kind = "im"
		case c.IsGroup:
			kind = "group"
		}
		_, _ = fmt.Fprintf(w, "%-12s %-30s %s\n", c.ID, c.Name, kind)
	}
	return 0
}

func runSlackThread(ctx context.Context, ad *slack.Adapter, args []string, w io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah slack thread <channel> <ts>")
		return 2
	}
	th, err := ad.GetThread(ctx, args[0], args[1])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah slack thread: %v\n", err)
		return 1
	}
	for _, m := range th.Replies {
		_, _ = fmt.Fprintf(w, "[%s] %s: %s\n", m.TS, m.User, m.Text)
	}
	return 0
}

func runSlackSearch(ctx context.Context, ad *slack.Adapter, args []string, w io.Writer) int {
	if len(args) < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah slack search <query...>")
		return 2
	}
	query := strings.Join(args, " ")
	hits, err := ad.Search(ctx, query)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah slack search: %v\n", err)
		return 1
	}
	// Top-5 — operator wants the head of the result set, not a paginated dump
	// they have to scroll. Slack's relevance order is the upstream signal.
	limit := 5
	if len(hits) < limit {
		limit = len(hits)
	}
	for i := 0; i < limit; i++ {
		h := hits[i]
		_, _ = fmt.Fprintf(w, "%d. %s\n   %s\n   %s\n", i+1, h.Channel, h.Snippet, h.URL)
	}
	return 0
}

// loadSlackToken reads the connect-staged JSON token; (token, true) on
// success, ("", false) when the file is absent or empty. Format matches
// the `leah connect slack` paste flow.
func loadSlackToken() (string, bool) {
	t := staticTokenForName("slack")
	if t == "" {
		return "", false
	}
	return string(t), true
}

func printSlackHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah slack <send|list|thread|search> [args...]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "  send <channel> <text...>   post a message to a channel")
	_, _ = fmt.Fprintln(w, "  list                       list channels visible to the bot")
	_, _ = fmt.Fprintln(w, "  thread <channel> <ts>      fetch a thread's replies")
	_, _ = fmt.Fprintln(w, "  search <query...>          top-5 results (requires User Token)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Run `leah connect slack` to paste the Bot Token (xoxb-…).")
}
