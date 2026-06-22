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
