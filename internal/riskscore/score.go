// Package riskscore implements the per-PR risk formula + tier classifier
// from docs/engineer/specs/2026-06-10-selfbuild-attestation-risk.md §2-§3.
//
// risk = BR × historical_failure_rate × log10(max(diff_LOC, 10))
//
// Pure functions, zero LLM cost. Consumed by W117 (pool selection) and W118
// (dispatcher.selfbuild.Run) to tier attestation difficulty.
package riskscore

import "math"

// diffLOCFloor caps the log10 input so 1-line PRs score ≥1.0 rather than 0.
// §2 rationale: BR + failure-rate must remain the dominant signals; clamping
// at 10 keeps log10 ≥ 1.
const diffLOCFloor = 10

// brMax is the canonical BR ceiling from spec §2.1. Out-of-range BR (negative
// or >brMax) is treated as brMax — fail-closed per §2.1, so a writer bug
// stamping BR=-1 cannot downgrade attestation by producing a negative score.
const brMax = 4

// Compute returns the risk score for one PR. Multiplicative so BR=0 zeroes
// out — a read-only meta PR costs no attestation effort regardless of size.
// Out-of-range BR clamps to brMax (§2.1 fail-closed).
func Compute(br int, failureRate float64, diffLOC int) float64 {
	if br < 0 || br > brMax {
		br = brMax
	}
	if diffLOC < diffLOCFloor {
		diffLOC = diffLOCFloor
	}
	return float64(br) * failureRate * math.Log10(float64(diffLOC))
}

// Tier maps a score to one of low/medium/high/critical per §3 boundaries.
// Boundaries are inclusive at the upper edge of the next tier (≥2 medium,
// ≥5 high, ≥10 critical) so a score sitting exactly on a boundary lands in
// the stricter tier — fail-closed on numeric ambiguity.
func Tier(score float64) string {
	switch {
	case score >= 10.0:
		return "critical"
	case score >= 5.0:
		return "high"
	case score >= 2.0:
		return "medium"
	default:
		return "low"
	}
}
