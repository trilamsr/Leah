package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PanicRateRule scans the obs metrics snapshot for per-goroutine-name
// panic-counter growth since the last baseline. Crossing Threshold emits
// a Candidate for the bug-fix self-build hook
// (see docs/specs/2026-06-09-bug-fix-self-build-hook.md). Detect is
// safe to call from the weekly selflearn tick.
type PanicRateRule struct {
	// Threshold is the minimum per-name delta that emits a Candidate.
	// Default 3 — empirically large enough to filter single-run noise
	// while still firing within a week on a real defect.
	Threshold int64
}

// Candidate is a single per-name panic-rate spike worth surfacing to
// the operator. Persisted to ~/.leah-state/bug-fix-candidates.md by the
// daemon — NEVER auto-dispatched to regatta (operator gate is the only
// structural defense against self-modification drift).
type Candidate struct {
	Name       string
	Delta      int64
	Total      int64
	PanicFile  string // empty when no matching file exists
	PanicStack string // raw bytes; intentionally NOT Leah-summarized
}

// Name identifies the rule in audit logs + operator-facing output.
// Satisfies selflearn.PanicDetector.
func (r PanicRateRule) Name() string { return "panic_rate" }

// Detect compares the latest metrics snapshot under
// <stateDir>/metrics/latest.json against the per-name counter baseline
// at <stateDir>/selflearn/panic-baseline.json and emits a Candidate per
// name whose delta meets Threshold. On the cold-start case (no metrics
// snapshot yet) Detect returns (nil, nil) — the first run primes the
// baseline implicitly; selflearn callers persist Candidate.Total back
// to the baseline on success.
func (r PanicRateRule) Detect(ctx context.Context, stateDir string) ([]Candidate, error) {
	if r.Threshold <= 0 {
		r.Threshold = 3
	}

	current, err := readCounters(filepath.Join(stateDir, "metrics", "latest.json"))
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil // cold start
	}

	baseline, err := readBaselineCounters(filepath.Join(stateDir, "selflearn", "panic-baseline.json"))
	if err != nil {
		return nil, err
	}

	// Sort to keep output deterministic — caller writes markdown, so
	// stable order matters for diffing across weekly runs.
	keys := make([]string, 0, len(current))
	for k := range current {
		if strings.HasPrefix(k, "leah_panic_total|") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var out []Candidate
	for _, k := range keys {
		name := extractName(k)
		if name == "" {
			continue
		}
		delta := current[k] - baseline[k]
		if delta < r.Threshold {
			continue
		}
		c := Candidate{Name: name, Delta: delta, Total: current[k]}
		if path, body, ok := mostRecentPanicFile(stateDir, name); ok {
			c.PanicFile = path
			c.PanicStack = body
		}
		out = append(out, c)
	}
	return out, nil
}

// BuildIssueBody renders a regatta-issue body. Format is intentionally
// raw-context-only: Leah does NOT interpret the stack. The downstream
// regatta agent (with Leah's reviewer subagent gating it) does the
// analysis — keeping Leah-narrative out of the body avoids
// prompt-injection-via-self-narrative (adversarial review §goodhart).
func BuildIssueBody(c Candidate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Leah self-noticed panic-rate spike: %s\n\n", c.Name)
	fmt.Fprintf(&b, "- goroutine name: `%s`\n", c.Name)
	fmt.Fprintf(&b, "- panics this window: %d\n", c.Delta)
	fmt.Fprintf(&b, "- cumulative total: %d\n", c.Total)
	if c.PanicFile != "" {
		fmt.Fprintf(&b, "- most-recent panic file: `%s`\n", c.PanicFile)
	}
	b.WriteString("\n## Raw panic stack\n\n")
	b.WriteString("Leah does not summarize this — investigate the raw bytes.\n\n")
	b.WriteString("```\n")
	if c.PanicStack == "" {
		b.WriteString("(no panic file matched the name; check $LEAH_STATE_DIR/panics/)\n")
	} else {
		b.WriteString(c.PanicStack)
		if !strings.HasSuffix(c.PanicStack, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("```\n")
	return b.String()
}

// readCounters loads the counters map from an obs snapshot, returning
// (nil, nil) when the file does not exist (cold-start signal).
func readCounters(path string) (map[string]int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read metrics: %w", err)
	}
	var snap struct {
		Counters map[string]int64 `json:"counters"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("parse metrics: %w", err)
	}
	return snap.Counters, nil
}

// readBaselineCounters loads the per-name counter snapshot from the
// previous selflearn run. Missing file → empty baseline (every counter
// is treated as new).
func readBaselineCounters(path string) (map[string]int64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]int64{}, nil
		}
		return nil, fmt.Errorf("read baseline: %w", err)
	}
	var snap struct {
		Counters map[string]int64 `json:"counters"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}
	if snap.Counters == nil {
		snap.Counters = map[string]int64{}
	}
	return snap.Counters, nil
}

// extractName parses "leah_panic_total|name=<n>" → "<n>". Returns ""
// when the bucket key has no name label (the obs registry guarantees
// it always does, but we defensively skip malformed keys).
func extractName(bucketKey string) string {
	const prefix = "leah_panic_total|name="
	if !strings.HasPrefix(bucketKey, prefix) {
		return ""
	}
	return bucketKey[len(prefix):]
}

// mostRecentPanicFile scans <stateDir>/panics/ for the newest file
// whose suffix matches "-<name>.txt" (the obs.writePanicFile shape).
// Returns (path, body, true) on hit; (_, _, false) when nothing
// matches. Sort by filename — the obs writer prefixes a UTC timestamp,
// so lexicographic order == chronological order without a stat() call.
func mostRecentPanicFile(stateDir, name string) (string, string, bool) {
	dir := filepath.Join(stateDir, "panics")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", false
	}
	suffix := "-" + name + ".txt"
	var match string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, suffix) {
			continue
		}
		if n > match {
			match = n
		}
	}
	if match == "" {
		return "", "", false
	}
	path := filepath.Join(dir, match)
	body, err := os.ReadFile(path)
	if err != nil {
		return "", "", false
	}
	return path, string(body), true
}
