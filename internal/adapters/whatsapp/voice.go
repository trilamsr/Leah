package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// AudioExec abstracts the ffmpeg / whisper-cli shell hops so tests inject a
// fake without those binaries on the host. Same shape as internal/voice.Executor.
type AudioExec interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	LookPath(name string) (string, error)
}

type shellAudioExec struct{}

func (shellAudioExec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
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

func (shellAudioExec) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (a *Adapter) audioExec() AudioExec {
	if a.audio != nil {
		return a.audio
	}
	return shellAudioExec{}
}

// TranscribeAudio fetches an inbound voice note (Bearer media fetch),
// transcodes ogg/opus→wav via ffmpeg, and transcribes via whisper-cli. A
// missing binary degrades to an empty transcript + audit row, never a crash.
func (a *Adapter) TranscribeAudio(ctx context.Context, m Message) (string, error) {
	if m.MediaID == "" {
		return "", fmt.Errorf("whatsapp: audio message missing media id")
	}
	ax := a.audioExec()
	if _, err := ax.LookPath("ffmpeg"); err != nil {
		a.record(AuditRow{Kind: "whatsapp_audio_in", Success: false, RecipientHash: hashRecipient(m.From), Reason: "ffmpeg_absent"})
		return "", nil
	}
	if _, err := ax.LookPath("whisper-cli"); err != nil {
		a.record(AuditRow{Kind: "whatsapp_audio_in", Success: false, RecipientHash: hashRecipient(m.From), Reason: "whisper_absent"})
		return "", nil
	}

	ogg, err := a.fetchMedia(ctx, m.MediaID)
	if err != nil {
		a.record(AuditRow{Kind: "whatsapp_audio_in", Success: false, RecipientHash: hashRecipient(m.From), Reason: "media_fetch"})
		return "", err
	}

	text, err := transcribe(ctx, ax, ogg)
	if err != nil {
		a.record(AuditRow{Kind: "whatsapp_audio_in", Success: false, RecipientHash: hashRecipient(m.From), Reason: "transcode"})
		return "", err
	}
	a.record(AuditRow{Kind: "whatsapp_audio_in", Success: true, RecipientHash: hashRecipient(m.From), BodyLen: len(text)})
	return text, nil
}

func (a *Adapter) fetchMedia(ctx context.Context, mediaID string) ([]byte, error) {
	tok, err := a.ts.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: token load: %w", err)
	}
	lookupURL := "https://graph.facebook.com/" + graphAPIVersion + "/" + mediaID
	url, err := a.getMediaURL(ctx, lookupURL, tok)
	if err != nil {
		return nil, err
	}
	return a.getBytes(ctx, url, tok)
}

func (a *Adapter) getMediaURL(ctx context.Context, lookupURL, tok string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lookupURL, nil)
	if err != nil {
		return "", fmt.Errorf("whatsapp: media lookup request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := a.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("whatsapp: media lookup: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("whatsapp: media lookup status %d: %s", resp.StatusCode, string(body))
	}
	var meta struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &meta); err != nil || meta.URL == "" {
		return "", fmt.Errorf("whatsapp: media lookup missing url: %s", string(body))
	}
	return meta.URL, nil
}

func (a *Adapter) getBytes(ctx context.Context, url, tok string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: media download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: media download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("whatsapp: media download status %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

func transcribe(ctx context.Context, ax AudioExec, ogg []byte) (string, error) {
	in, err := os.CreateTemp("", "leah-wa-*.ogg")
	if err != nil {
		return "", fmt.Errorf("whatsapp: temp ogg: %w", err)
	}
	oggPath := in.Name()
	defer func() { _ = os.Remove(oggPath) }()
	if _, err := in.Write(ogg); err != nil {
		_ = in.Close()
		return "", fmt.Errorf("whatsapp: write ogg: %w", err)
	}
	_ = in.Close()

	wavPath := strings.TrimSuffix(oggPath, ".ogg") + ".wav"
	defer func() { _ = os.Remove(wavPath) }()
	if _, err := ax.Run(ctx, "ffmpeg", "-y", "-i", oggPath, "-ar", "16000", "-ac", "1", wavPath); err != nil {
		return "", fmt.Errorf("whatsapp: ffmpeg transcode: %w", err)
	}

	txtBase := strings.TrimSuffix(oggPath, ".ogg")
	defer func() { _ = os.Remove(txtBase + ".txt") }()
	if _, err := ax.Run(ctx, "whisper-cli", "-m", "models/ggml-large-v3-turbo-q5_0.bin", "-f", wavPath, "-nt", "-np", "-otxt", "-of", txtBase); err != nil {
		return "", fmt.Errorf("whatsapp: whisper-cli transcribe: %w", err)
	}
	raw, err := os.ReadFile(txtBase + ".txt")
	if err != nil {
		return "", fmt.Errorf("whatsapp: read transcript: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}
