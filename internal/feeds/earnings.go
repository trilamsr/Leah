package feeds

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ScopeEarningsFetch is the operator-attestation scope for a per-symbol
// or watchlist-wide earnings-calendar pull.
const ScopeEarningsFetch = "feeds:earnings:fetch"

// ErrNoEarnings fires when the provider returns a header-only CSV — the
// symbol is unknown OR has no upcoming report in the requested horizon.
// CLI treats this as silent-success so the operator doesn't see a stack
// trace for "AAPL has no upcoming earnings".
var ErrNoEarnings = errors.New("feeds.earnings: no upcoming earnings")

// EarningsEvent is one upcoming-report row. Estimate left as a string —
// the CSV ships empty for unrated symbols and a float-zero would be a
// silent misread.
type EarningsEvent struct {
	Symbol     string
	Name       string
	ReportDate time.Time
	Estimate   string
	Currency   string
	TimeOfDay  string // "pre-market" | "post-market" | ""
}

// EarningsConfig wires the Alpha Vantage EARNINGS_CALENDAR adapter. Same
// shape as MarketConfig — caller reads APIKey from the 0600 secrets file
// and passes it in. BaseURL blank → AV prod; Now blank → wall clock.
type EarningsConfig struct {
	Attestor   Attestor
	HTTPClient *http.Client
	APIKey     string
	BaseURL    string
	Now        func() time.Time
}

// Earnings is the AV EARNINGS_CALENDAR adapter. Stateless — daemon owns
// lifecycle.
type Earnings struct {
	att     Attestor
	client  *http.Client
	apiKey  string
	baseURL string
	now     func() time.Time
}

func NewEarnings(cfg EarningsConfig) (*Earnings, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("feeds.NewEarnings: Attestor required (operator-attestation gate)")
	}
	if cfg.HTTPClient == nil {
		return nil, errors.New("feeds.NewEarnings: HTTPClient required")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("feeds.NewEarnings: APIKey required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultAlphaVantageBaseURL
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Earnings{
		att:     cfg.Attestor,
		client:  cfg.HTTPClient,
		apiKey:  cfg.APIKey,
		baseURL: base,
		now:     now,
	}, nil
}

func (e *Earnings) Name() string { return "earnings" }

// FetchSymbol returns the NEXT scheduled report for symbol (AV returns
// the soonest first when horizon=3month). Attestation runs before HTTP.
func (e *Earnings) FetchSymbol(ctx context.Context, symbol string) (EarningsEvent, error) {
	if err := e.att.Attest(ctx, ScopeEarningsFetch); err != nil {
		return EarningsEvent{}, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	rows, err := e.fetchCSV(ctx, symbol)
	if err != nil {
		return EarningsEvent{}, err
	}
	// Symbol-filter even when AV returns a multi-row payload: the demo
	// endpoint, sandboxed mirrors, and (occasionally) prod itself ignore
	// the per-symbol query param and ship the full calendar.
	want := strings.ToUpper(symbol)
	filtered := rows[:0]
	for _, r := range rows {
		if strings.ToUpper(r.Symbol) == want {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return EarningsEvent{}, fmt.Errorf("%w: %s", ErrNoEarnings, symbol)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ReportDate.Before(filtered[j].ReportDate) })
	return filtered[0], nil
}

// FetchUpcoming pulls the full-calendar CSV once and filters in-memory to
// (a) watchlist membership and (b) reportDate within [now, now+window].
// One HTTP call regardless of watchlist length — AV per-symbol queries
// would burn the daily-25 budget on a typical 10-symbol watchlist.
func (e *Earnings) FetchUpcoming(ctx context.Context, watchlist []string, window time.Duration) ([]EarningsEvent, error) {
	if len(watchlist) == 0 {
		return nil, nil
	}
	if err := e.att.Attest(ctx, ScopeEarningsFetch); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	rows, err := e.fetchCSV(ctx, "")
	if err != nil {
		return nil, err
	}
	want := make(map[string]struct{}, len(watchlist))
	for _, s := range watchlist {
		want[strings.ToUpper(strings.TrimSpace(s))] = struct{}{}
	}
	now := e.now()
	cutoff := now.Add(window)
	out := make([]EarningsEvent, 0, len(watchlist))
	for _, r := range rows {
		if _, ok := want[r.Symbol]; !ok {
			continue
		}
		if r.ReportDate.Before(now) || r.ReportDate.After(cutoff) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReportDate.Before(out[j].ReportDate) })
	return out, nil
}

func (e *Earnings) fetchCSV(ctx context.Context, symbol string) ([]EarningsEvent, error) {
	q := url.Values{}
	q.Set("function", "EARNINGS_CALENDAR")
	q.Set("horizon", "3month")
	q.Set("apikey", e.apiKey)
	if symbol != "" {
		q.Set("symbol", symbol)
	}
	reqURL := e.baseURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("feeds.earnings: build request: %w", err)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("feeds.earnings: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feeds.earnings: provider %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("feeds.earnings: read body: %w", err)
	}
	return parseAlphaVantageEarningsCSV(body)
}

// parseAlphaVantageEarningsCSV decodes the AV CSV shape
// symbol,name,reportDate,fiscalDateEnding,estimate,currency,timeOfTheDay.
// Header line is skipped; rows with an unparseable reportDate are dropped
// rather than aborting the whole pull — AV occasionally ships TBA rows.
func parseAlphaVantageEarningsCSV(body []byte) ([]EarningsEvent, error) {
	r := csv.NewReader(strings.NewReader(string(body)))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("feeds.earnings: csv: %w", err)
	}
	out := make([]EarningsEvent, 0, len(records))
	for i, rec := range records {
		if i == 0 || len(rec) < 7 {
			continue
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(rec[2]))
		if err != nil {
			continue
		}
		out = append(out, EarningsEvent{
			Symbol:     strings.TrimSpace(rec[0]),
			Name:       strings.TrimSpace(rec[1]),
			ReportDate: d,
			Estimate:   strings.TrimSpace(rec[4]),
			Currency:   strings.TrimSpace(rec[5]),
			TimeOfDay:  strings.TrimSpace(rec[6]),
		})
	}
	return out, nil
}
