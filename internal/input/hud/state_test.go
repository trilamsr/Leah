package hud

import "testing"

func TestHUDState_HiddenToAmbient_OnShow(t *testing.T) {
	m := NewMachine()
	if got := m.State(); got != StateHidden {
		t.Fatalf("initial state: got %v want %v", got, StateHidden)
	}
	m.Show()
	if got := m.State(); got != StateAmbient {
		t.Fatalf("after Show: got %v want %v", got, StateAmbient)
	}
}

func TestHUDState_AmbientToFocus_OnSummon(t *testing.T) {
	m := NewMachine()
	m.Show()
	m.Summon()
	if got := m.State(); got != StateFocus {
		t.Fatalf("after Summon from ambient: got %v want %v", got, StateFocus)
	}
}

func TestHUDState_FocusToAmbient_OnIdle30s(t *testing.T) {
	m := NewMachine()
	m.Show()
	m.Summon()
	m.OnIdle()
	if got := m.State(); got != StateAmbient {
		t.Fatalf("after OnIdle from focus: got %v want %v", got, StateAmbient)
	}
}

func TestHUDState_AnyToHidden_OnEsc(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Machine)
	}{
		{"from hidden", func(m *Machine) {}},
		{"from ambient", func(m *Machine) { m.Show() }},
		{"from focus", func(m *Machine) { m.Show(); m.Summon() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMachine()
			tc.setup(m)
			m.Dismiss()
			if got := m.State(); got != StateHidden {
				t.Fatalf("after Dismiss (%s): got %v want %v", tc.name, got, StateHidden)
			}
		})
	}
}

func TestHUDState_SummonFromHidden_GoesToFocus(t *testing.T) {
	m := NewMachine()
	m.Summon()
	if got := m.State(); got != StateFocus {
		t.Fatalf("Summon from hidden should jump to focus: got %v want %v", got, StateFocus)
	}
}

func TestHUDState_OnIdleFromAmbient_HidesAfterTimeout(t *testing.T) {
	m := NewMachine()
	m.Show()
	m.OnIdle()
	if got := m.State(); got != StateAmbient {
		t.Fatalf("OnIdle from ambient must NOT change ambient: got %v want %v", got, StateAmbient)
	}
}
