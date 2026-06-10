package main

import "io"

// syncer is the subset of *os.File the flushingSink needs — extracted so
// tests can substitute a buffer without touching the real fd.
type syncer interface {
	io.Writer
	Sync() error
}

// flushingSink wraps the reviewer's token sink so each chunk is flushed
// (per finding #6 on PR #231: piped `leah review | tee log` was batching
// stdio until the process ended) and tracks whether the body ended in '\n'
// — so the caller can avoid printing a duplicate blank line before the
// Verdict: footer when the stream already ended with one.
type flushingSink struct {
	w             syncer
	endedNewline  bool
	receivedBytes bool
}

func newFlushingSink(w syncer) *flushingSink { return &flushingSink{w: w} }

func (s *flushingSink) Write(p []byte) (int, error) {
	if len(p) == 0 {
		// No bytes => no change to last-byte tracking; still surface a Sync
		// so a trailing zero-length flush call has its expected effect.
		_ = s.w.Sync()
		return 0, nil
	}
	n, err := s.w.Write(p)
	s.receivedBytes = true
	s.endedNewline = p[len(p)-1] == '\n'
	_ = s.w.Sync()
	return n, err
}

// EndedInNewline reports whether the last non-empty write ended with '\n'.
// False when no bytes were ever written.
func (s *flushingSink) EndedInNewline() bool {
	return s.receivedBytes && s.endedNewline
}
