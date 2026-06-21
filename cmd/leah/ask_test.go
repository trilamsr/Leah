package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/budget"
)

// fakeAskStreamer scripts a delta channel for runAskWith — mirrors what
// reasoner.AskStream returns to runAsk in production.
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

// trackingWriter records every Write call separately — proves each delta
// hit stdout as its own Write (the firstByteWriter A6 contract).
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

// failingWriter simulates a closed pipe — fails on the Nth Write call.
type failingWriter struct {
	failOn int
	calls  int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	f.calls++
	if f.calls == f.failOn {
		return 0, errors.New("pipe closed")
	}
	return len(p), nil
}

func newTestAudit(t *testing.T) *audit.Logger {
	t.Helper()
	return &audit.Logger{Path: filepath.Join(t.TempDir(), "audit.jsonl")}
}

func readAuditEntries(t *testing.T, a *audit.Logger) []audit.Entry {
	t.Helper()
	raw, err := os.ReadFile(a.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read audit: %v", err)
	}
	var entries []audit.Entry
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var e audit.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal audit row %q: %v", line, err)
		}
		entries = append(entries, e)
	}
	return entries
}

func TestRunAskWith_StreamsDeltasInOrder(t *testing.T) {
	s := &fakeAskStreamer{deltas: []string{"Hel", "lo ", "world."}}
	var buf bytes.Buffer
	a := newTestAudit(t)
	b := &budget.Budget{Ceiling: 1.0}
	code := runAskWith(context.Background(), s, &buf, a, b, "say hi")
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
	a := newTestAudit(t)
	b := &budget.Budget{Ceiling: 1.0}
	if code := runAskWith(context.Background(), s, &buf, a, b, "q"); code != 0 {
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
	a := newTestAudit(t)
	b := &budget.Budget{Ceiling: 1.0}
	if code := runAskWith(context.Background(), s, w, a, b, "q"); code != 0 {
		t.Fatalf("runAskWith = %d; want 0", code)
	}
	if len(w.writes) < 3 {
		t.Fatalf("got %d writes (%q); want at least 3 (one per delta)", len(w.writes), w.writes)
	}
	if w.joined() != "abc\n" {
		t.Fatalf("joined writes = %q; want %q", w.joined(), "abc\n")
	}
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
	a := newTestAudit(t)
	b := &budget.Budget{Ceiling: 1.0}
	if code := runAskWith(context.Background(), s, &buf, a, b, "q"); code != 1 {
		t.Fatalf("runAskWith on stream-open error = %d; want 1", code)
	}
	entries := readAuditEntries(t, a)
	if len(entries) != 1 || entries[0].Outcome != "failed" {
		t.Fatalf("audit on stream error = %+v; want one failed entry", entries)
	}
}

func TestRunAskWith_WriteErrorReturnsOne(t *testing.T) {
	s := &fakeAskStreamer{deltas: []string{"a", "b", "c"}}
	w := &failingWriter{failOn: 2}
	a := newTestAudit(t)
	b := &budget.Budget{Ceiling: 1.0}
	if code := runAskWith(context.Background(), s, w, a, b, "q"); code != 1 {
		t.Fatalf("runAskWith on mid-stream write error = %d; want 1", code)
	}
	entries := readAuditEntries(t, a)
	if len(entries) != 1 || entries[0].Outcome != "failed" {
		t.Fatalf("audit on write error = %+v; want one failed entry", entries)
	}
}

func TestRunAskWith_TrailingNewlineWriteErrorReturnsOne(t *testing.T) {
	s := &fakeAskStreamer{deltas: []string{"hello"}}
	w := &failingWriter{failOn: 2} // first delta writes ok, trailing \n fails
	a := newTestAudit(t)
	b := &budget.Budget{Ceiling: 1.0}
	if code := runAskWith(context.Background(), s, w, a, b, "q"); code != 1 {
		t.Fatalf("runAskWith on trailing-newline error = %d; want 1 (consistent with delta-write errors)", code)
	}
	entries := readAuditEntries(t, a)
	if len(entries) != 1 || entries[0].Outcome != "failed" {
		t.Fatalf("audit on trailing-newline error = %+v; want one failed entry", entries)
	}
}

func TestRunAskWith_SuccessWritesAuditRow(t *testing.T) {
	s := &fakeAskStreamer{deltas: []string{"ok"}}
	var buf bytes.Buffer
	a := newTestAudit(t)
	b := &budget.Budget{Ceiling: 1.0}
	if code := runAskWith(context.Background(), s, &buf, a, b, "q"); code != 0 {
		t.Fatalf("runAskWith = %d; want 0", code)
	}
	entries := readAuditEntries(t, a)
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d; want 1", len(entries))
	}
	e := entries[0]
	if e.Kind != "ask" || e.Outcome != "success" || e.ArgsHash == "" {
		t.Fatalf("audit row = %+v; want kind=ask outcome=success non-empty hash", e)
	}
}
