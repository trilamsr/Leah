// Package bluetooth reads macOS Bluetooth power + connected devices via system_profiler.
package bluetooth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/trilam/leah/internal/contracts"
)

const Scope = "macos:bluetooth:query"

const dataType = "SPBluetoothDataType"

var (
	ErrAttestationDenied = errors.New("bluetooth: attestation denied")
	ErrSourceUnavailable = errors.New("bluetooth: system_profiler unavailable")
	ErrPermissionDenied  = errors.New("bluetooth: permission denied")
)

type State struct {
	Powered   bool
	Connected []string
}

type Config struct {
	Attestor contracts.Attestor
	Exec     contracts.OSExec
	Bin      string
}

type Bluetooth struct {
	att contracts.Attestor
	ex  contracts.OSExec
	bin string
}

func New(cfg Config) (*Bluetooth, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("bluetooth: Config.Attestor required")
	}
	if cfg.Exec == nil {
		return nil, errors.New("bluetooth: Config.Exec required")
	}
	bin := cfg.Bin
	if bin == "" {
		bin = "system_profiler"
	}
	return &Bluetooth{att: cfg.Attestor, ex: cfg.Exec, bin: bin}, nil
}

func (b *Bluetooth) Name() string { return "Bluetooth" }

func (b *Bluetooth) Available(ctx context.Context) bool {
	_, _, err := b.ex.Run(ctx, b.bin, dataType)
	return err == nil
}

func (b *Bluetooth) Query(ctx context.Context) (State, error) {
	if err := b.att.Attest(ctx, Scope); err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	stdout, stderr, err := b.ex.Run(ctx, b.bin, dataType)
	if err != nil {
		if isPermissionDenied(stderr) {
			return State{}, fmt.Errorf("%w: %v", ErrPermissionDenied, err)
		}
		return State{}, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	return parseProfiler(string(stdout)), nil
}

func isPermissionDenied(stderr []byte) bool {
	s := string(stderr)
	return strings.Contains(s, "-1743") || strings.Contains(s, "Not authorized")
}

func parseProfiler(out string) State {
	var s State
	inConnected := false
	connectedIndent := 0
	for _, raw := range strings.Split(out, "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		line := strings.TrimSpace(raw)

		if strings.HasPrefix(line, "State:") {
			s.Powered = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "State:")), "On")
			continue
		}
		if line == "Connected:" {
			inConnected = true
			connectedIndent = indent
			continue
		}
		if inConnected {
			if indent <= connectedIndent {
				inConnected = false
				continue
			}
			if indent == connectedIndent+4 && strings.HasSuffix(line, ":") {
				s.Connected = append(s.Connected, strings.TrimSuffix(line, ":"))
			}
		}
	}
	if !s.Powered {
		s.Connected = nil
	}
	return s
}
