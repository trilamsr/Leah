//go:build darwin && cgo && integration

package osevent

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// Verifies the cgo bridge actually delivers an NSWorkspace event end-to-end.
// Driven by activating Finder via osascript so the pump must observe a real
// notification and dispatch to a Go channel. Run with `go test -tags=integration`.
func TestDarwinBridge_DeliversActiveAppEvent(t *testing.T) {
	src, err := NewSource(Config{})
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	t.Cleanup(func() { _ = src.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch, err := src.Subscribe(ctx, WorkspaceActiveAppChanged)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Settle observer registration before triggering. NSWorkspace observer
	// registration is synchronous but the dispatch source the pump thread
	// installs needs one runloop iteration to attach.
	time.Sleep(100 * time.Millisecond) // allow-sleep: bounded one-shot settle before triggering external activation; not a polling loop

	if err := exec.Command("osascript", "-e", `tell application "Finder" to activate`).Run(); err != nil {
		t.Skipf("osascript activate failed (likely sandboxed CI): %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Kind != WorkspaceActiveAppChanged {
			t.Fatalf("kind: got %q want %q", ev.Kind, WorkspaceActiveAppChanged)
		}
	case <-time.After(2 * time.Second): // allow-sleep: single deadline for the event to arrive after activation
		t.Fatal("no WorkspaceActiveAppChanged event within 2s — pump not delivering")
	}
}
