package brief

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeWatchlistQuoter returns canned quotes/err for the watchlist tests; the
// brief contract is "give me a quote for each symbol", matching the
// MarketQuoter shape so the live wire (internal/feeds/market.go FetchAll)
// drops straight in.
type fakeWatchlistQuoter struct {
	quotes []WatchlistQuote
	err    error
	called []string
}

func (f *fakeWatchlistQuoter) FetchAll(ctx context.Context, symbols []string) ([]WatchlistQuote, error) {
	f.called = append([]string(nil), symbols...)
	return f.quotes, f.err
}

// TestBrief_WatchlistRendered_HappyPath asserts per-symbol line shape
// "AAPL +1.2% @ 203.40" per the spec.
func TestBrief_WatchlistRendered_HappyPath(t *testing.T) {
	d := Data{
		Now: time.Now(),
		Watchlist: []WatchlistQuote{
			{Symbol: "AAPL", PercentChange: 1.2, Price: 203.40},
			{Symbol: "MSFT", PercentChange: -0.4, Price: 412.10},
		},
	}
	out := Render(d)
	if !strings.Contains(out, "## Watchlist") {
		t.Fatalf("missing Watchlist header:\n%s", out)
	}
	for _, want := range []string{"AAPL", "+1.2%", "203.40", "MSFT", "-0.4%", "412.10"} {
		if !strings.Contains(out, want) {
			t.Errorf("watchlist section missing %q in:\n%s", want, out)
		}
	}
}

// TestBrief_WatchlistUnavailable_RendersFallback asserts the runtime-failure flag.
func TestBrief_WatchlistUnavailable_RendersFallback(t *testing.T) {
	d := Data{Now: time.Now(), WatchlistUnavailable: true}
	out := Render(d)
	if !strings.Contains(out, "## Watchlist") || !strings.Contains(out, "unavailable") {
		t.Errorf("expected watchlist unavailable fallback, got:\n%s", out)
	}
}

// TestBrief_WatchlistEmpty_OmitSection asserts silent absence when the
// watchlist file is unconfigured (nil quoter → no section).
func TestBrief_WatchlistEmpty_OmitSection(t *testing.T) {
	d := Data{Now: time.Now()}
	out := Render(d)
	if strings.Contains(out, "## Watchlist") {
		t.Errorf("unconfigured watchlist should omit section, got:\n%s", out)
	}
}

// TestGatherCallsWatchlistQuoter wires Gather → fakeWatchlistQuoter with a
// concrete watchlist.json under sd, confirming both the symbols pass through
// to the quoter and the resulting quotes populate Data.
func TestGatherCallsWatchlistQuoter(t *testing.T) {
	dir := t.TempDir()
	writeWatchlistJSON(t, dir, `{"symbols": ["AAPL", "MSFT"]}`)
	q := &fakeWatchlistQuoter{quotes: []WatchlistQuote{{Symbol: "AAPL", PercentChange: 1.0, Price: 200}}}

	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Watchlist: q})

	if d.WatchlistUnavailable {
		t.Fatalf("watchlist marked unavailable on happy path")
	}
	if len(q.called) != 2 || q.called[0] != "AAPL" || q.called[1] != "MSFT" {
		t.Errorf("watchlist.json symbols not forwarded to quoter, got %v", q.called)
	}
	if len(d.Watchlist) != 1 || d.Watchlist[0].Symbol != "AAPL" {
		t.Errorf("quoter result not propagated: %+v", d.Watchlist)
	}
}

// TestGatherWatchlistErrorMarksUnavailable asserts an upstream error flips
// the flag without poisoning sibling sections.
func TestGatherWatchlistErrorMarksUnavailable(t *testing.T) {
	dir := t.TempDir()
	writeWatchlistJSON(t, dir, `{"symbols": ["AAPL"]}`)
	q := &fakeWatchlistQuoter{err: errors.New("boom")}

	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Watchlist: q})

	if !d.WatchlistUnavailable {
		t.Errorf("expected WatchlistUnavailable=true, got false")
	}
	if len(d.Watchlist) != 0 {
		t.Errorf("expected no watchlist quotes on error, got %v", d.Watchlist)
	}
}

// TestGatherWatchlistMissingFile_SilentOmit asserts Gather skips the quoter
// (no consent prompt, no AV call) when watchlist.json is absent — matches the
// worktool/gmail "silent absence" gating.
func TestGatherWatchlistMissingFile_SilentOmit(t *testing.T) {
	dir := t.TempDir()
	q := &fakeWatchlistQuoter{quotes: []WatchlistQuote{{Symbol: "AAPL"}}}

	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Watchlist: q})

	if q.called != nil {
		t.Errorf("quoter must not fire when watchlist.json absent, got call for %v", q.called)
	}
	if d.WatchlistUnavailable {
		t.Errorf("missing file is silent-absence, not unavailable")
	}
	if len(d.Watchlist) != 0 {
		t.Errorf("expected empty watchlist on missing file, got %v", d.Watchlist)
	}
}

// TestGatherWatchlistEmptySymbols_SilentOmit asserts an empty symbol list
// degrades silently — the file is a stub, not a runtime failure.
func TestGatherWatchlistEmptySymbols_SilentOmit(t *testing.T) {
	dir := t.TempDir()
	writeWatchlistJSON(t, dir, `{"symbols": []}`)
	q := &fakeWatchlistQuoter{quotes: []WatchlistQuote{{Symbol: "AAPL"}}}

	d := Gather(context.Background(), time.Now(), dir, nil, GatherOpts{Watchlist: q})

	if q.called != nil {
		t.Errorf("quoter must not fire on empty symbol list, got %v", q.called)
	}
	if d.WatchlistUnavailable || len(d.Watchlist) != 0 {
		t.Errorf("empty symbols should be silent: unavail=%v quotes=%v", d.WatchlistUnavailable, d.Watchlist)
	}
}

// TestLoadWatchlistSymbols_MalformedJSON returns no symbols + no error so the
// brief degrades silently rather than refusing to render.
func TestLoadWatchlistSymbols_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	writeWatchlistJSON(t, dir, `not-json`)
	got := LoadWatchlistSymbols(filepath.Join(dir, "watchlist.json"))
	if len(got) != 0 {
		t.Errorf("malformed JSON should yield no symbols, got %v", got)
	}
}

func writeWatchlistJSON(t *testing.T, sd, body string) {
	t.Helper()
	path := filepath.Join(sd, "watchlist.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write watchlist.json: %v", err)
	}
}
