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
