package brief

// Watchlist is the per-symbol price-and-change strip rendered in the morning
// brief. The polymath audit (MAY-203) called out that an operator tracking
// AAPL/MSFT/etc. wants the same one-glance shape that the Market pulse uses
// for the broad index — but driven by a personal symbol list at
// ~/.leah-state/watchlist.json, not a fixed feed.
//
// Mirrors the feeds composer (W33) and worktools (W53) patterns:
//   - Brief-local interface; the wire site adapts feeds.Market.FetchAll onto it.
//   - Silent absence when the watchlist file is missing or its symbol list is
//     empty — operator has not opted in. The runtime-failure "(unavailable)"
//     fallback only fires when the quoter returned an error.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// watchlistCap bounds the rendered symbol count to a one-glance strip; an
// operator tracking 30 tickers should rely on the HUD, not the brief.
const watchlistCap = 10

// WatchlistQuote is the brief-local per-symbol snapshot. Price + percent
// change are the two numbers the spec line ("AAPL +1.2% @ 203.40") needs;
// other feeds.Quote fields are dropped at the wire boundary.
type WatchlistQuote struct {
	Symbol        string
	PercentChange float64
	Price         float64
}

// WatchlistQuoter is the brief-facing contract for the symbol-batch quote
// fetcher. Shape mirrors feeds.Market.FetchAll so the wire site adapts the
// real adapter onto it without a second prompt.
type WatchlistQuoter interface {
	FetchAll(ctx context.Context, symbols []string) ([]WatchlistQuote, error)
}

// watchlistFile is the JSON shape the operator edits — only "symbols" is read
// today; extra keys are tolerated so a future schema bump does not break
// today's brief.
type watchlistFile struct {
	Symbols []string `json:"symbols"`
}

// LoadWatchlistSymbols reads ~/.leah-state/watchlist.json and returns the
// symbol list. Missing file, malformed JSON, or absent "symbols" key all
// yield an empty slice — the brief degrades silently rather than refusing to
// render on a typo.
func LoadWatchlistSymbols(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f watchlistFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil
	}
	return f.Symbols
}

// renderWatchlist appends the Watchlist section. Silent absence when nil + no
// runtime-failure flag, matching the mail/calendar/feeds gating contract.
func renderWatchlist(b *strings.Builder, d Data) {
	if len(d.Watchlist) == 0 && !d.WatchlistUnavailable {
		return
	}
	fmt.Fprintln(b, "## Watchlist")
	if d.WatchlistUnavailable {
		fmt.Fprintln(b, "  (unavailable)")
		fmt.Fprintln(b)
		return
	}
	max := watchlistCap
	if len(d.Watchlist) < max {
		max = len(d.Watchlist)
	}
	for _, q := range d.Watchlist[:max] {
		fmt.Fprintf(b, "  %s %+.1f%% @ %.2f\n", q.Symbol, q.PercentChange, q.Price)
	}
	if len(d.Watchlist) > max {
		fmt.Fprintf(b, "  …and %d more\n", len(d.Watchlist)-max)
	}
	fmt.Fprintln(b)
}
