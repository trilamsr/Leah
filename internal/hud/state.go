// Package hud is the operator-overlay surface state + IPC layer. See
// docs/engineer/specs/2026-06-10-hud-ui.md.
package hud

import (
	"embed"
	"io/fs"
	"sync"
)

//go:embed static
var staticRoot embed.FS

// Static returns the HUD static-asset filesystem rooted at `static/` so
// servers expose `/static/ambient.css` rather than `/static/static/...`.
func Static() fs.FS {
	sub, err := fs.Sub(staticRoot, "static")
	if err != nil {
		panic(err) // unreachable: //go:embed guarantees the dir.
	}
	return sub
}

// State is one of {hidden, ambient, focus}; single-surface invariant.
type State int

const (
	StateHidden State = iota
	StateAmbient
	StateFocus
)

func (s State) String() string {
	switch s {
	case StateHidden:
		return "hidden"
	case StateAmbient:
		return "ambient"
	case StateFocus:
		return "focus"
	default:
		return "unknown"
	}
}

// Machine is the HUD's surface state. Goroutine-safe; transitions are total
// (every input is defined for every state).
type Machine struct {
	mu sync.RWMutex
	s  State
}

func NewMachine() *Machine { return &Machine{s: StateHidden} }

func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.s
}

// Show: hidden→ambient. No-op from ambient/focus.
func (m *Machine) Show() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s == StateHidden {
		m.s = StateAmbient
	}
}

// Summon: hidden→focus or ambient→focus. No-op from focus.
func (m *Machine) Summon() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s != StateFocus {
		m.s = StateFocus
	}
}

// OnIdle: focus→ambient after 30s idle (caller drives the timer). No-op
// from ambient/hidden — idleness only collapses focus, never demotes
// ambient to hidden (operator opt-out is via Dismiss).
func (m *Machine) OnIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s == StateFocus {
		m.s = StateAmbient
	}
}

// Dismiss: *→hidden. Operator Esc or DND/screen-recording auto-hide.
func (m *Machine) Dismiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s = StateHidden
}
