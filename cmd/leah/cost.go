package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/trilam/leah/internal/costview"
)

// defaultCostWindow is the default --since value when the operator does
// not supply one. 30 days lines up with audit retention + Anthropic billing
// cycle.
const defaultCostWindow = 30 * 24 * time.Hour

// runCost is the `leah cost` entry point. Single positional command, three
// flags: --since DURATION, --by kind|day|model, --json.
func runCost(args []string) {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah cost [--since D] [--by kind|day|model] [--json]")
		return
	}
	window := defaultCostWindow
	by := ""
	jsonMode := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			jsonMode = true
		case strings.HasPrefix(a, "--since="):
			d, err := parseCostDuration(strings.TrimPrefix(a, "--since="))
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "leah cost: %v\n", err)
				os.Exit(2)
			}
			window = d
		case a == "--since":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(os.Stderr, "leah cost: --since needs DURATION (e.g. 7d, 24h)")
				os.Exit(2)
			}
			i++
			d, err := parseCostDuration(args[i])
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "leah cost: %v\n", err)
				os.Exit(2)
			}
			window = d
		case strings.HasPrefix(a, "--by="):
			by = strings.TrimPrefix(a, "--by=")
		case a == "--by":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(os.Stderr, "leah cost: --by needs kind|day|model")
				os.Exit(2)
			}
			i++
			by = args[i]
		default:
			_, _ = fmt.Fprintf(os.Stderr, "leah cost: unknown arg %q\n", a)
			os.Exit(2)
		}
	}

	since := time.Now().Add(-window)
	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	summary, err := costview.Aggregate(auditPath, since)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah cost: aggregate: %v\n", err)
		os.Exit(1)
	}

	if jsonMode {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(summary); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah cost: encode: %v\n", err)
			os.Exit(1)
		}
		return
	}

	printCostText(os.Stdout, summary, window, by)
}

// printCostText renders the terse 2-column layout. Header always shows
// total + count + window; body switches on --by.
func printCostText(w *os.File, s costview.Summary, window time.Duration, by string) {
	_, _ = fmt.Fprintf(w, "leah cost — last %s\n", humanDuration(window))
	_, _ = fmt.Fprintf(w, "  total   $%.4f\n", s.TotalUSD)
	_, _ = fmt.Fprintf(w, "  rows    %d\n", s.Count)
	_, _ = fmt.Fprintln(w)

	switch by {
	case "kind":
		printBuckets(w, "by kind", s.TopKinds(0))
	case "day":
		printDayTable(w, s.ByDay)
	case "model":
		printBuckets(w, "by model", s.TopModels(0))
	case "":
		printBuckets(w, "top kinds", s.TopKinds(3))
	default:
		_, _ = fmt.Fprintf(os.Stderr, "leah cost: --by %q not in {kind,day,model}\n", by)
		os.Exit(2)
	}
}

func printBuckets(w *os.File, title string, buckets []costview.Bucket) {
	if len(buckets) == 0 {
		_, _ = fmt.Fprintf(w, "  (no rows in window)\n")
		return
	}
	_, _ = fmt.Fprintf(w, "  %s:\n", title)
	for _, b := range buckets {
		_, _ = fmt.Fprintf(w, "    %-24s  $%.4f\n", b.Name, b.USD)
	}
}

func printDayTable(w *os.File, byDay map[string]float64) {
	if len(byDay) == 0 {
		_, _ = fmt.Fprintf(w, "  (no rows in window)\n")
		return
	}
	days := make([]string, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Strings(days)
	_, _ = fmt.Fprintf(w, "  by day:\n")
	for _, d := range days {
		_, _ = fmt.Fprintf(w, "    %s  $%.4f\n", d, byDay[d])
	}
}

// parseCostDuration accepts stdlib time.ParseDuration units plus an
// operator-friendly Nd suffix (days). 30d → 720h.
func parseCostDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid duration %q (try 7d, 24h)", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (try 7d, 24h)", s)
	}
	return d, nil
}

// humanDuration renders 720h as "30d", 24h as "1d", 12h as "12h".
func humanDuration(d time.Duration) string {
	hours := int(d / time.Hour)
	if hours%24 == 0 && hours >= 24 {
		return fmt.Sprintf("%dd", hours/24)
	}
	return fmt.Sprintf("%dh", hours)
}
