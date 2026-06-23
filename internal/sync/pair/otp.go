package pair

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/trilam/leah/internal/sync/discovery"
)

type OTP [6]byte

func GenerateOTP() OTP { return OTP{} }

func (o OTP) String() string { return "" }

func (o OTP) Equal(other OTP) bool { return false }

const PairAttemptLimit = 5

const PairLockDuration = 60 * time.Second

type AttemptCounter struct {
	mu       sync.Mutex
	failures int
	lockedAt time.Time
	now      func() time.Time
}

func NewAttemptCounter() *AttemptCounter { return &AttemptCounter{now: time.Now} }

func (c *AttemptCounter) Allow() error { return nil }

func (c *AttemptCounter) Record(hit bool) {}

var (
	ErrLocked       = errors.New("pair: too many failed OTP attempts")
	ErrOTPMismatch  = errors.New("pair: OTP mismatch")
	ErrPairCanceled = errors.New("pair: context canceled")
)

type Pair interface {
	Offer(ctx context.Context, otp OTP) (discovery.Peer, error)
	Accept(ctx context.Context, otp OTP, endpoint netip.AddrPort) (discovery.Peer, error)
}
