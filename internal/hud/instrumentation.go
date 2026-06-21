package hud

import (
	"sync"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// stateToWidgetBuckets — A5 target is ≤1s p95, so buckets cluster sub-second
// with a 2s tail to catch regressions without inflating cardinality.
var stateToWidgetBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2}

// StateInstrumentation owns the state-change → widget-render latency series.
// Per-state latch: mutating state stamps a pending-since timestamp; the next
// render at that state observes elapsed and clears the latch so re-renders
// during a stable state don't double-count.
type StateInstrumentation struct {
	hist *obs.Histogram
	mu   sync.Mutex
	mark map[State]time.Time
}

// NewStateInstrumentation registers the histogram. nil registry returns nil so
// callers can wire unconditionally.
func NewStateInstrumentation(reg *obs.Registry) *StateInstrumentation {
	if reg == nil {
		return nil
	}
	return &StateInstrumentation{
		hist: reg.Histogram("leah_hud_state_to_widget_seconds", stateToWidgetBuckets),
		mark: map[State]time.Time{},
	}
}

func (s *StateInstrumentation) markChanged(to State, at time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.mark[to] = at
	s.mu.Unlock()
}

func (s *StateInstrumentation) observeRender(label string, to State, at time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	started, ok := s.mark[to]
	delete(s.mark, to)
	s.mu.Unlock()
	if !ok {
		return
	}
	s.hist.Observe(map[string]string{"state": label}, at.Sub(started).Seconds())
}

// BindStateInstrumentation wires Machine mutations to the histogram. Calls are
// nil-safe on both sides so production wiring may pass a nil registry without
// per-call guards.
func BindStateInstrumentation(m *Machine, s *StateInstrumentation) {
	if m == nil {
		return
	}
	m.instr = s
}

// MarkRendered observes elapsed since the most recent transition into label's
// state. Caller invokes this when a widget frame for that state actually
// ships (SSE flush, /api/state response). No-op if the state hasn't changed
// since the last render — re-rendering a stable state isn't a transition.
func (m *Machine) MarkRendered(label string) {
	if m == nil {
		return
	}
	to := parseState(label)
	m.instr.observeRender(label, to, time.Now())
}

func parseState(label string) State {
	switch label {
	case "ambient":
		return StateAmbient
	case "focus":
		return StateFocus
	case "hidden":
		return StateHidden
	default:
		return StateHidden
	}
}
