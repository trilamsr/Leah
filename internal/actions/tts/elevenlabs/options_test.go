package elevenlabs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/keychain"
	"github.com/trilam/leah/internal/tts/elevenlabs"
)

// stubMissingSecurity swaps security(1) for a script that always reports
// "not found" so FromEnv's Keychain fallback doesn't surface a real stored
// key on a developer laptop that happens to have one.
func stubMissingSecurity(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "security")
	script := "#!/bin/sh\necho 'The specified item could not be found in the keychain.' 1>&2\nexit 44\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	prev := keychain.SetSecurityBin(bin)
	t.Cleanup(func() { keychain.SetSecurityBin(prev) })
}

// FromEnv requires the ElevenLabs Keychain slot OR LEAH_ELEVENLABS_API_KEY.
func TestFromEnv_MissingKey(t *testing.T) {
	t.Setenv("LEAH_ELEVENLABS_API_KEY", "")
	t.Setenv("LEAH_ELEVENLABS_VOICE_ID", "")
	stubMissingSecurity(t)
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
