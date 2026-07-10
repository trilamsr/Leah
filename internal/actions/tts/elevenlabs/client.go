// Package elevenlabs is the §17.17 cloud TTS provider. Flash v2.5 model
// only — multilingual_v2 is on the §15 rejection list (TTFB 600–1200 ms
// breaks the voice canon).
package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/trilam/leah/internal/tts"
)

const (
	defaultBaseURL = "https://api.elevenlabs.io"
	modelID        = "eleven_flash_v2_5"
)

// Client is the §17.17 ElevenLabs provider. apiKey + voiceID flow from the
// daemon side only; HUD never sees them.
type Client struct {
	apiKey  string
	voiceID string
	hc      *http.Client
	baseURL string
}

// New constructs a Client; nil hc falls back to http.DefaultClient.
func New(apiKey, voiceID string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{apiKey: apiKey, voiceID: voiceID, hc: hc, baseURL: defaultBaseURL}
}

// SetBaseURL retargets the HTTP base; httptest fixtures only.
func (c *Client) SetBaseURL(u string) { c.baseURL = u }

// VoiceID returns the constructor-supplied voice id.
func (c *Client) VoiceID() string { return c.voiceID }

func (c *Client) Name() string { return "elevenlabs" }

// PreWarm fires a tiny synthesis to /dev/null at daemon boot — amortizes the
// first-utterance TTFB (decision-log #81).
func (c *Client) PreWarm(ctx context.Context) error {
	stream, err := c.Speak(ctx, ".", "")
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	_, _ = io.Copy(io.Discard, stream)
	return nil
}

// Speak posts text to /v1/text-to-speech/<voice>/stream with Flash v2.5 and
// the lowest-latency optimization tier the API exposes. The returned stream
// is bound to ctx — cancel terminates mid-read via the response body.
func (c *Client) Speak(ctx context.Context, text, voice string) (tts.AudioStream, error) {
	if voice == "" {
		voice = c.voiceID
	}
	url := fmt.Sprintf("%s/v1/text-to-speech/%s/stream?optimize_streaming_latency=4&output_format=mp3_44100_128", c.baseURL, voice)
	payload, err := json.Marshal(map[string]any{
		"text":     text,
		"model_id": modelID,
		"voice_settings": map[string]any{
			"stability":        0.5,
			"similarity_boost": 0.75,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: request: %w", err)
	}
	req.Header.Set("xi-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: do: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("elevenlabs: status %d: %s", resp.StatusCode, body)
	}
	return &stream{rc: resp.Body, mime: resp.Header.Get("Content-Type")}, nil
}

type stream struct {
	rc   io.ReadCloser
	mime string
}

func (s *stream) Read(p []byte) (int, error) { return s.rc.Read(p) }
func (s *stream) Close() error               { return s.rc.Close() }
func (s *stream) MIME() string               { return s.mime }
