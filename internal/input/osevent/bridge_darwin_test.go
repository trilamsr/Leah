//go:build darwin && cgo

package osevent

import (
	"context"
	"testing"
	"time"
)

// macOS-only smoke: pump constructs, Subscribe returns a usable channel, Close
// is idempotent. We do NOT assert real NSWorkspace deliveries — CI has no
// active app to switch to. Functional verification happens via fake; this
// test guards the cgo link + observer-registration path against bitrot.
func TestDarwinSource_ConstructsAndCloses(t *testing.T) {
	src, err := NewSource(Config{Blocklist: []string{"com.bank."}})
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch, err := src.Subscribe(ctx, WorkspaceActiveAppChanged)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if ch == nil {
		t.Fatal("nil channel")
	}

	select {
	case <-ch:
		// Real event would be unexpected here but harmless.
	case <-time.After(50 * time.Millisecond): // allow-sleep: bounded smoke window; we are NOT polling — single deadline check
	}

	if err := src.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := src.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
