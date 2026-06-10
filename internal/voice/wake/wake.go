// Package wake detects when the session should leave the idle state.
//
// W13 ships only the energy-threshold detector — RMS over the supplied frame.
// Porcupine and the magic-phrase post-filter (spec §3.4) defer to W14+.
package wake

import (
	"context"
	"math"
)

// Detector decides whether a mic frame is loud enough to arm the STT.
type Detector interface {
	Detect(ctx context.Context, audio []float32) (bool, error)
	Threshold() float64
}

// EnergyDetector fires when frame RMS ≥ threshold. Stateless; safe for
// concurrent calls.
type EnergyDetector struct {
	threshold float64
}

// NewEnergy returns a Detector that triggers when RMS reaches threshold.
func NewEnergy(threshold float64) *EnergyDetector {
	return &EnergyDetector{threshold: threshold}
}

// Detect returns true when sqrt(mean(audio²)) ≥ threshold. ctx-cancel before
// the cheap arithmetic still surfaces as ctx.Err() so callers can compose with
// a long-running audio pipeline.
func (e *EnergyDetector) Detect(ctx context.Context, audio []float32) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if len(audio) == 0 {
		return false, nil
	}
	var sumSq float64
	for _, s := range audio {
		f := float64(s)
		sumSq += f * f
	}
	rms := math.Sqrt(sumSq / float64(len(audio)))
	return rms >= e.threshold, nil
}

// Threshold exposes the configured floor — the session layer logs it on
// calibration boot so operators can tune via env without code edits.
func (e *EnergyDetector) Threshold() float64 { return e.threshold }
