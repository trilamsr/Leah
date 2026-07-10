package keychain

import "os"

// Service identifiers must match the Swift-side wizard's Keychain slot names
// verbatim. Changing any value here silently orphans the wizard's stored
// secret — treat as frozen.
const (
	AnthropicService  = "com.maydow.leah.anthropic"
	ElevenLabsService = "com.maydow.leah.elevenlabs"
	VoyageService     = "com.maydow.leah.voyage"
	OpenAIService     = "com.maydow.leah.openai"
	PushoverService   = "com.maydow.leah.pushover"

	DefaultAccount = "default"

	// PushoverUserAccount + PushoverTokenAccount split the two Pushover
	// credentials under one service so the wizard/CLI treats them as a pair.
	PushoverUserAccount  = "user"
	PushoverTokenAccount = "token"
)

// loadWithEnvFallback returns env when set (transition path) and falls back
// to Keychain otherwise. Env is checked first because operators overriding a
// stored secret via `ANTHROPIC_API_KEY=... leah ask` expect the env to win.
func loadWithEnvFallback(envKey, service, account string) (string, error) {
	if v := os.Getenv(envKey); v != "" {
		return v, nil
	}
	return Load(service, account)
}

// LoadAnthropicKey returns ANTHROPIC_API_KEY (env first, Keychain fallback).
func LoadAnthropicKey() (string, error) {
	return loadWithEnvFallback("ANTHROPIC_API_KEY", AnthropicService, DefaultAccount)
}

// LoadElevenLabsKey returns LEAH_ELEVENLABS_API_KEY (env first, Keychain fallback).
func LoadElevenLabsKey() (string, error) {
	return loadWithEnvFallback("LEAH_ELEVENLABS_API_KEY", ElevenLabsService, DefaultAccount)
}

// LoadVoyageKey returns VOYAGE_API_KEY (env first, Keychain fallback).
func LoadVoyageKey() (string, error) {
	return loadWithEnvFallback("VOYAGE_API_KEY", VoyageService, DefaultAccount)
}

// LoadOpenAIKey returns OPENAI_API_KEY (env first, Keychain fallback).
func LoadOpenAIKey() (string, error) {
	return loadWithEnvFallback("OPENAI_API_KEY", OpenAIService, DefaultAccount)
}

// LoadPushoverUser returns LEAH_PUSHOVER_USER (env first, Keychain fallback).
func LoadPushoverUser() (string, error) {
	return loadWithEnvFallback("LEAH_PUSHOVER_USER", PushoverService, PushoverUserAccount)
}

// LoadPushoverToken returns LEAH_PUSHOVER_TOKEN (env first, Keychain fallback).
func LoadPushoverToken() (string, error) {
	return loadWithEnvFallback("LEAH_PUSHOVER_TOKEN", PushoverService, PushoverTokenAccount)
}
