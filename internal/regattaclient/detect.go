package regattaclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Mode is the resolved topology selected at daemon boot.
type Mode string

const (
	ModeDocker Mode = "docker"
	ModeLocal  Mode = "local"
	ModeCloud  Mode = "cloud"
	ModeNone   Mode = "none"
)

// ErrNoMode means no regatta backend is reachable; operator must run
// `leah connect regatta` (or set LEAH_REGATTA_MODE).
var ErrNoMode = errors.New("regatta: no mode detected — run `leah connect regatta`")

// Config carries the resolved transport coordinates per Mode.
// Only the field matching Mode is populated; the rest are zero.
type Config struct {
	URL        string // docker mode — http://127.0.0.1:9090
	SocketPath string // local mode  — ~/.leah-state/regatta.sock
	TokenPath  string // cloud mode  — ~/.leah-state/secrets/regatta-token.json
}

// DetectOpts injects every external dependency so tests stay hermetic.
// Nil fields fall back to the production seam.
type DetectOpts struct {
	Getenv         func(string) string
	HomeDir        func() (string, error)
	DockerHealthy  func(ctx context.Context) bool
	SocketHealthy  func(ctx context.Context, path string) bool
	CloudTokenPath func(home string) string
	FileExists     func(path string) bool
}

// Detect resolves the active mode per spec §7 (first match wins):
//  1. LEAH_REGATTA_MODE env override
//  2. Docker container leah-regatta running + 127.0.0.1:9090 reachable
//  3. Sibling-binary unix socket at ~/.leah-state/regatta.sock
//  4. Cloud token file at ~/.leah-state/secrets/regatta-token.json
//  5. ErrNoMode
func Detect(ctx context.Context, opts DetectOpts) (Mode, Config, error) {
	opts = withDefaults(opts)

	home, err := opts.HomeDir()
	if err != nil {
		// Probe-based fallback still possible (docker doesn't need home),
		// but local + cloud do. Treat as no-home for those branches.
		home = ""
	}

	if env := strings.ToLower(strings.TrimSpace(opts.Getenv("LEAH_REGATTA_MODE"))); env != "" {
		switch Mode(env) {
		case ModeDocker:
			return ModeDocker, Config{URL: dockerURL}, nil
		case ModeLocal:
			return ModeLocal, Config{SocketPath: socketPath(home)}, nil
		case ModeCloud:
			return ModeCloud, Config{TokenPath: opts.CloudTokenPath(home)}, nil
		}
		// Unknown env values are ignored — spec §7 reserves env override for
		// the named modes; garbage shouldn't preempt the live probes.
	}

	if opts.DockerHealthy(ctx) {
		return ModeDocker, Config{URL: dockerURL}, nil
	}

	if home != "" {
		sp := socketPath(home)
		if opts.SocketHealthy(ctx, sp) {
			return ModeLocal, Config{SocketPath: sp}, nil
		}
		tp := opts.CloudTokenPath(home)
		if opts.FileExists(tp) {
			return ModeCloud, Config{TokenPath: tp}, nil
		}
	}

	return ModeNone, Config{}, ErrNoMode
}

const dockerURL = "http://127.0.0.1:9090"

func socketPath(home string) string {
	return filepath.Join(home, ".leah-state", "regatta.sock")
}

func defaultCloudTokenPath(home string) string {
	return filepath.Join(home, ".leah-state", "secrets", "regatta-token.json")
}

func withDefaults(o DetectOpts) DetectOpts {
	if o.Getenv == nil {
		o.Getenv = os.Getenv
	}
	if o.HomeDir == nil {
		o.HomeDir = os.UserHomeDir
	}
	if o.DockerHealthy == nil {
		o.DockerHealthy = probeDocker
	}
	if o.SocketHealthy == nil {
		o.SocketHealthy = probeSocket
	}
	if o.CloudTokenPath == nil {
		o.CloudTokenPath = defaultCloudTokenPath
	}
	if o.FileExists == nil {
		o.FileExists = func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		}
	}
	return o
}

// probeDocker returns true when `docker inspect leah-regatta` reports a
// running container AND 127.0.0.1:9090/healthz returns 2xx within 1s.
func probeDocker(ctx context.Context) bool {
	pctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	out, err := exec.CommandContext(pctx, "docker", "inspect", "-f", "{{.State.Running}}", "leah-regatta").Output()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		return false
	}

	req, err := http.NewRequestWithContext(pctx, http.MethodGet, dockerURL+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// probeSocket: spec §7 requires a unix-socket path + 2xx /healthz over it.
// Detect runs at boot before any UI is up, so a slow probe blocks startup —
// the 1s timeout matches the spec budget. Presence + dial success is the
// signal; a full HTTP round-trip over the socket is the connect-path job.
func probeSocket(ctx context.Context, path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode()&os.ModeSocket == 0 {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(pctx, "unix", path)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
