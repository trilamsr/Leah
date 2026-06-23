package crdt

import (
	"bytes"
	"testing"

	"github.com/trilam/leah/internal/sync/discovery"
)

// Resolve must order on (lamport DESC, device lex ASC) — the spec invariant in §2.3.
// A wall-clock-based tiebreak would silently lose writes on clock skew.
func TestResolve_LamportThenDeviceLex(t *testing.T) {
	cases := []struct {
		name          string
		local, remote LWWValue
		wantPayload   []byte
		wantLamport   Lamport
		wantDevice    discovery.DeviceID
	}{
		{
			name:        "higher lamport wins regardless of device",
			local:       LWWValue{Payload: []byte("L"), Lamport: 5, Device: "aaaa"},
			remote:      LWWValue{Payload: []byte("R"), Lamport: 6, Device: "zzzz"},
			wantPayload: []byte("R"), wantLamport: 6, wantDevice: "zzzz",
		},
		{
			name:        "lamport tie, lower device-id lex wins",
			local:       LWWValue{Payload: []byte("L"), Lamport: 7, Device: "bbbb"},
			remote:      LWWValue{Payload: []byte("R"), Lamport: 7, Device: "aaaa"},
			wantPayload: []byte("R"), wantLamport: 7, wantDevice: "aaaa",
		},
		{
			name:        "lamport tie, local has lex-smaller device — local wins",
			local:       LWWValue{Payload: []byte("L"), Lamport: 7, Device: "aaaa"},
			remote:      LWWValue{Payload: []byte("R"), Lamport: 7, Device: "bbbb"},
			wantPayload: []byte("L"), wantLamport: 7, wantDevice: "aaaa",
		},
		{
			name:        "nil local accepts remote",
			local:       LWWValue{},
			remote:      LWWValue{Payload: []byte("R"), Lamport: 1, Device: "x"},
			wantPayload: []byte("R"), wantLamport: 1, wantDevice: "x",
		},
		{
			name:        "nil remote keeps local",
			local:       LWWValue{Payload: []byte("L"), Lamport: 4, Device: "x"},
			remote:      LWWValue{},
			wantPayload: []byte("L"), wantLamport: 4, wantDevice: "x",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(tc.local, tc.remote)
			if !bytes.Equal(got.Payload, tc.wantPayload) || got.Lamport != tc.wantLamport || got.Device != tc.wantDevice {
				t.Fatalf("Resolve = %+v, want payload=%q lamport=%d device=%s", got, tc.wantPayload, tc.wantLamport, tc.wantDevice)
			}
		})
	}
}

// Resolve must be order-independent — swapping local/remote yields the same winner.
func TestResolve_Commutative(t *testing.T) {
	a := LWWValue{Payload: []byte("A"), Lamport: 7, Device: "aaaa"}
	b := LWWValue{Payload: []byte("B"), Lamport: 7, Device: "bbbb"}
	if !Resolve(a, b).Equal(Resolve(b, a)) {
		t.Fatalf("Resolve not commutative: %+v vs %+v", Resolve(a, b), Resolve(b, a))
	}
}
