package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPurgeEverything_RequiresAttestation: bare `leah purge` (no --everything)
// is a usage error so a typo never advances to the destructive path.
func TestPurgeEverything_RequiresAttestation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)

	var buf bytes.Buffer
	if code := runPurge(context.Background(), []string{}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2 (usage error)", code)
	}
}

// TestPurgeEverything_AbortedOnAttestationDeny: missing auto-attest env +
// no stdin "yes" → returns 1, state dir intact, OAuth tokens intact.
func TestPurgeEverything_AbortedOnAttestationDeny(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	// Explicit empty (clear any inherited env).
	t.Setenv("LEAH_PURGE_AUTO_ATTEST", "")

	// Seed a token file to assert it is NOT removed.
	secrets := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tokenPath := filepath.Join(secrets, "gmail-token.json")
	if err := os.WriteFile(tokenPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Stdin closed → Fscanln returns EOF → attestor denies.
	r, w, _ := os.Pipe()
	_ = w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin; _ = r.Close() }()

	var buf bytes.Buffer
	if code := runPurge(context.Background(), []string{"--everything"}, &buf); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if _, err := os.Stat(tokenPath); err != nil {
		t.Fatalf("token removed on denied attest: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("state dir removed on denied attest: %v", err)
	}
}

// TestPurgeEverything_DeletesStateDir: with auto-attest, ~/.leah-state goes away.
func TestPurgeEverything_DeletesStateDir(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "leah-state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "audit.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("LEAH_STATE_DIR", stateDir)
	t.Setenv("LEAH_PURGE_AUTO_ATTEST", "1")

	var buf bytes.Buffer
	if code := runPurge(context.Background(), []string{"--everything"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("state dir still present: %v", err)
	}
}

// TestPurgeEverything_RevokesAllProviders: every authorized provider's token
// file is removed before the state dir is wiped.
func TestPurgeEverything_RevokesAllProviders(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_PURGE_AUTO_ATTEST", "1")

	secrets := filepath.Join(dir, "secrets")
	if err := os.MkdirAll(secrets, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gmailTok := filepath.Join(secrets, "gmail-token.json")
	gcalTok := filepath.Join(secrets, "gcal-token.json")
	for _, p := range []string{gmailTok, gcalTok} {
		if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	var buf bytes.Buffer
	if code := runPurge(context.Background(), []string{"--everything"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	// Both token files gone (revoked + local delete via state-dir wipe).
	for _, p := range []string{gmailTok, gcalTok} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("token %s still present: %v", p, err)
		}
	}
	out := buf.String()
	if !strings.Contains(out, "gmail") || !strings.Contains(out, "gcal") {
		t.Fatalf("revoke report missing providers: %s", out)
	}
}

// TestPurgeEverything_PrintsBrewHints: distribution-cleanup stage prints
// brew uninstall + PATH cleanup hints (no auto-execution).
func TestPurgeEverything_PrintsBrewHints(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_PURGE_AUTO_ATTEST", "1")

	var buf bytes.Buffer
	if code := runPurge(context.Background(), []string{"--everything"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	out := buf.String()
	for _, want := range []string{"brew uninstall", "PATH", "which leah"} {
		if !strings.Contains(out, want) {
			t.Fatalf("hint %q missing from output:\n%s", want, out)
		}
	}
}

// TestPurgeEverything_Help short-circuits with exit 0.
func TestPurgeEverything_Help(t *testing.T) {
	var buf bytes.Buffer
	if code := runPurge(context.Background(), []string{"--help"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
}
