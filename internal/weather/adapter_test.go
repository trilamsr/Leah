package weather_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/weather"
	"github.com/trilam/leah/internal/widget"
)

func loadFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "sf_forecast.json"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return b
}

func writeJWT(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "weatherkit.p8")
	if err := os.WriteFile(path, []byte("-----BEGIN PRIVATE KEY-----\nstub\n-----END PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return path
}

func newServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/v1/weather/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func TestAdapter_Type(t *testing.T) {
	a, err := weather.NewAdapter(weather.Options{Lat: 37.77, Lon: -122.41, JWTPath: writeJWT(t)})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if a.Type() != "weather" {
		t.Fatalf("type: %q", a.Type())
	}
	// Conform to widget.WidgetAdapter.
	var _ widget.WidgetAdapter = a
}

func TestAdapter_ValidateRequiresLatLon(t *testing.T) {
	a, _ := weather.NewAdapter(weather.Options{Lat: 1, Lon: 1, JWTPath: writeJWT(t)})
	if err := a.Validate(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for empty props")
	}
	if err := a.Validate(json.RawMessage(`{"lat":37.77,"lon":-122.41}`)); err != nil {
		t.Fatalf("valid props rejected: %v", err)
	}
}

func TestAdapter_FetchParsesForecast(t *testing.T) {
	srv := newServer(t, loadFixture(t))
	defer srv.Close()
	a, _ := weather.NewAdapter(weather.Options{
		Lat: 37.7749, Lon: -122.4194, JWTPath: writeJWT(t), BaseURL: srv.URL,
	})
	p, err := a.Fetch(context.Background(), json.RawMessage(`{"lat":37.7749,"lon":-122.4194}`))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if p.Source != "weatherkit" {
		t.Fatalf("source: %q", p.Source)
	}
	if p.StaleAfter != 15*time.Minute {
		t.Fatalf("StaleAfter: %v", p.StaleAfter)
	}
	if p.Etag == "" {
		t.Fatal("etag empty")
	}
	var out struct {
		Condition string  `json:"condition"`
		TempC     float64 `json:"temp_c"`
		Lat       float64 `json:"lat"`
		Lon       float64 `json:"lon"`
	}
	if err := json.Unmarshal(p.Data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Condition == "" || out.Lat == 0 {
		t.Fatalf("parsed empty: %+v", out)
	}
}

func TestAdapter_EtagDeterministic(t *testing.T) {
	srv := newServer(t, loadFixture(t))
	defer srv.Close()
	a, _ := weather.NewAdapter(weather.Options{
		Lat: 37.7749, Lon: -122.4194, JWTPath: writeJWT(t), BaseURL: srv.URL,
		Now: func() time.Time { return time.Date(2026, 6, 22, 12, 30, 0, 0, time.UTC) },
	})
	props := json.RawMessage(`{"lat":37.7749,"lon":-122.4194}`)
	p1, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch1: %v", err)
	}
	p2, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch2: %v", err)
	}
	if p1.Etag != p2.Etag {
		t.Fatalf("etag drift: %q vs %q", p1.Etag, p2.Etag)
	}
	if !strings.HasPrefix(p1.Etag, "2026-06-22:") {
		t.Fatalf("etag should embed ISO-day: %q", p1.Etag)
	}
}

func TestAdapter_CtxCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()
	a, _ := weather.NewAdapter(weather.Options{
		Lat: 37.77, Lon: -122.41, JWTPath: writeJWT(t), BaseURL: srv.URL,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Fetch(ctx, json.RawMessage(`{"lat":37.77,"lon":-122.41}`)); err == nil {
		t.Fatal("expected ctx-cancel error")
	}
}

func TestAdapter_RefreshHonorsCancel(t *testing.T) {
	a, _ := weather.NewAdapter(weather.Options{Lat: 1, Lon: 1, JWTPath: writeJWT(t)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Refresh(ctx, "id1", json.RawMessage(`{"lat":1,"lon":1}`), nil); err == nil {
		t.Fatal("expected ctx-cancel error in Refresh")
	}
}
