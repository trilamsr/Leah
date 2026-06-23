package crdt

import "bytes"

// Lamport is a logical clock value (§2.3). Wall-clock skew is intentionally absent —
// LWW resolves on declared lamport then DeviceID lex tiebreak so a 5-min clock drift
// between peers cannot lose a write (§2.7 row "Clock skew > 5 min").
type Lamport int64

// Op enumerates the CRDT operation kinds on the add-only log (§2.4.1).
type Op string

const (
	OpInsert Op = "insert"
	OpUpdate Op = "update"
	OpDelete Op = "delete"
)

// LWWValue is a register entry: payload + the (lamport, device) pair that wrote it.
type LWWValue struct {
	Payload []byte
	Lamport Lamport
	Device  DeviceID
}

// Resolve picks the winner between local + remote LWW writes (§2.3).
//
// Order: lamport DESC, then DeviceID lex ASC as deterministic tiebreaker.
// A nil local resolves to remote (and vice versa) — a peer's first observation
// of a register row must not be discarded just because we have no prior write.
func Resolve(local, remote LWWValue) LWWValue {
	if local.Payload == nil && remote.Payload == nil {
		return local
	}
	if local.Payload == nil {
		return remote
	}
	if remote.Payload == nil {
		return local
	}
	switch {
	case remote.Lamport > local.Lamport:
		return remote
	case remote.Lamport < local.Lamport:
		return local
	}
	// Lamport tie — lex order on DeviceID. Lower string wins so the choice is
	// stable regardless of which side called Resolve.
	if string(remote.Device) < string(local.Device) {
		return remote
	}
	return local
}

// Equal reports identical payload + clock + device — used by tests + GC to skip
// idempotent applies without allocating.
func (v LWWValue) Equal(o LWWValue) bool {
	return v.Lamport == o.Lamport && v.Device == o.Device && bytes.Equal(v.Payload, o.Payload)
}
