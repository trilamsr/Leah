// Package shortcuts wraps the macOS `shortcuts` CLI: attestation-gated List (read) and Run (action).
package shortcuts

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/trilam/leah/internal/contracts"
)

// Separate scopes so the operator can grant discovery without granting execution.
const (
	ScopeList = "macos:shortcuts:list"
	ScopeRun  = "macos:shortcuts:run"
)

var (
	ErrAttestationDenied = errors.New("shortcuts: attestation denied")
	ErrSourceUnavailable = errors.New("shortcuts: shortcuts CLI unavailable")
	ErrInvalidName       = errors.New("shortcuts: invalid name")
	ErrNotFound          = errors.New("shortcuts: no such shortcut")
)

type Config struct {
	Attestor contracts.Attestor
	Exec     contracts.OSExec
	Bin      string
}

type Shortcuts struct {
	att contracts.Attestor
	ex  contracts.OSExec
	bin string
}

func New(cfg Config) (*Shortcuts, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("shortcuts: Config.Attestor required")
	}
	if cfg.Exec == nil {
		return nil, errors.New("shortcuts: Config.Exec required")
	}
	bin := cfg.Bin
	if bin == "" {
		bin = "shortcuts"
	}
	return &Shortcuts{att: cfg.Attestor, ex: cfg.Exec, bin: bin}, nil
}

func (s *Shortcuts) Name() string { return "Shortcuts" }

func (s *Shortcuts) Available(_ context.Context) bool {
	_, err := exec.LookPath(s.bin)
	return err == nil
}

func (s *Shortcuts) List(ctx context.Context) ([]string, error) {
	if err := s.att.Attest(ctx, ScopeList); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	stdout, _, err := s.ex.Run(ctx, s.bin, "list")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	return parseNames(stdout), nil
}

// Run is a side effect: ScopeRun gates it and denial short-circuits before the subprocess.
func (s *Shortcuts) Run(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidName
	}
	if err := s.att.Attest(ctx, ScopeRun); err != nil {
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	_, stderr, err := s.ex.Run(ctx, s.bin, "run", name)
	if err != nil {
		if isNotFound(stderr) {
			return fmt.Errorf("%w: %q", ErrNotFound, name)
		}
		return fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	return nil
}

func isNotFound(stderr []byte) bool {
	return strings.Contains(string(stderr), "No shortcut named")
}

func parseNames(stdout []byte) []string {
	var out []string
	for _, line := range strings.Split(string(stdout), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			out = append(out, name)
		}
	}
	return out
}
