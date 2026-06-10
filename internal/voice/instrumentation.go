package voice

import (
	"context"
	"errors"

	"github.com/trilam/leah/internal/obs"
)

func Bind(c *ChainTTS, registry *obs.Registry) {
	if registry == nil || c == nil {
		return
	}
	cnt := registry.Counter("leah_voice_speak_total")
	c.OnSpeak = func(backend string) {
		cnt.Inc(map[string]string{"backend": backend})
	}
}

func EmitSpeak(registry *obs.Registry, backend string) {
	if registry == nil {
		return
	}
	registry.Counter("leah_voice_speak_total").Inc(map[string]string{"backend": backend})
}

type SelfChecker struct{ Chain *ChainTTS }

func (c *SelfChecker) SelfCheck(ctx context.Context) error {
	_ = ctx
	if c == nil || c.Chain == nil {
		return nil
	}
	if !c.Chain.Available() {
		return errors.New("voice: no backends available")
	}
	return nil
}

func backendName(t TTS) string {
	switch t.(type) {
	case *KokoroTTS:
		return "kokoro"
	case *OpenAITTS:
		return "openai"
	case *SayTTS:
		return "say"
	default:
		return "unknown"
	}
}
