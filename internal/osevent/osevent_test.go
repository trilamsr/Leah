package osevent

import (
	"context"
	"testing"
	"time"
)

func TestFake_SubscribeReceivesInjectedEvent(t *testing.T) {
	t.Parallel()
	f := NewFake(Config{})
	t.Cleanup(func() { _ = f.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, err := f.Subscribe(ctx, WorkspaceActiveAppChanged)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	want := Event{
		Kind:   WorkspaceActiveAppChanged,
		Detail: map[string]any{"bundle_id": "com.apple.Terminal", "name": "Terminal"},
	}
	f.Inject(want)

	select {
	case got := <-ch:
		if got.Kind != want.Kind {
			t.Fatalf("kind: got %q want %q", got.Kind, want.Kind)
		}
		if got.Detail["bundle_id"] != "com.apple.Terminal" {
			t.Fatalf("bundle_id: %v", got.Detail["bundle_id"])
		}
		if got.At.IsZero() {
			t.Fatal("At not stamped")
		}
	case <-time.After(time.Second):
		t.Fatal("no event delivered")
	}
}

func TestFake_ContextCancel_ClosesChannel(t *testing.T) {
	t.Parallel()
	f := NewFake(Config{})
	t.Cleanup(func() { _ = f.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := f.Subscribe(ctx, ContactStoreChanged)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel, got event")
		}
	case <-time.After(time.Second):
		t.Fatal("channel did not close after ctx cancel")
	}
}

func TestFake_BlockedApp_NotEmitted(t *testing.T) {
	t.Parallel()
	f := NewFake(Config{Blocklist: []string{"com.bank."}})
	t.Cleanup(func() { _ = f.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, err := f.Subscribe(ctx, WorkspaceActiveAppChanged)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	f.Inject(Event{
		Kind:   WorkspaceActiveAppChanged,
		Detail: map[string]any{"bundle_id": "com.bank.chase", "name": "Chase"},
	})
	f.Inject(Event{
		Kind:   WorkspaceActiveAppChanged,
		Detail: map[string]any{"bundle_id": "com.apple.Safari", "name": "Safari"},
	})

	select {
	case got := <-ch:
		if got.Detail["bundle_id"] != "com.apple.Safari" {
			t.Fatalf("got blocked event: %v", got.Detail)
		}
	case <-time.After(time.Second):
		t.Fatal("safari event missing")
	}

	select {
	case got := <-ch:
		t.Fatalf("unexpected second event: %v", got)
	case <-time.After(50 * time.Millisecond): // allow-sleep: bounded negative-assertion window for unwanted second delivery
	}
}

func TestFake_MultipleSubscribers_AllReceive(t *testing.T) {
	t.Parallel()
	f := NewFake(Config{})
	t.Cleanup(func() { _ = f.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a, err := f.Subscribe(ctx, WorkspaceLaunched)
	if err != nil {
		t.Fatalf("Subscribe a: %v", err)
	}
	b, err := f.Subscribe(ctx, WorkspaceLaunched)
	if err != nil {
		t.Fatalf("Subscribe b: %v", err)
	}
	c, err := f.Subscribe(ctx, WorkspaceTerminated)
	if err != nil {
		t.Fatalf("Subscribe c: %v", err)
	}

	f.Inject(Event{Kind: WorkspaceLaunched, Detail: map[string]any{"bundle_id": "com.x"}})

	for _, ch := range []<-chan Event{a, b} {
		select {
		case got := <-ch:
			if got.Kind != WorkspaceLaunched {
				t.Fatalf("kind: %v", got.Kind)
			}
		case <-time.After(time.Second):
			t.Fatal("missing fan-out delivery")
		}
	}
	select {
	case got := <-c:
		t.Fatalf("kind-mismatch subscriber got event: %v", got)
	case <-time.After(50 * time.Millisecond): // allow-sleep: bounded negative-assertion window for wrong-kind subscriber
	}
}

func TestFake_Close_ClosesAllSubscriptions(t *testing.T) {
	t.Parallel()
	f := NewFake(Config{})

	ctx := context.Background()
	ch, err := f.Subscribe(ctx, ContactStoreChanged)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel did not close after Source.Close")
	}

	if _, err := f.Subscribe(ctx, ContactStoreChanged); err == nil {
		t.Fatal("Subscribe after Close: want error, got nil")
	}
}

// Concurrent Inject across goroutines is race-free under -race.
func TestFake_ConcurrentInject_RaceFree(t *testing.T) {
	t.Parallel()
	f := NewFake(Config{})
	t.Cleanup(func() { _ = f.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, err := f.Subscribe(ctx, WorkspaceLaunched)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	const writers, perWriter = 8, 32
	done := make(chan struct{})
	for w := 0; w < writers; w++ {
		go func() {
			for i := 0; i < perWriter; i++ {
				f.Inject(Event{Kind: WorkspaceLaunched, Detail: map[string]any{"bundle_id": "com.x"}})
			}
			done <- struct{}{}
		}()
	}
	for w := 0; w < writers; w++ {
		<-done
	}
	// Drain whatever survived drop-on-full to ensure no deadlock.
	drained := 0
	for {
		select {
		case <-ch:
			drained++
		case <-time.After(50 * time.Millisecond): // allow-sleep: single drain deadline; not a polling loop
			if drained == 0 {
				t.Fatal("expected at least one delivered event after concurrent inject")
			}
			return
		}
	}
}
