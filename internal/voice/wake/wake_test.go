package wake

import (
	"context"
	"math"
	"testing"
)

// TestEnergy_AboveThreshold_True: RMS above threshold fires the detector.
func TestEnergy_AboveThreshold_True(t *testing.T) {
	t.Parallel()
	d := NewEnergy(0.1)
	loud := make([]float32, 256)
	for i := range loud {
		loud[i] = 0.5
	}
	got, err := d.Detect(context.Background(), loud)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !got {
		t.Fatalf("want true for RMS=0.5 ≥ 0.1, got false")
	}
}

// TestEnergy_BelowThreshold_False: RMS below threshold stays silent.
func TestEnergy_BelowThreshold_False(t *testing.T) {
	t.Parallel()
	d := NewEnergy(0.1)
	quiet := make([]float32, 256)
	for i := range quiet {
		quiet[i] = 0.01
	}
	got, err := d.Detect(context.Background(), quiet)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got {
		t.Fatalf("want false for RMS=0.01 < 0.1, got true")
	}
}

// TestEnergy_EmptyAudio_False: empty buffer is not a trigger.
func TestEnergy_EmptyAudio_False(t *testing.T) {
	t.Parallel()
	d := NewEnergy(0.1)
	got, err := d.Detect(context.Background(), nil)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got {
		t.Fatalf("want false for empty audio, got true")
	}
}

// TestEnergy_RMS_CorrectMath: RMS of constant 0.5 samples equals 0.5.
func TestEnergy_RMS_CorrectMath(t *testing.T) {
	t.Parallel()
	d := NewEnergy(0.5)
	samples := []float32{0.5, 0.5, 0.5, 0.5}
	// threshold equals RMS — comparison is ≥, so this fires.
	got, err := d.Detect(context.Background(), samples)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !got {
		t.Fatalf("want true: RMS of [0.5,0.5,0.5,0.5]=0.5 ≥ threshold 0.5")
	}
	// And a hair above 0.5 must NOT fire.
	d2 := NewEnergy(0.5 + 1e-6)
	got2, err := d2.Detect(context.Background(), samples)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got2 {
		t.Fatalf("want false: RMS 0.5 < threshold 0.5+ε")
	}
	if got := d.Threshold(); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("Threshold()=%v want 0.5", got)
	}
}
