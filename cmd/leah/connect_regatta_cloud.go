package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/contracts"
)

// cloudModeFile holds a path pointer to the token file — never the token bytes.
type cloudModeFile struct {
	Mode        string `json:"mode"`
	URL         string `json:"url"`
	TokenPath   string `json:"token_path"`
	ConnectedAt string `json:"connected_at,omitempty"`
}

type cloudTokenFile struct {
	Token string `json:"token"`
}

type cloudDeps struct {
	HTTP      contracts.HTTPClient
	Attestor  contracts.Attestor
	Now       func() time.Time
	ConfirmFn func() bool
	Allowlist []string
}

// runConnectRegattaCloud — ordering is load-bearing: attest → URL/allowlist →
// confirm → healthz → token+mode write → audit. Any failure short-circuits
// before token bytes touch disk.
func runConnectRegattaCloud(ctx context.Context, args []string, w io.Writer, deps *cloudDeps) int {
	fs := flag.NewFlagSet("connect regatta --cloud", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	urlFlag := fs.String("url", "", "cloud regatta base URL (https://...)")
	tokenFlag := fs.String("token", "", "bearer token")
	stripped := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--cloud" {
			continue
		}
		stripped = append(stripped, a)
	}
	if err := fs.Parse(stripped); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: %v\n", err)
		return 2
	}
	if *urlFlag == "" || *tokenFlag == "" {
		_, _ = fmt.Fprintln(os.Stderr, "leah connect regatta --cloud: --url and --token are required")
		return 2
	}

	if deps == nil {
		deps = &cloudDeps{}
	}
	if deps.HTTP == nil {
		deps.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	if deps.Attestor == nil {
		deps.Attestor = newConnectAttestor()
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Now().UTC() }
	}

	logger := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}

	if err := deps.Attestor.Attest(ctx, "connect:regatta:cloud"); err != nil {
		_ = logger.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "attestation_denied",
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: %v\n", err)
		return 1
	}

	u, err := url.Parse(*urlFlag)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		_ = logger.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "url_not_https",
		})
		_, _ = fmt.Fprintln(os.Stderr, "leah connect regatta --cloud: --url must be a valid https:// URL")
		return 1
	}
	// userinfo would land credentials in the 0o600 mode file AND ship
	// Basic + Bearer simultaneously on every healthz call.
	if u.User != nil {
		_ = logger.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "url_has_userinfo",
		})
		_, _ = fmt.Fprintln(os.Stderr, "leah connect regatta --cloud: --url must not include userinfo (user:pass@)")
		return 1
	}
	allow := deps.Allowlist
	if len(allow) == 0 {
		allow = cloudAllowlistFromEnv()
	}
	if !hostAllowed(u.Hostname(), allow) {
		_ = logger.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "host_not_allowlisted:" + u.Hostname(),
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: host %q not in cloud allowlist\n", u.Hostname())
		return 1
	}

	canonURL := canonicalCloudURL(u)

	if !confirmCost(w, deps.ConfirmFn) {
		_ = logger.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "cost_declined:" + u.Hostname(),
		})
		_, _ = fmt.Fprintln(os.Stderr, "leah connect regatta --cloud: declined cost prompt")
		return 1
	}

	healthURL := u.JoinPath("healthz")
	healthURL.RawQuery = ""
	healthURL.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL.String(), nil)
	if err != nil {
		_ = logger.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "request_build_failed:" + u.Hostname(),
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: %v\n", err)
		return 1
	}
	req.Header.Set("Authorization", "Bearer "+*tokenFlag)
	resp, err := deps.HTTP.Do(req)
	if err != nil {
		_ = logger.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "healthz_transport:" + u.Hostname(),
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: healthz: %v\n", err)
		return 1
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = logger.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      fmt.Sprintf("healthz_status_%d:%s", resp.StatusCode, u.Hostname()),
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: healthz returned %d\n", resp.StatusCode)
		return 1
	}

	secretsDir := filepath.Join(stateDir(), "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		_ = logger.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "secrets_mkdir_failed:" + u.Hostname(),
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: %v\n", err)
		return 1
	}
	tokenPath := filepath.Join(secretsDir, "regatta-token.json")
	if err := writeJSONFile0600(tokenPath, cloudTokenFile{Token: *tokenFlag}); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: write token: %v\n", err)
		return 1
	}
	modePath := filepath.Join(secretsDir, "regatta-mode.json")
	if err := writeJSONFile0600(modePath, cloudModeFile{
		Mode:        "cloud",
		URL:         canonURL,
		TokenPath:   tokenPath,
		ConnectedAt: deps.Now().Format(time.RFC3339),
	}); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: write mode: %v\n", err)
		return 1
	}

	_ = logger.Append(audit.Entry{
		Kind:        "connect_regatta_cloud",
		BlastRadius: 2,
		Outcome:     "success",
		Detail:      u.Hostname(),
	})
	_, _ = fmt.Fprintf(w, "ok: regatta cloud connected (host: %s)\n", u.Hostname())
	return 0
}

// canonicalCloudURL strips userinfo / query / fragment so the mode file
// never persists credentials or stray ?token=… remnants.
func canonicalCloudURL(u *url.URL) string {
	return u.Scheme + "://" + u.Host + u.Path
}

// writeJSONFile0600 atomically lands v at path with 0o600 (tmp + rename).
// MarshalIndent of the local string-only structs is structurally infallible;
// the panic flags a future field type slipping in.
func writeJSONFile0600(path string, v any) error {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("unreachable: marshal %T: %v", v, err))
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func cloudAllowlistFromEnv() []string {
	raw := os.Getenv("LEAH_REGATTA_CLOUD_ALLOWLIST")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hostAllowed(host string, allow []string) bool {
	if len(allow) == 0 {
		return true
	}
	for _, a := range allow {
		if a == host {
			return true
		}
	}
	return false
}

// confirmCost declines safely under LEAH_NONINTERACTIVE=1 so background
// callers (cron, hooks) can never hang on stdin.
func confirmCost(w io.Writer, fn func() bool) bool {
	if fn != nil {
		return fn()
	}
	if os.Getenv("LEAH_CONNECT_AUTO_ATTEST") == "1" {
		return true
	}
	if os.Getenv("LEAH_NONINTERACTIVE") == "1" {
		return false
	}
	_, _ = fmt.Fprint(w, "Cloud regatta is billed per use. Proceed? [y/N] ")
	var resp string
	_, _ = fmt.Fscanln(os.Stdin, &resp)
	switch resp {
	case "y", "Y", "yes", "YES":
		return true
	}
	return false
}
