package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openai tts: status %d: %s", resp.StatusCode, string(b))
	}

	tmp, err := os.CreateTemp("", "leah-voice-openai-*.wav")
	if err != nil {
		return fmt.Errorf("openai tts: temp: %w", err)
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("openai tts: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("openai tts: close: %w", err)
	}

	if _, err := o.Exec.Run(ctx, "afplay", path); err != nil {
		return fmt.Errorf("openai tts afplay: %w", err)
	}
	return nil
}
