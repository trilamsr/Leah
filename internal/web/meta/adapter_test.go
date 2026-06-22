package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func citationServer(t *testing.T) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "arxiv_2024_01234.html"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	}))
}

func TestCitationAdapter_Type(t *testing.T) {
	a := NewCitationAdapter()
	if a.Type() != "citation" {
		t.Errorf("Type = %q", a.Type())
	}
}

func TestCitationAdapter_ValidateRejectsHTTP(t *testing.T) {
	a := NewCitationAdapter()
	props := json.RawMessage(`{"url":"http://example.com"}`)
	if err := a.Validate(props); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected https rejection, got %v", err)
	}
}

func TestCitationAdapter_ValidateRejectsMalformed(t *testing.T) {
	a := NewCitationAdapter()
	if err := a.Validate(json.RawMessage(`{}`)); err == nil {
		t.Fatal("empty url should fail")
	}
	if err := a.Validate(json.RawMessage(`not json`)); err == nil {
		t.Fatal("malformed json should fail")
	}
}

func TestCitationAdapter_Fetch(t *testing.T) {
	srv := citationServer(t)
	defer srv.Close()
	a := newCitationAdapterWithClient(srv.Client())
	props, _ := json.Marshal(map[string]string{"url": srv.URL + "/abs/2024.01234"})
	p, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var out struct {
		Title       string `json:"title"`
		SiteName    string `json:"site_name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(p.Data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Title == "" {
		t.Error("title empty")
	}
	if out.SiteName != "arXiv" {
		t.Errorf("SiteName = %q", out.SiteName)
	}
}

func TestCitationAdapter_RejectsPrivateIP(t *testing.T) {
	a := NewCitationAdapter()
	props := json.RawMessage(`{"url":"https://10.0.0.1/x"}`)
	if _, err := a.Fetch(context.Background(), props); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("expected private-ip rejection, got %v", err)
	}
}

func TestCitationAdapter_RejectsLoopback(t *testing.T) {
	a := NewCitationAdapter()
	props := json.RawMessage(`{"url":"https://127.0.0.1/x"}`)
	if _, err := a.Fetch(context.Background(), props); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("expected loopback rejection, got %v", err)
	}
}

func TestImageAdapter_Type(t *testing.T) {
	if NewImageAdapter(t.TempDir()).Type() != "image" {
		t.Fatal("Type")
	}
}

func TestImageAdapter_Fetch_SmallInlineBase64(t *testing.T) {
	body := newPNG(t, 2, 2)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	a := newImageAdapterWithClient(t.TempDir(), srv.Client())
	props, _ := json.Marshal(map[string]string{"url": srv.URL + "/x.png"})
	p, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var out struct {
		Inline string `json:"inline_base64"`
		Path   string `json:"path"`
		Mime   string `json:"mime"`
	}
	if err := json.Unmarshal(p.Data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Inline == "" {
		t.Error("small image should be inline base64")
	}
	if out.Mime != "image/png" {
		t.Errorf("mime = %q", out.Mime)
	}
}

func TestImageAdapter_Fetch_LargeUsesPath(t *testing.T) {
	body := newPNG(t, 256, 256) // >32 KB
	if len(body) < 32*1024 {
		t.Skipf("fixture too small (%d)", len(body))
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	dir := t.TempDir()
	a := newImageAdapterWithClient(dir, srv.Client())
	props, _ := json.Marshal(map[string]string{"url": srv.URL + "/big.png"})
	p, err := a.Fetch(context.Background(), props)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	var out struct {
		Inline string `json:"inline_base64"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(p.Data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Inline != "" {
		t.Error("large image should not be inlined")
	}
	if out.Path == "" {
		t.Error("large image should have path")
	}
	if _, err := os.Stat(out.Path); err != nil {
		t.Errorf("cached file missing: %v", err)
	}
}

func TestImageAdapter_RejectsBadMIME(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("bin"))
	}))
	defer srv.Close()
	a := newImageAdapterWithClient(t.TempDir(), srv.Client())
	props, _ := json.Marshal(map[string]string{"url": srv.URL + "/x"})
	if _, err := a.Fetch(context.Background(), props); err == nil || !strings.Contains(err.Error(), "mime") {
		t.Fatalf("expected mime rejection, got %v", err)
	}
}

func TestImageAdapter_RejectsOversize(t *testing.T) {
	// Lie about content-type then send >10MB. Server streams oversized body.
	big := make([]byte, 11*1024*1024)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(big)
	}))
	defer srv.Close()
	a := newImageAdapterWithClient(t.TempDir(), srv.Client())
	props, _ := json.Marshal(map[string]string{"url": srv.URL + "/huge.png"})
	if _, err := a.Fetch(context.Background(), props); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

func TestImageAdapter_RejectsHTTP(t *testing.T) {
	a := NewImageAdapter(t.TempDir())
	props := json.RawMessage(`{"url":"http://example.com/x.png"}`)
	if err := a.Validate(props); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected https rejection, got %v", err)
	}
}

func TestImageAdapter_RedirectLimit(t *testing.T) {
	var hits int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Redirect(w, r, r.URL.Path+"/r", http.StatusFound)
	}))
	defer srv.Close()
	a := newImageAdapterWithClient(t.TempDir(), srv.Client())
	props, _ := json.Marshal(map[string]string{"url": srv.URL + "/start.png"})
	if _, err := a.Fetch(context.Background(), props); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("expected redirect-limit error, got %v", err)
	}
	if hits > 5 {
		t.Errorf("redirect cap leaked: %d hits", hits)
	}
}
