package listener

import "github.com/trilam/leah/internal/platform/telemetry"

// utteranceBuckets — A2 SLO: end-of-utterance → final transcript ≤600ms p50,
// ≤1.2s p95. Bucket edges straddle both thresholds so the histogram resolves
// SLO compliance without re-bucketing.
var utteranceBuckets = []float64{0.1, 0.3, 0.6, 1.2, 2.0}

// Instrumentation owns the utterance→transcript histogram. nil-safe: a nil
// receiver no-ops so the production listener can omit a registry without
// per-call guards.
type Instrumentation struct {
	utterance *telemetry.Histogram
}

// NewInstrumentation registers the A2 histogram. Returns nil when reg is nil
// so RecordFinal is a no-op.
func NewInstrumentation(reg *telemetry.Registry) *Instrumentation {
	if reg == nil {
		return nil
	}
	return &Instrumentation{
		utterance: reg.Histogram("leah_voice_utterance_to_transcript_seconds", utteranceBuckets),
	}
}

// RegisterMetrics cold-seeds the A2 series so /metrics surfaces it pre-event.
func RegisterMetrics(reg *telemetry.Registry) {
	if reg == nil {
		return
	}
	reg.Histogram("leah_voice_utterance_to_transcript_seconds", utteranceBuckets).Declare(map[string]string{"outcome": "cold"})
}

// RecordFinal observes FinishedAt-StartedAt on Final segments. Partials are
// ignored — the SLO targets end-of-utterance latency, not streaming chunks.
func (i *Instrumentation) RecordFinal(seg Segment) {
	if i == nil || !seg.Final {
		return
	}
	if seg.StartedAt.IsZero() || seg.FinishedAt.IsZero() {
		return
	}
	dur := seg.FinishedAt.Sub(seg.StartedAt).Seconds()
	if dur < 0 {
		return
	}
	i.utterance.Observe(map[string]string{"outcome": "ok"}, dur)
}
