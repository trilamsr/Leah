package reasoner

import "io"

// syncer is the optional Sync hook *os.File satisfies. When stdout is piped
// the Go runtime makes it fully buffered, so a per-chunk flush is the only
// way the operator sees tokens before EOF.
type syncer interface {
	Sync() error
}

// ChunkWriter wraps an io.Writer so streaming token writes both flush per
// chunk and end with exactly one trailing newline regardless of whether
// the last chunk already ended in \n. Returned by newChunkWriter.
type ChunkWriter struct {
	w      io.Writer
	wrote  bool
	lastNL bool
}

// NewChunkWriter wraps w so streaming token writes flush per chunk and end
// with exactly one trailing newline.
func NewChunkWriter(w io.Writer) *ChunkWriter { return &ChunkWriter{w: w} }

// WriteChunk writes p, flushes if the underlying writer supports Sync, and
// tracks whether the last byte was \n so Finish can decide.
func (c *ChunkWriter) WriteChunk(p string) (int, error) {
	if p == "" {
		return 0, nil
	}
	n, err := io.WriteString(c.w, p)
	if n > 0 {
		c.wrote = true
		c.lastNL = p[len(p)-1] == '\n'
	}
	if err != nil {
		return n, err
	}
	if s, ok := c.w.(syncer); ok {
		_ = s.Sync()
	}
	return n, nil
}

// Finish writes a trailing newline iff something was written and the last
// byte wasn't already \n. Empty streams stay empty so callers branching on
// "did we stream?" don't double-print.
func (c *ChunkWriter) Finish() error {
	if !c.wrote || c.lastNL {
		return nil
	}
	_, err := io.WriteString(c.w, "\n")
	return err
}
