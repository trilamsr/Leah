package feeds_test

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/feeds"
)

// TestSynth_News_Top3 ranks by recency and trims to top-3 with attribution.
func TestSynth_News_Top3(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	in := []feeds.Article{
		{Title: "oldest", URL: "u1", Source: "hn", Published: now.Add(-5 * time.Hour)},
		{Title: "middle", URL: "u2", Source: "google-news", Published: now.Add(-2 * time.Hour)},
		{Title: "newest", URL: "u3", Source: "reddit", Published: now.Add(-30 * time.Minute)},
		{Title: "older-mid", URL: "u4", Source: "hn", Published: now.Add(-3 * time.Hour)},
		{Title: "ancient", URL: "u5", Source: "hn", Published: now.Add(-24 * time.Hour)},
	}
	d := feeds.SummarizeNews(in)
	if len(d.Headlines) != 3 {
		t.Fatalf("Headlines = %d, want 3", len(d.Headlines))
	}
	want := []string{"newest", "middle", "older-mid"}
	got := []string{d.Headlines[0].Title, d.Headlines[1].Title, d.Headlines[2].Title}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Top-3 order = %v, want %v", got, want)
	}
	// Source attribution present on every headline.
	for i, h := range d.Headlines {
		if h.Source == "" {
			t.Errorf("Headlines[%d].Source empty", i)
		}
	}
	if len(d.Sources) == 0 {
		t.Errorf("Sources list empty")
	}
}

// TestSynth_News_FewerThanThree_NoPanic — synth handles short input.
func TestSynth_News_FewerThanThree_NoPanic(t *testing.T) {
	in := []feeds.Article{
		{Title: "only one", Source: "hn", Published: time.Now()},
	}
	d := feeds.SummarizeNews(in)
	if len(d.Headlines) != 1 {
		t.Errorf("Headlines = %d, want 1", len(d.Headlines))
	}
}

// TestSynth_News_Empty_ReturnsEmptyDigest — synth handles zero input.
func TestSynth_News_Empty_ReturnsEmptyDigest(t *testing.T) {
	d := feeds.SummarizeNews(nil)
	if len(d.Headlines) != 0 {
		t.Errorf("Headlines = %d, want 0", len(d.Headlines))
	}
}

// TestSynth_Market_PercentChange formats per-symbol % delta line.
func TestSynth_Market_PercentChange(t *testing.T) {
	in := []feeds.Quote{
		{Symbol: "AAPL", Price: 203.40, PrevClose: 200.00, Change: 3.40, ChangePct: 1.7},
		{Symbol: "MSFT", Price: 400.00, PrevClose: 402.00, Change: -2.00, ChangePct: -0.5},
	}
	p := feeds.SummarizeMarket(in)
	if len(p.Lines) != 2 {
		t.Fatalf("Lines = %d, want 2", len(p.Lines))
	}
	if !strings.Contains(p.Lines[0], "AAPL") || !strings.Contains(p.Lines[0], "+1.7") {
		t.Errorf("Line[0] = %q, want AAPL +1.7%%", p.Lines[0])
	}
	if !strings.Contains(p.Lines[1], "MSFT") || !strings.Contains(p.Lines[1], "-0.5") {
		t.Errorf("Line[1] = %q, want MSFT -0.5%%", p.Lines[1])
	}
}

// TestSynth_Market_NoAdvice_Lint scans output for forbidden advice tokens.
// Per spec §7: market synthesizer NEVER emits buy/sell/should/recommend.
// Operator-protection lint, not a behavior test.
func TestSynth_Market_NoAdvice_Lint(t *testing.T) {
	in := []feeds.Quote{
		{Symbol: "AAPL", ChangePct: 5.0},
		{Symbol: "MSFT", ChangePct: -10.0}, // would tempt a "sell" hint
		{Symbol: "TSLA", ChangePct: 25.0},  // would tempt a "buy" hint
	}
	p := feeds.SummarizeMarket(in)
	combined := strings.ToLower(strings.Join(p.Lines, " ") + " " + p.Summary)
	forbidden := []string{"buy", "sell", "should", "recommend"}
	for _, tok := range forbidden {
		// Word-boundary match so e.g. "buyback" wouldn't false-positive but
		// neither would Symbol "BUY" — we have none, so plain substring suffices.
		re := regexp.MustCompile(`\b` + tok + `\b`)
		if re.MatchString(combined) {
			t.Errorf("MarketPulse output contains forbidden advice token %q in %q", tok, combined)
		}
	}
}

// TestSynth_Market_Empty_ReturnsEmpty — handles zero input.
func TestSynth_Market_Empty_ReturnsEmpty(t *testing.T) {
	p := feeds.SummarizeMarket(nil)
	if len(p.Lines) != 0 {
		t.Errorf("Lines = %d, want 0", len(p.Lines))
	}
}

// TestSynthesizer_GenericInterface_Compiles — guard the generic shape compiles.
// Both rule-based synthesizers fit Synthesizer[T] via a thin adapter.
func TestSynthesizer_GenericInterface_Compiles(t *testing.T) {
	var _ feeds.Synthesizer[feeds.DailyDigest, feeds.Article] = feeds.SynthFunc[feeds.DailyDigest, feeds.Article](feeds.SummarizeNews)
	var _ feeds.Synthesizer[feeds.MarketPulse, feeds.Quote] = feeds.SynthFunc[feeds.MarketPulse, feeds.Quote](feeds.SummarizeMarket)
}
