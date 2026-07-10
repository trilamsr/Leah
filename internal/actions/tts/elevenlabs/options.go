package elevenlabs

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/trilam/leah/internal/platform/keychain"
)

// avaAltoDefaultVoiceID is the built-in mapping for tts.DefaultVoice
// (ava-alto-145wpm) — ElevenLabs library voice "Bella" (alto, mid-register).
// Override via LEAH_ELEVENLABS_VOICE_ID; operator-tuned voices live there.
const avaAltoDefaultVoiceID = "EXAVITQu4vr4xnSDxMaL"

// ErrMissingAPIKey is returned when neither LEAH_ELEVENLABS_API_KEY nor a
// keychain entry yields an api key. Daemon must fall back to local provider.
var ErrMissingAPIKey = errors.New("elevenlabs: LEAH_ELEVENLABS_API_KEY unset")

// FromEnv builds a Client. The api key resolves env-first then falls back to
// the Keychain slot the Swift wizard writes. LEAH_ELEVENLABS_VOICE_ID
// overrides the ava-alto-145wpm default.
func FromEnv(hc *http.Client) (*Client, error) {
	key, err := keychain.LoadElevenLabsKey()
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: load key: %w", err)
	}
	if key == "" {
		return nil, ErrMissingAPIKey
	}
	voice := os.Getenv("LEAH_ELEVENLABS_VOICE_ID")
	if voice == "" {
		voice = avaAltoDefaultVoiceID
	}
	return New(key, voice, hc), nil
}
