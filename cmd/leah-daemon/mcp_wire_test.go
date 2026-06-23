package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMCPPublishWired_GatedOn asserts startMCPPublishAt binds the publish
// socket when LEAH_MCP_PUBLISH=1 and removes the socket file on ctx cancel.
// Uses /tmp because macOS sun_path is ~104 bytes and t.TempDir under
// /var/folders/... overflows once a filename is appended.
func TestMCPPublishWired_GatedOn(t *testing.T) {
	t.Setenv("LEAH_MCP_PUBLISH", "1")
	sock := shortSockPath(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startMCPPublishAt(ctx, nil, nil, os.Stderr, sock)

	if !waitFor(sock, true, 2*time.Second) {
		t.Fatalf("publish socket not created at %s", sock)
	}
	cancel()
	if !waitFor(sock, false, 2*time.Second) {
		t.Fatalf("publish socket not removed after ctx cancel: %s", sock)
	}
}

// TestMCPPublishWired_GatedOff asserts startMCPPublishAt creates no socket
// when the env var is unset — the Phase 3 default-off ship gate.
func TestMCPPublishWired_GatedOff(t *testing.T) {
	_ = os.Unsetenv("LEAH_MCP_PUBLISH")
	sock := shortSockPath(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startMCPPublishAt(ctx, nil, nil, os.Stderr, sock)

	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(sock); err == nil {
		t.Fatalf("publish socket created with gate off: %s", sock)
	}
}

// shortSockPath returns a /tmp-rooted path that fits inside macOS sun_path.
// t.TempDir() under /var/folders/... exceeds 104 bytes once a filename is
// appended, so we rely on Cleanup instead.
func shortSockPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "leahmcp-")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "p.sock")
}

func waitFor(path string, want bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		_, err := os.Stat(path)
		exists := err == nil
		if exists == want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
