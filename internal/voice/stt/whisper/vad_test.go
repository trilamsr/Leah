package whisper

import (
	"testing"
	"time"

	"github.com/trilam/leah/internal/voice/stt"
)

func TestVAD_SilenceIsNotVoice(t *testing.T) {
	v := &VAD{NoiseFloorDBFS: -55}
	frame := stt.AudioFrame{PCM: make([]int16, 320), SampleRate: 16000, Ts: time.Now()}
	if v.IsVoice(frame) {
		t.Fatal("zero PCM must not register as voice")
	}
}

func TestVAD_LoudSineIsVoice(t *testing.T) {
	v := &VAD{NoiseFloorDBFS: -55}
	pcm := make([]int16, 320)
	for i := range pcm {
		pcm[i] = 20000
	}
	frame := stt.AudioFrame{PCM: pcm, SampleRate: 16000, Ts: time.Now()}
	v.Adapt(frame)
	if !v.IsVoice(frame) {
		t.Fatal("loud constant signal must register as voice")
	}
}

func TestVAD_AdaptiveNoiseFloorRises(t *testing.T) {
	v := &VAD{NoiseFloorDBFS: -55}
	noisy := make([]int16, 320)
	for i := range noisy {
		noisy[i] = 4000
	}
	frame := stt.AudioFrame{PCM: noisy, SampleRate: 16000, Ts: time.Now()}
	for i := 0; i < 30; i++ {
		v.Adapt(frame)
	}
	if v.NoiseFloorDBFS < -50 {
		t.Fatalf("noise floor should have adapted up, got %v", v.NoiseFloorDBFS)
	}
}
