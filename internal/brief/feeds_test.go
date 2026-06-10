package brief

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeWeather returns a canned forecast+err pair.
type fakeWeather struct {
	f   Forecast
	err error
}

func (w *fakeWeather) Today(ctx context.Context) (Forecast, error) { return w.f, w.err }

// fakeNews returns a canned article+err pair.
type fakeNews struct {
	a   Article
	err error
}

func (n *fakeNews) Top(ctx context.Context) (Article, error) { return n.a, n.err }

// fakeMarket returns a canned pulse+err pair.
type fakeMarket struct {
	p   Pulse
	err error
}

func (m *fakeMarket) Snapshot(ctx context.Context) (Pulse, error) { return m.p, m.err }

// TestBrief_WeatherRendered_HappyPath asserts a populated Weather payload
// renders the temp + description one-liner.
func TestBrief_WeatherRendered_HappyPath(t *testing.T) {
	d := Data{
		Now: time.Now(),
		Weather: &Forecast{
			TempC:       18,
			HighC:       21,
			LowC:        9,
			Description: "light rain",
			PrecipPct:   60,
		},
	}
	out := Render(d)
	if !strings.Contains(out, "## Weather") {
		t.Fatalf("missing Weather header:\n%s", out)
	}
	for _, want := range []string{"18", "21", "9", "light rain"} {
		if !strings.Contains(out, want) {
			t.Errorf("weather section missing %q in:\n%s", want, out)
		}
	}
}

// TestBrief_WeatherUnavailable_RendersFallback asserts the WeatherUnavailable
// flag renders "unavailable" mirroring the mail/calendar pattern.
func TestBrief_WeatherUnavailable_RendersFallback(t *testing.T) {
	d := Data{Now: time.Now(), WeatherUnavailable: true}
	out := Render(d)
	if !strings.Contains(out, "## Weather") || !strings.Contains(out, "unavailable") {
		t.Errorf("expected weather unavailable fallback, got:\n%s", out)
	}
}

// TestBrief_NewsRendered_HappyPath asserts the top article appears with
// title and source attribution.
func TestBrief_NewsRendered_HappyPath(t *testing.T) {
	d := Data{
		Now:  time.Now(),
		News: &Article{Title: "Rust 2026 lands", Source: "HN", URL: "https://news.example/1"},
	}
	out := Render(d)
	if !strings.Contains(out, "## News") {
		t.Fatalf("missing News header:\n%s", out)
	}
	for _, want := range []string{"Rust 2026 lands", "HN"} {
		if !strings.Contains(out, want) {
			t.Errorf("news section missing %q in:\n%s", want, out)
		}
	}
}

// TestBrief_NewsUnavailable_RendersFallback asserts the flag renders "unavailable".
func TestBrief_NewsUnavailable_RendersFallback(t *testing.T) {
	d := Data{Now: time.Now(), NewsUnavailable: true}
	out := Render(d)
	if !strings.Contains(out, "## News") || !strings.Contains(out, "unavailable") {
		t.Errorf("expected news unavailable fallback, got:\n%s", out)
	}
}

// TestBrief_MarketRendered_HappyPath asserts each tracked symbol's %-change
// appears in the snapshot line.
func TestBrief_MarketRendered_HappyPath(t *testing.T) {
	d := Data{
		Now: time.Now(),
		Market: &Pulse{Quotes: []Quote{
			{Symbol: "AAPL", PercentChange: 1.2},
			{Symbol: "MSFT", PercentChange: -0.4},
		}},
	}
	out := Render(d)
	if !strings.Contains(out, "## Market") {
		t.Fatalf("missing Market header:\n%s", out)
	}
	for _, want := range []string{"AAPL", "MSFT", "1.2", "-0.4"} {
		if !strings.Contains(out, want) {
			t.Errorf("market section missing %q in:\n%s", want, out)
		}
	}
}

// TestBrief_MarketUnavailable_RendersFallback asserts the flag renders "unavailable".
func TestBrief_MarketUnavailable_RendersFallback(t *testing.T) {
	d := Data{Now: time.Now(), MarketUnavailable: true}
	out := Render(d)
	if !strings.Contains(out, "## Market") || !strings.Contains(out, "unavailable") {
		t.Errorf("expected market unavailable fallback, got:\n%s", out)
	}
}

// TestBrief_AllFeedsAvailable_AllRendered asserts every feed section appears
// when populated in a single Data.
func TestBrief_AllFeedsAvailable_AllRendered(t *testing.T) {
	d := Data{
		Now:     time.Now(),
		Weather: &Forecast{TempC: 18, HighC: 21, LowC: 9, Description: "clear"},
		News:    &Article{Title: "Headline", Source: "HN"},
		Market:  &Pulse{Quotes: []Quote{{Symbol: "AAPL", PercentChange: 0.5}}},
	}
	out := Render(d)
	for _, want := range []string{"## Weather", "## News", "## Market", "clear", "Headline", "AAPL"} {
		if !strings.Contains(out, want) {
			t.Errorf("all-feeds brief missing %q in:\n%s", want, out)
		}
	}
}

// TestBrief_OrderConsistent asserts Weather always renders before Market —
// the spec-pinned order keeps the operator's eye-flow consistent across days.
func TestBrief_OrderConsistent(t *testing.T) {
	d := Data{
		Now:     time.Now(),
		Weather: &Forecast{TempC: 18, HighC: 21, LowC: 9, Description: "clear"},
		Market:  &Pulse{Quotes: []Quote{{Symbol: "AAPL", PercentChange: 0.5}}},
	}
	out := Render(d)
	wIdx := strings.Index(out, "## Weather")
	mIdx := strings.Index(out, "## Market")
	if wIdx < 0 || mIdx < 0 {
		t.Fatalf("missing section(s): weather=%d market=%d in:\n%s", wIdx, mIdx, out)
	}
	if wIdx >= mIdx {
		t.Errorf("weather must precede market (w=%d m=%d):\n%s", wIdx, mIdx, out)
	}
}

// TestBrief_NilFeeds_OmitSections asserts unset feed integrations stay silent
// — silent absence beats noisy "unavailable" for unconfigured features.
func TestBrief_NilFeeds_OmitSections(t *testing.T) {
	d := Data{Now: time.Now()}
	out := Render(d)
	for _, header := range []string{"## Weather", "## News", "## Market"} {
		if strings.Contains(out, header) {
			t.Errorf("unconfigured feed should omit %q, got:\n%s", header, out)
		}
	}
}

// TestGatherCallsWeatherReporter wires Gather → fakeWeather and confirms
// the forecast propagates.
func TestGatherCallsWeatherReporter(t *testing.T) {
	dir := t.TempDir()
	w := &fakeWeather{f: Forecast{TempC: 12, HighC: 15, LowC: 8, Description: "fog"}}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Weather: w})
	if d.Weather == nil || d.WeatherUnavailable {
		t.Fatalf("weather not propagated: %+v unavail=%v", d.Weather, d.WeatherUnavailable)
	}
	if d.Weather.Description != "fog" {
		t.Errorf("got desc=%q want fog", d.Weather.Description)
	}
}

// TestGatherWeatherErrorMarksUnavailable asserts an error sets the flag.
func TestGatherWeatherErrorMarksUnavailable(t *testing.T) {
	dir := t.TempDir()
	w := &fakeWeather{err: errors.New("boom")}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Weather: w})
	if !d.WeatherUnavailable || d.Weather != nil {
		t.Errorf("expected unavailable flag, got %+v unavail=%v", d.Weather, d.WeatherUnavailable)
	}
}

// TestGatherCallsNewsReporter wires Gather → fakeNews and confirms propagation.
func TestGatherCallsNewsReporter(t *testing.T) {
	dir := t.TempDir()
	n := &fakeNews{a: Article{Title: "x", Source: "HN"}}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{News: n})
	if d.News == nil || d.NewsUnavailable {
		t.Fatalf("news not propagated: %+v unavail=%v", d.News, d.NewsUnavailable)
	}
}

// TestGatherNewsErrorMarksUnavailable asserts an error sets the flag.
func TestGatherNewsErrorMarksUnavailable(t *testing.T) {
	dir := t.TempDir()
	n := &fakeNews{err: errors.New("boom")}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{News: n})
	if !d.NewsUnavailable || d.News != nil {
		t.Errorf("expected unavailable flag, got %+v unavail=%v", d.News, d.NewsUnavailable)
	}
}

// TestGatherCallsMarketReporter wires Gather → fakeMarket and confirms propagation.
func TestGatherCallsMarketReporter(t *testing.T) {
	dir := t.TempDir()
	m := &fakeMarket{p: Pulse{Quotes: []Quote{{Symbol: "AAPL", PercentChange: 1.0}}}}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Market: m})
	if d.Market == nil || d.MarketUnavailable {
		t.Fatalf("market not propagated: %+v unavail=%v", d.Market, d.MarketUnavailable)
	}
}

// TestGatherMarketErrorMarksUnavailable asserts an error sets the flag.
func TestGatherMarketErrorMarksUnavailable(t *testing.T) {
	dir := t.TempDir()
	m := &fakeMarket{err: errors.New("boom")}
	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Market: m})
	if !d.MarketUnavailable || d.Market != nil {
		t.Errorf("expected unavailable flag, got %+v unavail=%v", d.Market, d.MarketUnavailable)
	}
}
