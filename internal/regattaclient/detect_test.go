package regattaclient

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// stubProbes returns DetectOpts whose seams answer from in-memory state.
// docker, socket, cloud each toggle individually so tests can assert priority.
type stubState struct {
	envMode    string
	dockerUp   bool
	socketPath string // empty = no socket
	cloudToken bool
	homeDir    string
}

func optsFrom(s stubState) DetectOpts {
	return DetectOpts{
		Getenv: func(k string) string {
			if k == "LEAH_REGATTA_MODE" {
				return s.envMode
			}
			return ""
		},
		HomeDir:       func() (string, error) { return s.homeDir, nil },
		DockerHealthy: func(ctx context.Context) bool { return s.dockerUp },
		SocketHealthy: func(ctx context.Context, path string) bool { return s.socketPath != "" && path == s.socketPath },
		CloudTokenPath: func(home string) string {
			p := filepath.Join(home, ".leah-state", "secrets", "regatta-token.json")
			if s.cloudToken {
				return p
			}
			return p // returned regardless; file existence check is separate
		},
		FileExists: func(p string) bool {
			if s.cloudToken && p == filepath.Join(s.homeDir, ".leah-state", "secrets", "regatta-token.json") {
				return true
			}
			if s.socketPath != "" && p == s.socketPath {
				return true
			}
			return false
		},
	}
}

func TestDetect_EnvOverride_DockerWins(t *testing.T) {
	// Env override forces docker even when docker probe would fail.
	opts := optsFrom(stubState{envMode: "docker", dockerUp: false, homeDir: t.TempDir()})
	mode, cfg, err := Detect(context.Background(), opts)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeDocker {
		t.Fatalf("mode = %q, want %q", mode, ModeDocker)
	}
	if cfg.URL == "" {
		t.Fatalf("docker mode must populate cfg.URL")
	}
}

func TestDetect_EnvOverride_LocalWins(t *testing.T) {
	home := t.TempDir()
	opts := optsFrom(stubState{envMode: "local", homeDir: home})
	mode, cfg, err := Detect(context.Background(), opts)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeLocal {
		t.Fatalf("mode = %q, want %q", mode, ModeLocal)
	}
	wantSock := filepath.Join(home, ".leah-state", "regatta.sock")
	if cfg.SocketPath != wantSock {
		t.Fatalf("socket = %q, want %q", cfg.SocketPath, wantSock)
	}
}

func TestDetect_EnvOverride_CloudWins(t *testing.T) {
	home := t.TempDir()
	opts := optsFrom(stubState{envMode: "cloud", homeDir: home})
	mode, cfg, err := Detect(context.Background(), opts)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeCloud {
		t.Fatalf("mode = %q, want %q", mode, ModeCloud)
	}
	wantToken := filepath.Join(home, ".leah-state", "secrets", "regatta-token.json")
	if cfg.TokenPath != wantToken {
		t.Fatalf("token = %q, want %q", cfg.TokenPath, wantToken)
	}
}

func TestDetect_DockerContainer_Detected(t *testing.T) {
	opts := optsFrom(stubState{dockerUp: true, homeDir: t.TempDir()})
	mode, cfg, err := Detect(context.Background(), opts)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeDocker {
		t.Fatalf("mode = %q, want %q", mode, ModeDocker)
	}
	if cfg.URL != "http://127.0.0.1:9090" {
		t.Fatalf("URL = %q, want http://127.0.0.1:9090", cfg.URL)
	}
}

func TestDetect_LocalSocket_Detected(t *testing.T) {
	home := t.TempDir()
	sock := filepath.Join(home, ".leah-state", "regatta.sock")
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	opts := optsFrom(stubState{socketPath: sock, homeDir: home})
	mode, cfg, err := Detect(context.Background(), opts)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeLocal {
		t.Fatalf("mode = %q, want %q", mode, ModeLocal)
	}
	if cfg.SocketPath != sock {
		t.Fatalf("socket = %q, want %q", cfg.SocketPath, sock)
	}
}

func TestDetect_CloudToken_Detected(t *testing.T) {
	home := t.TempDir()
	opts := optsFrom(stubState{cloudToken: true, homeDir: home})
	mode, cfg, err := Detect(context.Background(), opts)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeCloud {
		t.Fatalf("mode = %q, want %q", mode, ModeCloud)
	}
	wantToken := filepath.Join(home, ".leah-state", "secrets", "regatta-token.json")
	if cfg.TokenPath != wantToken {
		t.Fatalf("token = %q, want %q", cfg.TokenPath, wantToken)
	}
}

func TestDetect_None_ErrNoMode(t *testing.T) {
	opts := optsFrom(stubState{homeDir: t.TempDir()})
	mode, _, err := Detect(context.Background(), opts)
	if !errors.Is(err, ErrNoMode) {
		t.Fatalf("err = %v, want ErrNoMode", err)
	}
	if mode != ModeNone {
		t.Fatalf("mode = %q, want %q", mode, ModeNone)
	}
}

func TestDetect_PriorityOrder_DockerOverLocalOverCloud(t *testing.T) {
	home := t.TempDir()
	sock := filepath.Join(home, ".leah-state", "regatta.sock")

	// All three available, no env override → docker wins.
	all := optsFrom(stubState{dockerUp: true, socketPath: sock, cloudToken: true, homeDir: home})
	mode, _, err := Detect(context.Background(), all)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeDocker {
		t.Fatalf("docker+local+cloud → mode = %q, want %q", mode, ModeDocker)
	}

	// Docker off, local + cloud available → local wins.
	noDocker := optsFrom(stubState{socketPath: sock, cloudToken: true, homeDir: home})
	mode, _, err = Detect(context.Background(), noDocker)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeLocal {
		t.Fatalf("local+cloud → mode = %q, want %q", mode, ModeLocal)
	}

	// Only cloud available → cloud wins.
	onlyCloud := optsFrom(stubState{cloudToken: true, homeDir: home})
	mode, _, err = Detect(context.Background(), onlyCloud)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeCloud {
		t.Fatalf("cloud-only → mode = %q, want %q", mode, ModeCloud)
	}
}

// Defensive: an unknown env value falls through to probe-based detection
// rather than silently coercing to a wrong mode.
func TestDetect_UnknownEnvFallsThrough(t *testing.T) {
	opts := optsFrom(stubState{envMode: "garbage", dockerUp: true, homeDir: t.TempDir()})
	mode, _, err := Detect(context.Background(), opts)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeDocker {
		t.Fatalf("unknown env → mode = %q, want %q (probe fallback)", mode, ModeDocker)
	}
}

// Probe sanity: confirms a real unix socket on disk satisfies the default
// socket-healthy seam when it is wired through Detect.
func TestDetect_DefaultSocketProbe_RealSocket(t *testing.T) {
	// macOS sun_path is 104 bytes; t.TempDir() under /var/folders blows
	// that budget, so anchor the home under /tmp where the full path fits.
	home, err := os.MkdirTemp("/tmp", "leah-detect-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(home) }()
	if err := os.MkdirAll(filepath.Join(home, ".leah-state"), 0o700); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(home, ".leah-state", "regatta.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix listen unsupported: %v", err)
	}
	defer func() { _ = ln.Close() }()

	opts := DetectOpts{
		Getenv:        func(string) string { return "" },
		HomeDir:       func() (string, error) { return home, nil },
		DockerHealthy: func(context.Context) bool { return false },
		// Use default socket-healthy: presence of the path is sufficient.
	}
	mode, cfg, err := Detect(context.Background(), opts)
	if err != nil {
		t.Fatalf("Detect err: %v", err)
	}
	if mode != ModeLocal {
		t.Fatalf("mode = %q, want %q", mode, ModeLocal)
	}
	if cfg.SocketPath != sock {
		t.Fatalf("socket = %q, want %q", cfg.SocketPath, sock)
	}
}
