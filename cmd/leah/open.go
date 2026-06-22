package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/trilam/leah/internal/adapters/launcher"
)

// openExecFactory is the test seam — production gets nativeExec; tests swap
// in a fake that records argv without touching the host UI.
var openExecFactory = func() launcher.ExecRunner { return nativeExec{} }

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
	if err := l.Open(ctx, target); err != nil {
		if errors.Is(err, launcher.ErrUnknownTarget) {
			_, _ = fmt.Fprintf(os.Stderr, "leah open: unknown target %q\n", target)
			return 2
		}
		_, _ = fmt.Fprintf(os.Stderr, "leah open: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(w, "opening %s\n", target)
	return 0
}

func printOpenHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah open <target>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Launches a streaming or social service via macOS open(1).")
	_, _ = fmt.Fprintln(w, "Prefers the native app's URL scheme when registered; falls back to web.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Targets: netflix, hulu, hbomax/max, disney/disneyplus, prime/primevideo,")
	_, _ = fmt.Fprintln(w, "         youtube, appletv, spotify, linkedin, facebook/fb, instagram/ig,")
	_, _ = fmt.Fprintln(w, "         x/twitter, tiktok, reddit, github/gh.")
}
