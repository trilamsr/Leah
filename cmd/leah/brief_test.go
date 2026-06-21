package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStderr swaps os.Stderr for the duration of fn and returns whatever
// was written. Brief's invalid-flag surface is stderr-only, mirroring recall
// and news; the cursor tests assert the same shape.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	os.Stderr = orig
	<-done
	return buf.String()
}

func TestBriefSinceInvalidRFC3339ExitsTwo(t *testing.T) {
	var rc int
	stderr := captureStderr(t, func() {
		rc = runBrief(context.Background(), []string{"--since=not-a-time", "--silent"})
	})
	if rc != 2 {
		t.Fatalf("rc = %d, want 2", rc)
	}
	if !strings.Contains(stderr, "invalid --since") {
		t.Fatalf("stderr missing invalid --since: %q", stderr)
	}
}

func TestBriefSinceMissingValueExitsTwo(t *testing.T) {
	var rc int
	stderr := captureStderr(t, func() {
		rc = runBrief(context.Background(), []string{"--since"})
	})
	if rc != 2 {
		t.Fatalf("rc = %d, want 2", rc)
	}
	if !strings.Contains(stderr, "invalid --since") {
		t.Fatalf("stderr missing invalid --since: %q", stderr)
	}
}

func TestBriefSinceValidRFC3339Accepted(t *testing.T) {
	// --silent writes to a file under stateDir() rather than network/voice,
	// so the test stays hermetic. Point stateDir at a fresh tmp dir.
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)
	var rc int
	stderr := captureStderr(t, func() {
		rc = runBrief(context.Background(), []string{"--since=2026-06-21T00:00:00Z", "--silent"})
	})
	if rc != 0 {
		t.Fatalf("rc = %d, want 0; stderr=%q", rc, stderr)
	}
	if strings.Contains(stderr, "invalid --since") {
		t.Fatalf("unexpected invalid --since: %q", stderr)
	}
}

func TestBriefRejectsUnknownFlag(t *testing.T) {
	// Regression: --since parsing must not break the existing unknown-flag
	// guard. A bogus flag still exits 2 with the existing message.
	var rc int
	stderr := captureStderr(t, func() {
		rc = runBrief(context.Background(), []string{"--nope"})
	})
	if rc != 2 {
		t.Fatalf("rc = %d, want 2", rc)
	}
	if !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("stderr missing unknown flag: %q", stderr)
	}
}
