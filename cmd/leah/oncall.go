package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
)

// defaultOncallWindow is the operator's mental default: "what fired in the
// last hour that looks like a problem?" — explicitly the polymath SWE
// triage-log gap the verb closes.
const defaultOncallWindow = time.Hour

// runOncall is the entry point shared by main.go and oncall_test.go. Takes
// the audit path + flag args + output writer so tests can drive it without
// stateDir/Stdout coupling.
func runOncall(auditPath string, args []string, out io.Writer) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah oncall [--since D] [--kind SUBSTR]")
		return 0
	}
	window := defaultOncallWindow
	kindFilter := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "--since="):
			d, err := parseCostDuration(strings.TrimPrefix(a, "--since="))
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "leah oncall: %v\n", err)
				return 2
			}
			window = d
		case a == "--since":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(os.Stderr, "leah oncall: --since needs DURATION (e.g. 1h, 24h, 7d)")
				return 2
			}
			i++
			d, err := parseCostDuration(args[i])
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "leah oncall: %v\n", err)
				return 2
			}
			window = d
		case strings.HasPrefix(a, "--kind="):
			kindFilter = strings.TrimPrefix(a, "--kind=")
		case a == "--kind":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(os.Stderr, "leah oncall: --kind needs SUBSTR")
				return 2
			}
			i++
			kindFilter = args[i]
		default:
			_, _ = fmt.Fprintf(os.Stderr, "leah oncall: unknown arg %q\n", a)
			return 2
		}
	}

	since := time.Now().Add(-window)
	rows, err := audit.Triage(auditPath, since, kindFilter)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah oncall: %v\n", err)
		return 1
	}
	if len(rows) == 0 {
		_, _ = fmt.Fprintf(out, "all clear (window=%s)\n", humanDuration(window))
		return 0
	}
	_, _ = fmt.Fprintf(out, "%d alarming signal(s) in last %s:\n", len(rows), humanDuration(window))
	for _, r := range rows {
		detail := r.Detail
		if len(detail) > 80 {
			detail = detail[:77] + "..."
		}
		_, _ = fmt.Fprintf(out, "  %s  %-28s  %-12s  %s\n", r.Timestamp, r.Kind, r.Outcome, detail)
	}
	return 0
}

// runOncallDefault wires runOncall to the operator's audit.jsonl. Kept as a
// thin shim so main.go stays uniform with the other verb registrations.
func runOncallDefault(args []string) int {
	return runOncall(filepath.Join(stateDir(), "audit.jsonl"), args, os.Stdout)
}
