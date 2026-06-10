package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
)

// regatta lives outside the OAuth Provider surface: there is no token, no
// device-code flow. Connect drives the local Docker daemon via OSExec, polls
// /healthz, and persists a mode-file. Implementing Provider keeps it visible
// in `leah connect --list`; authorize() is a tombstone that points callers at
// the dedicated Connect path.

const (
	// RegattaImageDigest pins the regatta image by sha256 digest. Spec §8
	// (threat model: supply-chain swap). Updating this constant is the only
	// supported way to bump the regatta version — :latest is forbidden.
	RegattaImageDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	regattaImageRepo   = "ghcr.io/trilamsr/regatta"
	regattaContainer   = "leah-regatta"
	regattaLoopback    = "127.0.0.1:9090:9090"
	regattaHealthzURL  = "http://127.0.0.1:9090/healthz"
	regattaURL         = "http://127.0.0.1:9090"
)

var (
	// ErrDockerUnavailable signals `docker info` failed — the daemon is
	// missing, stopped, or unreachable. The CLI surfaces a remediation hint;
	// we MUST NOT silently fall back to cloud (paid path without consent).
	ErrDockerUnavailable = errors.New("connect: docker daemon unavailable (install Docker Desktop or `brew install colima && colima start`)")

	// ErrRegattaHealthTimeout fires when the container started but never
	// served a 2xx on /healthz before the budget. Connect tears down the
	// container before returning so no orphan survives the failure.
	ErrRegattaHealthTimeout = errors.New("connect: regatta healthz budget exhausted")

	// ErrRegattaUseConnectRegatta is returned by RegattaProvider.authorize
	// to redirect any code that mistakenly calls connect.Authorize against
	// this provider — regatta does not use OAuth.
	ErrRegattaUseConnectRegatta = errors.New("connect: regatta uses Connect(), not Authorize()")
)

// OSExec mirrors contracts.OSExec but is restated here so this package stays
// free of an internal/contracts import cycle and the connect package owns the
// only seam tests need.
type OSExec interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

// AuditEntry is the local sink shape — distinct from audit.Entry so this
// package does not depend on internal/audit. The CLI translator in
// cmd/leah/connect_regatta.go converts to audit.Entry.
type AuditEntry struct {
	Kind        string
	Success     bool
	ImageDigest string
	Reason      string
}

// AuditSink captures one row per Connect call. nil-safe.
type AuditSink interface {
	Record(AuditEntry)
}

// ModeFile is the on-disk shape at ~/.leah-state/secrets/regatta-mode.json.
// daemon-side auto-detect reads it back to construct the right transport.
type ModeFile struct {
	Mode        string `json:"mode"`
	URL         string `json:"url"`
	Container   string `json:"container"`
	ImageDigest string `json:"image_digest"`
	ConnectedAt string `json:"connected_at"`
}

// RegattaProvider is the Docker-branch connect handler. All side-effecting
// dependencies (Exec, HTTP, time, audit) are fields so tests can drive every
// branch hermetically.
type RegattaProvider struct {
	Exec OSExec
	// HealthzURL defaults to http://127.0.0.1:9090/healthz when empty.
	HealthzURL string
	// HealthzPoll defaults to 250ms (spec §5 step 5).
	HealthzPoll time.Duration
	// HealthBudget defaults to 5s (spec §5 step 5).
	HealthBudget time.Duration
	// HTTPClient defaults to http.DefaultClient.
	HTTPClient *http.Client
	// Audit, if set, receives the success/failure row.
	Audit AuditSink
	// Now, if set, controls the connected_at timestamp.
	Now func() time.Time
}

// NewRegatta returns a RegattaProvider with production defaults. The CLI
// constructs this; tests build the struct literal directly.
func NewRegatta() *RegattaProvider {
	return &RegattaProvider{
		HealthzURL:   regattaHealthzURL,
		HealthzPoll:  250 * time.Millisecond,
		HealthBudget: 5 * time.Second,
	}
}

func (p *RegattaProvider) Name() string      { return "regatta" }
func (p *RegattaProvider) Scopes() []string  { return []string{"connect:regatta"} }
func (p *RegattaProvider) TokenPath() string { return regattaModePath() }

// authorize satisfies Provider so DefaultRegistry can list regatta; the OAuth
// shape is wrong for this transport so we surface a redirect error.
func (p *RegattaProvider) authorize(_ context.Context, _ PromptFn) (*oauth2.Token, error) {
	return nil, ErrRegattaUseConnectRegatta
}

// Connect drives the docker pipeline. Ordering is load-bearing:
//
//  1. attestation → 2. docker info → 3. pull → 4. run → 5. healthz poll →
//  6. mode-file write. Steps 3+ run only after consent. Step 4 success
//     enables a deferred teardown that fires if 5 or 6 fails — no orphan
//     containers, no half-written mode file.
func (p *RegattaProvider) Connect(ctx context.Context, att Attestor) (retErr error) {
	if err := att.Attest(ctx, "connect:regatta"); err != nil {
		p.audit(AuditEntry{Kind: "connect_regatta_docker", Reason: "attestation_denied"})
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}

	if _, _, err := p.Exec.Run(ctx, "docker", "info"); err != nil {
		p.audit(AuditEntry{Kind: "connect_regatta_docker", Reason: "docker_unavailable"})
		return ErrDockerUnavailable
	}

	image := regattaImageRepo + "@" + RegattaImageDigest
	if _, _, err := p.Exec.Run(ctx, "docker", "pull", image); err != nil {
		p.audit(AuditEntry{Kind: "connect_regatta_docker", Reason: "pull_failed"})
		return fmt.Errorf("connect: docker pull: %w", err)
	}

	dataDir, err := p.ensureDataDir()
	if err != nil {
		p.audit(AuditEntry{Kind: "connect_regatta_docker", Reason: "data_dir_failed"})
		return err
	}

	runArgs := []string{
		"run", "-d",
		"--name", regattaContainer,
		"-p", regattaLoopback,
		"-v", dataDir + ":/data",
		image,
	}
	if _, _, err := p.Exec.Run(ctx, "docker", runArgs...); err != nil {
		p.audit(AuditEntry{Kind: "connect_regatta_docker", Reason: "run_failed"})
		return fmt.Errorf("connect: docker run: %w", err)
	}

	// Deferred teardown — fires only if a step after `run` fails. Once the
	// happy path returns nil, we clear the closure so success doesn't tear
	// down a healthy container.
	teardown := func() {
		// Best-effort. The container may already be stopped; we still want
		// `rm` to run so a half-running state cannot survive.
		_, _, _ = p.Exec.Run(context.Background(), "docker", "stop", regattaContainer)
		_, _, _ = p.Exec.Run(context.Background(), "docker", "rm", regattaContainer)
	}
	defer func() {
		if retErr != nil && teardown != nil {
			teardown()
		}
	}()

	if err := p.waitHealthy(ctx); err != nil {
		p.audit(AuditEntry{Kind: "connect_regatta_docker", Reason: "health_timeout"})
		return err
	}

	if err := p.writeModeFile(); err != nil {
		p.audit(AuditEntry{Kind: "connect_regatta_docker", Reason: "mode_file_failed"})
		return err
	}

	teardown = nil // success — disarm the deferred teardown
	p.audit(AuditEntry{
		Kind:        "connect_regatta_docker",
		Success:     true,
		ImageDigest: RegattaImageDigest,
	})
	return nil
}

func (p *RegattaProvider) waitHealthy(ctx context.Context) error {
	poll := p.HealthzPoll
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	budget := p.HealthBudget
	if budget <= 0 {
		budget = 5 * time.Second
	}
	url := p.HealthzURL
	if url == "" {
		url = regattaHealthzURL
	}
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	deadline := time.Now().Add(budget)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return ErrRegattaHealthTimeout
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (p *RegattaProvider) ensureDataDir() (string, error) {
	d := stateDirEnv()
	dataDir := filepath.Join(d, "regatta-data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("connect: mkdir regatta-data: %w", err)
	}
	if err := os.Chmod(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("connect: chmod regatta-data: %w", err)
	}
	return dataDir, nil
}

func (p *RegattaProvider) writeModeFile() error {
	path := regattaModePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("connect: mkdir secrets: %w", err)
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	doc := ModeFile{
		Mode:        "docker",
		URL:         regattaURL,
		Container:   regattaContainer,
		ImageDigest: RegattaImageDigest,
		ConnectedAt: now().UTC().Format(time.RFC3339),
	}
	buf, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("connect: marshal mode: %w", err)
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		return fmt.Errorf("connect: write mode: %w", err)
	}
	// Re-assert 0600 in case the file pre-existed world-readable. Mirrors
	// WriteToken's defense against a hostile umask / world-readable seed.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("connect: chmod mode: %w", err)
	}
	return nil
}

func (p *RegattaProvider) audit(e AuditEntry) {
	if p.Audit == nil {
		return
	}
	p.Audit.Record(e)
}

func regattaModePath() string {
	return filepath.Join(stateDirEnv(), "secrets", "regatta-mode.json")
}

func stateDirEnv() string {
	if d := os.Getenv("LEAH_STATE_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".leah-state")
}
