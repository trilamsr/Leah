package ipc

import (
	"bytes"
	"encoding/binary"
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

// Oversized header from a malicious/buggy peer must be rejected without
// allocating the declared body — otherwise the parser becomes a DoS vector.
func TestReadFrameRejectsOversizedHeader(t *testing.T) {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], MaxFrameBytes+1)
	if _, err := ReadFrame(bytes.NewReader(hdr[:])); err == nil {
		t.Fatal("expected oversized-header error, got nil")
	}
}

// Truncated body (header promises N bytes, peer sends fewer then closes)
// must surface as an error, not a zero-value Frame.
func TestReadFrameRejectsTruncatedBody(t *testing.T) {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 100)
	if _, err := ReadFrame(bytes.NewReader(append(hdr[:], []byte("short")...))); err == nil {
		t.Fatal("expected truncated-body error, got nil")
	}
}

func TestFrameKindsWidgetMountUpdateStaleErrorDismissToast(t *testing.T) {
	for _, k := range []string{"widget.mount", "widget.update", "widget.stale", "widget.error", "widget.dismiss", "widget.unmount", "notification.toast"} {
		f := Frame{Kind: k, TurnID: "t1", Seq: 1}
		b, err := json.Marshal(f)
		if err != nil {
			t.Fatalf("%s marshal: %v", k, err)
		}
		var got Frame
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("%s unmarshal: %v", k, err)
		}
		if got.Kind != k {
			t.Fatalf("kind round-trip: want %q got %q", k, got.Kind)
		}
	}
}
