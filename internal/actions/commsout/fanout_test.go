package commsout

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/trilam/leah/internal/platform/contracts"
)

type fakeNotifier struct {
	calls []string
	err   error
}

func (f *fakeNotifier) Notify(_ context.Context, title, body string) error {
	f.calls = append(f.calls, title+"|"+body)
	return f.err
}

// TestFanoutCallsAllNotifiers asserts every wrapped Notifier sees the call.
func TestFanoutCallsAllNotifiers(t *testing.T) {
	a := &fakeNotifier{}
	b := &fakeNotifier{}
	c := &fakeNotifier{}
	f := &Fanout{Notifiers: []contracts.Notifier{a, b, c}}
	if err := f.Notify(context.Background(), "T", "B"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	for i, n := range []*fakeNotifier{a, b, c} {
		if len(n.calls) != 1 || n.calls[0] != "T|B" {
			t.Errorf("notifier %d: got %v", i, n.calls)
		}
	}
}

// TestFanoutReturnsJoinedErrors asserts partial failures still call every
// notifier and surface all errors via errors.Join.
func TestFanoutReturnsJoinedErrors(t *testing.T) {
	e1 := errors.New("boom1")
	e2 := errors.New("boom2")
	a := &fakeNotifier{err: e1}
	b := &fakeNotifier{}
	c := &fakeNotifier{err: e2}
	f := &Fanout{Notifiers: []contracts.Notifier{a, b, c}}
	err := f.Notify(context.Background(), "T", "B")
	if err == nil {
		t.Fatal("want joined error, got nil")
	}
	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Errorf("want both e1+e2, got %v", err)
	}
	// b still called despite a's failure.
	if len(b.calls) != 1 {
		t.Errorf("b not called: %v", b.calls)
	}
	// Sanity check on error formatting (errors.Join newline-separates).
	if !strings.Contains(err.Error(), "boom1") || !strings.Contains(err.Error(), "boom2") {
		t.Errorf("missing error text: %v", err)
	}
}

// TestFanoutEmptyReturnsNil asserts no notifiers = no error (cheap guard for
// composition roots that conditionally build the slice).
func TestFanoutEmptyReturnsNil(t *testing.T) {
	f := &Fanout{}
	if err := f.Notify(context.Background(), "T", "B"); err != nil {
		t.Errorf("empty fanout: %v", err)
	}
}

// TestFanoutSkipsNilEntries asserts a nil Notifier in the slice is skipped
// (no panic) and surrounding notifiers still fire — defensive guard for
// composition roots that conditionally build the slice from optional deps.
func TestFanoutSkipsNilEntries(t *testing.T) {
	a := &fakeNotifier{}
	b := &fakeNotifier{}
	f := &Fanout{Notifiers: []contracts.Notifier{a, nil, b}}
	if err := f.Notify(context.Background(), "T", "B"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(a.calls) != 1 || len(b.calls) != 1 {
		t.Errorf("a calls=%d b calls=%d (want 1/1)", len(a.calls), len(b.calls))
	}
}

// TestFanoutPreservesOrder asserts notifiers fire in slice order — godoc
// promises cheap-first / expensive-last scheduling, this is the executable
// proof.
func TestFanoutPreservesOrder(t *testing.T) {
	var calls []int
	mk := func(id int) contracts.Notifier {
		return notifierFunc(func(_ context.Context, _, _ string) error {
			calls = append(calls, id)
			return nil
		})
	}
	f := &Fanout{Notifiers: []contracts.Notifier{mk(1), mk(2), mk(3)}}
	if err := f.Notify(context.Background(), "T", "B"); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(calls) != 3 || calls[0] != 1 || calls[1] != 2 || calls[2] != 3 {
		t.Errorf("order: got %v want [1 2 3]", calls)
	}
}

type notifierFunc func(context.Context, string, string) error

func (n notifierFunc) Notify(ctx context.Context, title, body string) error {
	return n(ctx, title, body)
}
