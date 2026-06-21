package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type fakeExec struct {
	runs    [][]string
	lookErr map[string]error
	runErr  error
	// onRun lets a test write the whisper .txt output the production code reads.
	onRun func(name string, args []string)
}

func (f *fakeExec) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.runs = append(f.runs, append([]string{name}, args...))
	if f.onRun != nil {
		f.onRun(name, args)
	}
	return nil, f.runErr
}

func (f *fakeExec) LookPath(name string) (string, error) {
	if f.lookErr != nil {
		if err, ok := f.lookErr[name]; ok {
			return "", err
		}
	}
	return "/usr/bin/" + name, nil
}

func audioWebhook(t *testing.T, secret, mediaID string) ([]byte, string) {
	t.Helper()
	payload := []byte(`{"entry":[{"changes":[{"value":{"messages":[{"from":"14155551234","timestamp":"1700000000","type":"audio","audio":{"id":"` + mediaID + `"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return payload, "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookHandle_AudioMessage_ParsesMediaID(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{secret: "app-secret"}, &fakeHTTP{}, &recordingSink{}, []string{"+1"})

	payload, sig := audioWebhook(t, "app-secret", "MEDIA123")
	msgs, err := a.WebhookHandle(context.Background(), payload, sig)
	if err != nil {
		t.Fatalf("WebhookHandle: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Type != "audio" || msgs[0].MediaID != "MEDIA123" {
		t.Fatalf("messages=%+v want one audio msg with MediaID=MEDIA123", msgs)
	}
}

func TestWebhookHandle_AudioBadHMAC_NeverParses(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{secret: "app-secret"}, &fakeHTTP{}, sink, []string{"+1"})

	payload, _ := audioWebhook(t, "app-secret", "MEDIA123")
	_, err := a.WebhookHandle(context.Background(), payload, "sha256=deadbeef")
	if !errors.Is(err, ErrWebhookHMACInvalid) {
		t.Fatalf("err=%v want ErrWebhookHMACInvalid on audio path", err)
	}
	if len(sink.rows) == 0 || sink.rows[0].Kind != "whatsapp_webhook_hmac_invalid" {
		t.Fatalf("audio webhook bypassed HMAC verify: rows=%+v", sink.rows)
	}
}

func TestTranscribeAudio_FetchTranscodeTranscribe(t *testing.T) {
	t.Parallel()
	media := &mediaHTTP{lookupBody: `{"url":"https://media.example/dl/abc"}`, audioBody: "OGGOPUSBYTES"}
	fx := &fakeExec{onRun: func(name string, args []string) {
		if name == "whisper-cli" {
			writeWhisperOut(t, args, "hello from voice note")
		}
	}}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{tok: "bearer"}, media, &recordingSink{}, []string{"+1"})
	a.audio = fx

	text, err := a.TranscribeAudio(context.Background(), Message{From: "14155551234", Type: "audio", MediaID: "MEDIA123"})
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if text != "hello from voice note" {
		t.Fatalf("text=%q want transcript", text)
	}
	if media.lookupAuth != "Bearer bearer" || media.dlAuth != "Bearer bearer" {
		t.Fatalf("media fetch missing bearer: lookup=%q dl=%q", media.lookupAuth, media.dlAuth)
	}
	var sawFFmpeg, sawWhisper bool
	for _, r := range fx.runs {
		if r[0] == "ffmpeg" {
			sawFFmpeg = true
		}
		if r[0] == "whisper-cli" {
			sawWhisper = true
		}
	}
	if !sawFFmpeg || !sawWhisper {
		t.Fatalf("expected ffmpeg+whisper-cli runs, got %+v", fx.runs)
	}
}

func TestTranscribeAudio_FFmpegAbsent_DegradesNotCrash(t *testing.T) {
	t.Parallel()
	media := &mediaHTTP{lookupBody: `{"url":"https://media.example/dl/abc"}`, audioBody: "x"}
	fx := &fakeExec{lookErr: map[string]error{"ffmpeg": errors.New("not found")}}
	sink := &recordingSink{}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{tok: "bearer"}, media, sink, []string{"+1"})
	a.audio = fx

	text, err := a.TranscribeAudio(context.Background(), Message{Type: "audio", MediaID: "M"})
	if err != nil {
		t.Fatalf("ffmpeg-absent must degrade, got err=%v", err)
	}
	if text != "" {
		t.Fatalf("text=%q want empty on degrade", text)
	}
	if !hasReason(sink.rows, "ffmpeg_absent") {
		t.Fatalf("expected ffmpeg_absent audit row, got %+v", sink.rows)
	}
}

func TestTranscribeAudio_MediaFetch401(t *testing.T) {
	t.Parallel()
	media := &mediaHTTP{lookupStatus: 401, lookupBody: `{"error":"bad token"}`}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{tok: "stale"}, media, &recordingSink{}, []string{"+1"})
	a.audio = &fakeExec{}

	_, err := a.TranscribeAudio(context.Background(), Message{Type: "audio", MediaID: "M"})
	if err == nil {
		t.Fatal("expected error on 401 media lookup")
	}
}

func hasReason(rows []AuditRow, reason string) bool {
	for _, r := range rows {
		if r.Reason == reason {
			return true
		}
	}
	return false
}

func writeWhisperOut(t *testing.T, args []string, text string) {
	t.Helper()
	for i, a := range args {
		if a == "-of" && i+1 < len(args) {
			if err := os.WriteFile(args[i+1]+".txt", []byte(text+"\n"), 0o600); err != nil {
				t.Fatalf("write whisper out: %v", err)
			}
		}
	}
}

// mediaHTTP fakes the two-hop WhatsApp media flow: GET /{media-id} returns a
// JSON {url}; GET {url} returns the audio bytes. Captures the bearer per hop.
type mediaHTTP struct {
	lookupBody, audioBody string
	lookupStatus          int
	lookupAuth, dlAuth    string
}

func (m *mediaHTTP) Do(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Host, "media.example") {
		m.dlAuth = req.Header.Get("Authorization")
		return resp(200, m.audioBody), nil
	}
	m.lookupAuth = req.Header.Get("Authorization")
	st := m.lookupStatus
	if st == 0 {
		st = 200
	}
	return resp(st, m.lookupBody), nil
}

func resp(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader([]byte(body))), Header: http.Header{}}
}
