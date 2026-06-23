package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// One shared fallback client so a future reader sees the singleton intent;
// callers that want a custom Transport set OpenAITTS.HTTPClient.
var defaultHTTPClient = sync.OnceValue(func() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
})

// OpenAITTS posts text to /v1/audio/speech, writes the returned audio to
// a temp file, then plays via `afplay`. Used as a fallback when Kokoro is
// unavailable. Metered: $30 / 1M chars at tts-1-hd.
type OpenAITTS struct {
	APIKey string
	Exec   Executor
	// HTTPClient overridable for tests. nil → 30s-timeout singleton.
	HTTPClient *http.Client
	// Voice defaults to "nova" (closest to Kokoro af_bella).
	Voice string
	// Model defaults to "tts-1-hd".
	Model string
	// Endpoint overridable for tests. Empty → real OpenAI endpoint.
	Endpoint string
}

const defaultOpenAIEndpoint = "https://api.openai.com/v1/audio/speech"

// writerCap is a bounded Writer that silently discards bytes past cap.
// Used to keep diagnostic-prefix capture O(cap), not O(stream).
type writerCap struct {
	w   *bytes.Buffer
	cap int
}

func (c *writerCap) Write(p []byte) (int, error) {
	if room := c.cap - c.w.Len(); room > 0 {
		if len(p) > room {
			c.w.Write(p[:room])
		} else {
			c.w.Write(p)
		}
	}
	return len(p), nil
}

// Single doorway for picking the request client; keeps the fallback singleton
// invisible from Speak and gives tests a non-flaky pointer-identity hook.
func (o *OpenAITTS) resolveHTTPClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return defaultHTTPClient()
}

// synth POSTs to OpenAI, writes the wav to a temp file, and returns its path;
// the caller removes it.
func (o *OpenAITTS) synth(ctx context.Context, text string) (string, error) {
	if o.APIKey == "" {
		return "", fmt.Errorf("openai tts: missing api key")
	}
	endpoint := o.Endpoint
	if endpoint == "" {
		endpoint = defaultOpenAIEndpoint
	}
	model := o.Model
	if model == "" {
		model = "tts-1-hd"
	}
	voice := o.Voice
	if voice == "" {
		voice = "nova"
	}

	body, err := json.Marshal(map[string]string{
		"model":           model,
		"voice":           voice,
		"input":           text,
		"response_format": "wav",
	})
	if err != nil {
		return "", fmt.Errorf("openai tts: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("openai tts: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.resolveHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("openai tts: post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai tts: status %d: %s", resp.StatusCode, string(b))
	}

	tmp, err := os.CreateTemp("", "leah-voice-openai-*.wav")
	if err != nil {
		return "", fmt.Errorf("openai tts: temp: %w", err)
	}
	path := tmp.Name()
	// Tee the first 512 bytes so a mid-stream copy failure still surfaces
	// the OpenAI error frame (often JSON) in the wrapped error.
	var head bytes.Buffer
	src := io.TeeReader(resp.Body, &writerCap{w: &head, cap: 512})
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("openai tts: copy: %w; body_prefix=%s", err, strconv.Quote(head.String()))
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("openai tts: close: %w", err)
	}
	return path, nil
}

// Speak synthesizes via OpenAI and plays the resulting audio via afplay.
func (o *OpenAITTS) Speak(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	path, err := o.synth(ctx, text)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()
	return withAudioDevice(func() error {
		if _, err := o.Exec.Run(ctx, "afplay", path); err != nil {
			return fmt.Errorf("openai tts afplay: %w", err)
		}
		return nil
	})
}

// Synthesize returns the wav bytes OpenAI produces, without playing them.
func (o *OpenAITTS) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	if text == "" {
		return nil, "", nil
	}
	path, err := o.synth(ctx, text)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = os.Remove(path) }()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("openai tts: read wav: %w", err)
	}
	return b, "audio/wav", nil
}
