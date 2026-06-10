package voice

import (
	"context"
	"fmt"
)

// SayTTS shells out to the macOS `say` binary. Robotic but always
// available on macOS, no network, no model download. Last-resort backend.
type SayTTS struct {
	Exec Executor
}

// Speak invokes `say <text>`. Blocks until playback completes.
func (s *SayTTS) Speak(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	return withAudioDevice(func() error {
		if _, err := s.Exec.Run(ctx, "say", text); err != nil {
			return fmt.Errorf("say: %w", err)
		}
		return nil
	})
}
