package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/patterns"
)

// runPatterns renders skill-candidates markdown to stdout.
func runPatterns(args []string) int {
	fs := flag.NewFlagSet("patterns", flag.ExitOnError)
	weekly := fs.Bool("weekly", false, "use 7-day window instead of default 30-day")
	minCount := fs.Int("min-count", patterns.DefaultMinCount, "min cluster size to emit")
	_ = fs.Parse(args)

	window := 30 * 24 * time.Hour
	if *weekly {
		window = 7 * 24 * time.Hour
	}
	since := time.Now().Add(-window)

	auditPath := filepath.Join(stateDir(), "audit.jsonl")
	clusters, err := patterns.DetectWithThreshold(auditPath, since, *minCount)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah patterns: %v\n", err)
		return 1
	}

	a := &audit.Logger{Path: auditPath}
	_ = a.Append(audit.Entry{Kind: "patterns", BlastRadius: 0, Outcome: "success", Detail: fmt.Sprintf("clusters=%d window=%s", len(clusters), window)})
	fmt.Print(patterns.Propose(clusters))
	return 0
}
