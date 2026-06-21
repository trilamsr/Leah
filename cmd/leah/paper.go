package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/papers"
)

// arxivEndpointOverride is the test-only injection point for FetchMetadata.
// Package-scoped variable so a stray ENV cannot redirect the production
// binary at runtime. Empty in every shipped build.
var arxivEndpointOverride string

// runPaper is the `leah paper <save|list|read>` dispatcher. State file is
// $LEAH_STATE_DIR/papers.jsonl, shared with future brief/dashboard consumers.
func runPaper(ctx context.Context, args []string, out io.Writer, errOut io.Writer) int {
	if shouldShowHelp(args) {
		printPaperHelp(out)
		return 0
	}
	if len(args) == 0 {
		printPaperHelp(errOut)
		return 2
	}

	store := &papers.Store{Path: filepath.Join(stateDir(), "papers.jsonl")}

	switch args[0] {
	case "save":
		return runPaperSave(ctx, args[1:], store, out, errOut)
	case "list":
		return runPaperList(args[1:], store, out, errOut)
	case "read":
		return runPaperRead(args[1:], store, out, errOut)
	default:
		_, _ = fmt.Fprintf(errOut, "leah paper: unknown subcommand %q\n", args[0])
		printPaperHelp(errOut)
		return 2
	}
}

func runPaperSave(ctx context.Context, args []string, store *papers.Store, out, errOut io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(errOut, "usage: leah paper save <arxiv-id|url>")
		return 2
	}
	id, err := papers.NormalizeArxivID(args[0])
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "leah paper save: %v\n", err)
		return 2
	}
	fetcher := &papers.ArxivFetcher{
		Endpoint: arxivEndpointOverride,
		Client:   &http.Client{Timeout: 10 * time.Second},
	}
	p, err := fetcher.FetchMetadata(ctx, id)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "leah paper save: %v\n", err)
		return 1
	}
	p.SavedTS = time.Now().UTC()
	p.Status = papers.StatusUnread
	if err := store.Save(p); err != nil {
		_, _ = fmt.Fprintf(errOut, "leah paper save: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "saved %s: %s\n", p.ID, p.Title)
	return 0
}

func runPaperList(args []string, store *papers.Store, out, errOut io.Writer) int {
	status, rest, err := parseStatusFlag(args)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "leah paper list: %v\n", err)
		return 2
	}
	if len(rest) > 0 {
		_, _ = fmt.Fprintf(errOut, "leah paper list: unexpected arg %q\n", rest[0])
		return 2
	}
	list, err := store.List(papers.Status(status))
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "leah paper list: %v\n", err)
		return 1
	}
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "(no papers)")
		return 0
	}
	for _, p := range list {
		_, _ = fmt.Fprintf(out, "%s  [%s]  %s\n  %s\n", p.ID, p.Status, p.Title, p.Link)
	}
	return 0
}

func runPaperRead(args []string, store *papers.Store, out, errOut io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(errOut, "usage: leah paper read <arxiv-id>")
		return 2
	}
	id, err := papers.NormalizeArxivID(args[0])
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "leah paper read: %v\n", err)
		return 2
	}
	if err := store.SetStatus(id, papers.StatusRead); err != nil {
		_, _ = fmt.Fprintf(errOut, "leah paper read: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "marked %s as read\n", id)
	return 0
}

// parseStatusFlag consumes a single --status <value> pair. Mirrors news.go's
// --bundle / --since parser so operator muscle-memory transfers.
func parseStatusFlag(args []string) (status string, rest []string, err error) {
	rest = make([]string, 0, len(args))
	seen := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--status":
			if seen {
				return "", nil, errors.New("--status may only be specified once")
			}
			if i+1 >= len(args) {
				return "", nil, errors.New("--status requires a value")
			}
			status = args[i+1]
			seen = true
			i++
		case strings.HasPrefix(a, "--status="):
			if seen {
				return "", nil, errors.New("--status may only be specified once")
			}
			status = strings.TrimPrefix(a, "--status=")
			if status == "" {
				return "", nil, errors.New("--status requires a value")
			}
			seen = true
		default:
			rest = append(rest, a)
		}
	}
	return status, rest, nil
}

func printPaperHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah paper <save|list|read> [...]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "  save <arxiv-id|url>     fetch arXiv metadata and queue for later")
	_, _ = fmt.Fprintln(w, "  list [--status S]       show queue (S=unread|reading|read)")
	_, _ = fmt.Fprintln(w, "  read <arxiv-id>         mark a saved paper as read")
}
