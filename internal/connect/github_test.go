package connect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// TestConnect_GitHub_DeviceCodeFlow exercises the github RFC-8628 happy path
// against an httptest stand-in for github.com's /device/code + /oauth/access_token
// endpoints. The two providers share runDeviceCodeFlow, so this test is the
// thin pin that the github-shaped wiring (URLs, scopes, name) is intact.
func TestConnect_GitHub_DeviceCodeFlow(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "github-token.json")

	mux := http.NewServeMux()
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if got := r.Form.Get("scope"); got != "repo read:user" {
			t.Errorf("scope=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "dev-gh",
			"user_code":        "WXYZ-1234",
			"verification_url": "https://github.com/login/device",
			"expires_in":       900,
			"interval":         0,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		body, _ := url.ParseQuery(readBody(r))
		if body.Get("device_code") != "dev-gh" {
			http.Error(w, "bad device_code", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "gho_access",
			"token_type":   "bearer",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := &githubProvider{
		clientID:     "Iv1.test",
		clientSecret: "ghs_test",
		tokenPath:    tokenPath,
		deviceURL:    srv.URL + "/device/code",
		tokenURL:     srv.URL + "/token",
		pollInterval: 0,
	}
	tok, err := Authorize(context.Background(), p, &fakeAttestor{}, nopPrompt)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if tok.AccessToken != "gho_access" {
		t.Fatalf("token: got %q", tok.AccessToken)
	}
	st, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v", st.Mode().Perm())
	}
}

// TestNewGitHub_Identity pins surface (name/scopes/token-path) so a careless
// rename downstream doesn't drift the registry key the CLI dispatches against.
func TestNewGitHub_Identity(t *testing.T) {
	t.Setenv("LEAH_STATE_DIR", t.TempDir())
	p := NewGitHub("id", "secret")
	if p.Name() != "github" {
		t.Fatalf("name=%q", p.Name())
	}
	if got := p.Scopes(); len(got) != 2 || got[0] != "repo" || got[1] != "read:user" {
		t.Fatalf("scopes=%v", got)
	}
	if !filepath.IsAbs(p.TokenPath()) {
		t.Fatalf("token path not absolute: %q", p.TokenPath())
	}
}
