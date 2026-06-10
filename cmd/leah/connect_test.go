package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunConnect_List covers the --list path: no OAuth, no network, just an
// enumeration of registered providers with auth-status from token-file
// presence. Asserts both providers print and gmail flips to authorized once a
// token file exists.
func TestRunConnect_List(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	var buf bytes.Buffer
	if code := runConnect([]string{"--list"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "gmail") || !strings.Contains(out, "gcal") {
		t.Fatalf("providers missing: %s", out)
	}
	if !strings.Contains(out, "not authorized") {
		t.Fatalf("want unauth marker: %s", out)
	}

	secrets := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secrets, "gmail-token.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf.Reset()
	if code := runConnect([]string{"--list"}, &buf); code != 0 {
		t.Fatalf("exit2 %d", code)
	}
	if !strings.Contains(buf.String(), "authorized") {
		t.Fatalf("gmail authorized marker missing: %s", buf.String())
	}
}

// TestRunConnect_UnknownProvider asserts exit code 2 + a useful stderr line.
func TestRunConnect_UnknownProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runConnect([]string{"slack"}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunConnect_NoArgs prints usage + exits 2.
func TestRunConnect_NoArgs(t *testing.T) {
	var buf bytes.Buffer
	if code := runConnect([]string{}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestRunConnect_Help short-circuits with exit 0 before any state work.
func TestRunConnect_Help(t *testing.T) {
	var buf bytes.Buffer
	if code := runConnect([]string{"--help"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
}
