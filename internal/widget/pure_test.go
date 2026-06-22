package widget

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPureAdapter_ValidateRejectsEmpty(t *testing.T) {
	a := NewPureAdapter("stat")
	if err := a.Validate(nil); err == nil {
		t.Fatalf("nil props must error")
	}
	if err := a.Validate(json.RawMessage(``)); err == nil {
		t.Fatalf("empty props must error")
	}
}

func TestPureAdapter_ValidateRejectsOversized(t *testing.T) {
	a := NewPureAdapter("stat")
	huge := bytes.Repeat([]byte("a"), MaxEnvelopeBytes+1)
	if err := a.Validate(huge); err == nil {
		t.Fatalf("oversized props must error")
	} else if !strings.Contains(err.Error(), "256") {
		t.Fatalf("err should reference 256 KB cap; got %v", err)
	}
}

func TestPureAdapter_ValidateAcceptsTypicalKinds(t *testing.T) {
	for _, kind := range []string{"stat", "table", "list", "code", "diff"} {
		a := NewPureAdapter(kind)
		if err := a.Validate(json.RawMessage(`{"k":"v"}`)); err != nil {
			t.Fatalf("kind=%s: %v", kind, err)
		}
		p, err := a.Fetch(context.Background(), json.RawMessage(`{"k":"v"}`))
		if err != nil {
			t.Fatalf("kind=%s fetch: %v", kind, err)
		}
		if p.Source != "pure" {
			t.Fatalf("kind=%s: want source pure got %q", kind, p.Source)
		}
		if p.FetchedAt.IsZero() {
			t.Fatalf("kind=%s: FetchedAt unset", kind)
		}
	}
}

func TestPureAdapter_RefreshNilPrevFallsBackToFetch(t *testing.T) {
	a := NewPureAdapter("stat")
	props := json.RawMessage(`{"x":1}`)
	p, err := a.Refresh(context.Background(), "id", props, nil)
	if err != nil {
		t.Fatalf("refresh nil prev: %v", err)
	}
	if string(p.Data) != string(props) {
		t.Fatalf("nil prev should re-fetch from props; got %s", p.Data)
	}
}
