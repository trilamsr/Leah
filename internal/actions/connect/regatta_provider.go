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

	"github.com/trilam/leah/internal/contracts"
)

const (
	// :latest is forbidden; bumping regatta = editing this constant. Spec §8.
	RegattaImageDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	regattaImageRepo   = "ghcr.io/trilamsr/regatta"
	regattaContainer   = "leah-regatta"
	regattaLoopback    = "127.0.0.1:9090:9090"
	regattaHealthzURL  = "http://127.0.0.1:9090/healthz"
	regattaURL         = "http://127.0.0.1:9090"
)

var (
	ErrDockerUnavailable        = errors.New("connect: docker daemon unavailable (install Docker Desktop or `brew install colima && colima start`)")
	ErrRegattaHealthTimeout     = errors.New("connect: regatta healthz budget exhausted")
	ErrRegattaUseConnectRegatta = errors.New("connect: regatta uses Connect(), not Authorize()")
)

type AuditEntry struct {
	Kind        string
	Success     bool
	ImageDigest string
	Reason      string
}

type AuditSink interface {
	Record(AuditEntry)
}

type ModeFile struct {
	Mode        string `json:"mode"`
	URL         string `json:"url"`
	Container   string `json:"container"`
	ImageDigest string `json:"image_digest"`
	ConnectedAt string `json:"connected_at"`
}

type RegattaProvider struct {
	Exec         contracts.OSExec
	HealthzURL   string
	HealthzPoll  time.Duration
	HealthBudget time.Duration
	HTTPClient   *http.Client
	Audit        AuditSink
	Now          func() time.Time
}

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

// authorize satisfies Provider for --list discoverability; regatta is not OAuth.
func (p *RegattaProvider) authorize(_ context.Context, _ PromptFn) (*oauth2.Token, error) {
	return nil, ErrRegattaUseConnectRegatta
}

// Connect ordering is load-bearing: attest → info → pull → run → healthz → mode-file.
// A success after `run` arms a deferred teardown; later failures cannot leak a leah-regatta.
func (p *RegattaProvider) Connect(ctx context.Context, att contracts.Attestor) (retErr error) {
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
	teardown := func() {
		// Bounded teardown — parent ctx is already cancelled by this point,
		// so we can't inherit it. 15s upper bound: Docker's own stop-grace
		// is typically 10s + SIGKILL; we add 5s slack to absorb cleanup.
		tctx, tcancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer tcancel()
		_, _, _ = p.Exec.Run(tctx, "docker", "stop", regattaContainer)
		_, _, _ = p.Exec.Run(tctx, "docker", "rm", regattaContainer)
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
	teardown = nil
	p.audit(AuditEntry{
		Kind:        "connect_regatta_docker",
		Success:     true,
		ImageDigest: RegattaImageDigest,
	})
	return nil
}

// Revoke is the Disconnect-side inverse of Connect's `docker run`: it tears
// down the pinned container. Errors are swallowed so an already-stopped or
// already-removed container disconnects cleanly (idempotent).
func (p *RegattaProvider) Revoke(ctx context.Context) error {
	_, _, _ = p.Exec.Run(ctx, "docker", "stop", regattaContainer)
	_, _, _ = p.Exec.Run(ctx, "docker", "rm", regattaContainer)
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
		client = &http.Client{Timeout: 2 * time.Second}
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
	// Re-assert 0600 against a hostile pre-existing world-readable file.
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
