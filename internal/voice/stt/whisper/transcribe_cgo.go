//go:build cgo

package whisper

import (
	"sync"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/trilam/leah/internal/voice/stt"
)

type session struct {
	s *ort.DynamicAdvancedSession
}

var whisperInitOnce sync.Once
var whisperInitErr error

func (r *Runner) ensureSession() error {
	whisperInitOnce.Do(func() {
		whisperInitErr = ort.InitializeEnvironment()
	})
	if whisperInitErr != nil {
		return whisperInitErr
	}
	if r.session != nil {
		return nil
	}
	s, err := ort.NewDynamicAdvancedSession(r.modelPath,
		[]string{"audio"},
		[]string{"tokens"},
		nil,
	)
	if err != nil {
		return err
	}
	r.session = &session{s: s}
	return nil
}

func (r *Runner) transcribe(window []stt.AudioFrame) string {
	if len(window) == 0 {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureSession(); err != nil {
		return ""
	}
	floatPCM := pcmToFloat32(window)
	in, err := ort.NewTensor(ort.NewShape(1, int64(len(floatPCM))), floatPCM)
	if err != nil {
		return ""
	}
	defer func() { _ = in.Destroy() }()
	outputs := []ort.Value{nil}
	if err := r.session.s.Run([]ort.Value{in}, outputs); err != nil {
		return ""
	}
	defer func() {
		if outputs[0] != nil {
			_ = outputs[0].Destroy()
		}
	}()
	tok, ok := outputs[0].(*ort.Tensor[int32])
	if !ok {
		return ""
	}
	return decodeTokens(tok.GetData())
}

func closeSession(s *session) error {
	if s == nil || s.s == nil {
		return nil
	}
	return s.s.Destroy()
}

func pcmToFloat32(window []stt.AudioFrame) []float32 {
	total := 0
	for _, fr := range window {
		total += len(fr.PCM)
	}
	out := make([]float32, 0, total)
	for _, fr := range window {
		for _, s := range fr.PCM {
			out = append(out, float32(s)/32768)
		}
	}
	return out
}
