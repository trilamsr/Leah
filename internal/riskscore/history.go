package riskscore

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

// bootstrapPrior is the cold-start failure rate when fewer than
// bootstrapMinSamples same-Kind rows exist in the window. See spec §5: 0.1
// biases first-of-a-Kind PRs to MEDIUM, not HIGH (Laplace would mis-tier).
const (
	bootstrapPrior      = 0.1
	bootstrapMinSamples = 10
)

// failureOutcomes is the closed set from spec §5 — adding a new outcome to
// the codebase requires updating this map in the same PR (CLAUDE.md root-cause).
// Unknown outcomes fail-open (counted as non-failure) to avoid rate inflation
// forcing critical-tier on every PR.
var failureOutcomes = map[string]bool{
	"block":              true,
	"failed":             true,
	"rejected":           true,
	"locked":             true,
	"attestation_denied": true,
}

// FailureRate walks the audit.jsonl at auditPath and returns the fraction of
// rows matching `kind` whose Outcome marks a failure, restricted to rows with
// timestamp ≥ since. samples is the in-window same-kind count; if below
// bootstrapMinSamples, returns the bootstrap prior 0.1.
//
// Missing or unreadable audit file is treated as zero samples (bootstrap path).
// Malformed JSON lines are skipped — audit replay tolerates partial corruption
// by design (selflearn/resolver.go pattern).
func FailureRate(auditPath string, kind string, since time.Time) (float64, int) {
	f, err := os.Open(auditPath)
	if err != nil {
		return bootstrapPrior, 0
	}
	defer func() { _ = f.Close() }()

	var total, failed int
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var e struct {
			TS      string `json:"ts"`
			Kind    string `json:"kind"`
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if e.Kind != kind {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.TS)
		if err != nil || ts.Before(since) {
			continue
		}
		total++
		if failureOutcomes[e.Outcome] {
			failed++
		}
	}
	if total < bootstrapMinSamples {
		return bootstrapPrior, total
	}
	return float64(failed) / float64(total), total
}
