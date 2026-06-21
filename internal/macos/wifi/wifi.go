// Package wifi reads the current macOS Wi-Fi SSID via networksetup.
package wifi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/trilam/leah/internal/contracts"
)

const Scope = "macos:wifi:query"

const iface = "en0"

const ssidPrefix = "Current Wi-Fi Network: "

var (
	ErrAttestationDenied = errors.New("wifi: attestation denied")
	ErrSourceUnavailable = errors.New("wifi: networksetup unavailable")
	ErrPermissionDenied  = errors.New("wifi: permission denied")
)

type State struct {
	Connected bool
	SSID      string
}

type Config struct {
	Attestor contracts.Attestor
	Exec     contracts.OSExec
	Bin      string
	Iface    string
}

type WiFi struct {
	att   contracts.Attestor
	ex    contracts.OSExec
	bin   string
	iface string
}

func New(cfg Config) (*WiFi, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("wifi: Config.Attestor required")
	}
	if cfg.Exec == nil {
		return nil, errors.New("wifi: Config.Exec required")
	}
	bin := cfg.Bin
	if bin == "" {
		bin = "networksetup"
	}
	ifc := cfg.Iface
	if ifc == "" {
		ifc = iface
	}
	return &WiFi{att: cfg.Attestor, ex: cfg.Exec, bin: bin, iface: ifc}, nil
}

func (w *WiFi) Name() string { return "Wi-Fi" }

func (w *WiFi) Available(ctx context.Context) bool {
	_, _, err := w.ex.Run(ctx, w.bin, "-getairportnetwork", w.iface)
	return err == nil
}

func (w *WiFi) Query(ctx context.Context) (State, error) {
	if err := w.att.Attest(ctx, Scope); err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	stdout, stderr, err := w.ex.Run(ctx, w.bin, "-getairportnetwork", w.iface)
	if err != nil {
		if isPermissionDenied(stderr) {
			return State{}, fmt.Errorf("%w: %v", ErrPermissionDenied, err)
		}
		return State{}, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	return parseNetwork(string(stdout)), nil
}

func isPermissionDenied(stderr []byte) bool {
	s := string(stderr)
	return strings.Contains(s, "-1743") || strings.Contains(s, "Not authorized")
}

func parseNetwork(out string) State {
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, ssidPrefix) {
			ssid := strings.TrimSpace(strings.TrimPrefix(line, ssidPrefix))
			if ssid == "" {
				return State{}
			}
			return State{Connected: true, SSID: ssid}
		}
	}
	return State{}
}
