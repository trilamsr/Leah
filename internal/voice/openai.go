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
	"time"
)

// OpenAITTS posts text to /v1/audio/speech, writes the returned audio to
// a temp file, then plays via `afplay`. Used as a fallback when Kokoro is
// unavailable. Metered: $30 / 1M chars at tts-1-hd.
type OpenAITTS struct {
	APIKey string
	Exec   Executor
	// HTTPClient overridable for tests. nil → http.DefaultClient.
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

// Speak synthesizes via OpenAI and plays the resulting audio via afplay.
func (o *OpenAITTS) Speak(ctx context.Context, text string) error {
	if text == "" {
		return nil
	}
	if o.APIKey == "" {
		return fmt.Errorf("openai tts: missing api key")
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
		return fmt.Errorf("openai tts: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("openai tts: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := o.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("openai tts: post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openai tts: status %d: %s", resp.StatusCode, string(b))
	}

	tmp, err := os.CreateTemp("", "leah-voice-openai-*.wav")
	if err != nil {
		return fmt.Errorf("openai tts: temp: %w", err)
	}
	path := tmp.Name()
	defer func() { _ = os.Remove(path) }()
	// Tee the first 512 bytes so a mid-stream copy failure still surfaces
	// the OpenAI error frame (often JSON) in the wrapped error.
	var head bytes.Buffer
	src := io.TeeReader(resp.Body, &writerCap{w: &head, cap: 512})
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("openai tts: copy: %w; body_prefix=%s", err, strconv.Quote(head.String()))
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("openai tts: close: %w", err)
	}

	return withAudioDevice(func() error {
		if _, err := o.Exec.Run(ctx, "afplay", path); err != nil {
			return fmt.Errorf("openai tts afplay: %w", err)
		}
		return nil
	})
}
