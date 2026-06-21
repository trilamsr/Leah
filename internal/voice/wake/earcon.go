package wake

import (
	"time"

	"github.com/trilam/leah/internal/obs"
)

// EarconBuckets — wake→earcon p95 target is ≤150ms (MAY-A1); buckets
// straddle the threshold so the p95 line lands inside a bucket, not +Inf.
var EarconBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.15, 0.25, 0.5}

// EarconTracker times wake-detected → first-earcon. The earcon is the
// perceptible "I heard you" cue — until it plays, the user has no signal
// that the wake word landed, so this span is the felt-latency of waking.
//
// Not goroutine-safe. WakeDetected and EarconPlayed must be called from a
// single owner goroutine (the session-loop turn driver) — the histogram
// handle is registry-locked, but the wakeAt cursor is single-writer.
type EarconTracker struct {
	hist     *obs.Histogram
	detector string
	wakeAt   time.Time
}

// NewEarconTracker registers the histogram. Nil-safe: a nil registry yields
// a nil receiver whose methods no-op, so production wiring can omit the
// registry without per-call guards.
func NewEarconTracker(reg *obs.Registry, detector string) *EarconTracker {
	if reg == nil {
		return nil
	}
	if detector == "" {
		detector = "unknown"
	}
	return &EarconTracker{
		hist:     reg.Histogram("leah_voice_wake_to_earcon_seconds", EarconBuckets),
		detector: detector,
	}
}

// WakeDetected starts the clock at the instant Detect returned true.
func (e *EarconTracker) WakeDetected(at time.Time) {
	if e == nil {
		return
	}
	e.wakeAt = at
}

// EarconPlayed stops the clock when the earcon audio buffer has been
// handed to the output device. Drops the observation if WakeDetected
// hasn't fired so cleanup paths don't poison the histogram.
func (e *EarconTracker) EarconPlayed(at time.Time) {
	if e == nil || e.wakeAt.IsZero() {
		return
	}
	e.hist.Observe(map[string]string{"detector": e.detector}, at.Sub(e.wakeAt).Seconds())
	e.wakeAt = time.Time{}
}
