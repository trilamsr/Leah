package ipc

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRoundTripFrame(t *testing.T) {
	in := Frame{Kind: "prose.delta", TurnID: "t1", Seq: 7, Payload: json.RawMessage(`{"text":"hi"}`)}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out.Kind != in.Kind || out.TurnID != in.TurnID || out.Seq != in.Seq {
		t.Fatalf("mismatch: got %+v want %+v", out, in)
	}
	if string(out.Payload) != string(in.Payload) {
		t.Fatalf("payload mismatch: %s vs %s", out.Payload, in.Payload)
	}
}

func TestFrameSizeCap(t *testing.T) {
	huge := Frame{Kind: "prose.delta", TurnID: "t1", Seq: 1, Payload: json.RawMessage(`"` + strings.Repeat("x", 300_000) + `"`)}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, huge); err == nil {
		t.Fatal("expected size-cap error, got nil")
	}
}
