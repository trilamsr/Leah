package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const avEarningsBody = `symbol,name,reportDate,fiscalDateEnding,estimate,currency,timeOfTheDay
AAPL,APPLE INC,2026-07-31,2026-06-28,1.42,USD,post-market
MSFT,MICROSOFT CORP,2026-07-29,2026-06-30,3.10,USD,post-market
`

func TestRunEarnings_Symbol_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(avEarningsBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	t.Setenv("LEAH_ALPHAVANTAGE_BASE_URL", srv.URL)
	writeKeyFile(t, dir, "k")

	var buf bytes.Buffer
	if code := runEarnings(context.Background(), []string{"AAPL"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "AAPL") || !strings.Contains(out, "2026-07-31") {
		t.Errorf("output missing AAPL 2026-07-31: %q", out)
	}
}

func TestRunEarnings_NoKey_SilentZero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	// No key file written — operator hasn't connected Alpha Vantage yet.
	var buf bytes.Buffer
	if code := runEarnings(context.Background(), []string{"AAPL"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0 (silent no-op)", code)
	}
	if buf.Len() != 0 {
		t.Errorf("stdout=%q, want empty", buf.String())
	}
}

func TestRunEarnings_NextWatchlist_NoSymbols_SilentZero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	writeKeyFile(t, dir, "k")
	// Empty watchlist — no calls, no output, exit 0.
	var buf bytes.Buffer
	if code := runEarnings(context.Background(), []string{"--next", "7d"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if buf.Len() != 0 {
		t.Errorf("stdout=%q, want empty", buf.String())
	}
}

func TestRunEarnings_Help(t *testing.T) {
	var buf bytes.Buffer
	if code := runEarnings(context.Background(), []string{"--help"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(buf.String(), "usage: leah earnings") {
		t.Errorf("help missing usage: %q", buf.String())
	}
}

func TestRunEarnings_UsageError(t *testing.T) {
	var buf bytes.Buffer
	if code := runEarnings(context.Background(), nil, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestParseEarningsWindow(t *testing.T) {
	cases := []struct {
		in   string
		want string
		err  bool
	}{
		{"7d", "168h0m0s", false},
		{"1d", "24h0m0s", false},
		{"30d", "720h0m0s", false},
		{"7", "", true},
		{"7w", "", true},
		{"", "", true},
		{"-3d", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			d, err := parseEarningsWindow(tc.in)
			if tc.err {
				if err == nil {
					t.Fatalf("err=nil, want non-nil for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("err=%v", err)
			}
			if d.String() != tc.want {
				t.Errorf("got %s, want %s", d, tc.want)
			}
		})
	}
}
