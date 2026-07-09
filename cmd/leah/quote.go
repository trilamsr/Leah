package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/feeds"
)

// runQuote prints a MarketPulse for the given symbols. The AV key sits at
// $LEAH_STATE_DIR/secrets/alphavantage-key.json (mode 0600); provisioned
// by `leah connect alphavantage`.
func runQuote(parent context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		_, _ = fmt.Fprintln(w, "usage: leah quote <symbol> [<symbol>...]")
		_, _ = fmt.Fprintln(w, "")
		_, _ = fmt.Fprintln(w, "Fetch + render market quotes for the given symbols (Alpha Vantage).")
		_, _ = fmt.Fprintln(w, "API key read from $LEAH_STATE_DIR/secrets/alphavantage-key.json (mode 0600).")
		return 0
	}
	if len(args) < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: leah quote <symbol> [<symbol>...]")
		return 2
	}

	key, err := loadAlphaVantageKey(stateDir())
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah quote: %v\n", err)
		_, _ = fmt.Fprintln(os.Stderr, "  hint: place api_key in $LEAH_STATE_DIR/secrets/alphavantage-key.json (0600)")
		return 1
	}

	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	m, err := feeds.NewMarket(feeds.MarketConfig{
		Attestor:   newFeedsAttestor(),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		APIKey:     key,
		// Test seam: production runs against AV's prod endpoint (BaseURL "");
		// httptest fakes pass LEAH_ALPHAVANTAGE_BASE_URL so we don't hit the
		// network in CI.
		BaseURL: os.Getenv("LEAH_ALPHAVANTAGE_BASE_URL"),
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah quote: %v\n", err)
		return 1
	}

	symbols := normalizeSymbols(args)
	quotes, err := m.FetchAll(ctx, symbols)
	if err != nil {
		if errors.Is(err, feeds.ErrAttestationDenied) {
			_, _ = fmt.Fprintln(os.Stderr, "leah quote: attestation denied")
			return 1
		}
		_, _ = fmt.Fprintf(os.Stderr, "leah quote: fetch: %v\n", err)
		return 1
	}
	if len(quotes) == 0 {
		// FetchAll swallows per-symbol errors; an empty result means the
		// operator typed only unknown tickers. Surface as exit=1 so cron
		// scripting catches the regression.
		_, _ = fmt.Fprintf(os.Stderr, "leah quote: no quotes returned (check symbols: %v)\n", symbols)
		return 1
	}
	pulse := feeds.SummarizeMarket(quotes)
	for _, line := range pulse.Lines {
		_, _ = fmt.Fprintln(w, line)
	}
	return 0
}

// loadAlphaVantageKey reads the API key secret. Mode-check the file (must be
// 0600) before reading — the operator's secret hygiene is part of the threat
// model in spec §7.
func loadAlphaVantageKey(dir string) (string, error) {
	path := filepath.Join(dir, "secrets", "alphavantage-key.json")
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read api key: %w", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		return "", fmt.Errorf("api key file %s has mode %o, want 0600", path, mode)
	}
	raw, err := os.ReadFile(path) // #nosec G304 — operator-owned state dir
	if err != nil {
		return "", fmt.Errorf("read api key: %w", err)
	}
	var doc struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("decode api key: %w", err)
	}
	if doc.APIKey == "" {
		return "", errors.New("api key json missing api_key field")
	}
	return doc.APIKey, nil
}

// normalizeSymbols upper-cases + de-dupes the operator's argv tickers so
// "aapl AAPL" hits AV once. Order-preserving — first occurrence wins.
func normalizeSymbols(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// feedsAttestor is the operator-attestation gate news.go + quote.go share.
// Same shape as connectAttestor — auto-attest under env var so tests +
// daemon-driven brief don't block on stdin; otherwise prompts the operator.
type feedsAttestor struct{}

func newFeedsAttestor() contracts.Attestor { return feedsAttestor{} }

func (feedsAttestor) Attest(_ context.Context, scope string) error {
	if v := os.Getenv("LEAH_FEEDS_AUTO_ATTEST"); v == "1" {
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "attest %s? [y/N] ", scope)
	var resp string
	_, _ = fmt.Fscanln(os.Stdin, &resp)
	// Empty / EOF / closed stdin defaults to NO — see connectAttestor.
	if resp == "" {
		return errors.New("denied")
	}
	switch resp {
	case "y", "Y", "yes", "YES":
		return nil
	}
	return errors.New("denied")
}
