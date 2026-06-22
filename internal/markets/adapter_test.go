package markets_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/markets"
	"github.com/trilam/leah/internal/widget"
)

func TestMarketAdapter_Type(t *testing.T) {
	a := markets.NewAdapter(markets.Options{Endpoint: "http://x", APIKey: "k"})
	if a.Type() != "market" {
		t.Fatalf("Type: %q", a.Type())
	}
	var _ widget.WidgetAdapter = a
}

func TestMarketAdapter_FetchAAPL(t *testing.T) {
	fixture, err := os.ReadFile("testdata/aapl_quote.json")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var gotFn, gotSym string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFn = r.URL.Query().Get("function")
		gotSym = r.URL.Query().Get("symbol")
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	a := markets.NewAdapter(markets.Options{Endpoint: srv.URL, APIKey: "demo"})
	props := json.RawMessage(`{"symbols":["AAPL"]}`)
	p, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotFn != "GLOBAL_QUOTE" || gotSym != "AAPL" {
		t.Fatalf("upstream args: function=%q symbol=%q", gotFn, gotSym)
	}
	if p.Source != "alpha_vantage" {
		t.Fatalf("Source: %q", p.Source)
	}
	if p.StaleAfter != 60*time.Second {
		t.Fatalf("StaleAfter: %v", p.StaleAfter)
	}
	if p.Etag == "" {
		t.Fatalf("Etag empty")
	}
	var out struct {
		Quotes []markets.Quote `json:"quotes"`
	}
	if err := json.Unmarshal(p.Data, &out); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if len(out.Quotes) != 1 || out.Quotes[0].Symbol != "AAPL" || out.Quotes[0].Price != "213.40" {
		t.Fatalf("quotes: %+v", out.Quotes)
	}
}

func TestMarketAdapter_RejectsEmptySymbols(t *testing.T) {
	a := markets.NewAdapter(markets.Options{Endpoint: "http://x", APIKey: "k"})
	cases := []string{`{}`, `{"symbols":[]}`, `not-json`}
	for _, c := range cases {
		if err := a.Validate(json.RawMessage(c)); err == nil {
			t.Fatalf("Validate(%q): want error", c)
		}
	}
	if err := a.Validate(json.RawMessage(`{"symbols":["AAPL"]}`)); err != nil {
		t.Fatalf("Validate good: %v", err)
	}
}

func TestMarketAdapter_FetchHonorsCancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	a := markets.NewAdapter(markets.Options{Endpoint: srv.URL, APIKey: "demo"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Fetch(ctx, json.RawMessage(`{"symbols":["AAPL"]}`))
	if err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("want context-cancel error, got: %v", err)
	}
}

func TestMarketAdapter_EtagDeterministic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Global Quote":{"01. symbol":"AAPL","05. price":"1","09. change":"0","10. change percent":"0%","07. latest trading day":"2026-06-22"}}`))
	}))
	defer srv.Close()
	a := markets.NewAdapter(markets.Options{Endpoint: srv.URL, APIKey: "k"})
	p1, err := a.Fetch(context.Background(), json.RawMessage(`{"symbols":["AAPL","GOOG"]}`))
	if err != nil {
		t.Fatalf("p1: %v", err)
	}
	p2, err := a.Fetch(context.Background(), json.RawMessage(`{"symbols":["GOOG","AAPL"]}`))
	if err != nil {
		t.Fatalf("p2: %v", err)
	}
	if p1.Etag != p2.Etag {
		t.Fatalf("etag order-sensitive: %q vs %q", p1.Etag, p2.Etag)
	}
}

func TestMarketAdapter_RateLimitDenies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Global Quote":{"01. symbol":"AAPL","05. price":"1","09. change":"0","10. change percent":"0%","07. latest trading day":"2026-06-22"}}`))
	}))
	defer srv.Close()
	a := markets.NewAdapter(markets.Options{Endpoint: srv.URL, APIKey: "k", RateLimit: 2})
	for i := 0; i < 2; i++ {
		if _, err := a.Fetch(context.Background(), json.RawMessage(`{"symbols":["AAPL"]}`)); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if _, err := a.Fetch(context.Background(), json.RawMessage(`{"symbols":["AAPL"]}`)); err == nil {
		t.Fatalf("3rd call should rate-limit")
	}
}
