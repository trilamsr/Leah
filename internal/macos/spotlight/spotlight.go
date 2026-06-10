// Package spotlight wraps macOS mdfind as an attestation-gated Spotlight signal.
package spotlight

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/trilam/leah/internal/contracts"
)

const Scope = "macos:spotlight:query"

var (
	ErrAttestationDenied = errors.New("spotlight: attestation denied")
	ErrSourceUnavailable = errors.New("spotlight: mdfind unavailable")
	ErrInvalidQuery      = errors.New("spotlight: invalid query")
	ErrPermissionDenied  = errors.New("spotlight: permission denied")
)

type Item struct {
	URL   string
	Title string
	Date  time.Time
}

type Query struct {
	Text  string
	Limit int
}

type Config struct {
	Attestor contracts.Attestor
	Exec     contracts.OSExec
	Bin      string
}

type Spotlight struct {
	att contracts.Attestor
	ex  contracts.OSExec
	bin string
}

func New(cfg Config) (*Spotlight, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("spotlight: Config.Attestor required")
	}
	if cfg.Exec == nil {
		return nil, errors.New("spotlight: Config.Exec required")
	}
	bin := cfg.Bin
	if bin == "" {
		bin = "mdfind"
	}
	return &Spotlight{att: cfg.Attestor, ex: cfg.Exec, bin: bin}, nil
}

func (s *Spotlight) Name() string { return "Spotlight" }

// `mdfind -version` exits non-zero on macOS so LookPath is the load-bearing probe.
func (s *Spotlight) Available(ctx context.Context) bool {
	_, err := exec.LookPath(s.bin)
	return err == nil
}

// Empty Text rejected — mdfind without a query returns the whole disk.
func (s *Spotlight) Query(ctx context.Context, q Query) ([]Item, error) {
	if strings.TrimSpace(q.Text) == "" {
		return nil, ErrInvalidQuery
	}
	if err := s.att.Attest(ctx, Scope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	stdout, stderr, err := s.ex.Run(ctx, s.bin, "-0", q.Text)
	if err != nil {
		if isPermissionDenied(stderr) {
			return nil, fmt.Errorf("%w: %v", ErrPermissionDenied, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	return parsePaths(stdout, q.Limit), nil
}

func isPermissionDenied(stderr []byte) bool {
	s := string(stderr)
	return strings.Contains(s, "-1743") || strings.Contains(s, "Not authorized")
}

// NUL split is load-bearing: paths legitimately contain spaces and newlines.
func parsePaths(stdout []byte, limit int) []Item {
	if len(stdout) == 0 {
		return nil
	}
	parts := strings.Split(strings.TrimRight(string(stdout), "\x00"), "\x00")
	var out []Item
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, Item{URL: "file://" + p, Title: baseName(p)})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func baseName(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return p
	}
	return p[i+1:]
}
