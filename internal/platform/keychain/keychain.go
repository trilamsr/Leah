// Package keychain bridges Go secret reads to the macOS Keychain slot the
// Swift wizard writes to. Wraps `security(1)` so no CGO/Security.framework
// dependency lands in the Go build.
package keychain

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// securityBin is overridable in tests via SetSecurityBin — production always
// resolves to /usr/bin/security through PATH.
var securityBin = "security"

// SetSecurityBin swaps the binary path (test seam). Returns the previous
// value so callers can restore via defer.
func SetSecurityBin(path string) string {
	prev := securityBin
	securityBin = path
	return prev
}

// Load reads the generic-password entry for (service, account). Returns
// ("", nil) when the entry is absent — that is not an error condition, callers
// fall back to env or degrade gracefully.
func Load(service, account string) (string, error) {
	if service == "" || account == "" {
		return "", errors.New("keychain: service and account required")
	}
	cmd := exec.Command(securityBin, "find-generic-password", "-s", service, "-a", account, "-w")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		stderr := errBuf.String()
		if strings.Contains(stderr, "could not be found") || strings.Contains(stderr, "The specified item could not be found") {
			return "", nil
		}
		return "", fmt.Errorf("keychain load %s/%s: %w: %s", service, account, err, strings.TrimSpace(stderr))
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// Save upserts (service, account) → secret. security(1) add-generic-password
// -U updates in place when the item exists so the Swift wizard's slot stays
// authoritative even if leah CLI writes after wizard-write.
func Save(service, account, secret string) error {
	if service == "" || account == "" {
		return errors.New("keychain: service and account required")
	}
	cmd := exec.Command(securityBin, "add-generic-password", "-s", service, "-a", account, "-w", secret, "-U")
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keychain save %s/%s: %w: %s", service, account, err, strings.TrimSpace(errBuf.String()))
	}
	return nil
}

// Delete removes (service, account). Returns nil when the entry was already
// absent — idempotent for the disconnect path.
func Delete(service, account string) error {
	if service == "" || account == "" {
		return errors.New("keychain: service and account required")
	}
	cmd := exec.Command(securityBin, "delete-generic-password", "-s", service, "-a", account)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		stderr := errBuf.String()
		if strings.Contains(stderr, "could not be found") || strings.Contains(stderr, "The specified item could not be found") {
			return nil
		}
		return fmt.Errorf("keychain delete %s/%s: %w: %s", service, account, err, strings.TrimSpace(stderr))
	}
	return nil
}
