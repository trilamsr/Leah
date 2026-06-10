package session

import "strings"

// SentenceChunker accumulates streaming text deltas and emits a chunk when
// the buffer ends in a [.!?] followed by whitespace AND contains ≥MinChars
// speakable characters (ignoring leading whitespace). MinChars defaults to
// 12 — short utterances like "OK." amortize poorly against TTS spin-up.
//
// Not goroutine-safe; the streaming consumer owns one chunker per turn.
type SentenceChunker struct {
	MinChars int
	buf      strings.Builder
}

// Push appends delta to the buffer; if the buffer now ends in a sentence
// terminator + whitespace and contains ≥MinChars speakable chars, returns
// (chunk, true) and resets the buffer. Otherwise returns ("", false).
func (c *SentenceChunker) Push(delta string) (string, bool) {
	c.buf.WriteString(delta)
	s := c.buf.String()
	if len(s) < 2 {
		return "", false
	}
	// Walk backwards to find the last sentence terminator followed by
	// whitespace. We emit only at the latest boundary so partial trailing
	// fragments stay buffered for the next delta.
	for i := len(s) - 1; i >= 1; i-- {
		if !isSpace(s[i]) {
			continue
		}
		prev := s[i-1]
		if prev != '.' && prev != '!' && prev != '?' {
			continue
		}
		// Speakable-char floor on the candidate chunk.
		chunk := s[:i+1]
		if speakableLen(chunk) < c.minChars() {
			return "", false
		}
		c.buf.Reset()
		c.buf.WriteString(s[i+1:])
		return chunk, true
	}
	return "", false
}

// Flush returns whatever remains in the buffer and resets. Empty if nothing
// is buffered.
func (c *SentenceChunker) Flush() string {
	s := c.buf.String()
	c.buf.Reset()
	return strings.TrimSpace(s)
}

func (c *SentenceChunker) minChars() int {
	if c.MinChars <= 0 {
		return 12
	}
	return c.MinChars
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

func speakableLen(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		n++
	}
	return n
}
