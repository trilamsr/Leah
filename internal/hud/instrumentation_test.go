package hud

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
)

func TestStateInstrumentation_RecordsStateToWidgetLatency(t *testing.T) {
	reg := obs.NewRegistry()
	m := NewMachine()
	BindStateInstrumentation(m, NewStateInstrumentation(reg))

	m.Show() // hidden → ambient marks pending-since
	time.Sleep(20 * time.Millisecond)
	m.MarkRendered("ambient") // widget frame shipped

	var buf bytes.Buffer
	if err := reg.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "leah_hud_state_to_widget_seconds_count") {
		t.Fatalf("expected leah_hud_state_to_widget_seconds histogram in prom output, got:\n%s", out)
	}
	if !strings.Contains(out, `state="ambient"`) {
		t.Fatalf("expected state=\"ambient\" label, got:\n%s", out)
	}
	if !strings.Contains(out, `leah_hud_state_to_widget_seconds_count{state="ambient"} 1`) {
		t.Fatalf("expected exactly one ambient observation, got:\n%s", out)
	}
}

func TestStateInstrumentation_NilSafe(t *testing.T) {
	m := NewMachine()
	BindStateInstrumentation(m, nil) // nil registry path — production may omit
	m.Show()
	m.MarkRendered("ambient") // must not panic
}

func TestStateInstrumentation_RenderWithoutMarkIsNoOp(t *testing.T) {
	reg := obs.NewRegistry()
	m := NewMachine()
	BindStateInstrumentation(m, NewStateInstrumentation(reg))

	// No state change → no pending timestamp → render must not record.
	m.MarkRendered("ambient")

	var buf bytes.Buffer
	_ = reg.WritePrometheus(&buf)
	if strings.Contains(buf.String(), `leah_hud_state_to_widget_seconds_count{state="ambient"} 1`) {
		t.Fatalf("unmarked render leaked an observation:\n%s", buf.String())
	}
}

// Unbound Machine: production may construct without calling Bind*.
func TestStateInstrumentation_UnboundMachineMarkRenderedDoesNotPanic(t *testing.T) {
	m := NewMachine()
	m.Show()
	m.MarkRendered("ambient")
}

func TestStateInstrumentation_LatchReplacedByLaterTransition(t *testing.T) {
	reg := obs.NewRegistry()
	m := NewMachine()
	BindStateInstrumentation(m, NewStateInstrumentation(reg))

	m.Show()                          // hidden → ambient
	time.Sleep(15 * time.Millisecond) // older ambient latch
	m.Summon()                        // ambient → focus
	m.MarkRendered("focus")           // measures from Summon

	var buf bytes.Buffer
	_ = reg.WritePrometheus(&buf)
	out := buf.String()
	if !strings.Contains(out, `leah_hud_state_to_widget_seconds_count{state="focus"} 1`) {
		t.Fatalf("expected focus observation, got:\n%s", out)
	}
}

// Redundant Dismiss on already-hidden Machine must not latch.
func TestStateInstrumentation_RedundantDismissIsNoOp(t *testing.T) {
	reg := obs.NewRegistry()
	m := NewMachine()
	BindStateInstrumentation(m, NewStateInstrumentation(reg))

	m.Dismiss()
	m.MarkRendered("hidden")

	var buf bytes.Buffer
	_ = reg.WritePrometheus(&buf)
	if strings.Contains(buf.String(), `leah_hud_state_to_widget_seconds_count{state="hidden"} 1`) {
		t.Fatalf("redundant Dismiss leaked an observation:\n%s", buf.String())
	}
}

func TestStateInstrumentation_OnIdleFocusToAmbient(t *testing.T) {
	reg := obs.NewRegistry()
	m := NewMachine()
	BindStateInstrumentation(m, NewStateInstrumentation(reg))

	m.Show()
	m.Summon()
	m.MarkRendered("focus")
	time.Sleep(10 * time.Millisecond)
	m.OnIdle() // focus → ambient
	m.MarkRendered("ambient")

	var buf bytes.Buffer
	_ = reg.WritePrometheus(&buf)
	if !strings.Contains(buf.String(), `leah_hud_state_to_widget_seconds_count{state="ambient"} 1`) {
		t.Fatalf("expected ambient observation after OnIdle, got:\n%s", buf.String())
	}
}
