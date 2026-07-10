package voice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// ListenConfig holds the knobs for one push-to-talk session.
//
// Duration caps the maximum recording length (sox -d --duration semantics).
// When zero, sox runs without an explicit cap and the silence-detector is the
// only stop condition — operators using Ctrl-C still abort cleanly because
// the underlying context is honored by Executor.Run.
//
// Model is the whisper.cpp ggml model filename relative to ModelDir. The
// default (large-v3-turbo-q5_0) was chosen by the adopt-vs-build survey for
// the quality/RAM tradeoff that fits a 16GB-class personal machine.
type ListenConfig struct {
	Duration time.Duration
	Model    string
	ModelDir string // directory containing ggml-*.bin
	WavPath  string // temp wav path; empty → /tmp/leah-listen.wav
	TxtPath  string // whisper-cli -of base path; empty → /tmp/leah-listen (will write .txt)
}

// Listen records a single utterance via `sox` (auto-stops on silence) then
// transcribes it via `whisper-cli`. The returned text is the trimmed body
// of the .txt file whisper-cli writes.
//
// Both binaries are probed via exec.LookPath first so a missing dep returns
// an actionable install-hint error rather than a cryptic exec failure.
func Listen(ctx context.Context, exe Executor, cfg ListenConfig) (string, error) {
	if _, err := exe.LookPath("sox"); err != nil {
		return "", errors.New("sox not found in PATH — install: brew install sox")
	}
	if _, err := exe.LookPath("whisper-cli"); err != nil {
		return "", errors.New("whisper-cli not found in PATH — install: brew install whisper-cpp")
	}

	wav := cfg.WavPath
	if wav == "" {
		wav = "/tmp/leah-listen.wav"
	}
	txtBase := cfg.TxtPath
	if txtBase == "" {
		txtBase = "/tmp/leah-listen"
	}
	model := cfg.Model
	if model == "" {
		model = "ggml-large-v3-turbo-q5_0.bin"
	}
	modelDir := cfg.ModelDir
	if modelDir == "" {
		modelDir = "models"
	}
	modelPath := modelDir + "/" + model

	// Clean any prior artifacts so a partial run cannot be silently
	// re-transcribed.
	_ = os.Remove(wav)
	_ = os.Remove(txtBase + ".txt")

	// sox flags:
	//   -d                read from default audio input device
	//   <wav>             write to wav
	//   silence 1 0.1 3%  start recording immediately
	//   1 1.0 3%          stop after 1s of audio < 3% threshold
	//   trim 0 <dur>      cap total length when Duration > 0
	soxArgs := []string{"-d", wav, "silence", "1", "0.1", "3%", "1", "1.0", "3%"}
	if cfg.Duration > 0 {
		soxArgs = append(soxArgs, "trim", "0", fmt.Sprintf("%.2f", cfg.Duration.Seconds()))
	}
	if _, err := exe.Run(ctx, "sox", soxArgs...); err != nil {
		return "", fmt.Errorf("sox record: %w", err)
	}

	// Sanity check — sox should have produced a non-empty wav.
	if fi, err := os.Stat(wav); err != nil || fi.Size() == 0 {
		return "", fmt.Errorf("sox produced no audio at %s", wav)
	}

	// whisper-cli flags:
	//   -m <model>        ggml model
	//   -f <wav>          input audio
	//   -nt               no timestamps in output
	//   -np               no progress prints to stderr
	//   -otxt             write plain .txt
	//   -of <base>        output file base (no extension)
	whisperArgs := []string{"-m", modelPath, "-f", wav, "-nt", "-np", "-otxt", "-of", txtBase}
	if _, err := exe.Run(ctx, "whisper-cli", whisperArgs...); err != nil {
		return "", fmt.Errorf("whisper-cli transcribe: %w", err)
	}

	raw, err := os.ReadFile(txtBase + ".txt")
	if err != nil {
		return "", fmt.Errorf("read transcript: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}
