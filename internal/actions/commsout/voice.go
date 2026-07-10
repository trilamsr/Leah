package commsout

import (
	"context"
	"fmt"

	"github.com/trilam/leah/internal/input/voice"
)

// VoiceNotify speaks notifications via the voice.TTS chain. Use when
// operator wants audio confirmation alongside desktop/push notifiers.
type VoiceNotify struct {
	TTS voice.TTS
}

// NewVoice returns a VoiceNotify wired to the default TTS chain
// (Kokoro → OpenAI → say).
func NewVoice() *VoiceNotify {
	return &VoiceNotify{TTS: voice.NewTTS()}
}

// Notify speaks "<title>: <body>" via the wrapped TTS chain.
func (v *VoiceNotify) Notify(ctx context.Context, title, body string) error {
	text := fmt.Sprintf("%s: %s", title, body)
	if err := v.TTS.Speak(ctx, text); err != nil {
		return fmt.Errorf("voice notify: %w", err)
	}
	return nil
}
