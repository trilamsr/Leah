package main

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
)

// TestRunPushSources_FansFocusEventToIPC: producer kind
// "focus.state_changed" must translate to IPC kind "push.focus".
func TestRunPushSources_FansFocusEventToIPC(t *testing.T) {
	var (
		mu      sync.Mutex
		gotKind string
	)
	emit := func(kind string, _ []byte) {
		mu.Lock()
		defer mu.Unlock()
		gotKind = kind
	}
	source := make(chan telemetry.Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		runPushSourcesFromChan(ctx, source, emit)
		close(done)
	}()

	source <- telemetry.Event{Kind: "focus.state_changed"}

	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		got := gotKind
		mu.Unlock()
		if got != "" {
			if got != "push.focus" {
				t.Fatalf("kind: got %q want push.focus", got)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("emit never called within 500ms")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestRunPushSources_TranslatesAllKinds covers the full switch table so a
// missed case (e.g. someone renames "mail_changed") trips a unit test rather
// than a silent drop in production.
func TestRunPushSources_TranslatesAllKinds(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"mail_changed", "push.mail"},
		{"contact_store_changed", "push.contacts"},
		{"focus.state_changed", "push.focus"},
		{"workspace.active_app_changed", "push.activeapp"},
	}
	for _, tc := range cases {
		if got := translatePushKind(tc.in); got != tc.out {
			t.Errorf("translatePushKind(%q) = %q; want %q", tc.in, got, tc.out)
		}
	}
}

// TestRunPushSources_UnknownKindIgnored: a producer kind that does not match
// the switch must NOT call emit (otherwise HUD subscribers see an empty
// "push." kind and crash JSON-decoding).
func TestRunPushSources_UnknownKindIgnored(t *testing.T) {
	called := 0
	emit := func(string, []byte) { called++ }
	source := make(chan telemetry.Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		runPushSourcesFromChan(ctx, source, emit)
		close(done)
	}()

	source <- telemetry.Event{Kind: "totally.unrelated.kind"}
	// Give the goroutine a tick to process before we conclude it did not emit.
	time.Sleep(50 * time.Millisecond)
	if called != 0 {
		t.Fatalf("emit called %d times on unknown kind", called)
	}
}

// TestRunPushSources_CtxCancelStopsAllGoroutines: canceling the context must
// return runPushSourcesFromChan within a short window — otherwise the daemon
// shutdown path leaks goroutines on every reload.
func TestRunPushSources_CtxCancelStopsAllGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	source := make(chan telemetry.Event)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		runPushSourcesFromChan(ctx, source, func(string, []byte) {})
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("runPushSourcesFromChan did not return within 500ms of cancel")
	}
	// Settle the scheduler. Stable goroutine count = no leak.
	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+1 {
		t.Fatalf("goroutine leak: before=%d after=%d", before, after)
	}
}

// TestRunPushSources_PayloadIsJSON: the emit payload must be a JSON-encoded
// obs.Event so HUD widgets can decode it without a custom wire format.
func TestRunPushSources_PayloadIsJSON(t *testing.T) {
	var (
		mu      sync.Mutex
		payload []byte
	)
	emit := func(_ string, p []byte) {
		mu.Lock()
		defer mu.Unlock()
		payload = p
	}
	source := make(chan telemetry.Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runPushSourcesFromChan(ctx, source, emit)

	source <- telemetry.Event{Kind: "mail_changed", Actor: "mail", Outcome: "ok"}

	deadline := time.After(500 * time.Millisecond)
	for {
		mu.Lock()
		got := payload
		mu.Unlock()
		if got != nil {
			if len(got) == 0 || got[0] != '{' {
				t.Fatalf("payload not JSON object: %q", string(got))
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("payload never emitted within 500ms")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
