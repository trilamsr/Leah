package selflearn

import (
	"bytes"
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
)

// TestResolverEmitsOutcomeToCallback asserts a resolved row fires the
// injected callback with the verdict so a downstream observer sees it.
func TestResolverEmitsOutcomeToCallback(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	logger := &audit.Logger{
		Path: auditPath,
		Now:  func() time.Time { return time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC) },
	}
	if err := logger.Append(audit.Entry{Kind: "ship", ArgsHash: "abc", Outcome: "pending", Detail: "x"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var got []Outcome
	var mu sync.Mutex
	sink := NewOutcomeSink(1, func(o Outcome) {
		mu.Lock()
		got = append(got, o)
		mu.Unlock()
	})
	defer sink.Close()

	r := &Resolver{
		AuditPath: auditPath,
		Logger:    logger,
		Rules:     map[string]Rule{"ship": stubRule{verdict: OutcomeFailed, detail: "state=CLOSED mergedAt=nil"}},
		Since:     7 * 24 * time.Hour,
		Now:       func() time.Time { return time.Date(2026, 6, 9, 11, 0, 0, 0, time.UTC) },
		Out:       new(bytes.Buffer),
		OnOutcome: sink.Emit,
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	sink.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("want 1 outcome delivered, got %d", len(got))
	}
	if got[0] != OutcomeFailed {
		t.Errorf("verdict: want %q, got %q", OutcomeFailed, got[0])
	}
}

// TestResolverNilCallbackIsNoOp asserts a Resolver with no OnOutcome still
// resolves rows (callback is optional).
func TestResolverNilCallbackIsNoOp(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")
	logger := &audit.Logger{Path: auditPath}
	if err := logger.Append(audit.Entry{Kind: "ship", ArgsHash: "abc", Outcome: "pending", Detail: "x"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r := &Resolver{
		AuditPath: auditPath,
		Logger:    logger,
		Rules:     map[string]Rule{"ship": stubRule{verdict: OutcomeSuccess, detail: "ok"}},
		Since:     7 * 24 * time.Hour,
		Now:       func() time.Time { return time.Now() },
		Out:       new(bytes.Buffer),
	}
	if err := r.Run(context.Background()); err != nil {
		t.Errorf("Run with nil OnOutcome should not error, got %v", err)
	}
}

// TestOutcomeSinkDropsOnOverflow asserts Emit never blocks: when the queue
// is full the outcome is dropped, not back-pressured onto the resolver.
func TestOutcomeSinkDropsOnOverflow(t *testing.T) {
	release := make(chan struct{})
	var delivered int
	var mu sync.Mutex
	sink := NewOutcomeSink(1, func(_ Outcome) {
		<-release // block the single worker so the buffer saturates
		mu.Lock()
		delivered++
		mu.Unlock()
	})

	done := make(chan struct{})
	go func() {
		// Far more emits than capacity; must return promptly without blocking.
		for i := 0; i < 10_000; i++ {
			sink.Emit(OutcomeSuccess)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Emit blocked — queue-with-drop violated; resolver would stall")
	}
	close(release)
	sink.Close()

	mu.Lock()
	defer mu.Unlock()
	if delivered > 2 { // worker + one buffered, at most
		t.Errorf("want most outcomes dropped on overflow, delivered %d", delivered)
	}
}
