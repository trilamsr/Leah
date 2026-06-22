package obs

import "sync"

// ErrorRing retains the last N ERROR-level log lines for diag.state exposure.
// Thread-safe; zero value is not usable — construct with NewErrorRing.
type ErrorRing struct {
	mu   sync.Mutex
	buf  []string
	head int
	n    int
}

// NewErrorRing returns a ring buffer that holds the last cap ERROR lines.
func NewErrorRing(cap int) *ErrorRing {
	return &ErrorRing{buf: make([]string, cap)}
}

// Add records one error line. Overwrites oldest when full.
func (r *ErrorRing) Add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.head%len(r.buf)] = line
	r.head++
	if r.n < len(r.buf) {
		r.n++
	}
}

// Last returns the most recent line, or "" if empty.
func (r *ErrorRing) Last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.n == 0 {
		return ""
	}
	idx := (r.head - 1 + len(r.buf)) % len(r.buf)
	return r.buf[idx]
}
