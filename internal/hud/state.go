// Package hud is the operator-overlay surface state + IPC layer. See
// docs/engineer/specs/2026-06-10-hud-ui.md.
package hud

import (
	"embed"
	"io/fs"
	"sync"
	"time"
)

//go:embed static
var staticRoot embed.FS

func Static() fs.FS {
	sub, err := fs.Sub(staticRoot, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

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

type Machine struct {
	mu    sync.RWMutex
	s     State
	instr *StateInstrumentation
}

func NewMachine() *Machine { return &Machine{s: StateHidden} }

func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.s
}

// transitionTo stamps the A5 instrumentation latch on the destination state so
// the next widget render at that state observes elapsed. Caller holds m.mu.
func (m *Machine) transitionTo(to State) {
	m.s = to
	m.instr.markChanged(to, time.Now())
}

func (m *Machine) Show() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s == StateHidden {
		m.transitionTo(StateAmbient)
	}
}

func (m *Machine) Summon() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s != StateFocus {
		m.transitionTo(StateFocus)
	}
}

// OnIdle collapses focus → ambient; never demotes ambient (spec §2).
func (m *Machine) OnIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s == StateFocus {
		m.transitionTo(StateAmbient)
	}
}

func (m *Machine) Dismiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.s != StateHidden {
		m.transitionTo(StateHidden)
	}
}
