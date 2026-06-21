package feeds

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeAttestor inlined per-test — keeps the contract local + avoids
// dragging market_test wiring through a different file's expectations.
type fakeEarningsAttestor struct{ deny bool }

func (f fakeEarningsAttestor) Attest(_ context.Context, _ string) error {
	if f.deny {
		return errors.New("denied")
	}
	return nil
}

const avEarningsCSV = `symbol,name,reportDate,fiscalDateEnding,estimate,currency,timeOfTheDay
AAPL,APPLE INC,2026-07-31,2026-06-28,1.42,USD,post-market
MSFT,MICROSOFT CORP,2026-07-29,2026-06-30,3.10,USD,post-market
NVDA,NVIDIA CORP,2026-08-27,2026-07-31,,USD,
`

func TestEarnings_FetchSymbol_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("function"); got != "EARNINGS_CALENDAR" {
			t.Errorf("function=%q, want EARNINGS_CALENDAR", got)
		}
		if got := r.URL.Query().Get("symbol"); got != "AAPL" {
			t.Errorf("symbol=%q, want AAPL", got)
		}
		_, _ = w.Write([]byte(avEarningsCSV))
	}))
	defer srv.Close()

	e, err := NewEarnings(EarningsConfig{
		Attestor:   fakeEarningsAttestor{},
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		APIKey:     "test-key",
		BaseURL:    srv.URL,
	})
	if err != nil {
		t.Fatalf("NewEarnings: %v", err)
	}
	got, err := e.FetchSymbol(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("FetchSymbol: %v", err)
	}
	if got.Symbol != "AAPL" {
		t.Errorf("Symbol=%q, want AAPL", got.Symbol)
	}
	if got.ReportDate.Format("2006-01-02") != "2026-07-31" {
		t.Errorf("ReportDate=%v, want 2026-07-31", got.ReportDate)
	}
	if got.TimeOfDay != "post-market" {
		t.Errorf("TimeOfDay=%q, want post-market", got.TimeOfDay)
	}
}

func TestEarnings_FetchSymbol_UnknownSymbol(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("symbol,name,reportDate,fiscalDateEnding,estimate,currency,timeOfTheDay\n"))
	}))
	defer srv.Close()

	e, _ := NewEarnings(EarningsConfig{
		Attestor:   fakeEarningsAttestor{},
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		APIKey:     "k",
		BaseURL:    srv.URL,
	})
	_, err := e.FetchSymbol(context.Background(), "ZZZZ")
	if !errors.Is(err, ErrNoEarnings) {
		t.Fatalf("err=%v, want ErrNoEarnings", err)
	}
}

func TestEarnings_FetchUpcoming_FiltersWindow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("symbol"); got != "" {
			t.Errorf("symbol=%q, want empty (full-calendar pull)", got)
		}
		_, _ = w.Write([]byte(avEarningsCSV))
	}))
	defer srv.Close()

	e, _ := NewEarnings(EarningsConfig{
		Attestor:   fakeEarningsAttestor{},
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		APIKey:     "k",
		BaseURL:    srv.URL,
		Now:        func() time.Time { return time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC) },
	})

	// 5-day window from 2026-07-28 → catches MSFT (07-29) + AAPL (07-31), skips NVDA (08-27).
	got, err := e.FetchUpcoming(context.Background(), []string{"AAPL", "MSFT", "NVDA"}, 5*24*time.Hour)
	if err != nil {
		t.Fatalf("FetchUpcoming: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2; got=%+v", len(got), got)
	}
	// Sorted ascending by ReportDate — MSFT (07-29) before AAPL (07-31).
	if got[0].Symbol != "MSFT" || got[1].Symbol != "AAPL" {
		t.Errorf("order wrong: %q,%q; want MSFT,AAPL", got[0].Symbol, got[1].Symbol)
	}
}

func TestEarnings_FetchUpcoming_EmptyWatchlist(t *testing.T) {
	e, _ := NewEarnings(EarningsConfig{
		Attestor:   fakeEarningsAttestor{},
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		APIKey:     "k",
		BaseURL:    "http://unused",
	})
	got, err := e.FetchUpcoming(context.Background(), nil, 24*time.Hour)
	if err != nil {
		t.Fatalf("err=%v, want nil for empty watchlist", err)
	}
	if len(got) != 0 {
		t.Errorf("len=%d, want 0", len(got))
	}
}

func TestEarnings_AttestationDenied_NoHTTPCall(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { hit = true }))
	defer srv.Close()

	e, _ := NewEarnings(EarningsConfig{
		Attestor:   fakeEarningsAttestor{deny: true},
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		APIKey:     "k",
		BaseURL:    srv.URL,
	})
	_, err := e.FetchSymbol(context.Background(), "AAPL")
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v, want ErrAttestationDenied", err)
	}
	if hit {
		t.Error("HTTP fired despite denied attestation")
	}
}

func TestNewEarnings_RejectsHalfWiredConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  EarningsConfig
	}{
		{"no Attestor", EarningsConfig{HTTPClient: &http.Client{}, APIKey: "k"}},
		{"no HTTPClient", EarningsConfig{Attestor: fakeEarningsAttestor{}, APIKey: "k"}},
		{"no APIKey", EarningsConfig{Attestor: fakeEarningsAttestor{}, HTTPClient: &http.Client{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewEarnings(tc.cfg); err == nil {
				t.Fatal("err=nil, want non-nil")
			}
		})
	}
}

func TestEarnings_FetchSymbol_ProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	e, _ := NewEarnings(EarningsConfig{
		Attestor:   fakeEarningsAttestor{},
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
		APIKey:     "k",
		BaseURL:    srv.URL,
	})
	_, err := e.FetchSymbol(context.Background(), "AAPL")
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("err=%v, want provider 429", err)
	}
}
