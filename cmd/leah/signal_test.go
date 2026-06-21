package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
)

// TestWriteInterruptedAudit_WritesRow asserts the cancel-aware path emits a
// single audit row with Outcome="interrupted" when ctx was canceled mid-run.
func TestWriteInterruptedAudit_WritesRow(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)
	auditPath := filepath.Join(tmp, "audit.jsonl")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	writeInterruptedAudit(ctx, auditPath)

	b, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 audit row, got %d: %q", len(lines), string(b))
	}
	var e audit.Entry
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Outcome != "interrupted" {
		t.Errorf("outcome = %q; want %q", e.Outcome, "interrupted")
	}
	if e.Kind == "" {
		t.Error("kind is empty")
	}
}

// TestWriteInterruptedAudit_NoopWhenCtxAlive asserts no row is emitted when
// the program exits cleanly (ctx never canceled).
func TestWriteInterruptedAudit_NoopWhenCtxAlive(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)
	auditPath := filepath.Join(tmp, "audit.jsonl")

	ctx := context.Background()
	writeInterruptedAudit(ctx, auditPath)

	if _, err := os.Stat(auditPath); !os.IsNotExist(err) {
		t.Errorf("audit.jsonl should not exist on clean exit, stat err=%v", err)
	}
}

// TestRunCommand_HonorsCancellation drives runCommand with a canceled ctx +
// the `version` subcommand (cheapest path); asserts it returns within the
// deadline rather than blocking on the now-removed context.Background.
func TestRunCommand_HonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan int, 1)
	go func() { done <- runCommand(ctx, nil, []string{"version"}) }()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("runCommand did not return within 100ms after ctx cancel")
	}
}

// TestRunCommand_DispatchesVersion is a baseline smoke test that the
// signal-aware dispatcher still routes recognized subcommands.
func TestRunCommand_DispatchesVersion(t *testing.T) {
	ctx := context.Background()
	if got := runCommand(ctx, nil, []string{"version"}); got != 0 {
		t.Errorf("runCommand version = %d; want 0", got)
	}
}

// TestRunCommand_BriefBadFlag_ReturnsTwo pins the subcommand-returns-int
// contract: an unknown flag must propagate exit code 2 back through
// runCommand instead of os.Exit-ing the process (review #55: subcommand
// os.Exit skips the deferred writeInterruptedAudit in main()).
func TestRunCommand_BriefBadFlag_ReturnsTwo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)
	if got := runCommand(context.Background(), nil, []string{"brief", "--no-such-flag"}); got != 2 {
		t.Errorf("runCommand brief --no-such-flag = %d; want 2", got)
	}
}

// TestRunCommand_BackupBadTarget_ReturnsTwo same pin for backup —
// unknown --target value used to call os.Exit(2) inside runBackup,
// bypassing main()'s defer. After the fix it returns 2.
func TestRunCommand_BackupBadTarget_ReturnsTwo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)
	if got := runCommand(context.Background(), nil, []string{"backup", "--target", "bogus"}); got != 2 {
		t.Errorf("runCommand backup --target bogus = %d; want 2", got)
	}
}

// TestRun_FlushesInterruptedAudit pins the end-to-end fix: when run()
// finishes with a canceled ctx, the deferred writeInterruptedAudit MUST
// produce a row before main() calls os.Exit. Previously the defer was
// either skipped (os.Exit inside subcommand) or never reached (os.Exit
// in main() bypassing defers).
func TestRun_FlushesInterruptedAudit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LEAH_STATE_DIR", tmp)
	auditPath := filepath.Join(tmp, "audit.jsonl")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// run() owns the defer chain; emulate by calling the layered helper.
	_ = runAndFlush(ctx, auditPath, []string{"version"})

	b, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("audit.jsonl missing — defer writeInterruptedAudit never fired: %v", err)
	}
	if !strings.Contains(string(b), `"outcome":"interrupted"`) {
		t.Errorf("audit body lacks interrupted row: %q", string(b))
	}
}
