package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"github.com/trilam/leah/internal/memory/watchlist"
)

// runWatch is the `leah watch` verb: zero-arg lists, `<sym>` adds,
// `--rm <sym>` removes. State file is shared with the morning-brief
// consumer at $LEAH_STATE_DIR/watchlist.json.
func runWatch(args []string, out io.Writer, errOut io.Writer) int {
	store := &watchlist.Store{Path: filepath.Join(stateDir(), "watchlist.json")}

	if len(args) == 0 {
		syms, err := store.List()
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "leah watch: %v\n", err)
			return 1
		}
		sort.Strings(syms)
		for _, s := range syms {
			_, _ = fmt.Fprintln(out, s)
		}
		return 0
	}

	if args[0] == "--rm" {
		if len(args) < 2 {
			_, _ = fmt.Fprintln(errOut, "usage: leah watch --rm <symbol>")
			return 2
		}
		if err := store.Remove(args[1]); err != nil {
			_, _ = fmt.Fprintf(errOut, "leah watch: %v\n", err)
			return 2
		}
		return 0
	}

	if err := store.Add(args[0]); err != nil {
		_, _ = fmt.Fprintf(errOut, "leah watch: %v\n", err)
		return 2
	}
	return 0
}
