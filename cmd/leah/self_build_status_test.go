package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAudit(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "audit.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEAH_STATE_DIR", dir)
	return dir
}

func TestSelfBuildStatus_ClosedLoopExit0(t *testing.T) {
	writeAudit(t,
		`{"kind":"self-build","args_hash":"h1","outcome":"dispatched","detail":"url=https://github.com/x/y/issues/1","ts":"2026-06-20T10:00:00Z"}`,
		`{"kind":"ship","args_hash":"h1","outcome":"pending","ts":"2026-06-20T10:01:00Z"}`,
		`{"kind":"daemon.transition","args_hash":"h1","outcome":"observed","detail":"agent a1: running → merged (PR #1)","ts":"2026-06-20T11:00:00Z"}`,
		`{"kind":"self-build.outcome","args_hash":"h1","outcome":"MERGED","detail":"state=MERGED","ts":"2026-06-20T11:01:00Z"}`,
	)
	var out bytes.Buffer
	if code := runSelfBuildStatus(nil, &out); code != 0 {
		t.Fatalf("exit: got %d want 0", code)
	}
	if !strings.Contains(out.String(), "CLOSED") {
		t.Errorf("output missing CLOSED: %s", out.String())
	}
}

func TestSelfBuildStatus_PendingExit0(t *testing.T) {
	writeAudit(t,
		`{"kind":"self-build","args_hash":"h1","outcome":"dispatched","ts":"2026-06-20T10:00:00Z"}`,
	)
	var out bytes.Buffer
	if code := runSelfBuildStatus(nil, &out); code != 0 {
		t.Fatalf("exit: got %d want 0 (pending is not a failure)", code)
	}
	if !strings.Contains(out.String(), "PENDING") {
		t.Errorf("output missing PENDING: %s", out.String())
	}
}

func TestSelfBuildStatus_MalformedExitNonZero(t *testing.T) {
	writeAudit(t, `{not json`)
	var out bytes.Buffer
	if code := runSelfBuildStatus(nil, &out); code == 0 {
		t.Fatalf("malformed audit must exit non-zero, got 0")
	}
}

func TestSelfBuildStatus_NoLoopsExit0(t *testing.T) {
	writeAudit(t, `{"kind":"ask","outcome":"ok","ts":"2026-06-20T10:00:00Z"}`)
	var out bytes.Buffer
	if code := runSelfBuildStatus(nil, &out); code != 0 {
		t.Fatalf("exit: got %d want 0", code)
	}
}
