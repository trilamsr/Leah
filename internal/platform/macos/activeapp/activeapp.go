// Package activeapp reads the macOS foreground application via osascript.
package activeapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/trilam/leah/internal/platform/contracts"
)

const Scope = "macos:activeapp:query"

const redactedLabel = "(redacted)"

// DefaultBlocklist is the operator-trust default for foreground-app
// observation. Prefix match — applies to Query() redaction and to the push
// source (osevent.Config.Blocklist), so the same privacy floor holds whether
// the active app is sampled pull-style or pushed via NSWorkspace.
var DefaultBlocklist = []string{
	"com.bank.",
	"com.chase.",
	"com.apple.Passwords",
	"com.1password.",
	"com.agilebits.onepassword",
}

const script = `tell application "System Events"
	set frontApp to first application process whose frontmost is true
	set appName to name of frontApp
	set appBundle to bundle identifier of frontApp
end tell
return appName & "|" & appBundle`

var (
	ErrAttestationDenied = errors.New("activeapp: attestation denied")
	ErrSourceUnavailable = errors.New("activeapp: osascript unavailable")
	ErrParseFailed       = errors.New("activeapp: parse failed")
	ErrPermissionDenied  = errors.New("activeapp: permission denied")
)

type Item struct {
	Name     string
	BundleID string
	Redacted bool
}

type Config struct {
	Attestor  contracts.Attestor
	Exec      contracts.OSExec
	Blocklist []string
	Bin       string
}

type ActiveApp struct {
	att   contracts.Attestor
	ex    contracts.OSExec
	block []string
	bin   string
}

func New(config Config) (*ActiveApp, error) {
	if config.Attestor == nil {
		return nil, errors.New("activeapp: Config.Attestor required")
	}
	if config.Exec == nil {
		return nil, errors.New("activeapp: Config.Exec required")
	}
	bin := config.Bin
	if bin == "" {
		bin = "osascript"
	}
	return &ActiveApp{
		att:   config.Attestor,
		ex:    config.Exec,
		block: append([]string(nil), config.Blocklist...),
		bin:   bin,
	}, nil
}

func (a *ActiveApp) Name() string { return "ActiveApp" }

func (a *ActiveApp) Available(ctx context.Context) bool {
	_, _, err := a.ex.Run(ctx, a.bin, "-e", "return 1")
	return err == nil
}

// Query is DEPRECATED for hot-path use; prefer PushSource via osevent. Kept
// for caller-driven inspection, tests, and cold start before the first push
// event arrives.
func (a *ActiveApp) Query(ctx context.Context) (Item, error) {
	if err := a.att.Attest(ctx, Scope); err != nil {
		return Item{}, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	stdout, stderr, err := a.ex.Run(ctx, a.bin, "-e", script)
	if err != nil {
		if isPermissionDenied(stderr) {
			return Item{}, fmt.Errorf("%w: %v", ErrPermissionDenied, err)
		}
		return Item{}, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	name, bundle, ok := parse(string(stdout))
	if !ok {
		return Item{}, ErrParseFailed
	}
	if a.blocked(bundle) {
		return Item{Name: redactedLabel, BundleID: redactedLabel, Redacted: true}, nil
	}
	return Item{Name: name, BundleID: bundle}, nil
}

// LastIndex split: app display names may contain `|`; bundle IDs never do.
func parse(out string) (name, bundle string, ok bool) {
	line := strings.TrimSpace(out)
	i := strings.LastIndex(line, "|")
	if i < 0 {
		return "", "", false
	}
	name = strings.TrimSpace(line[:i])
	bundle = strings.TrimSpace(line[i+1:])
	if name == "" || bundle == "" {
		return "", "", false
	}
	return name, bundle, true
}

func isPermissionDenied(stderr []byte) bool {
	s := string(stderr)
	return strings.Contains(s, "-1743") || strings.Contains(s, "Not authorized")
}

// Prefix match: `com.bank.` blocks all `com.bank.*` bundles via one rule.
func (a *ActiveApp) blocked(bundle string) bool {
	for _, p := range a.block {
		if p != "" && strings.HasPrefix(bundle, p) {
			return true
		}
	}
	return false
}
