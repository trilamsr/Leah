// Package brief — feeds composer.
//
// W33 wires the morning brief to the info-feeds layer (weather, news,
// market) defined in docs/engineer/specs/2026-06-10-info-feeds.md.
//
// Design lessons inherited from PR #65 (gmail + gcal):
//   - Brief-local structural-typed interfaces — wiring code maps the concrete
//     adapter (feeds.Weather, feeds.News, feeds.Market) to the brief-facing
//     contract. Brief never imports the feeds package, so a feeds-side
//     refactor cannot ripple into rendering.
//   - Soft-fail per feed: a single source going down marks only that
//     section "unavailable"; the brief still ships.
//   - Silent absence for unconfigured features. Nil reporter → section is
//     omitted entirely (not "unavailable"). "Unavailable" is the runtime-
//     failure signal, not the not-configured one.
package brief

import (
	"context"
	"fmt"
	"strings"
)

// Forecast is the brief-local weather payload. Mirrors feeds.Forecast field-
// for-field; wiring code copies the values across the package boundary.
type Forecast struct {
	TempC       float64
	Description string
	HighC       float64
	LowC        float64
	PrecipPct   int
}

// Article is the brief-local headline payload. One article surfaces in the
// brief — the synth layer (W32) picks the top item from the multi-source
// digest before handing it across.
type Article struct {
	Title   string
	Source  string
	URL     string
	Summary string
}

// Quote is one tracked symbol's snapshot — % change is the only number the
// brief currently renders; absolute price is left for the HUD ticker.
type Quote struct {
	Symbol        string
	PercentChange float64
}

// Pulse is the brief-local market payload: a slice of per-symbol quotes.
type Pulse struct {
	Quotes []Quote
}

// WeatherReporter is the brief-facing weather contract. Today returns the
// operator's location forecast; structural-typed so a fake test double or
// the real feeds.Weather both satisfy.
type WeatherReporter interface {
	Today(ctx context.Context) (Forecast, error)
}

// NewsReporter is the brief-facing news contract. Top returns the single
// highest-ranked headline from the synth layer.
type NewsReporter interface {
	Top(ctx context.Context) (Article, error)
}

// MarketReporter is the brief-facing market contract. Snapshot returns the
// pulse across the operator's tracked symbols.
type MarketReporter interface {
	Snapshot(ctx context.Context) (Pulse, error)
}

// gatherFeeds populates the feeds-related Data fields. Pulled out so
// Gather stays readable; each feed soft-fails independently.
func gatherFeeds(ctx context.Context, d *Data, o GatherOpts) {
	if o.Weather != nil {
		if f, err := o.Weather.Today(ctx); err != nil {
			d.WeatherUnavailable = true
		} else {
			fc := f
			d.Weather = &fc
		}
	}
	if o.News != nil {
		if a, err := o.News.Top(ctx); err != nil {
			d.NewsUnavailable = true
		} else {
			ar := a
			d.News = &ar
		}
	}
	if o.Market != nil {
		if p, err := o.Market.Snapshot(ctx); err != nil {
			d.MarketUnavailable = true
		} else {
			pl := p
			d.Market = &pl
		}
	}
}

// renderWeather appends the Weather section. Silent absence when nil + not
// flagged unavailable.
func renderWeather(b *strings.Builder, d Data) {
	if d.Weather == nil && !d.WeatherUnavailable {
		return
	}
	fmt.Fprintln(b, "## Weather")
	if d.WeatherUnavailable {
		fmt.Fprintln(b, "  (unavailable)")
	} else {
		f := d.Weather
		line := fmt.Sprintf("  %.0f°C (high %.0f° / low %.0f°)", f.TempC, f.HighC, f.LowC)
		if f.Description != "" {
			line += " — " + f.Description
		}
		if f.PrecipPct > 0 {
			line += fmt.Sprintf(", precip %d%%", f.PrecipPct)
		}
		fmt.Fprintln(b, line)
	}
	fmt.Fprintln(b)
}

func renderNews(b *strings.Builder, d Data) {
	if d.News == nil && !d.NewsUnavailable {
		return
	}
	fmt.Fprintln(b, "## News")
	if d.NewsUnavailable {
		fmt.Fprintln(b, "  (unavailable)")
	} else {
		a := d.News
		line := "  " + a.Title
		if a.Source != "" {
			line += " (" + a.Source + ")"
		}
		fmt.Fprintln(b, line)
		if a.Summary != "" {
			fmt.Fprintf(b, "  %s\n", a.Summary)
		}
	}
	fmt.Fprintln(b)
}

func renderMarket(b *strings.Builder, d Data) {
	if d.Market == nil && !d.MarketUnavailable {
		return
	}
	fmt.Fprintln(b, "## Market")
	if d.MarketUnavailable {
		fmt.Fprintln(b, "  (unavailable)")
	} else {
		parts := make([]string, 0, len(d.Market.Quotes))
		for _, q := range d.Market.Quotes {
			parts = append(parts, fmt.Sprintf("%s %+.1f%%", q.Symbol, q.PercentChange))
		}
		fmt.Fprintf(b, "  %s\n", strings.Join(parts, ", "))
	}
	fmt.Fprintln(b)
}
