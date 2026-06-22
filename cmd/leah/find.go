package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/trilam/leah/internal/adapters/tmdb"
)

// tmdbTransportFactory is the test seam — overridden in find_test.go to point
// at an httptest server instead of api.themoviedb.org.
var tmdbTransportFactory = func() tmdb.Transport {
	base := os.Getenv("LEAH_TMDB_BASE_URL")
	if base == "" {
		base = "https://api.themoviedb.org/3"
	}
	return tmdb.NewHTTPTransport(&http.Client{Timeout: 10 * time.Second}, base)
}

// runFind dispatches `leah find <title...>`. Region defaults to US, overridable
// via --region or LEAH_TMDB_REGION. Missing token is a friendly exit-2 with
// the connect hint.
func runFind(ctx context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printFindHelp(w)
		return 0
	}
	fs := flag.NewFlagSet("find", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	region := fs.String("region", "", "ISO-3166 region code (default: $LEAH_TMDB_REGION or US)")
	if err := fs.Parse(args); err != nil {
		printFindHelp(os.Stderr)
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		printFindHelp(os.Stderr)
		return 2
	}

	tok, ok := loadTMDBToken()
	if !ok {
		_, _ = fmt.Fprintln(os.Stderr, "tmdb not connected — run: leah connect tmdb")
		return 2
	}

	chosen := *region
	if chosen == "" {
		chosen = os.Getenv("LEAH_TMDB_REGION")
	}
	if chosen == "" {
		chosen = "US"
	}

	ad, err := tmdb.New(tmdb.Config{
		Attestor:    noopAttestor{},
		TokenSource: staticToken(tok),
		Transport:   tmdbTransportFactory(),
		Region:      chosen,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah find: %v\n", err)
		return 1
	}

	query := strings.Join(rest, " ")
	res, err := ad.Find(ctx, query)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			_, _ = fmt.Fprintln(os.Stderr, "no results")
			return 1
		}
		_, _ = fmt.Fprintf(os.Stderr, "leah find: %v\n", err)
		return 1
	}
	renderFind(w, res)
	return 0
}

func renderFind(w io.Writer, r tmdb.Result) {
	if r.Year > 0 {
		_, _ = fmt.Fprintf(w, "%s (%d)\n", r.Title, r.Year)
	} else {
		_, _ = fmt.Fprintln(w, r.Title)
	}
	if names := providerNames(r.Flatrate); names != "" {
		_, _ = fmt.Fprintf(w, "  stream: %s\n", names)
	}
	if names := providerNames(r.Rent); names != "" {
		_, _ = fmt.Fprintf(w, "  rent:   %s\n", names)
	}
	if names := providerNames(r.Buy); names != "" {
		_, _ = fmt.Fprintf(w, "  buy:    %s\n", names)
	}
	if r.Flatrate == nil && r.Rent == nil && r.Buy == nil {
		_, _ = fmt.Fprintln(w, "  no providers available in this region")
	}
}

func providerNames(ps []tmdb.Provider) string {
	if len(ps) == 0 {
		return ""
	}
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.Name
	}
	return strings.Join(parts, ", ")
}

func loadTMDBToken() (string, bool) {
	t := staticTokenForName("tmdb")
	if t == "" {
		return "", false
	}
	return string(t), true
}

func printFindHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah find [--region XX] <title...>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "  --region XX    ISO-3166 region code (default: $LEAH_TMDB_REGION or US)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Run `leah connect tmdb` to paste the TMDB v4 read token.")
}
