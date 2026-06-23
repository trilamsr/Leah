package pair

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/trilam/leah/internal/sync/discovery"
)

// OTP is a 6-digit human-spoken one-time pad used for peer pairing per spec
// §2.4.1. Wire form is "NNN-NNN" (e.g. "482-913").
type OTP [6]byte

// GenerateOTP draws 6 uniform decimal digits from crypto/rand. Returning an
// error here would force every caller into a panic path for a failure mode
// that crypto/rand never exhibits in practice; surface it via panic so the
// supervisor restarts (per §2 watchdog contract) rather than mint a weak OTP.
func GenerateOTP() OTP {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(fmt.Sprintf("pair: crypto/rand failure: %v", err))
	}
	// Map 64 bits of entropy onto 6 digits via modulo 1_000_000. The bias
	// from 2^64 mod 1_000_000 is < 2^-44 — well below the ~20-bit OTP space
	// itself, so it's not the limiting security factor.
	n := binary.BigEndian.Uint64(buf[:]) % 1_000_000
	var o OTP
	for i := 5; i >= 0; i-- {
		o[i] = byte('0' + n%10)
		n /= 10
	}
	return o
}

func (o OTP) String() string {
	return string([]byte{o[0], o[1], o[2], '-', o[3], o[4], o[5]})
}

// Equal compares two OTPs in constant time. Pair handshake verifies the
// operator-spoken code against the daemon-issued one; a timing leak there
// would let a co-located attacker scrape OTP digits via repeated probes.
func (o OTP) Equal(other OTP) bool {
	return subtle.ConstantTimeCompare(o[:], other[:]) == 1
}

// PairAttemptLimit caps the number of OTP guesses across the pairing window.
// 6 digits = ~20 bits of entropy — without rate limiting a co-located peer
// could brute-force in seconds. 5 attempts then lock-out matches Apple's
// AirPlay pairing UX and keeps the success probability < 2^-17.
const PairAttemptLimit = 5

// PairLockDuration is the cool-down after PairAttemptLimit failures. After
// the window expires the counter resets.
const PairLockDuration = 60 * time.Second

// AttemptCounter is the rate-limit ledger for Accept(). It is safe for
// concurrent use.
type AttemptCounter struct {
	mu       sync.Mutex
	failures int
	lockedAt time.Time
	now      func() time.Time
}

func NewAttemptCounter() *AttemptCounter {
	return &AttemptCounter{now: time.Now}
}

// Allow returns nil if a new attempt may proceed. After PairAttemptLimit
// failures within PairLockDuration it returns ErrLocked.
func (c *AttemptCounter) Allow() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failures >= PairAttemptLimit {
		if c.now().Sub(c.lockedAt) < PairLockDuration {
			return ErrLocked
		}
		c.failures = 0
	}
	return nil
}

// Record stamps the outcome of an attempt. A miss bumps the failure counter
// and arms the lock on the threshold crossing; a hit resets the ledger.
func (c *AttemptCounter) Record(hit bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if hit {
		c.failures = 0
		return
	}
	c.failures++
	if c.failures == PairAttemptLimit {
		c.lockedAt = c.now()
	}
}

var (
	ErrLocked       = errors.New("pair: too many failed OTP attempts")
	ErrOTPMismatch  = errors.New("pair: OTP mismatch")
	ErrPairCanceled = errors.New("pair: context canceled")
)

// Pair is the handshake interface. Offer is invoked on the existing Mac
// (showing the OTP to the operator); Accept runs on the new Mac (operator
// types the OTP). Both return the resolved Peer after mTLS pinning succeeds.
type Pair interface {
	Offer(ctx context.Context, otp OTP) (discovery.Peer, error)
	Accept(ctx context.Context, otp OTP, endpoint netip.AddrPort) (discovery.Peer, error)
}
