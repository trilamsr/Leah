package discord

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

const ScopePostVoice = "discord:post_voice"

// Discord caps bot file uploads at 8MB on the free tier. Reject larger
// payloads BEFORE attestation so a wrong-sized buffer never burns a prompt.
const maxVoiceBytes = 8 * 1024 * 1024

// voiceAttachmentName is the filename Discord renders as a playable audio
// attachment. The `.ogg` extension is load-bearing — Discord uses it (not
// content-type) to decide whether to render the inline audio player.
const voiceAttachmentName = "leah-voice.ogg"

var (
	ErrAttachmentTooLarge = errors.New("discord: attachment exceeds 8MB limit")
	ErrEmptyAudio         = errors.New("discord: empty audio buffer")
)

// PostVoice uploads audio as a voice-message attachment on a channel.
// Order mirrors PostMessage: validate -> rate-limit -> attest -> token -> POST.
// Rate limiter is shared with PostMessage on purpose: a compromised Leah
// must not be able to bypass the per-channel 10/60s cap by alternating
// text + voice.
func (a *Adapter) PostVoice(ctx context.Context, channelID string, audio []byte) error {
	hash := shortHash(channelID)
	voiceHash := voiceShortHash(audio)

	if channelID == "" {
		return ErrEmptyChannel
	}
	if len(audio) == 0 {
		return ErrEmptyAudio
	}
	if len(audio) > maxVoiceBytes {
		a.record(AuditRow{Kind: "discord_post_voice", ChannelHash: hash, VoiceSHA: voiceHash, Reason: "attachment_too_large"})
		return ErrAttachmentTooLarge
	}
	if !a.limiter.Allow(channelID) {
		a.record(AuditRow{Kind: "discord_post_voice", ChannelHash: hash, VoiceSHA: voiceHash, Reason: "rate_limited"})
		return ErrRateLimited
	}
	if err := a.att.Attest(ctx, ScopePostVoice); err != nil {
		a.record(AuditRow{Kind: "discord_post_voice", ChannelHash: hash, VoiceSHA: voiceHash, Reason: "attestation_denied"})
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.Token(ctx)
	if err != nil {
		a.record(AuditRow{Kind: "discord_post_voice", ChannelHash: hash, VoiceSHA: voiceHash, Reason: "token_load_failed"})
		return fmt.Errorf("discord: token load: %w", err)
	}

	body, ctype, err := buildVoiceMultipart(audio)
	if err != nil {
		a.record(AuditRow{Kind: "discord_post_voice", ChannelHash: hash, VoiceSHA: voiceHash, Reason: "multipart_build"})
		return fmt.Errorf("%w: %v", ErrPostFailed, err)
	}

	url := fmt.Sprintf("%s/channels/%s/messages", a.baseURL, channelID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		a.record(AuditRow{Kind: "discord_post_voice", ChannelHash: hash, VoiceSHA: voiceHash, Reason: "request_build"})
		return fmt.Errorf("%w: %v", ErrPostFailed, err)
	}
	req.Header.Set("Authorization", "Bot "+tok)
	req.Header.Set("Content-Type", ctype)

	start := time.Now()
	resp, err := a.http.Do(req)
	a.m.ObserveAPI("post_voice", time.Since(start).Seconds())
	if err != nil {
		a.record(AuditRow{Kind: "discord_post_voice", ChannelHash: hash, VoiceSHA: voiceHash, Reason: "http_error"})
		return fmt.Errorf("%w: %v", ErrPostFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		a.record(AuditRow{Kind: "discord_post_voice", ChannelHash: hash, VoiceSHA: voiceHash, Reason: fmt.Sprintf("status_%d", resp.StatusCode)})
		return fmt.Errorf("%w: status %d", ErrPostFailed, resp.StatusCode)
	}
	a.record(AuditRow{Kind: "discord_post_voice", Success: true, ChannelHash: hash, VoiceSHA: voiceHash, BodyLen: len(audio)})
	return nil
}

// buildVoiceMultipart wraps the audio bytes in a Discord-compatible
// multipart/form-data body. The `files[0]` field name is Discord's
// documented attachment slot name.
func buildVoiceMultipart(audio []byte) (io.Reader, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files[0]"; filename=%q`, voiceAttachmentName))
	h.Set("Content-Type", "audio/ogg")
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(audio); err != nil {
		return nil, "", err
	}

	if err := w.WriteField("payload_json", `{"attachments":[{"id":0,"filename":"`+voiceAttachmentName+`"}]}`); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return &buf, w.FormDataContentType(), nil
}

// voiceShortHash returns sha256(audio)[:8] hex, "" when audio is empty.
// Audit rows store this instead of the bytes themselves.
func voiceShortHash(audio []byte) string {
	if len(audio) == 0 {
		return ""
	}
	sum := sha256.Sum256(audio)
	return hex.EncodeToString(sum[:])[:8]
}

// fetchVoiceAttachment downloads an inbound voice attachment by URL.
// The bot token is required: Discord CDN URLs accept the bot
// Authorization header. Returns nil bytes on any error — voice is
// best-effort enrichment of the inbound Message, never a hard failure.
func (a *Adapter) fetchVoiceAttachment(ctx context.Context, url string) []byte {
	if url == "" {
		return nil
	}
	tok, err := a.ts.Token(ctx)
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bot "+tok)
	start := time.Now()
	resp, err := a.http.Do(req)
	a.m.ObserveAPI("fetch_voice", time.Since(start).Seconds())
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil
	}
	// Cap inbound at maxVoiceBytes so a malicious CDN response cannot
	// balloon memory; matches the outbound limit.
	r := io.LimitReader(resp.Body, maxVoiceBytes+1)
	data, err := io.ReadAll(r)
	if err != nil || len(data) > maxVoiceBytes {
		return nil
	}
	return data
}

// isVoiceAttachment recognizes Discord's voice-message attachments.
// Discord marks them with content_type starting with "audio/" — the
// official voice-message client uploads `audio/ogg`. We accept any
// audio/* to stay forward-compatible with future codecs.
func isVoiceAttachment(contentType, filename string) bool {
	if strings.HasPrefix(strings.ToLower(contentType), "audio/") {
		return true
	}
	low := strings.ToLower(filename)
	return strings.HasSuffix(low, ".ogg") || strings.HasSuffix(low, ".opus") || strings.HasSuffix(low, ".mp3") || strings.HasSuffix(low, ".m4a") || strings.HasSuffix(low, ".wav")
}
