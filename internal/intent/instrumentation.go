package intent

import (
	"time"

	"github.com/trilam/leah/internal/obs"
)

// classifyBuckets — A3 SLO is p95 ≤50ms, so the top finite bound is 0.05.
// Sub-ms buckets resolve the regex-only happy path; +Inf catches pathological
// inputs without skewing distribution.
var classifyBuckets = []float64{0.0001, 0.001, 0.01, 0.05}

// ClassifyTimed wraps Classify with a wall-clock observation into
// leah_intent_classify_seconds{kind=…}. Nil registry yields plain Classify so
// callers can omit instrumentation without per-call guards.
func ClassifyTimed(reg *telemetry.Registry, s string) Kind {
	if reg == nil {
		return Classify(s)
	}
	start := time.Now()
	kind := Classify(s)
	reg.Histogram("leah_intent_classify_seconds", classifyBuckets).
		Observe(map[string]string{"kind": kind.String()}, time.Since(start).Seconds())
	return kind
}

// RegisterMetrics cold-seeds leah_intent_classify_seconds so the series
// surfaces on /metrics before the first utterance.
func RegisterMetrics(reg *telemetry.Registry) {
	if reg == nil {
		return
	}
	h := reg.Histogram("leah_intent_classify_seconds", classifyBuckets)
	for k := KindAsk; k < kindMax; k++ {
		h.Declare(map[string]string{"kind": k.String()})
	}
}
