package feeds

import (
	"fmt"
	"sort"
)

// Synthesizer is the generic two-param shape every domain summarizer fits.
// T is the synth payload (DailyDigest, MarketPulse); In is the per-domain
// input slice element (Article, Quote). Two type params keep call-sites
// honest: news.Summarize over a []Quote wouldn't type-check.
type Synthesizer[T any, In any] interface {
	Summarize(items []In) T
}

// SynthFunc adapts a plain function to Synthesizer — same trick http.HandlerFunc
// uses. Lets `SummarizeNews` satisfy the interface without a wrapper type.
type SynthFunc[T any, In any] func(items []In) T

// Summarize satisfies Synthesizer[T, In].
func (f SynthFunc[T, In]) Summarize(items []In) T { return f(items) }

// SourceRef is the attribution chip the HUD + brief render under a synth
// result. Every output carries the source list so the operator can verify.
type SourceRef struct {
	Name string
	URL  string
}

// DailyDigest is the news.Summarize output: top-3 headlines + source list.
// Operator-facing — the brief one-liner reads from .Headlines[0].
type DailyDigest struct {
	Headlines []Headline
	Sources   []SourceRef
}

// Headline is one entry in the digest. Source name doubles as attribution
// chip; URL is the click-through.
type Headline struct {
	Title  string
	URL    string
	Source string
}

// SummarizeNews ranks by recency (zero-time sorts last as "unknown age") and
// returns the top-3. Source-trust and dedup-cluster ranking are follow-ups;
// current output is pure recency, fast and deterministic.
func SummarizeNews(items []Article) DailyDigest {
	if len(items) == 0 {
		return DailyDigest{}
	}
	sorted := make([]Article, len(items))
	copy(sorted, items)
	sort.SliceStable(sorted, func(i, j int) bool {
		// Zero (unknown) sorts last; otherwise newer first.
		if sorted[i].Published.IsZero() {
			return false
		}
		if sorted[j].Published.IsZero() {
			return true
		}
		return sorted[i].Published.After(sorted[j].Published)
	})
	n := 3
	if n > len(sorted) {
		n = len(sorted)
	}
	heads := make([]Headline, 0, n)
	srcSeen := make(map[string]struct{})
	var srcs []SourceRef
	for i := 0; i < n; i++ {
		a := sorted[i]
		heads = append(heads, Headline{Title: a.Title, URL: a.URL, Source: a.Source})
		if _, ok := srcSeen[a.Source]; !ok {
			srcSeen[a.Source] = struct{}{}
			srcs = append(srcs, SourceRef{Name: a.Source, URL: a.URL})
		}
	}
	return DailyDigest{Headlines: heads, Sources: srcs}
}

// MarketPulse is the market.Summarize output: one descriptive line per
// symbol + an optional plain-language summary. Per spec §7 threat model:
// NEVER carries advice verbs (buy/sell/should/recommend) — the no-advice
// lint test enforces.
type MarketPulse struct {
	Lines   []string
	Summary string
}

// SummarizeMarket renders per-symbol % change. Format is deliberately
// boring — "AAPL +1.7% @ 203.40" — to short-circuit any operator
// interpretation as a recommendation. Anomaly-flagging (|Δ| > 3σ vs 30d) is
// a follow-up; current output is purely descriptive.
func SummarizeMarket(items []Quote) MarketPulse {
	if len(items) == 0 {
		return MarketPulse{}
	}
	lines := make([]string, 0, len(items))
	for _, q := range items {
		sign := "+"
		if q.ChangePct < 0 {
			sign = ""
		}
		if q.Price > 0 {
			lines = append(lines, fmt.Sprintf("%s %s%.1f%% @ %.2f", q.Symbol, sign, q.ChangePct, q.Price))
		} else {
			lines = append(lines, fmt.Sprintf("%s %s%.1f%%", q.Symbol, sign, q.ChangePct))
		}
	}
	return MarketPulse{Lines: lines}
}
