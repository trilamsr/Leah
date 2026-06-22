package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/trilam/leah/internal/adapters/launcher"
	"github.com/trilam/leah/internal/adapters/tmdb"
)

// openExecFactory is the test seam — production gets nativeExec; tests swap
// in a fake that records argv without touching the host UI.
var openExecFactory = func() launcher.ExecRunner { return nativeExec{} }

// titleResolver maps a free-form title to an ordered list of flatrate provider
// names. The interface stays minimal so smart-route stays decoupled from TMDB.
type titleResolver interface {
	Resolve(ctx context.Context, query string) ([]string, error)
}

// errNoTMDBToken signals that the optional TMDB fallback is unavailable so
// runOpen can treat the unknown arg as a launcher-level unknown target rather
// than fataling on the missing token.
var errNoTMDBToken = errors.New("open: tmdb token absent")

// titleResolverFactory is the test seam — production builds a TMDB-backed
// resolver if a token is configured; tests inject a fake.
var titleResolverFactory = newDefaultTitleResolver

func runOpen(ctx context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printOpenHelp(w)
		return 0
	}
	if len(args) < 1 {
		printOpenHelp(os.Stderr)
		return 2
	}
	target := args[0]

	l, err := launcher.New(openExecFactory(), launcher.DefaultIntents())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah open: %v\n", err)
		return 1
	}
	err = l.Open(ctx, target)
	if err == nil {
		_, _ = fmt.Fprintf(w, "opening %s\n", target)
		return 0
	}
	if !errors.Is(err, launcher.ErrUnknownTarget) {
		_, _ = fmt.Fprintf(os.Stderr, "leah open: %v\n", err)
		return 1
	}
	return resolveAndOpen(ctx, l, target, w)
}

// resolveAndOpen is the smart-route fallback: TMDB → first mapped flatrate →
// launcher. Token-absent collapses back to the original "unknown target"
// because the operator may have just typo'd a known intent.
func resolveAndOpen(ctx context.Context, l *launcher.Launcher, target string, w io.Writer) int {
	res, err := titleResolverFactory()
	if err != nil {
		if errors.Is(err, errNoTMDBToken) {
			_, _ = fmt.Fprintf(os.Stderr, "leah open: unknown target %q\n", target)
			return 2
		}
		_, _ = fmt.Fprintf(os.Stderr, "leah open: %v\n", err)
		return 1
	}
	names, err := res.Resolve(ctx, target)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah open: %v\n", err)
		return 1
	}
	if len(names) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "leah open: no results")
		return 1
	}
	for _, n := range names {
		if key, ok := providerIntentKey(n); ok {
			if err := l.Open(ctx, key); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "leah open: %v\n", err)
				return 1
			}
			_, _ = fmt.Fprintf(w, "opening %s on %s\n", target, n)
			return 0
		}
	}
	_, _ = fmt.Fprintf(w, "%s streams on: %s\n", target, strings.Join(names, ", "))
	_, _ = fmt.Fprintln(w, "hint: leah open <provider> (none of the above are wired into the launcher yet)")
	return 0
}

// providerIntentKey maps a TMDB provider_name to a launcher intent key. Drift
// risk (e.g. Max ↔ HBO Max rebrand) is the cost of a static table; the
// alternative — config-driven mapping — is premature for v1.
func providerIntentKey(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "netflix":
		return "netflix", true
	case "hulu":
		return "hulu", true
	case "max", "hbo max":
		return "max", true
	case "disney plus", "disney+":
		return "disneyplus", true
	case "amazon prime video", "prime video":
		return "primevideo", true
	case "apple tv plus", "apple tv+":
		return "appletv", true
	}
	return "", false
}

func newDefaultTitleResolver() (titleResolver, error) {
	tok, ok := loadTMDBToken()
	if !ok {
		return nil, errNoTMDBToken
	}
	region := os.Getenv("LEAH_TMDB_REGION")
	if region == "" {
		region = "US"
	}
	ad, err := tmdb.New(tmdb.Config{
		Attestor:    noopAttestor{},
		TokenSource: staticToken(tok),
		Transport:   tmdbTransportFactory(),
		Region:      region,
	})
	if err != nil {
		return nil, err
	}
	return &tmdbResolver{ad: ad}, nil
}

type tmdbResolver struct{ ad *tmdb.Adapter }

func (r *tmdbResolver) Resolve(ctx context.Context, query string) ([]string, error) {
	res, err := r.ad.Find(ctx, query)
	if err != nil {
		if errors.Is(err, tmdb.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, len(res.Flatrate))
	for i, p := range res.Flatrate {
		out[i] = p.Name
	}
	return out, nil
}

func printOpenHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah open <target>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Launches a streaming or social service via macOS open(1).")
	_, _ = fmt.Fprintln(w, "Prefers the native app's URL scheme when registered; falls back to web.")
	_, _ = fmt.Fprintln(w, "Unknown args fall back to TMDB title-lookup when `leah connect tmdb` ran.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Targets: netflix, hulu, hbomax/max, disney/disneyplus, prime/primevideo,")
	_, _ = fmt.Fprintln(w, "         youtube, appletv, spotify, linkedin, facebook/fb, instagram/ig,")
	_, _ = fmt.Fprintln(w, "         x/twitter, tiktok, reddit, github/gh.")
}
