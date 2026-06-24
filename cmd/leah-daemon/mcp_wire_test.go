package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/testutil"
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

	done := startMCPPublishAt(ctx, nil, nil, os.Stderr, sock)

	testutil.Eventually(t, 2*time.Second, 10*time.Millisecond, func() bool {
		_, err := os.Stat(sock)
		return err == nil
	})
	cancel()
	<-done
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("publish socket not removed after ctx cancel: %s (stat err=%v)", sock, err)
	}
}

// TestMCPPublishWired_GatedOff asserts startMCPPublishAt creates no socket
// when the env var is unset — the Phase 3 default-off ship gate.
func TestMCPPublishWired_GatedOff(t *testing.T) {
	_ = os.Unsetenv("LEAH_MCP_PUBLISH")
	sock := shortSockPath(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ServePublish returns ErrPublishDisabled synchronously when the gate is
	// off, so <-done is a deterministic gate — no wall-clock sleep needed.
	<-startMCPPublishAt(ctx, nil, nil, os.Stderr, sock)

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
