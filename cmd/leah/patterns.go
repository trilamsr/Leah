package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/thinking/patterns"
)

// runPatterns renders skill-candidates markdown to stdout.
func runPatterns(args []string) int {
	fs := flag.NewFlagSet("patterns", flag.ContinueOnError)
	weekly := fs.Bool("weekly", false, "use 7-day window instead of default 30-day")
	minCount := fs.Int("min-count", patterns.DefaultMinCount, "min cluster size to emit")
	if err := fs.Parse(args); err != nil {
		fs.Usage()
		return 2
	}

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

	logger := &audit.Logger{Path: auditPath}
	_ = logger.Append(audit.Entry{Kind: "patterns", BlastRadius: 0, Outcome: "success", Detail: fmt.Sprintf("clusters=%d window=%s", len(clusters), window)})
	fmt.Print(patterns.Propose(clusters))
	return 0
}
