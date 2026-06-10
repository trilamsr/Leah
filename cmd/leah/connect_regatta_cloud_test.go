package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeAttestor returns its preset error from Attest. Used to flip the
// attestation outcome per test.
type fakeAttestor struct {
	err error
}

func (f fakeAttestor) Attest(_ context.Context, _ string) error { return f.err }

// insecureClient is an HTTPClient that trusts httptest's self-signed cert.
// Tests run against httptest.NewTLSServer so we don't hard-code a real CA.
func insecureClient() *http.Client {
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
}

func newHealthzTLSServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(status)
	}))
	return srv
}

// allowlistForServer pulls the host out of an httptest server URL so the test
// can pin the allowlist to exactly the ephemeral 127.0.0.1:NNNN.
func allowlistForServer(t *testing.T, srvURL string) []string {
	t.Helper()
	u, err := url.Parse(srvURL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return []string{u.Hostname()}
}

// TestConnectRegattaCloud_HappyPath wires attestation+healthz+cost prompt all
// to pass and asserts the token file, mode file and audit row land correctly.
func TestConnectRegattaCloud_HappyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	srv := newHealthzTLSServer(t, http.StatusOK)
	defer srv.Close()

	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: allowlistForServer(t, srv.URL),
	}

	var buf bytes.Buffer
	code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", srv.URL, "--token", "sekret"},
		&buf, deps)
	if code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "ok: regatta cloud connected") {
		t.Fatalf("ok line missing: %s", buf.String())
	}

	tokenPath := filepath.Join(dir, "secrets", "regatta-token.json")
	modePath := filepath.Join(dir, "secrets", "regatta-mode.json")
	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("token file missing: %v", err)
	}
	if _, err := os.Stat(modePath); err != nil {
		t.Fatalf("mode file missing: %v", err)
	}

	// Mode file points at the token file by path (not inline).
	var mode cloudModeFile
	raw, _ := os.ReadFile(modePath)
	if err := json.Unmarshal(raw, &mode); err != nil {
		t.Fatalf("mode unmarshal: %v", err)
	}
	if mode.Mode != "cloud" {
		t.Fatalf("mode.Mode = %q, want cloud", mode.Mode)
	}
	if mode.TokenPath != tokenPath {
		t.Fatalf("mode.TokenPath = %q, want %q", mode.TokenPath, tokenPath)
	}
	if strings.Contains(string(raw), "sekret") {
		t.Fatalf("token leaked into mode file: %s", raw)
	}
}

// TestConnectRegattaCloud_AttestationDenied_NoWrite asserts a denied
// attestation writes no token, no mode file, and an audit row with
// outcome=failed.
func TestConnectRegattaCloud_AttestationDenied_NoWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	srv := newHealthzTLSServer(t, http.StatusOK)
	defer srv.Close()

	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{err: errors.New("denied")},
		Allowlist: allowlistForServer(t, srv.URL),
	}

	var buf bytes.Buffer
	code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", srv.URL, "--token", "sekret"},
		&buf, deps)
	if code == 0 {
		t.Fatalf("expected non-zero exit on attestation denial")
	}
	for _, p := range []string{
		filepath.Join(dir, "secrets", "regatta-token.json"),
		filepath.Join(dir, "secrets", "regatta-mode.json"),
	} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("file leaked despite denial: %s", p)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"connect_regatta_cloud"`)) ||
		!bytes.Contains(raw, []byte(`"attestation_denied"`)) {
		t.Fatalf("audit row mismatch: %s", raw)
	}
}

// TestConnectRegattaCloud_BadURL_NotHTTPS_Rejects guards against a typo or
// downgrade attack — http:// must be refused before any network call.
func TestConnectRegattaCloud_BadURL_NotHTTPS_Rejects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	deps := &cloudDeps{
		HTTP:     insecureClient(),
		Attestor: fakeAttestor{},
	}

	var buf bytes.Buffer
	code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", "http://regatta.example.com", "--token", "sekret"},
		&buf, deps)
	if code == 0 {
		t.Fatalf("expected non-zero exit on non-HTTPS URL")
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-token.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token leaked on non-https URL")
	}
}

// TestConnectRegattaCloud_HealthCheck404_NoTokenWrite asserts a non-2xx
// healthz aborts before token write.
func TestConnectRegattaCloud_HealthCheck404_NoTokenWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	srv := newHealthzTLSServer(t, http.StatusNotFound)
	defer srv.Close()

	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: allowlistForServer(t, srv.URL),
	}

	var buf bytes.Buffer
	code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", srv.URL, "--token", "sekret"},
		&buf, deps)
	if code == 0 {
		t.Fatalf("expected non-zero exit on 404 healthz")
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-token.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token leaked on failed healthz")
	}
}

// TestConnectRegattaCloud_TokenFile0600 — mode + token files MUST be 0600 so
// neighbouring uids on a shared workstation can't read the bearer.
// Skipped on Windows where unix perms aren't meaningful.
func TestConnectRegattaCloud_TokenFile0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("0600 perm semantics are unix-only")
	}
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	srv := newHealthzTLSServer(t, http.StatusOK)
	defer srv.Close()

	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: allowlistForServer(t, srv.URL),
	}

	var buf bytes.Buffer
	if code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", srv.URL, "--token", "sekret"},
		&buf, deps); code != 0 {
		t.Fatalf("exit %d: %s", code, buf.String())
	}
	for _, p := range []string{
		filepath.Join(dir, "secrets", "regatta-token.json"),
		filepath.Join(dir, "secrets", "regatta-mode.json"),
	} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s perm = %v, want 0600", p, info.Mode().Perm())
		}
	}
}

// TestConnectRegattaCloud_AuditRowOnlyHostNoToken — the audit row records
// host only. Token bytes (and other URL fragments that would leak the host
// path or token via stray query strings) MUST NOT appear.
func TestConnectRegattaCloud_AuditRowOnlyHostNoToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	srv := newHealthzTLSServer(t, http.StatusOK)
	defer srv.Close()

	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: allowlistForServer(t, srv.URL),
	}

	var buf bytes.Buffer
	if code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", srv.URL, "--token", "super-secret-bearer"},
		&buf, deps); code != 0 {
		t.Fatalf("exit %d: %s", code, buf.String())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if bytes.Contains(raw, []byte("super-secret-bearer")) {
		t.Fatalf("token bytes leaked into audit row: %s", raw)
	}
	u, _ := url.Parse(srv.URL)
	if !bytes.Contains(raw, []byte(u.Hostname())) {
		t.Fatalf("expected url_host %q in audit row: %s", u.Hostname(), raw)
	}
	// Sanity: the row records the success outcome with the connect_regatta_cloud kind.
	if !bytes.Contains(raw, []byte(`"connect_regatta_cloud"`)) ||
		!bytes.Contains(raw, []byte(`"success"`)) {
		t.Fatalf("expected success row: %s", raw)
	}
}

// TestConnectRegattaCloud_HostNotAllowlisted — operator-configured allowlist
// rejects unknown hosts even when the URL is otherwise valid HTTPS.
func TestConnectRegattaCloud_HostNotAllowlisted(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	srv := newHealthzTLSServer(t, http.StatusOK)
	defer srv.Close()

	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: []string{"regatta.example.com"}, // does not match httptest 127.0.0.1
	}

	var buf bytes.Buffer
	code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", srv.URL, "--token", "sekret"},
		&buf, deps)
	if code == 0 {
		t.Fatalf("expected non-zero exit on host-not-allowlisted")
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-token.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token leaked on disallowed host")
	}
}

// TestConnectRegattaCloud_CostDeclined_NoWrite — operator declines the cost
// prompt → token / mode files stay absent and the audit row records the
// declination.
func TestConnectRegattaCloud_CostDeclined_NoWrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	srv := newHealthzTLSServer(t, http.StatusOK)
	defer srv.Close()

	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: allowlistForServer(t, srv.URL),
		ConfirmFn: func() bool { return false },
	}

	var buf bytes.Buffer
	code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", srv.URL, "--token", "sekret"},
		&buf, deps)
	if code == 0 {
		t.Fatalf("expected non-zero exit on cost decline")
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-token.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token leaked despite cost decline")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if !bytes.Contains(raw, []byte("cost_declined")) {
		t.Fatalf("expected cost_declined detail: %s", raw)
	}
}

// TestConnectRegattaCloud_DispatchedFromConnectRegatta — the existing
// runConnectRegatta entrypoint routes --cloud to the new handler. Without
// --token the cloud branch fails with exit 2 (flag parse error) rather than
// trying to spin up Docker.
func TestConnectRegattaCloud_DispatchedFromConnectRegatta(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	var buf bytes.Buffer
	code := runConnectRegatta(context.Background(),
		[]string{"--cloud", "--url", "https://regatta.example.com"},
		&buf, nil)
	if code == 0 {
		t.Fatalf("expected non-zero exit when --token is missing")
	}
	// No docker container should have been touched — assertion is implicit:
	// the dispatcher returned before reaching the docker provider, so the
	// nil provider in args[3] never panicked.
}
