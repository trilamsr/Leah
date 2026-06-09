package voice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// KokoroTTS shells out to the `kokoro` CLI to synthesize a wav file, then
// plays it via `afplay`. Cost-zero, local, ~500ms cold-start on Apple MPS.
// The two-step (synthesize → play) keeps the audio path identical across
// Kokoro and OpenAI backends so cancellation semantics match.
type KokoroTTS struct {
	Exec Executor
	// Voice is the Kokoro voice id (e.g. "af_bella"). Empty = Kokoro default.
	Voice string
}

// Speak synthesizes text to a temp wav and plays it via afplay.
func (k *KokoroTTS) Speak(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	tmp, err := os.CreateTemp("", "leah-voice-*.wav")
	if err != nil {
		return fmt.Errorf("kokoro: temp file: %w", err)
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(path) }()

	args := []string{"-t", text, "-o", path}
	if k.Voice != "" {
		args = append(args, "--voice", k.Voice)
	}
	if _, err := k.Exec.Run(ctx, "kokoro", args...); err != nil {
		return fmt.Errorf("kokoro synth: %w", err)
	}
	// Sanity check — kokoro should have written the file.
	if fi, err := os.Stat(path); err != nil || fi.Size() == 0 {
		return fmt.Errorf("kokoro: empty wav at %s", filepath.Base(path))
	}
	if _, err := k.Exec.Run(ctx, "afplay", path); err != nil {
		return fmt.Errorf("kokoro afplay: %w", err)
	}
	return nil
}
