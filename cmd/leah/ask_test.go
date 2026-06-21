package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// fakeAskStreamer scripts a delta channel for runAskWith — the test mimics
// what reasoner.AskStream returns to runAsk in production.
type fakeAskStreamer struct {
	deltas []string
	err    error
}

func (f *fakeAskStreamer) AskStream(ctx context.Context, user string) (<-chan string, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(chan string, len(f.deltas))
	go func() {
		defer close(out)
		for _, d := range f.deltas {
			select {
			case <-ctx.Done():
				return
			case out <- d:
			}
		}
	}()
	return out, nil
}

// trackingWriter records every Write call separately so the test can prove
// each delta hit stdout as its own Write (the firstByteWriter contract:
// each delta is a Write, not a flush at end).
type trackingWriter struct {
	mu     sync.Mutex
	writes []string
}

func (t *trackingWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.writes = append(t.writes, string(p))
	return len(p), nil
}

func (t *trackingWriter) joined() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.writes, "")
}

func TestRunAskWith_StreamsDeltasInOrder(t *testing.T) {
	s := &fakeAskStreamer{deltas: []string{"Hel", "lo ", "world."}}
	var buf bytes.Buffer
	code := runAskWith(context.Background(), s, &buf, "say hi")
	if code != 0 {
		t.Fatalf("runAskWith = %d; want 0", code)
	}
	got := buf.String()
	want := "Hello world.\n"
	if got != want {
		t.Fatalf("stdout = %q; want %q", got, want)
	}
}

func TestRunAskWith_TrailingNewlineOnce(t *testing.T) {
	s := &fakeAskStreamer{deltas: []string{"done."}}
	var buf bytes.Buffer
	if code := runAskWith(context.Background(), s, &buf, "q"); code != 0 {
		t.Fatalf("runAskWith = %d; want 0", code)
	}
	got := buf.String()
	if strings.HasSuffix(got, "\n\n") {
		t.Fatalf("double trailing newline in %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("missing trailing newline in %q", got)
	}
}

func TestRunAskWith_DeltasArriveAsSeparateWrites(t *testing.T) {
	s := &fakeAskStreamer{deltas: []string{"a", "b", "c"}}
	w := &trackingWriter{}
	if code := runAskWith(context.Background(), s, w, "q"); code != 0 {
		t.Fatalf("runAskWith = %d; want 0", code)
	}
	// Three delta writes + one trailing-newline write — the firstByteWriter
	// contract relies on each delta hitting Write individually so the first
	// model token trips the first-byte sniffer (if/when wrapped).
	if len(w.writes) < 3 {
		t.Fatalf("got %d writes (%q); want at least 3 (one per delta)", len(w.writes), w.writes)
	}
	if w.joined() != "abc\n" {
		t.Fatalf("joined writes = %q; want %q", w.joined(), "abc\n")
	}
	// Order check — independent of write coalescing, the textual sequence
	// must match the delta order.
	concat := ""
	for _, s := range w.writes {
		concat += s
	}
	if !strings.HasPrefix(concat, "abc") {
		t.Fatalf("delta order broken: writes=%q", w.writes)
	}
}

func TestRunAskWith_StreamErrorReturnsOne(t *testing.T) {
	s := &fakeAskStreamer{err: errors.New("boom")}
	var buf bytes.Buffer
	if code := runAskWith(context.Background(), s, &buf, "q"); code != 1 {
		t.Fatalf("runAskWith on stream-open error = %d; want 1", code)
	}
}
