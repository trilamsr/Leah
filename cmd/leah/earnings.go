package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/trilam/leah/internal/feeds"
	"github.com/trilam/leah/internal/watchlist"
)

// runEarnings surfaces upcoming earnings dates.
//
//	leah earnings <symbol>           — next report for symbol
//	leah earnings --next <window>    — watchlist symbols reporting within window (e.g. 7d)
//
// No working AV key OR empty watchlist OR no upcoming hits → silent exit 0
// (matches `leah news` shape: a feature gated on an absent integration should
// degrade quietly, not nag the operator on every cron tick).
func runEarnings(parent context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(w, "usage: leah earnings <symbol>")
		_, _ = fmt.Fprintln(w, "       leah earnings --next <window>   (e.g. 7d, 30d)")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "Upcoming earnings dates from Alpha Vantage EARNINGS_CALENDAR.")
		_, _ = fmt.Fprintln(w, "API key read from $LEAH_STATE_DIR/secrets/alphavantage-key.json (mode 0600).")
		return 0
	}
	if len(args) < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah earnings <symbol> | leah earnings --next <window>")
		return 2
	}

	key, err := loadAlphaVantageKey(stateDir())
	if err != nil {
		// Silent no-op: no Alpha Vantage key wired = feature dormant. Matches the
		// "absent integration is not an error" rule the prompt called out.
		return 0
	}

	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	e, err := feeds.NewEarnings(feeds.EarningsConfig{
		Attestor:   newFeedsAttestor(),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		APIKey:     key,
		BaseURL:    os.Getenv("LEAH_ALPHAVANTAGE_BASE_URL"),
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah earnings: %v\n", err)
		return 1
	}

	if args[0] == "--next" {
		if len(args) < 2 {
			_, _ = fmt.Fprintln(os.Stderr, "usage: leah earnings --next <window> (e.g. 7d)")
			return 2
		}
		window, err := parseEarningsWindow(args[1])
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "leah earnings: %v\n", err)
			return 2
		}
		return runEarningsNext(ctx, e, window, w)
	}

	return runEarningsSymbol(ctx, e, strings.ToUpper(strings.TrimSpace(args[0])), w)
}

func runEarningsSymbol(ctx context.Context, e *feeds.Earnings, symbol string, w io.Writer) int {
	ev, err := e.FetchSymbol(ctx, symbol)
	if err != nil {
		if errors.Is(err, feeds.ErrNoEarnings) {
			return 0
		}
		if errors.Is(err, feeds.ErrAttestationDenied) {
			_, _ = fmt.Fprintln(os.Stderr, "leah earnings: attestation denied")
			return 1
		}
		_, _ = fmt.Fprintf(os.Stderr, "leah earnings: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(w, formatEarningsLine(ev))
	return 0
}

func runEarningsNext(ctx context.Context, e *feeds.Earnings, window time.Duration, w io.Writer) int {
	store := &watchlist.Store{Path: filepath.Join(stateDir(), "watchlist.json")}
	symbols, err := store.List()
	if err != nil || len(symbols) == 0 {
		// No watchlist file OR empty list → silent. The operator hasn't
		// adopted `leah watch` yet; spamming an error is wrong UX.
		return 0
	}
	events, err := e.FetchUpcoming(ctx, symbols, window)
	if err != nil {
		if errors.Is(err, feeds.ErrAttestationDenied) {
			_, _ = fmt.Fprintln(os.Stderr, "leah earnings: attestation denied")
			return 1
		}
		_, _ = fmt.Fprintf(os.Stderr, "leah earnings: %v\n", err)
		return 1
	}
	for _, ev := range events {
		_, _ = fmt.Fprintln(w, formatEarningsLine(ev))
	}
	return 0
}

// formatEarningsLine: "AAPL 2026-07-31 post-market est=1.42 USD"
// — date first after symbol so `sort` over a multi-symbol batch is
// chronological by default.
func formatEarningsLine(ev feeds.EarningsEvent) string {
	var b strings.Builder
	b.WriteString(ev.Symbol)
	b.WriteString(" ")
	b.WriteString(ev.ReportDate.Format("2006-01-02"))
	if ev.TimeOfDay != "" {
		b.WriteString(" ")
		b.WriteString(ev.TimeOfDay)
	}
	if ev.Estimate != "" {
		b.WriteString(" est=")
		b.WriteString(ev.Estimate)
		if ev.Currency != "" {
			b.WriteString(" ")
			b.WriteString(ev.Currency)
		}
	}
	return b.String()
}

// parseEarningsWindow accepts "<N>d" — anything else is rejected. Hours/weeks
// would inflate the surface (and AV's horizon caps at 3 months anyway), so
// days is the only honest unit at this layer.
func parseEarningsWindow(s string) (time.Duration, error) {
	if s == "" || !strings.HasSuffix(s, "d") {
		return 0, fmt.Errorf("invalid window %q (want Nd, e.g. 7d)", s)
	}
	n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid window %q (want positive Nd)", s)
	}
	return time.Duration(n) * 24 * time.Hour, nil
}
