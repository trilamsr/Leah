package a2a

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// FrameKind is the §5.4 wire-level message discriminator. The const-set below
// is the closed universe of v1 kinds — unknown kinds are rejected on decode.
type FrameKind string

const (
	KindHelloOffer     FrameKind = "hello.offer"
	KindHelloAck       FrameKind = "hello.ack"
	KindIdentityProve  FrameKind = "identity.prove"
	KindIdentityVerify FrameKind = "identity.verify"
	KindAskRequest     FrameKind = "ask.request"
	KindAskPartial     FrameKind = "ask.partial"
	KindAskEnd         FrameKind = "ask.end"
	KindMemorySearch   FrameKind = "memory.search"
	KindMemoryResult   FrameKind = "memory.result"
	KindTaskOffer      FrameKind = "task.offer"
	KindTaskAccept     FrameKind = "task.accept"
	KindTaskReject     FrameKind = "task.reject"
	KindConsentRequire FrameKind = "consent.require"
	KindConsentGrant   FrameKind = "consent.grant"
	KindConsentDeny    FrameKind = "consent.deny"
	KindBye            FrameKind = "bye"
)

// ProtocolVersion marks v1 of the wire. Bumped on breaking wire changes.
const ProtocolVersion = 1

// MaxFrameBytes caps each CBOR frame. Spec §5.7 trusts the peer pubkey, not
// the network — bounded frames keep a flooded peer from exhausting memory.
const MaxFrameBytes = 1 << 20

// MaxTaskDepth is the §5.7 loop-protection ceiling — rejects A→B→A→B before
// it amplifies.
const MaxTaskDepth = 2

// CapabilitySet is the §5.4 feature-bit bitfield exchanged during hello.
type CapabilitySet uint64

const (
	CapAsk          CapabilitySet = 1 << 0
	CapMemorySearch CapabilitySet = 1 << 1
	CapTaskOffload  CapabilitySet = 1 << 2
	CapConsentFlow  CapabilitySet = 1 << 3
)

// Has reports whether every bit in want is set on c.
func (c CapabilitySet) Has(want CapabilitySet) bool {
	return c&want == want
}

// Frame is the §5.4 wire envelope. CBOR-encoded; Depth carries the loop-
// protection counter for task.* kinds (zero for other kinds).
type Frame struct {
	V       int       `cbor:"v"`
	ID      string    `cbor:"id"`
	Kind    FrameKind `cbor:"kind"`
	Depth   int       `cbor:"depth,omitempty"`
	Payload []byte    `cbor:"payload"`
}

// NewID returns a fresh hex-encoded 128-bit frame ID.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("a2a: rand.Read: %v", err))
	}
	return hex.EncodeToString(b[:])
}

var validKinds = map[FrameKind]struct{}{
	KindHelloOffer:     {},
	KindHelloAck:       {},
	KindIdentityProve:  {},
	KindIdentityVerify: {},
	KindAskRequest:     {},
	KindAskPartial:     {},
	KindAskEnd:         {},
	KindMemorySearch:   {},
	KindMemoryResult:   {},
	KindTaskOffer:      {},
	KindTaskAccept:     {},
	KindTaskReject:     {},
	KindConsentRequire: {},
	KindConsentGrant:   {},
	KindConsentDeny:    {},
	KindBye:            {},
}

// ErrFrameTooLarge fires when an encoded or supplied frame exceeds MaxFrameBytes.
var ErrFrameTooLarge = fmt.Errorf("a2a: frame exceeds %d bytes", MaxFrameBytes)

// ErrDepthExceeded fires when a task.* frame's depth exceeds MaxTaskDepth.
var ErrDepthExceeded = fmt.Errorf("a2a: task depth exceeds %d", MaxTaskDepth)

// Encode serialises f to CBOR.
func Encode(f Frame) ([]byte, error) {
	if f.V == 0 {
		f.V = ProtocolVersion
	}
	if _, ok := validKinds[f.Kind]; !ok {
		return nil, fmt.Errorf("a2a: invalid kind %q", f.Kind)
	}
	if f.ID == "" {
		return nil, fmt.Errorf("a2a: empty frame ID")
	}
	if f.Depth < 0 {
		return nil, fmt.Errorf("a2a: negative depth %d", f.Depth)
	}
	b, err := cbor.Marshal(f)
	if err != nil {
		return nil, fmt.Errorf("a2a: encode: %w", err)
	}
	if len(b) > MaxFrameBytes {
		return nil, ErrFrameTooLarge
	}
	return b, nil
}

// Decode parses a CBOR-encoded frame, rejecting unknown kinds, version drift,
// negative depth, and oversize input.
func Decode(b []byte) (Frame, error) {
	var f Frame
	if len(b) > MaxFrameBytes {
		return f, ErrFrameTooLarge
	}
	if err := cbor.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("a2a: decode: %w", err)
	}
	if f.V != ProtocolVersion {
		return f, fmt.Errorf("a2a: protocol version mismatch: got %d want %d", f.V, ProtocolVersion)
	}
	if _, ok := validKinds[f.Kind]; !ok {
		return f, fmt.Errorf("a2a: unknown kind %q", f.Kind)
	}
	if f.ID == "" {
		return f, fmt.Errorf("a2a: empty frame ID")
	}
	if f.Depth < 0 {
		return f, fmt.Errorf("a2a: negative depth %d", f.Depth)
	}
	return f, nil
}

// CheckTaskDepth validates an inbound or outbound task.* frame against the
// §5.7 loop-protection ceiling.
func CheckTaskDepth(f Frame) error {
	if f.Kind != KindTaskOffer && f.Kind != KindTaskAccept && f.Kind != KindTaskReject {
		return nil
	}
	if f.Depth > MaxTaskDepth {
		return ErrDepthExceeded
	}
	return nil
}
