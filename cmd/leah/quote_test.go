package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const avBody = `{
  "Global Quote": {
    "01. symbol": "AAPL",
    "05. price": "203.40",
    "08. previous close": "200.00",
    "09. change": "3.40",
    "10. change percent": "1.7000%"
  }
}`

// writeKeyFile drops a 0600 alphavantage-key.json under stateDir/secrets/.
func writeKeyFile(t *testing.T, dir, key string) {
	t.Helper()
	secrets := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	raw, _ := json.Marshal(map[string]string{"api_key": key})
	if err := os.WriteFile(filepath.Join(secrets, "alphavantage-key.json"), raw, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

// TestRunQuote_Happy renders a synthesized line for a known symbol. AV base
// URL injection happens via package-level seam — the CLI defaults to AV
// prod but the adapter accepts BaseURL override; we test the CLI via a
// matching key file + a fake AV server reached through default DNS would
// be fragile, so the CLI exposes LEAH_ALPHAVANTAGE_BASE_URL for tests.
func TestRunQuote_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(avBody))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	t.Setenv("LEAH_ALPHAVANTAGE_BASE_URL", srv.URL)
	writeKeyFile(t, dir, "test-key")

	var buf bytes.Buffer
	if code := runQuote(context.Background(), []string{"AAPL"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; output=%q", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "AAPL") || !strings.Contains(out, "+1.7") {
		t.Errorf("output missing AAPL +1.7%%: %q", out)
	}
}

// TestRunQuote_UsageError covers the no-arg branch.
func TestRunQuote_UsageError(t *testing.T) {
	var buf bytes.Buffer
	if code := runQuote(context.Background(), nil, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunQuote_Help prints help on -h and returns 0.
func TestRunQuote_Help(t *testing.T) {
	var buf bytes.Buffer
	if code := runQuote(context.Background(), []string{"--help"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(buf.String(), "usage: leah quote") {
		t.Errorf("help output missing usage line: %q", buf.String())
	}
}

// TestRunQuote_UnknownSymbol_ReturnsOne — empty AV payload exits 1, not 0,
// so cron scripting catches "all tickers wrong" regressions.
func TestRunQuote_UnknownSymbol_ReturnsOne(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Global Quote": {}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	t.Setenv("LEAH_ALPHAVANTAGE_BASE_URL", srv.URL)
	writeKeyFile(t, dir, "test-key")

	var buf bytes.Buffer
	if code := runQuote(context.Background(), []string{"ZZZZ"}, &buf); code != 1 {
		t.Fatalf("exit %d, want 1; output=%q", code, buf.String())
	}
}

// TestRunQuote_MissingKey_ReturnsOne — no key file => exit 1 with a hint.
func TestRunQuote_MissingKey_ReturnsOne(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	var buf bytes.Buffer
	if code := runQuote(context.Background(), []string{"AAPL"}, &buf); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

// TestRunQuote_BadKeyMode_ReturnsOne — non-0600 mode is rejected per threat
// model §7 (secret hygiene).
func TestRunQuote_BadKeyMode_ReturnsOne(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FEEDS_AUTO_ATTEST", "1")
	secrets := filepath.Join(dir, "secrets")
	_ = os.MkdirAll(secrets, 0o700)
	raw, _ := json.Marshal(map[string]string{"api_key": "k"})
	if err := os.WriteFile(filepath.Join(secrets, "alphavantage-key.json"), raw, 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}
	var buf bytes.Buffer
	if code := runQuote(context.Background(), []string{"AAPL"}, &buf); code != 1 {
		t.Fatalf("exit %d, want 1 (bad mode)", code)
	}
}

// TestNormalizeSymbols_UpperCaseAndDedup — argv hygiene.
func TestNormalizeSymbols_UpperCaseAndDedup(t *testing.T) {
	got := normalizeSymbols([]string{"aapl", "AAPL", " msft ", ""})
	want := []string{"AAPL", "MSFT"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("normalizeSymbols = %v, want %v", got, want)
	}
}
