package whisper

import (
	"math"

	"github.com/trilam/leah/internal/voice/stt"
)

// VAD tracks an adaptive noise floor in dBFS and gates voice on a fixed
// 6 dB headroom above it. Spec §1.2: continuous VAD, adaptive floor.
type VAD struct {
	NoiseFloorDBFS float64
	emaAlpha       float64
}

func (v *VAD) frameDBFS(frame stt.AudioFrame) float64 {
	if len(frame.PCM) == 0 {
		return -120
	}
	var sumSq float64
	for _, s := range frame.PCM {
		f := float64(s) / 32768
		sumSq += f * f
	}
	rms := math.Sqrt(sumSq / float64(len(frame.PCM)))
	if rms <= 0 {
		return -120
	}
	return 20 * math.Log10(rms)
}

func (v *VAD) Adapt(frame stt.AudioFrame) {
	if v.emaAlpha == 0 {
		v.emaAlpha = 0.05
	}
	d := v.frameDBFS(frame)
	v.NoiseFloorDBFS = (1-v.emaAlpha)*v.NoiseFloorDBFS + v.emaAlpha*d
}

func (v *VAD) IsVoice(frame stt.AudioFrame) bool {
	return v.frameDBFS(frame) > v.NoiseFloorDBFS+6
}
