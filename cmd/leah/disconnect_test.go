package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunDisconnect_UsageError_ReturnsTwo covers the no-arg branch.
func TestRunDisconnect_UsageError_ReturnsTwo(t *testing.T) {
	var buf bytes.Buffer
	if code := runDisconnect(context.Background(), []string{}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunDisconnect_UnknownIntegration_ReturnsTwo rejects names not in the registry.
func TestRunDisconnect_UnknownIntegration_ReturnsTwo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runDisconnect(context.Background(), []string{"slack"}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunDisconnect_List_RendersStatus shows installed providers + token-file presence.
func TestRunDisconnect_List_RendersStatus(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	secrets := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secrets, "gmail-token.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var buf bytes.Buffer
	if code := runDisconnect(context.Background(), []string{"--list"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "gmail") || !strings.Contains(out, "gcal") {
		t.Fatalf("providers missing: %s", out)
	}
	if !strings.Contains(out, "authorized") {
		t.Fatalf("auth marker missing: %s", out)
	}
}

// TestRunDisconnect_Help short-circuits with exit 0.
func TestRunDisconnect_Help(t *testing.T) {
	var buf bytes.Buffer
	if code := runDisconnect(context.Background(), []string{"--help"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
}

// TestRunDisconnect_HappyPath removes the token file via the auto-attest env hook.
func TestRunDisconnect_HappyPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")
	secrets := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tokenPath := filepath.Join(secrets, "gmail-token.json")
	if err := os.WriteFile(tokenPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var buf bytes.Buffer
	if code := runDisconnect(context.Background(), []string{"gmail"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("token file still present: %v", err)
	}
}

// TestRunDisconnect_NoTokenFile_ReturnsOne reports the sentinel error path.
func TestRunDisconnect_NoTokenFile_ReturnsOne(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_CONNECT_AUTO_ATTEST", "1")
	var buf bytes.Buffer
	if code := runDisconnect(context.Background(), []string{"gmail"}, &buf); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}
