package feeds_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/memory/feeds"
)

// avHappyBody mirrors Alpha Vantage's GLOBAL_QUOTE response shape. Field
// names match AV's documented JSON, including the ungainly "01. symbol"
// number-prefixed keys (AV's choice, not ours).
const avHappyBody = `{
  "Global Quote": {
    "01. symbol": "AAPL",
    "02. open": "200.10",
    "03. high": "205.50",
    "04. low": "199.20",
    "05. price": "203.40",
    "08. previous close": "200.00",
    "09. change": "3.40",
    "10. change percent": "1.7000%"
  }
}`

// TestMarket_FetchQuote_Happy parses an AV-shaped response into Quote.
func TestMarket_FetchQuote_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "symbol=AAPL") {
			t.Errorf("missing symbol=AAPL in query: %q", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "apikey=test-key") {
			t.Errorf("missing apikey in query: %q", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "function=GLOBAL_QUOTE") {
			t.Errorf("missing function=GLOBAL_QUOTE in query: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(avHappyBody))
	}))
	defer srv.Close()

	att := &fakeAttestor{}
	m, err := feeds.NewMarket(feeds.MarketConfig{
		Attestor:   att,
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		BaseURL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("NewMarket: %v", err)
	}

	q, err := m.Fetch(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if q.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want AAPL", q.Symbol)
	}
	if q.Price != 203.40 {
		t.Errorf("Price = %v, want 203.40", q.Price)
	}
	if q.PrevClose != 200.00 {
		t.Errorf("PrevClose = %v, want 200.00", q.PrevClose)
	}
	if q.Change != 3.40 {
		t.Errorf("Change = %v, want 3.40", q.Change)
	}
	// 1.7000% literal -> 1.7
	if q.ChangePct < 1.69 || q.ChangePct > 1.71 {
		t.Errorf("ChangePct = %v, want ~1.70", q.ChangePct)
	}
	if len(att.calls) != 1 || att.calls[0] != feeds.ScopeMarketFetch {
		t.Errorf("Attest calls = %v, want [%s]", att.calls, feeds.ScopeMarketFetch)
	}
}

// TestMarket_AttestationDenied_NoHTTPCall — denied gate aborts before network.
func TestMarket_AttestationDenied_NoHTTPCall(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hit = true
	}))
	defer srv.Close()

	m, err := feeds.NewMarket(feeds.MarketConfig{
		Attestor:   &fakeAttestor{err: errors.New("denied")},
		HTTPClient: srv.Client(),
		APIKey:     "test-key",
		BaseURL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("NewMarket: %v", err)
	}
	if _, err := m.Fetch(context.Background(), "AAPL"); !errors.Is(err, feeds.ErrAttestationDenied) {
		t.Fatalf("Fetch err = %v, want ErrAttestationDenied wrap", err)
	}
	if hit {
		t.Fatalf("HTTP server hit after denied attestation")
	}
}

// TestMarket_APIKeyMissing_FailsFast — no key means no construction.
func TestMarket_APIKeyMissing_FailsFast(t *testing.T) {
	_, err := feeds.NewMarket(feeds.MarketConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: http.DefaultClient,
		APIKey:     "",
		BaseURL:    "http://example.com",
	})
	if err == nil {
		t.Fatalf("NewMarket accepted empty API key")
	}
	if !strings.Contains(err.Error(), "APIKey") {
		t.Errorf("err = %v, want mention of APIKey", err)
	}
}

// TestMarket_UnknownSymbol_ReturnsErr — AV returns empty Global Quote map
// when symbol unknown; adapter surfaces that as ErrUnknownSymbol.
func TestMarket_UnknownSymbol_ReturnsErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Global Quote": {}}`))
	}))
	defer srv.Close()
	m, err := feeds.NewMarket(feeds.MarketConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: srv.Client(),
		APIKey:     "k",
		BaseURL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("NewMarket: %v", err)
	}
	if _, err := m.Fetch(context.Background(), "ZZZZ"); !errors.Is(err, feeds.ErrUnknownSymbol) {
		t.Fatalf("Fetch err = %v, want ErrUnknownSymbol", err)
	}
}

// TestMarket_FetchAll_PartialFailure — bad symbol doesn't blank good symbols.
func TestMarket_FetchAll_PartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "symbol=BAD") {
			_, _ = w.Write([]byte(`{"Global Quote": {}}`))
			return
		}
		_, _ = w.Write([]byte(avHappyBody))
	}))
	defer srv.Close()
	m, err := feeds.NewMarket(feeds.MarketConfig{
		Attestor:   &fakeAttestor{},
		HTTPClient: srv.Client(),
		APIKey:     "k",
		BaseURL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("NewMarket: %v", err)
	}
	quotes, err := m.FetchAll(context.Background(), []string{"AAPL", "BAD"})
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("got %d quotes, want 1 (BAD skipped)", len(quotes))
	}
	if quotes[0].Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want AAPL", quotes[0].Symbol)
	}
}
