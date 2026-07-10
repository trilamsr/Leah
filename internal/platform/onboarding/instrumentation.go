package onboarding

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
)

const firstReplySealRel = "onboarding/first_reply.done"

// FirstReplyBuckets — A9 SLA: brew install → first useful reply ≤5 min p95.
// Boundaries straddle the 300s target so the SLA line lands inside a bucket
// rather than at +Inf, with one bucket above the SLA to catch tail regressions.
var FirstReplyBuckets = []float64{30, 60, 120, 300, 600, 1200}

// RegisterMetrics declares the A9 install→first-reply series so /metrics
// surfaces it pre-event.
func RegisterMetrics(registry *telemetry.Registry) {
	if registry == nil {
		return
	}
	registry.Histogram("leah_onboarding_install_to_first_reply_seconds", FirstReplyBuckets).Declare(nil)
}

// FirstReplyObserver seals after the first observation. The A9 span is a
// once-per-install event — a second reply from the same install would skew
// the histogram with a near-zero sample.
type FirstReplyObserver struct {
	hist *telemetry.Histogram
	done atomic.Bool
}

// NewFirstReplyObserver is nil-safe: a nil registry yields nil so the caller
// can wire it unconditionally and let Record short-circuit.
func NewFirstReplyObserver(registry *telemetry.Registry) *FirstReplyObserver {
	if registry == nil {
		return nil
	}
	return &FirstReplyObserver{
		hist: registry.Histogram("leah_onboarding_install_to_first_reply_seconds", FirstReplyBuckets),
	}
}

// Record observes elapsed iff this is the first call on this observer.
// Returns true if the sample landed, false if the observer was already sealed
// (lets the caller skip subsequent computation cheaply). Negative durations
// are dropped (clock skew).
func (o *FirstReplyObserver) Record(elapsed time.Duration) bool {
	if o == nil || o.hist == nil {
		return false
	}
	if elapsed < 0 {
		return false
	}
	if !o.done.CompareAndSwap(false, true) {
		return false
	}
	o.hist.Observe(nil, elapsed.Seconds())
	return true
}

// RecordFirstReplyIfNotYetRecorded is the callsite contract used by short-lived
// processes (the CLI `leah ask`) where an in-memory atomic.Bool would not
// survive process exit. O_EXCL on the seal file is the only mechanism that
// makes "once per install" hold across N independent CLI invocations.
// No-op (returns false) when the install marker is missing — we cannot
// compute elapsed and a daemon-side MarkInstalled has not run yet.
func RecordFirstReplyIfNotYetRecorded(stateDir string, registry *telemetry.Registry, now time.Time) bool {
	if stateDir == "" || registry == nil {
		return false
	}
	installedAt, ok, err := InstalledAt(stateDir)
	if err != nil || !ok {
		return false
	}
	// Reject negative elapsed BEFORE writing the seal — a clock that briefly
	// jumped backwards must not burn the once-per-install slot with no sample.
	elapsed := now.Sub(installedAt)
	if elapsed < 0 {
		return false
	}
	sealPath := filepath.Join(stateDir, firstReplySealRel)
	if err := os.MkdirAll(filepath.Dir(sealPath), 0o755); err != nil {
		return false
	}
	f, err := os.OpenFile(sealPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return false
	}
	_ = f.Close()
	registry.Histogram("leah_onboarding_install_to_first_reply_seconds", FirstReplyBuckets).Observe(nil, elapsed.Seconds())
	return true
}
