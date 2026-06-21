package voice

import (
	"context"
	"fmt"
	"os"
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

// synth runs kokoro into a temp wav and returns its path; the caller removes it.
func (k *KokoroTTS) synth(ctx context.Context, text string) (string, error) {
	tmp, err := os.CreateTemp("", "leah-voice-*.wav")
	if err != nil {
		return "", fmt.Errorf("kokoro: temp file: %w", err)
	}
	path := tmp.Name()
	_ = tmp.Close()

	args := []string{"-t", text, "-o", path}
	if k.Voice != "" {
		args = append(args, "--voice", k.Voice)
	}
	if _, err := k.Exec.Run(ctx, "kokoro", args...); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("kokoro synth: %w", err)
	}
	// Sanity check — kokoro should have written the file.
	if fi, err := os.Stat(path); err != nil || fi.Size() == 0 {
		_ = os.Remove(path)
		// Full path (not basename) — caller needs the directory to
		// investigate why kokoro produced nothing (audit L5).
		return "", fmt.Errorf("kokoro: empty wav at %s", path)
	}
	return path, nil
}

// Speak synthesizes text to a temp wav and plays it via afplay.
func (k *KokoroTTS) Speak(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	path, err := k.synth(ctx, text)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()
	return withAudioDevice(func() error {
		if _, err := k.Exec.Run(ctx, "afplay", path); err != nil {
			return fmt.Errorf("kokoro afplay: %w", err)
		}
		return nil
	})
}

// Synthesize returns the wav bytes kokoro produces, without playing them.
func (k *KokoroTTS) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	if text == "" {
		return nil, "", nil
	}
	path, err := k.synth(ctx, text)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = os.Remove(path) }()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("kokoro: read wav: %w", err)
	}
	return b, "audio/wav", nil
}
