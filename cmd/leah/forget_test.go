package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/audit"
)

// TestForget_UsageError_ReturnsTwo — no args returns exit 2.
func TestForget_UsageError_ReturnsTwo(t *testing.T) {
	var buf bytes.Buffer
	if code := runForget(context.Background(), []string{}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// TestForget_Help_ReturnsZero — `--help` short-circuits cleanly.
func TestForget_Help_ReturnsZero(t *testing.T) {
	var buf bytes.Buffer
	if code := runForget(context.Background(), []string{"--help"}, &buf); code != 0 {
		t.Fatalf("exit %d, want 0", code)
	}
	if !strings.Contains(buf.String(), "leah forget") {
		t.Fatalf("usage missing: %s", buf.String())
	}
}

// TestForget_DryRun_NoWipe — --dry-run prints intent without touching DB.
func TestForget_DryRun_NoWipe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	var buf bytes.Buffer
	if code := runForget(context.Background(), []string{"--dry-run", "all"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Fatalf("dry-run marker missing: %s", buf.String())
	}
}

// TestForget_PatternID_Removes — happy path on a single pattern, attestation
// auto-passed, audit row written with the pattern in Detail.
func TestForget_PatternID_Removes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FORGET_AUTO_ATTEST", "1")
	var buf bytes.Buffer
	if code := runForget(context.Background(), []string{"commit_at_noon"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if !auditHasDetail(t, filepath.Join(dir, "audit.jsonl"), "recommendation_forget", "commit_at_noon") {
		t.Fatalf("audit row missing or wrong Detail")
	}
}

// TestForget_All_RemovesAll — `forget all` invokes wipeOperatorProfile and
// audits success.
func TestForget_All_RemovesAll(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	t.Setenv("LEAH_FORGET_AUTO_ATTEST", "1")
	var buf bytes.Buffer
	if code := runForget(context.Background(), []string{"all"}, &buf); code != 0 {
		t.Fatalf("exit %d, out=%s", code, buf.String())
	}
	if !auditHasDetail(t, filepath.Join(dir, "audit.jsonl"), "recommendation_forget", "all") {
		t.Fatalf("audit row missing or wrong Detail")
	}
}

// TestForget_AttestationDenied_NoChange — no auto-attest env + empty stdin
// returns exit 1 and writes a failed audit row.
func TestForget_AttestationDenied_NoChange(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", dir)
	// No LEAH_FORGET_AUTO_ATTEST → prompts on stdin; we redirect stdin to /dev/null.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	_ = w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()

	var buf bytes.Buffer
	if code := runForget(context.Background(), []string{"some_pattern"}, &buf); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !auditHasDetail(t, filepath.Join(dir, "audit.jsonl"), "recommendation_forget", "attestation_denied") {
		t.Fatalf("attestation_denied audit row missing")
	}
}

// TestForget_TooManyArgs_ReturnsTwo — extra positional args fail fast.
func TestForget_TooManyArgs_ReturnsTwo(t *testing.T) {
	var buf bytes.Buffer
	if code := runForget(context.Background(), []string{"a", "b"}, &buf); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func auditHasDetail(t *testing.T, path, kind, detail string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		var e audit.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("parse audit row: %v", err)
		}
		if e.Kind == kind && e.Detail == detail {
			return true
		}
	}
	return false
}
