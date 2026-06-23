package a2a

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestEncodeDecodeRoundtrip asserts payload+kind+id survive CBOR.
func TestEncodeDecodeRoundtrip(t *testing.T) {
	in := Frame{
		ID:      NewID(),
		Kind:    KindAskRequest,
		Payload: []byte("hello world"),
	}
	b, err := Encode(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := Decode(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.V != ProtocolVersion {
		t.Fatalf("V: got %d want %d", got.V, ProtocolVersion)
	}
	if got.ID != in.ID {
		t.Fatalf("ID: got %q want %q", got.ID, in.ID)
	}
	if got.Kind != in.Kind {
		t.Fatalf("Kind: got %q want %q", got.Kind, in.Kind)
	}
	if !bytes.Equal(got.Payload, in.Payload) {
		t.Fatalf("Payload: got %x want %x", got.Payload, in.Payload)
	}
}

// TestDecodeUnknownKindRejected asserts the closed-set gate.
func TestDecodeUnknownKindRejected(t *testing.T) {
	b, err := Encode(Frame{ID: NewID(), Kind: KindAskRequest, Payload: []byte("x")})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Hand-craft a frame with an unknown kind via raw CBOR.
	bad := mustEncodeRaw(t, map[string]any{
		"v":       ProtocolVersion,
		"id":      "deadbeef",
		"kind":    "evil.kind",
		"payload": []byte{},
	})
	if _, err := Decode(bad); err == nil {
		t.Fatal("expected error decoding unknown kind")
	}
	if _, err := Decode(b); err != nil {
		t.Fatalf("valid frame still must decode: %v", err)
	}
}

// TestDecodeVersionMismatch asserts v1-only gate.
func TestDecodeVersionMismatch(t *testing.T) {
	bad := mustEncodeRaw(t, map[string]any{
		"v":       99,
		"id":      "deadbeef",
		"kind":    string(KindHelloOffer),
		"payload": []byte{},
	})
	_, err := Decode(bad)
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
	if !strings.Contains(err.Error(), "protocol version mismatch") {
		t.Fatalf("err shape: %v", err)
	}
}

// TestCheckTaskDepthRejectsLoop asserts §5.7 loop protection.
func TestCheckTaskDepthRejectsLoop(t *testing.T) {
	cases := []struct {
		name    string
		kind    FrameKind
		depth   int
		wantErr error
	}{
		{"offer-depth-0", KindTaskOffer, 0, nil},
		{"offer-depth-2", KindTaskOffer, 2, nil},
		{"offer-depth-3", KindTaskOffer, 3, ErrDepthExceeded},
		{"accept-depth-3", KindTaskAccept, 3, ErrDepthExceeded},
		{"ask-depth-99", KindAskRequest, 99, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckTaskDepth(Frame{Kind: tc.kind, Depth: tc.depth})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v want %v", err, tc.wantErr)
			}
		})
	}
}

// TestEncodeRejectsEmptyID asserts ID required.
func TestEncodeRejectsEmptyID(t *testing.T) {
	if _, err := Encode(Frame{Kind: KindHelloOffer, Payload: []byte{}}); err == nil {
		t.Fatal("expected empty-ID error")
	}
}

// TestCapabilitySetHas asserts bitfield semantics.
func TestCapabilitySetHas(t *testing.T) {
	c := CapAsk | CapMemorySearch
	if !c.Has(CapAsk) {
		t.Fatal("expected CapAsk set")
	}
	if !c.Has(CapAsk | CapMemorySearch) {
		t.Fatal("expected both bits set")
	}
	if c.Has(CapTaskOffload) {
		t.Fatal("CapTaskOffload should be unset")
	}
}

// TestNewIDUnique asserts 128-bit randomness gives distinct IDs.
func TestNewIDUnique(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 1000; i++ {
		id := NewID()
		if _, dup := seen[id]; dup {
			t.Fatalf("dup at i=%d: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}

func mustEncodeRaw(t *testing.T, m map[string]any) []byte {
	t.Helper()
	// Reuse the Encode path's CBOR encoder via a helper Frame is too rigid for —
	// build via the cbor package directly.
	b, err := rawCBOR(m)
	if err != nil {
		t.Fatalf("rawCBOR: %v", err)
	}
	return b
}
