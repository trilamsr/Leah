package elevenlabs_test

import (
	"testing"

	"github.com/trilam/leah/internal/tts/elevenlabs"
)

// FromEnv requires LEAH_ELEVENLABS_API_KEY.
func TestFromEnv_MissingKey(t *testing.T) {
	t.Setenv("LEAH_ELEVENLABS_API_KEY", "")
	t.Setenv("LEAH_ELEVENLABS_VOICE_ID", "")
	if _, err := elevenlabs.FromEnv(nil); err == nil {
		t.Fatalf("expected ErrMissingAPIKey")
	}
}

// ava-alto-145wpm resolves via built-in default mapping when env is unset.
func TestFromEnv_VoiceMappingDefault(t *testing.T) {
	t.Setenv("LEAH_ELEVENLABS_API_KEY", "k")
	t.Setenv("LEAH_ELEVENLABS_VOICE_ID", "")
	c, err := elevenlabs.FromEnv(nil)
	if err != nil {
		t.Fatalf("from env: %v", err)
	}
	if c.VoiceID() == "" {
		t.Fatalf("voice id must default to built-in mapping for ava-alto-145wpm")
	}
}

// Env override wins over the default mapping.
func TestFromEnv_VoiceMappingOverride(t *testing.T) {
	t.Setenv("LEAH_ELEVENLABS_API_KEY", "k")
	t.Setenv("LEAH_ELEVENLABS_VOICE_ID", "custom-vid")
	c, err := elevenlabs.FromEnv(nil)
	if err != nil {
		t.Fatalf("from env: %v", err)
	}
	if c.VoiceID() != "custom-vid" {
		t.Fatalf("voice id: %q, want custom-vid", c.VoiceID())
	}
}
