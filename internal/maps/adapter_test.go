package maps_test

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

	"github.com/trilam/leah/internal/maps"
	"github.com/trilam/leah/internal/widget"
)

func loadFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "sf_geocode.json"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return b
}

func writeJWT(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mapkit.p8")
	if err := os.WriteFile(path, []byte("-----BEGIN PRIVATE KEY-----\nstub\n-----END PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return path
}

func newServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Both /token and /geocode are valid Apple Maps Server endpoints; the
		// adapter MUST NOT request any tile endpoint.
		if strings.Contains(r.URL.Path, "tile") || strings.Contains(r.URL.Path, "snapshot") {
			t.Errorf("forbidden endpoint hit: %s", r.URL.Path)
		}
		if strings.Contains(r.URL.Path, "/v1/token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"stubaccess","expiresInSeconds":1800}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func TestAdapter_Type(t *testing.T) {
	a, err := maps.NewAdapter(maps.Options{JWTPath: writeJWT(t)})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if a.Type() != "maps" {
		t.Fatalf("type: %q", a.Type())
	}
	var _ widget.WidgetAdapter = a
}

func TestAdapter_ValidateRequiresPlaceQuery(t *testing.T) {
	a, _ := maps.NewAdapter(maps.Options{JWTPath: writeJWT(t)})
	if err := a.Validate(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for empty props")
	}
	if err := a.Validate(json.RawMessage(`{"place_query":""}`)); err == nil {
		t.Fatal("expected error for empty place_query")
	}
	if err := a.Validate(json.RawMessage(`{"place_query":"Golden Gate Bridge"}`)); err != nil {
		t.Fatalf("valid props rejected: %v", err)
	}
}

func TestAdapter_FetchReturnsGeocode(t *testing.T) {
	srv := newServer(t, loadFixture(t))
	defer srv.Close()
	a, _ := maps.NewAdapter(maps.Options{JWTPath: writeJWT(t), BaseURL: srv.URL})
	p, err := a.Fetch(context.Background(), json.RawMessage(`{"place_query":"Golden Gate Bridge, San Francisco"}`))
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if p.Source != "apple_maps" {
		t.Fatalf("source: %q", p.Source)
	}
	var out struct {
		Lat   float64 `json:"lat"`
		Lon   float64 `json:"lon"`
		Label string  `json:"label"`
	}
	if err := json.Unmarshal(p.Data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Lat == 0 || out.Lon == 0 || out.Label == "" {
		t.Fatalf("parsed empty: %+v", out)
	}
	// Payload MUST NOT contain tile data — only geocode.
	if strings.Contains(string(p.Data), "tile") || strings.Contains(string(p.Data), "snapshot") {
		t.Fatalf("payload leaked tile/snapshot data: %s", p.Data)
	}
}

func TestAdapter_CtxCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()
	a, _ := maps.NewAdapter(maps.Options{JWTPath: writeJWT(t), BaseURL: srv.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Fetch(ctx, json.RawMessage(`{"place_query":"x"}`)); err == nil {
		t.Fatal("expected ctx-cancel error")
	}
}

func TestAdapter_RefreshHonorsCancel(t *testing.T) {
	a, _ := maps.NewAdapter(maps.Options{JWTPath: writeJWT(t)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Refresh(ctx, "id1", json.RawMessage(`{"place_query":"x"}`), nil); err == nil {
		t.Fatal("expected ctx-cancel error in Refresh")
	}
}
