package connect

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

type fakeAttestor struct {
	err   error
	calls []string
}

func (f *fakeAttestor) Attest(ctx context.Context, scope string) error {
	f.calls = append(f.calls, scope)
	return f.err
}

// TestConnect_Gmail_DeviceCodeFlow exercises the happy path against an httptest
// stand-in for Google's /device/code + /token endpoints and asserts the token
// file is written with 0600 mode + matching token bytes.
func TestConnect_Gmail_DeviceCodeFlow(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "gmail-token.json")

	srv := newDeviceAuthServer(t, "access-abc", "refresh-xyz")
	defer srv.Close()

	p := &gmailProvider{
		clientID:     "test-client",
		clientSecret: "test-secret",
		tokenPath:    tokenPath,
		deviceURL:    srv.URL + "/device/code",
		tokenURL:     srv.URL + "/token",
		pollInterval: 0,
	}
	att := &fakeAttestor{}
	tok, err := Authorize(context.Background(), p, att, nopPrompt)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if tok.AccessToken != "access-abc" {
		t.Fatalf("token: got %q", tok.AccessToken)
	}
	if len(att.calls) != 1 || att.calls[0] != "connect:gmail" {
		t.Fatalf("attestor calls: %v", att.calls)
	}
	st, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v", st.Mode().Perm())
	}
	b, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var on oauth2.Token
	if err := json.Unmarshal(b, &on); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if on.AccessToken != "access-abc" || on.RefreshToken != "refresh-xyz" {
		t.Fatalf("disk token: %+v", on)
	}
}

// TestConnect_AttestationDenied_NoTokenFile pins the gate ordering: a denied
// Attestor MUST prevent the OAuth exchange and the token file MUST NOT exist.
func TestConnect_AttestationDenied_NoTokenFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "gmail-token.json")

	hit := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		w.WriteHeader(500)
	}))
	defer srv.Close()

	p := &gmailProvider{
		clientID:  "x",
		tokenPath: tokenPath,
		deviceURL: srv.URL + "/device/code",
		tokenURL:  srv.URL + "/token",
	}
	att := &fakeAttestor{err: errors.New("operator denied")}

	_, err := Authorize(context.Background(), p, att, nopPrompt)
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err: got %v, want ErrAttestationDenied", err)
	}
	if hit != 0 {
		t.Fatalf("OAuth endpoint hit %d times despite attestation denial", hit)
	}
	if _, err := os.Stat(tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token file exists after denied attestation: %v", err)
	}
}

// TestConnect_List enumerates installed providers and toggles auth-status
// purely on token-file presence — the on-disk state IS the source of truth.
func TestConnect_List(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	r := newTestRegistry(dir)
	statuses := r.List()
	if len(statuses) != 2 {
		t.Fatalf("want 2 providers, got %d", len(statuses))
	}
	for _, s := range statuses {
		if s.Authorized {
			t.Fatalf("provider %s shouldn't be authorized yet", s.Name)
		}
	}

	gmailPath := r.byName["gmail"].TokenPath()
	if err := os.MkdirAll(filepath.Dir(gmailPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(gmailPath, []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	statuses = r.List()
	var gmailAuth bool
	for _, s := range statuses {
		if s.Name == "gmail" {
			gmailAuth = s.Authorized
		}
	}
	if !gmailAuth {
		t.Fatalf("gmail should be authorized after token write")
	}
}

// TestConnect_RewriteExistingToken proves the second authorize overwrites the
// first and the 0600 mode survives the rewrite (no widening on overwrite).
func TestConnect_RewriteExistingToken(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "gmail-token.json")
	if err := os.WriteFile(tokenPath, []byte(`{"access_token":"old"}`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	srv := newDeviceAuthServer(t, "new-access", "")
	defer srv.Close()

	p := &gmailProvider{
		clientID:  "x",
		tokenPath: tokenPath,
		deviceURL: srv.URL + "/device/code",
		tokenURL:  srv.URL + "/token",
	}
	if _, err := Authorize(context.Background(), p, &fakeAttestor{}, nopPrompt); err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	st, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode after rewrite: %v", st.Mode().Perm())
	}
	b, _ := os.ReadFile(tokenPath)
	if !strings.Contains(string(b), "new-access") {
		t.Fatalf("token not rewritten: %s", b)
	}
}

// TestWriteToken_PostChmodReassert covers a hostile umask: even when the
// initial create races a permissive mode, the post-write Chmod re-pins 0600.
func TestWriteToken_PostChmodReassert(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tok.json")
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := WriteToken(p, &oauth2.Token{AccessToken: "a"}); err != nil {
		t.Fatalf("WriteToken: %v", err)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v", st.Mode().Perm())
	}
}

func nopPrompt(verificationURL, userCode string) {}

func newDeviceAuthServer(t *testing.T, accessTok, refreshTok string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code":      "dev-123",
			"user_code":        "ABCD-EFGH",
			"verification_url": "https://example.test/verify",
			"expires_in":       1800,
			"interval":         0,
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		body, _ := url.ParseQuery(readBody(r))
		if body.Get("device_code") != "dev-123" {
			http.Error(w, "bad device_code", 400)
			return
		}
		resp := map[string]any{
			"access_token": accessTok,
			"token_type":   "Bearer",
			"expires_in":   3600,
		}
		if refreshTok != "" {
			resp["refresh_token"] = refreshTok
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return httptest.NewServer(mux)
}

func readBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	buf := make([]byte, 4096)
	n, _ := r.Body.Read(buf)
	return string(buf[:n])
}

func newTestRegistry(stateDir string) *Registry {
	return NewRegistry([]Provider{
		&gmailProvider{tokenPath: filepath.Join(stateDir, "secrets", "gmail-token.json")},
		&gcalProvider{tokenPath: filepath.Join(stateDir, "secrets", "gcal-token.json")},
	})
}
