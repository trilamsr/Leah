package main

import (
	"bytes"
	"errors"
	"testing"
)

// recordingSinkBacking captures writes + Sync calls so flushingSink behavior
// is observable without touching the real fd.
type recordingSinkBacking struct {
	buf     bytes.Buffer
	syncs   int
	syncErr error
}

func (r *recordingSinkBacking) Write(p []byte) (int, error) { return r.buf.Write(p) }
func (r *recordingSinkBacking) Sync() error {
	r.syncs++
	return r.syncErr
}

// TestFlushingSink_SyncsPerWrite guards finding #6: each chunk must call
// Sync so `leah review | tee log` shows progressive output instead of
// stdio-buffered batches.
func TestFlushingSink_SyncsPerWrite(t *testing.T) {
	back := &recordingSinkBacking{}
	s := newFlushingSink(back)
	_, _ = s.Write([]byte("chunk1"))
	_, _ = s.Write([]byte("chunk2"))
	if back.syncs != 2 {
		t.Errorf("want 2 syncs, got %d", back.syncs)
	}
	if back.buf.String() != "chunk1chunk2" {
		t.Errorf("buf: %q", back.buf.String())
	}
}

// TestFlushingSink_TracksTrailingNewline guards finding #6: the post-stream
// caller asks the sink whether the body already ended in '\n' so it can
// avoid a double blank line before "Verdict:".
func TestFlushingSink_TracksTrailingNewline(t *testing.T) {
	cases := []struct {
		name        string
		writes      []string
		wantNewline bool
	}{
		{"no writes", nil, false},
		{"no trailing newline", []string{"abc"}, false},
		{"trailing newline", []string{"abc\n"}, true},
		{"empty trailing write after newline", []string{"abc\n", ""}, true},
		{"newline then char", []string{"abc\n", "x"}, false},
		{"multibyte chunk ending newline", []string{"line1\nline2\n"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newFlushingSink(&recordingSinkBacking{})
			for _, w := range tc.writes {
				_, _ = s.Write([]byte(w))
			}
			if got := s.EndedInNewline(); got != tc.wantNewline {
				t.Errorf("EndedInNewline=%v want %v", got, tc.wantNewline)
			}
		})
	}
}

// TestFlushingSink_SyncErrorDoesNotFailWrite asserts that a Sync failure
// (e.g., stdout closed) does not bubble up — write returns success so the
// reviewer loop keeps making progress.
func TestFlushingSink_SyncErrorDoesNotFailWrite(t *testing.T) {
	back := &recordingSinkBacking{syncErr: errors.New("nope")}
	s := newFlushingSink(back)
	n, err := s.Write([]byte("x"))
	if err != nil {
		t.Errorf("write error: %v", err)
	}
	if n != 1 {
		t.Errorf("n: %d", n)
	}
}
