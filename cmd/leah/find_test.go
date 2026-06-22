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

// writeTMDBToken stages the connect-shaped token under stateDir.
func writeTMDBToken(t *testing.T, dir, token string) {
	t.Helper()
	path := filepath.Join(dir, "secrets", "tmdb-token.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"access_token": token})
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

// TestFindCLI_HappyPath_RendersTable — full e2e through the in-process HTTP server.
func TestFindCLI_HappyPath_RendersTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/multi"):
			_, _ = w.Write([]byte(`{"results":[{"id":1399,"media_type":"tv","name":"Game of Thrones","first_air_date":"2011-04-17"}]}`))
		case strings.HasPrefix(r.URL.Path, "/tv/1399/watch/providers"):
			_, _ = w.Write([]byte(`{"results":{"US":{"flatrate":[{"provider_name":"HBO Max"}],"rent":[{"provider_name":"Apple TV"}],"buy":[{"provider_name":"Apple TV"}]}}}`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_TMDB_BASE_URL", srv.URL)
	writeTMDBToken(t, dir, "eyJ-test")

	var buf bytes.Buffer
	if code := runFind(context.Background(), []string{"game", "of", "thrones"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, buf.String())
	}
	out := buf.String()
	for _, want := range []string{"Game of Thrones", "2011", "stream:", "HBO Max", "rent:", "Apple TV", "buy:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

// TestFindCLI_MissingArg_PrintsUsage — bare invocation exits 2.
func TestFindCLI_MissingArg_PrintsUsage(t *testing.T) {
	r, w, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	var buf bytes.Buffer
	code := runFind(context.Background(), nil, &buf)
	_ = w.Close()
	errBytes := make([]byte, 4096)
	n, _ := r.Read(errBytes)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(string(errBytes[:n]), "usage: leah find") {
		t.Errorf("expected usage in stderr, got %q", string(errBytes[:n]))
	}
}

// TestFindCLI_TokenAbsent_ExitsTwo — missing token gives the connect hint.
func TestFindCLI_TokenAbsent_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	r, w, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	var buf bytes.Buffer
	code := runFind(context.Background(), []string{"x"}, &buf)
	_ = w.Close()
	errBytes := make([]byte, 4096)
	n, _ := r.Read(errBytes)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(string(errBytes[:n]), "leah connect tmdb") {
		t.Errorf("expected connect hint, got %q", string(errBytes[:n]))
	}
}

// TestFindCLI_NoResults_ExitsOneWithMessage — empty search → exit 1.
func TestFindCLI_NoResults_ExitsOneWithMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_TMDB_BASE_URL", srv.URL)
	writeTMDBToken(t, dir, "eyJ-test")

	r, w, _ := os.Pipe()
	oldStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	var buf bytes.Buffer
	code := runFind(context.Background(), []string{"asdfqwerty"}, &buf)
	_ = w.Close()
	errBytes := make([]byte, 4096)
	n, _ := r.Read(errBytes)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(string(errBytes[:n]), "no results") {
		t.Errorf("expected 'no results' message: %q", string(errBytes[:n]))
	}
}

// TestFindCLI_RegionFlag_OverridesDefault — --region propagates into transport.
func TestFindCLI_RegionFlag_OverridesDefault(t *testing.T) {
	var gotRegion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/multi"):
			_, _ = w.Write([]byte(`{"results":[{"id":1,"media_type":"movie","title":"X","release_date":"2020-01-01"}]}`))
		case strings.HasPrefix(r.URL.Path, "/movie/1/watch/providers"):
			_, _ = w.Write([]byte(`{"results":{"GB":{"flatrate":[{"provider_name":"BBC"}]}}}`))
			// region is encoded in the result map key, not URL — but we can still
			// detect by which region's data the renderer prints.
			gotRegion = "served"
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_TMDB_BASE_URL", srv.URL)
	writeTMDBToken(t, dir, "eyJ")

	var buf bytes.Buffer
	if code := runFind(context.Background(), []string{"--region", "GB", "x"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0; out=%q", code, buf.String())
	}
	if gotRegion == "" {
		t.Fatalf("provider endpoint not hit")
	}
	if !strings.Contains(buf.String(), "BBC") {
		t.Errorf("missing GB provider: %q", buf.String())
	}
}
