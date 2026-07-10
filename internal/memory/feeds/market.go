package feeds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/trilam/leah/internal/platform/contracts"
)

// ScopeMarketFetch is the operator-attestation scope for a market-quote pull.
const ScopeMarketFetch = "feeds:market:fetch"

// defaultAlphaVantageBaseURL is AV's GLOBAL_QUOTE endpoint. Overridable via
// MarketConfig.BaseURL so httptest fakes stay hermetic.
const defaultAlphaVantageBaseURL = "https://www.alphavantage.co/query"

// ErrUnknownSymbol fires when AV returns an empty Global Quote payload. The
// operator typed a bad ticker; surfacing this lets `leah quote` give a clean
// "unknown symbol" message instead of a generic decode error.
var ErrUnknownSymbol = errors.New("feeds.market: unknown symbol")

// Quote is the per-symbol payload the synthesizer + ticker render. Fields
// chosen to feed the brief one-liner ("AAPL +1.2% @ 203.40").
type Quote struct {
	Symbol    string
	Price     float64
	PrevClose float64
	Change    float64
	ChangePct float64 // already in percent, not fraction (1.7, not 0.017)
}

// MarketConfig wires the adapter. All four fields required when BaseURL
// blank (defaults to AV prod); APIKey is read by the caller from
// $HOME/.leah-state/secrets/alphavantage-key.json (mode 0600) and passed in.
type MarketConfig struct {
	Attestor   contracts.Attestor
	HTTPClient *http.Client
	APIKey     string
	BaseURL    string
}

// Market is the Alpha Vantage adapter. No background goroutines — lifecycle
// owned by the caller so daemon shutdown is clean.
type Market struct {
	att     contracts.Attestor
	client  *http.Client
	apiKey  string
	baseURL string
}

// NewMarket fails fast on a half-wired config; a missing Attestor would
// silently bypass operator-consent.
func NewMarket(config MarketConfig) (*Market, error) {
	if config.Attestor == nil {
		return nil, errors.New("feeds.NewMarket: Attestor required (operator-attestation gate)")
	}
	if config.HTTPClient == nil {
		return nil, errors.New("feeds.NewMarket: HTTPClient required")
	}
	if config.APIKey == "" {
		return nil, errors.New("feeds.NewMarket: APIKey required")
	}
	base := config.BaseURL
	if base == "" {
		base = defaultAlphaVantageBaseURL
	}
	return &Market{
		att:     config.Attestor,
		client:  config.HTTPClient,
		apiKey:  config.APIKey,
		baseURL: base,
	}, nil
}

// Name satisfies a future Feed-style registry; the Quote-returning Fetch is
// distinct from generic Feed.Fetch since the synth layer wants the typed
// payload.
func (m *Market) Name() string { return "market" }

// Fetch gates on attestation BEFORE the HTTP call. Single-symbol shape keeps
// AV's per-key 25/day budget visible at the call site — batch wrapping
// happens in FetchAll, which is honest about being N calls.
func (m *Market) Fetch(ctx context.Context, symbol string) (Quote, error) {
	if err := m.att.Attest(ctx, ScopeMarketFetch); err != nil {
		return Quote{}, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	return m.fetchOne(ctx, symbol)
}

// FetchAll is a partial-failure-tolerant batch: returns every symbol that
// parsed cleanly and drops the rest. Attestation runs once for the whole
// batch — one consent prompt, not N.
func (m *Market) FetchAll(ctx context.Context, symbols []string) ([]Quote, error) {
	if err := m.att.Attest(ctx, ScopeMarketFetch); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	out := make([]Quote, 0, len(symbols))
	for _, s := range symbols {
		q, err := m.fetchOne(ctx, s)
		if err != nil {
			continue
		}
		out = append(out, q)
	}
	return out, nil
}

func (m *Market) fetchOne(ctx context.Context, symbol string) (Quote, error) {
	q := url.Values{}
	q.Set("function", "GLOBAL_QUOTE")
	q.Set("symbol", symbol)
	q.Set("apikey", m.apiKey)
	reqURL := m.baseURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Quote{}, fmt.Errorf("feeds.market: build request: %w", err)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return Quote{}, fmt.Errorf("feeds.market: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Quote{}, fmt.Errorf("feeds.market: provider %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Quote{}, fmt.Errorf("feeds.market: read body: %w", err)
	}

	var raw avResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return Quote{}, fmt.Errorf("feeds.market: decode: %w", err)
	}
	if raw.Quote.Symbol == "" {
		return Quote{}, fmt.Errorf("%w: %s", ErrUnknownSymbol, symbol)
	}
	return Quote{
		Symbol:    raw.Quote.Symbol,
		Price:     parseFloat(raw.Quote.Price),
		PrevClose: parseFloat(raw.Quote.PrevClose),
		Change:    parseFloat(raw.Quote.Change),
		ChangePct: parsePct(raw.Quote.ChangePct),
	}, nil
}

// avResponse trims Alpha Vantage's GLOBAL_QUOTE shape to the five numeric
// fields the synthesizer reads. AV uses number-prefixed keys
// ("05. price") so the tags carry the literal string.
type avResponse struct {
	Quote avQuote `json:"Global Quote"`
}

type avQuote struct {
	Symbol    string `json:"01. symbol"`
	Price     string `json:"05. price"`
	PrevClose string `json:"08. previous close"`
	Change    string `json:"09. change"`
	ChangePct string `json:"10. change percent"`
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func parsePct(s string) float64 {
	return parseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"))
}
