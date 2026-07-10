package hud

import (
	"sync"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
)

// A5 1s p95 — sub-second resolution with a 2s tail for regressions.
var stateToWidgetBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2}

type StateInstrumentation struct {
	hist *telemetry.Histogram
	mu   sync.Mutex
	mark map[State]time.Time
}

func NewStateInstrumentation(reg *telemetry.Registry) *StateInstrumentation {
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

func BindStateInstrumentation(m *Machine, s *StateInstrumentation) {
	if m == nil {
		return
	}
	m.instr = s
}

// MarkRendered observes elapsed since the latest transition into label's state
// and clears the latch — stable-state re-renders don't double-count.
func (m *Machine) MarkRendered(label string) {
	if m == nil {
		return
	}
	m.instr.observeRender(label, parseState(label), time.Now())
}

func parseState(label string) State {
	switch label {
	case "ambient":
		return StateAmbient
	case "focus":
		return StateFocus
	default:
		return StateHidden
	}
}
