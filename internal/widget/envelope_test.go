package widget

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnvelopeMustHaveWidgetIDSize(t *testing.T) {
	bad := `{"widget":"market"}`
	var e Envelope
	if err := json.Unmarshal([]byte(bad), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := e.Validate(); err == nil {
		t.Fatalf("expected validation error on missing id+size; got nil")
	}
}

func TestEnvelopeRejectsUnknownWidgetKind(t *testing.T) {
	bad := `{"widget":"hologram","id":"a","size":"small"}`
	var e Envelope
	_ = json.Unmarshal([]byte(bad), &e)
	if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "hologram") {
		t.Fatalf("expected reject unknown kind; got %v", err)
	}
}

func TestEnvelopeCap256KB(t *testing.T) {
	big := make([]byte, 257*1024)
	for i := range big {
		big[i] = 'x'
	}
	e := Envelope{Widget: "code", ID: "a", Size: "medium", Props: big}
	if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "256") {
		t.Fatalf("expected 256 KB cap error; got %v", err)
	}
}

func TestEnvelopeAcceptsHeroSize(t *testing.T) {
	e := Envelope{Widget: "chart", ID: "a", Size: "hero"}
	if err := e.Validate(); err != nil {
		t.Fatalf("hero size rejected: %v", err)
	}
}

func TestEnvelopeRefreshMinimum(t *testing.T) {
	low := 4
	e := Envelope{Widget: "market", ID: "a", Size: "small", Refresh: &low}
	if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "minimum") {
		t.Fatalf("expected refresh<5 error; got %v", err)
	}
	ok := 30
	e.Refresh = &ok
	if err := e.Validate(); err != nil {
		t.Fatalf("refresh=30 rejected: %v", err)
	}
}

func TestEnvelopeActionShape(t *testing.T) {
	payload := `{"widget":"market","id":"a","size":"small","actions":[{"label":"Refresh","callback":"leah://action/refresh","icon":"arrow.clockwise"}]}`
	var e Envelope
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(e.Actions) != 1 || e.Actions[0].Label != "Refresh" || e.Actions[0].Callback != "leah://action/refresh" {
		t.Fatalf("action object not parsed: %+v", e.Actions)
	}
	if err := e.Validate(); err != nil {
		t.Fatalf("valid action rejected: %v", err)
	}
	e.Actions = []Action{{Label: "", Callback: "leah://action/x"}}
	if err := e.Validate(); err == nil {
		t.Fatal("expected reject action with empty label")
	}
}
