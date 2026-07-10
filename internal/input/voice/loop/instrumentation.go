package loop

import (
	"time"

	"github.com/trilam/leah/internal/obs"
)

// intentToFirstAudioBuckets — A4 SLA 700ms; edges straddle so p50/p95 resolves.
var intentToFirstAudioBuckets = []float64{0.1, 0.3, 0.5, 0.7, 1.0, 2.0}

// IntentToFirstAudioTracker is not goroutine-safe — single-writer per turn.
type IntentToFirstAudioTracker struct {
	hist     *telemetry.Histogram
	intentAt time.Time
}

func NewIntentToFirstAudioTracker(reg *telemetry.Registry) *IntentToFirstAudioTracker {
	if reg == nil {
		return nil
	}
	return &IntentToFirstAudioTracker{
		hist: reg.Histogram("leah_voice_intent_to_first_audio_seconds", intentToFirstAudioBuckets),
	}
}

func RegisterMetrics(reg *telemetry.Registry) {
	if reg == nil {
		return
	}
	reg.Histogram("leah_voice_intent_to_first_audio_seconds", intentToFirstAudioBuckets).Declare(map[string]string{"outcome": "cold"})
}

func (t *IntentToFirstAudioTracker) MarkIntentDone(at time.Time) {
	if t == nil {
		return
	}
	t.intentAt = at
}

func (t *IntentToFirstAudioTracker) MarkFirstAudio(at time.Time, outcome string) {
	if t == nil || t.intentAt.IsZero() {
		return
	}
	dur := at.Sub(t.intentAt).Seconds()
	t.intentAt = time.Time{}
	if dur < 0 {
		return
	}
	t.hist.Observe(map[string]string{"outcome": outcome}, dur)
}
