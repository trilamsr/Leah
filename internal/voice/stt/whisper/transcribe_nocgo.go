//go:build !cgo

package whisper

import "github.com/trilam/leah/internal/voice/stt"

// session is a stub when the binary is built CGO_ENABLED=0. NewRunner still
// reads the model file (so missing-artifact surfaces the same way under both
// build modes), but inference is a no-op — the operator must rebuild with
// cgo to get real local STT. Failing closed beats silently degrading.
type session struct{}

func (r *Runner) transcribe(_ []stt.AudioFrame) string { return "" }

func closeSession(_ *session) error { return nil }
