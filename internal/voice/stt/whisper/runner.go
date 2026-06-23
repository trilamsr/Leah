// Package whisper is the Whisper-large-v3 ONNX local STT runner.
// Spec §1.3.1: streaming local primary. Audio stays on-device; ONNX Runtime
// loads the model from modelDir at NewRunner time and the session is held
// for the lifetime of the Runner.
package whisper

import (
	"context"
	"sync"
	"time"

	"github.com/trilam/leah/internal/voice/stt"
)

// flushEveryFrames emits a non-final partial every N voiced frames so the
// HUD live-transcript redraws under 150 ms at 20 ms frames.
const flushEveryFrames = 5

type Runner struct {
	modelPath string
	sha       string

	mu      sync.Mutex
	session *session
}

func NewRunner(modelDir string) (*Runner, error) {
	path, sha, err := loadModel(modelDir)
	if err != nil {
		return nil, err
	}
	return &Runner{modelPath: path, sha: sha}, nil
}

func (r *Runner) Info() stt.ProviderInfo {
	return stt.ProviderInfo{
		Name:    "whisper-large-v3-onnx",
		IsLocal: true,
		ModelID: "whisper-large-v3",
		RAMmb:   850,
	}
}

func (r *Runner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return closeSession(r.session)
}

// Stream consumes voiced AudioFrames, batches them through an adaptive VAD,
// and emits partials. Final emit on ctx.Done or audio close so the duplex
// coordinator (T02) always sees a terminal frame.
func (r *Runner) Stream(ctx context.Context, audio <-chan stt.AudioFrame) (<-chan stt.Partial, error) {
	out := make(chan stt.Partial, 8)
	go func() {
		defer close(out)
		vad := &VAD{NoiseFloorDBFS: -55}
		var window []stt.AudioFrame
		flush := func(final bool) {
			if len(window) == 0 {
				return
			}
			start := window[0].Ts
			text := r.transcribe(window)
			select {
			case out <- stt.Partial{
				Text:       text,
				IsFinal:    final,
				Confidence: 0.85,
				LatencyMS:  int(time.Since(start).Milliseconds()),
			}:
			case <-ctx.Done():
			}
			if final {
				window = window[:0]
			}
		}
		for {
			select {
			case <-ctx.Done():
				flush(true)
				return
			case f, ok := <-audio:
				if !ok {
					flush(true)
					return
				}
				vad.Adapt(f)
				if vad.IsVoice(f) {
					window = append(window, f)
					if len(window)%flushEveryFrames == 0 {
						flush(false)
					}
				} else if len(window) > 0 {
					flush(true)
				}
			}
		}
	}()
	return out, nil
}
