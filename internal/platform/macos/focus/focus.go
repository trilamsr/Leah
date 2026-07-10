// Package focus reads macOS Focus / Do-Not-Disturb state via `defaults read`.
package focus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/trilam/leah/internal/contracts"
)

const Scope = "macos:focus:query"

// User-defaults plist drives the prefs UI; the on-disk .db is per-assertion only.
const defaultsDomain = "com.apple.donotdisturb.prefs"

var (
	ErrAttestationDenied = errors.New("focus: attestation denied")
	ErrSourceUnavailable = errors.New("focus: defaults unavailable")
	ErrPermissionDenied  = errors.New("focus: permission denied")
)

type State struct {
	Active      bool
	Mode        string
	ActiveSince time.Time
}

type Config struct {
	Attestor contracts.Attestor
	Exec     contracts.OSExec
	Bin      string
}

type Focus struct {
	att contracts.Attestor
	ex  contracts.OSExec
	bin string
}

func New(cfg Config) (*Focus, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("focus: Config.Attestor required")
	}
	if cfg.Exec == nil {
		return nil, errors.New("focus: Config.Exec required")
	}
	bin := cfg.Bin
	if bin == "" {
		bin = "defaults"
	}
	return &Focus{att: cfg.Attestor, ex: cfg.Exec, bin: bin}, nil
}

func (f *Focus) Name() string { return "Focus" }

// Failure here usually means Focus was never enabled — not a system fault.
func (f *Focus) Available(ctx context.Context) bool {
	_, _, err := f.ex.Run(ctx, f.bin, "read", defaultsDomain)
	return err == nil
}

func (f *Focus) Query(ctx context.Context) (State, error) {
	if err := f.att.Attest(ctx, Scope); err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	stdout, stderr, err := f.ex.Run(ctx, f.bin, "read", defaultsDomain)
	if err != nil {
		if isPermissionDenied(stderr) {
			return State{}, fmt.Errorf("%w: %v", ErrPermissionDenied, err)
		}
		return State{}, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	return parseDefaults(string(stdout)), nil
}

func isPermissionDenied(stderr []byte) bool {
	s := string(stderr)
	return strings.Contains(s, "-1743") || strings.Contains(s, "Not authorized")
}

// Extra keys are ignored — Apple adds them every release.
func parseDefaults(out string) State {
	var s State
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimRight(line, ";")
		k, v, ok := splitKV(line)
		if !ok {
			continue
		}
		switch k {
		case "active":
			s.Active = v == "1" || strings.EqualFold(v, "true")
		case "mode":
			s.Mode = strings.Trim(v, `"`)
		case "dndStart":
			if t, err := time.Parse(`"2006-01-02 15:04:05 -0700"`, v); err == nil {
				s.ActiveSince = t
			}
		}
	}
	return s
}

func splitKV(line string) (string, string, bool) {
	i := strings.Index(line, "=")
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}
