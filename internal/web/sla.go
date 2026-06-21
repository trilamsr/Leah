package web

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
)

// SLARow surfaces one A1-A9 user-felt-latency histogram with the two
// quantiles the operator actually steers by (median + tail). Hidden until
// surfaced here: the histograms shipped in Prometheus text but had no UI.
type SLARow struct {
	Label     string  `json:"label"`     // A1..A9 — anchors the spec table.
	Histogram string  `json:"histogram"` // canonical metric name (for grep + /metrics cross-ref).
	P50       float64 `json:"p50_seconds"`
	P95       float64 `json:"p95_seconds"`
	Count     int64   `json:"count"`
}

// slaSpec is the source-of-truth ordering — UI renders A1..A9 top-to-bottom.
// Adding A10 means appending one row here; nothing else.
var slaSpec = []struct {
	Label string
	Name  string
}{
	{"A1", "leah_voice_wake_to_earcon_seconds"},
	{"A2", "leah_voice_utterance_to_transcript_seconds"},
	{"A3", "leah_intent_classify_seconds"},
	{"A4", "leah_voice_intent_to_first_audio_seconds"},
	{"A5", "leah_hud_state_to_widget_seconds"},
	{"A6", "leah_cli_dispatch_to_first_byte_seconds"},
	{"A7", "leah_cli_dispatch_to_first_progress_seconds"},
	{"A8", "leah_voice_barge_in_cancel_seconds"},
	{"A9", "leah_onboarding_install_to_first_reply_seconds"},
}

// readSLA derives the A1-A9 panel from the obs.Registry latest.json snapshot.
// Always returns nine rows — a zero row means "no observations yet" rather
// than "histogram missing", which is the failure mode the operator is
// actually watching for. File errors degrade to all-zero rows (same shape).
func readSLA(path string) []SLARow {
	out := make([]SLARow, len(slaSpec))
	for i, sp := range slaSpec {
		out[i] = SLARow{Label: sp.Label, Histogram: sp.Name}
	}
	if path == "" {
		return out
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var snap struct {
		Histograms map[string]struct {
			Count   int64            `json:"count"`
			Sum     float64          `json:"sum"`
			Buckets map[string]int64 `json:"buckets"`
		} `json:"histograms"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return out
	}
	for i, sp := range slaSpec {
		out[i] = aggregateSeries(sp.Label, sp.Name, snap.Histograms)
	}
	return out
}

// aggregateSeries collapses every labeled series sharing a histogram name
// (e.g. {kind=A}, {kind=B}) into a single row by summing per-bucket counts.
// Per-kind SLA hides the cross-cutting "is voice-intent slow?" signal.
func aggregateSeries(label, name string, hists map[string]struct {
	Count   int64            `json:"count"`
	Sum     float64          `json:"sum"`
	Buckets map[string]int64 `json:"buckets"`
}) SLARow {
	row := SLARow{Label: label, Histogram: name}
	merged := map[string]int64{}
	for k, v := range hists {
		base := k
		if i := strings.Index(k, "|"); i >= 0 {
			base = k[:i]
		}
		if base != name {
			continue
		}
		row.Count += v.Count
		for b, c := range v.Buckets {
			merged[b] += c
		}
	}
	if row.Count == 0 || len(merged) == 0 {
		return row
	}
	row.P50 = quantile(merged, 0.50)
	row.P95 = quantile(merged, 0.95)
	return row
}

// quantile estimates the qth quantile from a per-bucket count distribution
// using Prometheus' linear-interpolation-within-bucket model. Returns the
// upper bound of the +Inf bucket's predecessor when q falls past the last
// finite bucket — degrading toward "saturated" rather than NaN.
func quantile(buckets map[string]int64, q float64) float64 {
	type edge struct {
		ub    float64
		count int64
		inf   bool
	}
	edges := make([]edge, 0, len(buckets))
	for k, c := range buckets {
		if k == "+Inf" {
			edges = append(edges, edge{inf: true, count: c})
			continue
		}
		ub, err := strconv.ParseFloat(k, 64)
		if err != nil {
			continue
		}
		edges = append(edges, edge{ub: ub, count: c})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].inf {
			return false
		}
		if edges[j].inf {
			return true
		}
		return edges[i].ub < edges[j].ub
	})
	var total int64
	for _, e := range edges {
		total += e.count
	}
	if total == 0 {
		return 0
	}
	target := q * float64(total)
	var cum int64
	var prevUB float64
	for _, e := range edges {
		next := cum + e.count
		if float64(next) >= target {
			if e.inf {
				return prevUB
			}
			if e.count == 0 {
				return e.ub
			}
			frac := (target - float64(cum)) / float64(e.count)
			return prevUB + (e.ub-prevUB)*frac
		}
		cum = next
		if !e.inf {
			prevUB = e.ub
		}
	}
	return prevUB
}
