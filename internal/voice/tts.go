// Package voice provides a text-to-speech (TTS) abstraction with a
// 3-tier backend chain: Kokoro-82M local (default, cost-zero), OpenAI
// tts-1-hd (paid fallback), macOS `say` (last-resort offline). Backends
// satisfy the TTS interface; ChainTTS walks them in order on each Speak
// call, returning on first success. See docs/specs/2026-06-09-tts-kokoro.md.
package voice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// TTS speaks the given text aloud. Implementations may block until
// playback completes or return immediately and play in background — callers
// must not assume either. Cancellation propagates via ctx.
type TTS interface {
	Speak(ctx context.Context, text string) error
}

// Executor abstracts process spawn so tests can stub binary invocations
// without requiring kokoro / say / afplay to be installed on the test host.
type Executor interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	LookPath(name string) (string, error)
}

// ShellExec is the production Executor. Run wraps exec.CommandContext;
// LookPath wraps exec.LookPath.
type ShellExec struct{}

// Run executes name with args and returns combined stdout. ExitError stderr
// is folded into the returned error for log readability.
func (ShellExec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("%s: %s", name, string(ee.Stderr))
		}
		return out, err
	}
	return out, nil
}

// LookPath resolves name against $PATH.
func (ShellExec) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

// audioDeviceMu serializes the single macOS audio device across every
// backend. Held ONLY around the device-binding exec call (afplay, say) —
// synthesis (HTTP, file write, kokoro subprocess) runs unlocked so slow
// backends do not block queued callers. See #11.
var audioDeviceMu sync.Mutex

// withAudioDevice runs fn while holding the package-level device lock.
func withAudioDevice(fn func() error) error {
	audioDeviceMu.Lock()
	defer audioDeviceMu.Unlock()
	return fn()
}

// ChainTTS walks backends in order on each Speak call, returning on the
// first backend whose Speak returns nil. Device contention is handled by
// each backend via withAudioDevice; the chain itself holds no lock.
type ChainTTS struct {
	backends []TTS
	// OnSpeak fires once per successful Speak with the backend name
	// ("kokoro" / "openai" / "say"). Wired by main.go to feed
	// leah_voice_speak_total{backend}. nil-safe.
	OnSpeak func(backend string)
}

// NewChain constructs a ChainTTS over the given backends in priority order.
func NewChain(backends ...TTS) *ChainTTS {
	return &ChainTTS{backends: backends}
}

// Speak tries each backend in order; returns nil on first success.
// Returns the last backend's error if all fail, or a sentinel error if
// the chain is empty.
func (c *ChainTTS) Speak(ctx context.Context, text string) error {
	if len(c.backends) == 0 {
		return errors.New("voice: chain has no backends")
	}
	var lastErr error
	for _, b := range c.backends {
		if err := b.Speak(ctx, text); err != nil {
			lastErr = err
			continue
		}
		if c.OnSpeak != nil {
			c.OnSpeak(backendName(b))
		}
		return nil
	}
	return fmt.Errorf("voice: all backends failed: %w", lastErr)
}

// Available reports whether the chain has any backends. Used by the
// voice SelfChecker so /health surfaces "degraded" when TTS is fully unwired.
func (c *ChainTTS) Available() bool {
	return c != nil && len(c.backends) > 0
}

// NewTTS returns the default chain: Kokoro → OpenAI → say. Selection
// is based on what is installed/configured on the host at construction
// time. Backends whose precondition fails are omitted from the chain
// (not included as always-erroring entries) so the chain reports a clean
// "no backends" error when nothing is wired.
func NewTTS() TTS {
	return NewChain(pickBackends(ShellExec{}, os.Getenv)...)
}

// pickBackends builds the prioritized backend slice given an Executor
// (for kokoro/say detection) and an env lookup (for OPENAI_API_KEY).
func pickBackends(exec Executor, getenv func(string) string) []TTS {
	var out []TTS
	if _, err := exec.LookPath("kokoro"); err == nil {
		out = append(out, &KokoroTTS{Exec: exec})
	}
	// OpenAI tier requires explicit opt-in (LEAH_VOICE_ALLOW_OPENAI=1) so
	// notification bodies (intent strings, PR titles, agent IDs) are never
	// silently round-tripped to a third-party API just because a key happens
	// to be exported for unrelated tooling. See audit 2026-06-09 H5.
	if key := getenv("OPENAI_API_KEY"); key != "" && getenv("LEAH_VOICE_ALLOW_OPENAI") == "1" {
		out = append(out, &OpenAITTS{APIKey: key, Exec: exec})
	}
	if _, err := exec.LookPath("say"); err == nil {
		out = append(out, &SayTTS{Exec: exec})
	}
	return out
}
