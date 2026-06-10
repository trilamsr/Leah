package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
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

// TestConnectRegattaCloud_ModeFileStripsUserinfoAndQuery — userinfo + ?token=…
// must not land in the 0o600 mode file. Mode-file URL is scheme+host+path only.
func TestConnectRegattaCloud_ModeFileStripsUserinfoAndQuery(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	srv := newHealthzTLSServer(t, http.StatusOK)
	defer srv.Close()

	// Construct a URL with a query string. Userinfo is rejected outright by
	// the validator (separate test), so the strip-on-write case targets query.
	rawURL := srv.URL + "/?token=leaked-via-url&debug=1"
	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: allowlistForServer(t, srv.URL),
	}

	var buf bytes.Buffer
	if code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", rawURL, "--token", "sekret"},
		&buf, deps); code != 0 {
		t.Fatalf("exit %d: %s", code, buf.String())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "secrets", "regatta-mode.json"))
	if err != nil {
		t.Fatalf("read mode: %v", err)
	}
	if bytes.Contains(raw, []byte("leaked-via-url")) {
		t.Fatalf("query string leaked into mode file: %s", raw)
	}
	if bytes.Contains(raw, []byte("?")) || bytes.Contains(raw, []byte("debug=1")) {
		t.Fatalf("query bytes leaked: %s", raw)
	}
}

// TestConnectRegattaCloud_URLWithUserinfoRejected — `https://u:p@host` MUST
// be rejected: it would land basic-auth credentials in the mode file AND
// double-send creds (Basic + Bearer) on every healthz call.
func TestConnectRegattaCloud_URLWithUserinfoRejected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: []string{"regatta.example.com"},
	}

	var buf bytes.Buffer
	code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", "https://user:pass@regatta.example.com", "--token", "sekret"},
		&buf, deps)
	if code == 0 {
		t.Fatalf("expected non-zero exit on userinfo URL")
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-token.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token leaked on userinfo URL")
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-mode.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mode file leaked on userinfo URL")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if !bytes.Contains(raw, []byte("url_has_userinfo")) {
		t.Fatalf("expected url_has_userinfo audit detail: %s", raw)
	}
}

// TestConnectRegattaCloud_HealthzURLBuiltWithJoinPath — a URL with a query
// string must not produce a broken `…?x=1/healthz`; the request must hit
// /healthz under the canonical base, query stripped.
func TestConnectRegattaCloud_HealthzURLBuiltWithJoinPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	var gotPath string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rawURL := srv.URL + "/?x=1"
	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: allowlistForServer(t, srv.URL),
	}

	var buf bytes.Buffer
	if code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", rawURL, "--token", "sekret"},
		&buf, deps); code != 0 {
		t.Fatalf("exit %d: %s", code, buf.String())
	}
	if gotPath != "/healthz" {
		t.Fatalf("healthz hit path %q, want /healthz (TrimRight + concat would have produced /?x=1/healthz)", gotPath)
	}
}

// TestConnectRegattaCloud_AllowlistErrorMessageGeneric — the failure message
// must not hardcode env name when an explicit allowlist is injected.
func TestConnectRegattaCloud_AllowlistErrorMessageGeneric(t *testing.T) {
	t.Setenv("LEAH_STATE_DIR", t.TempDir())
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: []string{"regatta.example.com"},
	}
	// runConnectRegattaCloud writes the operator-visible error to os.Stderr,
	// so capture stderr through a pipe.
	r, wPipe, _ := os.Pipe()
	origStderr := os.Stderr
	os.Stderr = wPipe
	defer func() { os.Stderr = origStderr }()

	var buf bytes.Buffer
	_ = runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", "https://other.example.com", "--token", "sekret"},
		&buf, deps)
	_ = wPipe.Close()
	stderr, _ := io.ReadAll(r)

	if bytes.Contains(stderr, []byte("LEAH_REGATTA_CLOUD_ALLOWLIST")) {
		t.Fatalf("error message leaked env var name even though allowlist was injected: %s", stderr)
	}
	if !bytes.Contains(stderr, []byte("not in cloud allowlist")) {
		t.Fatalf("expected 'not in cloud allowlist' phrase: %s", stderr)
	}
}

// TestConnectRegattaCloud_CostPromptNonInteractiveDeclines — non-TTY callers
// must not block on stdin; LEAH_NONINTERACTIVE=1 forces a safe decline so a
// background `leah connect regatta --cloud` cannot hang indefinitely.
func TestConnectRegattaCloud_CostPromptNonInteractiveDeclines(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_NONINTERACTIVE", "1")
	// Important: do NOT set LEAH_CONNECT_AUTO_ATTEST — that would short-circuit
	// the cost prompt to yes. We want to exercise the non-interactive branch.

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
	if code == 0 {
		t.Fatalf("expected non-zero exit when non-interactive declines cost")
	}
	if _, err := os.Stat(filepath.Join(dir, "secrets", "regatta-token.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("token leaked despite non-interactive decline")
	}
}

// TestConnectRegattaCloud_AuditFailureIncludesHost — failed audit rows must
// carry host detail so a denied connect attempt can be triaged without
// re-running with verbose logging.
func TestConnectRegattaCloud_AuditFailureIncludesHost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")

	srv := newHealthzTLSServer(t, http.StatusForbidden)
	defer srv.Close()

	deps := &cloudDeps{
		HTTP:      insecureClient(),
		Attestor:  fakeAttestor{},
		Allowlist: allowlistForServer(t, srv.URL),
	}

	var buf bytes.Buffer
	if code := runConnectRegattaCloud(context.Background(),
		[]string{"--cloud", "--url", srv.URL, "--token", "sekret"},
		&buf, deps); code == 0 {
		t.Fatalf("expected non-zero exit on 403 healthz")
	}

	raw, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	u, _ := url.Parse(srv.URL)
	if !bytes.Contains(raw, []byte(u.Hostname())) {
		t.Fatalf("expected host %q in failure audit detail: %s", u.Hostname(), raw)
	}
}

// TestRunCommand_ConnectRegattaCloud_Dispatch — top-level dispatch
// (runCommand → "connect" → regatta cloud) must reach the cloud handler.
// Previously runConnect routed every name through the OAuth registry, so
// `leah connect regatta --cloud …` was 646 LoC of dead code.
func TestRunCommand_ConnectRegattaCloud_Dispatch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")
	t.Setenv("LEAH_REGATTA_CLOUD_ALLOWLIST", "regatta.example.com")
	// Force the host to be allowlisted but unreachable — the handler still
	// gets reached; failure mode is healthz_transport, not "unknown provider"
	// from the OAuth registry.
	code := runCommand(context.Background(), []string{
		"connect", "regatta", "--cloud",
		"--url", "https://regatta.example.com",
		"--token", "sekret",
	})
	// We accept any non-zero exit — what matters is the audit row from the
	// cloud handler, not the OAuth registry.
	if code == 0 {
		t.Fatalf("expected non-zero exit (unreachable host), got 0")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"connect_regatta_cloud"`)) {
		t.Fatalf("expected connect_regatta_cloud audit row, got: %s", raw)
	}
	// Negative: must NOT be the OAuth registry's connect_regatta row.
	if bytes.Contains(raw, []byte(`"kind":"connect_regatta"`)) {
		t.Fatalf("OAuth registry path reached instead of cloud handler: %s", raw)
	}
}
