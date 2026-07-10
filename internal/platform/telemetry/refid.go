package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

type refIDKeyType struct{}

var refIDKey refIDKeyType

// WithRefID stamps refID onto ctx; explicit (not goroutine-local) per spec §4.
func WithRefID(ctx context.Context, refID string) context.Context {
	return context.WithValue(ctx, refIDKey, refID)
}

// RefID reads the RefID stamped on ctx ("" if absent).
func RefID(ctx context.Context) string {
	v, _ := ctx.Value(refIDKey).(string)
	return v
}

// NewRefID returns a fresh 128-bit hex RefID for operation roots.
func NewRefID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		// Pseudo-ID fallback — diagnostic correlation OK if /dev/urandom broken.
		now := time.Now().UnixNano()
		for i := 0; i < 8; i++ {
			b[i] = byte(now >> (i * 8))
		}
	}
	return hex.EncodeToString(b[:])
}
