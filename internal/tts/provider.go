// Package tts defines the §17.17 voice-canon TTS provider contract.
//
// Two implementations live under subpackages (added by Tasks 2 + 3):
// tts/elevenlabs (Flash v2.5 cloud, default) and tts/apple
// (AVSpeechSynthesizer "Ava (Premium)", privacy/offline fallback). The
// privacy classifier in this package routes each utterance before the
// provider is invoked.
package tts

import (
	"context"
	"io"
)

// DefaultVoice is the Leah voice-canon identifier per §2.7 (alto, ~145 wpm,
// mid-register). Each provider maps this string to its own internal voice id.
const DefaultVoice = "ava-alto-145wpm"

// AudioStream carries synthesized audio bytes back to the HUD for AVAudioEngine
// playback. MIME tells the HUD which decoder to wire ("audio/mpeg" for cloud
// Opus/AAC; "audio/x-caf" or empty for Apple local — Apple plays through its
// own engine and the stream is a no-op).
type AudioStream interface {
	io.ReadCloser
	MIME() string
}

// Provider is the §17.17 contract. Speak synthesizes text at the named voice
// and returns a stream the caller drains until io.EOF. PreWarm runs at daemon
// boot to amortize first-utterance TTFB (decision-log #81).
type Provider interface {
	Name() string
	Speak(ctx context.Context, text, voice string) (AudioStream, error)
	PreWarm(ctx context.Context) error
}

// Route is the privacy-classifier verdict.
type Route int

const (
	// RouteCloud sends text to ElevenLabs Flash v2.5 (TTFB 75–150 ms).
	RouteCloud Route = iota
	// RouteLocal sends text to Apple Ava (fully on-device, zero exposure).
	RouteLocal
)

// Classifier decides which provider gets each utterance. Implementations
// must complete in < 5 ms per §17.17 (runs before any audio I/O).
type Classifier interface {
	Route(text string) Route
}
