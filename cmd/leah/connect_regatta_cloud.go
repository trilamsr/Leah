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

// cloudModeFile is the on-disk pointer at $LEAH_STATE_DIR/secrets/regatta-mode.json.
// token_path holds the SEPARATE 0600 file holding the bearer — never inline the
// token bytes here (spec §8: token leak surface is wider for the mode file).
type cloudModeFile struct {
	Mode        string `json:"mode"`
	URL         string `json:"url"`
	TokenPath   string `json:"token_path"`
	ConnectedAt string `json:"connected_at,omitempty"`
}

type cloudTokenFile struct {
	Token string `json:"token"`
}

// cloudDeps lets tests inject HTTP, Attestor, prompt and clock. Production
// passes nil + a constructed default in the runner.
type cloudDeps struct {
	HTTP       contracts.HTTPClient
	Attestor   contracts.Attestor
	Now        func() time.Time
	ConfirmFn  func() bool             // cost-implication confirmation; nil = auto-yes when LEAH_CONNECT_AUTO_ATTEST=1
	Allowlist  []string                // host allowlist; empty = derived from env
}

// runConnectRegattaCloud handles `leah connect regatta --cloud --url <u> --token <t>`.
// Ordering is load-bearing — attest → URL/allowlist → confirm → healthz → token write
// → mode-file write → audit. Any failure short-circuits without writing the token.
func runConnectRegattaCloud(ctx context.Context, args []string, w io.Writer, deps *cloudDeps) int {
	fs := flag.NewFlagSet("connect regatta --cloud", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	urlFlag := fs.String("url", "", "cloud regatta base URL (https://...)")
	tokenFlag := fs.String("token", "", "bearer token")
	// strip the leading --cloud that the dispatcher leaves in args
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

	a := &audit.Logger{Path: filepath.Join(stateDir(), "audit.jsonl"), DefaultWorkspace: activeWorkspace}

	// 1. Attest. Higher-risk scope names the cost implication explicitly.
	if err := deps.Attestor.Attest(ctx, "connect:regatta:cloud"); err != nil {
		_ = a.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "attestation_denied",
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: %v\n", err)
		return 1
	}

	// 2. Validate URL: parseable + HTTPS + allowlisted host.
	u, err := url.Parse(*urlFlag)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		_ = a.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "url_not_https",
		})
		_, _ = fmt.Fprintln(os.Stderr, "leah connect regatta --cloud: --url must be a valid https:// URL")
		return 1
	}
	allow := deps.Allowlist
	if len(allow) == 0 {
		allow = cloudAllowlistFromEnv()
	}
	if !hostAllowed(u.Hostname(), allow) {
		_ = a.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "host_not_allowlisted",
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: host %q not in LEAH_REGATTA_CLOUD_ALLOWLIST\n", u.Hostname())
		return 1
	}

	// 3. Cost-implication confirmation. Auto-attest env also auto-confirms so
	//    tests don't need a TTY; interactive runs require an explicit "y".
	if !confirmCost(w, deps.ConfirmFn) {
		_ = a.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "cost_declined",
		})
		_, _ = fmt.Fprintln(os.Stderr, "leah connect regatta --cloud: declined cost prompt")
		return 1
	}

	// 4. Single GET /healthz with Bearer token. 2xx wins.
	healthURL := strings.TrimRight(*urlFlag, "/") + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		_ = a.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "request_build_failed",
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: %v\n", err)
		return 1
	}
	req.Header.Set("Authorization", "Bearer "+*tokenFlag)
	resp, err := deps.HTTP.Do(req)
	if err != nil {
		_ = a.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "healthz_transport",
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: healthz: %v\n", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = a.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      fmt.Sprintf("healthz_status_%d", resp.StatusCode),
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: healthz returned %d\n", resp.StatusCode)
		return 1
	}

	// 5. Write token + mode files. Token in a separate 0600 file; mode file
	//    holds only a path pointer.
	secretsDir := filepath.Join(stateDir(), "secrets")
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		_ = a.Append(audit.Entry{
			Kind:        "connect_regatta_cloud",
			BlastRadius: 2,
			Outcome:     "failed",
			Detail:      "secrets_mkdir_failed",
		})
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: %v\n", err)
		return 1
	}
	tokenPath := filepath.Join(secretsDir, "regatta-token.json")
	tokBuf, err := json.MarshalIndent(cloudTokenFile{Token: *tokenFlag}, "", "  ")
	if err != nil {
		return 1
	}
	if err := os.WriteFile(tokenPath, tokBuf, 0o600); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: write token: %v\n", err)
		return 1
	}
	if err := os.Chmod(tokenPath, 0o600); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: chmod token: %v\n", err)
		return 1
	}
	modePath := filepath.Join(secretsDir, "regatta-mode.json")
	modeBuf, err := json.MarshalIndent(cloudModeFile{
		Mode:        "cloud",
		URL:         *urlFlag,
		TokenPath:   tokenPath,
		ConnectedAt: deps.Now().Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return 1
	}
	if err := os.WriteFile(modePath, modeBuf, 0o600); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: write mode: %v\n", err)
		return 1
	}
	if err := os.Chmod(modePath, 0o600); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah connect regatta --cloud: chmod mode: %v\n", err)
		return 1
	}

	// 6. Success audit row — host only, never the token, never the full URL
	//    (a stray ?token=… in the future would leak otherwise).
	_ = a.Append(audit.Entry{
		Kind:        "connect_regatta_cloud",
		BlastRadius: 2,
		Outcome:     "success",
		Detail:      u.Hostname(),
	})
	_, _ = fmt.Fprintf(w, "ok: regatta cloud connected (host: %s)\n", u.Hostname())
	return 0
}

// cloudAllowlistFromEnv parses LEAH_REGATTA_CLOUD_ALLOWLIST (comma-separated).
// Empty env returns nil → hostAllowed treats nil as "allow any host" so
// operators with their own private cloud do not need to enumerate. Operators
// who want pinned hosts set the env explicitly.
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

// confirmCost shows the cost-implication prompt before any token write. The
// auto-attest env shortcut keeps test runs hermetic; without it, the operator
// must type "y".
func confirmCost(w io.Writer, fn func() bool) bool {
	if fn != nil {
		return fn()
	}
	if os.Getenv("LEAH_CONNECT_AUTO_ATTEST") == "1" {
		return true
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
