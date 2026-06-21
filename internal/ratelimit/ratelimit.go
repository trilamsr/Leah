// Package ratelimit is a per-key sliding-window limiter; gate before attestation so a rejected call never consumes a consent prompt.
package ratelimit

import (
	"sync"
	"time"
)

type Window struct {
	window time.Duration
	budget int
	now    func() time.Time

	mu     sync.Mutex
	hits   map[string][]time.Time
	denied int
}

// Stat is a read-only snapshot of one limiter. Sends is the count still inside
// the window (all adapters use a 60s window, so this is the rolling 1-minute
// rate); Denied is cumulative rejections since construction.
type Stat struct {
	Sends  int
	Denied int
}

// NewWindow admits at most budget calls per key per window. A nil now uses time.Now.
func NewWindow(window time.Duration, budget int, now func() time.Time) *Window {
	if now == nil {
		now = time.Now
	}
	return &Window{window: window, budget: budget, now: now, hits: map[string][]time.Time{}}
}

func (w *Window) Allow(key string) bool {
	now := w.now()
	cutoff := now.Add(-w.window)
	w.mu.Lock()
	defer w.mu.Unlock()
	kept := w.hits[key][:0]
	for _, t := range w.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	w.hits[key] = kept
	if len(kept) >= w.budget {
		w.denied++
		return false
	}
	w.hits[key] = append(kept, now)
	return true
}

// Stats prunes expired hits across all keys, then returns the live send count
// plus cumulative denials. The dashboard spam widget reads this each poll.
func (w *Window) Stats() Stat {
	cutoff := w.now().Add(-w.window)
	w.mu.Lock()
	defer w.mu.Unlock()
	sends := 0
	for key, ts := range w.hits {
		kept := ts[:0]
		for _, t := range ts {
			if t.After(cutoff) {
				kept = append(kept, t)
			}
		}
		w.hits[key] = kept
		sends += len(kept)
	}
	return Stat{Sends: sends, Denied: w.denied}
}
