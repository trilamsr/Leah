package discord

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/testutil"
)

// TestPostVoice_HappyPath: audio bytes reach Discord REST as multipart
// after attestation + token load, with the leah-voice.ogg filename intact.
func TestPostVoice_HappyPath(t *testing.T) {
	t.Parallel()
	var gotAuth, gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg-voice-1"}`))
	}))
	defer srv.Close()

	att := &fakeAttestor{}
	ts := &fakeTokenSource{}
	audit := &fakeAudit{}
	a := newTestAdapter(t, att, ts, srv, nil, audit)

	audio := []byte("OggS-fake-voice-payload-bytes")
	if err := a.PostVoice(context.Background(), "chan-vc", audio); err != nil {
		t.Fatalf("PostVoice: %v", err)
	}
	if att.lastScope != ScopePostVoice {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopePostVoice)
	}
	if ts.called != 1 {
		t.Fatalf("token loaded %d times, want 1", ts.called)
	}
	if gotAuth != "Bot bot-token-xyz" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !strings.HasPrefix(gotCT, "multipart/form-data") {
		t.Fatalf("Content-Type = %q, want multipart/form-data", gotCT)
	}
	if !bytes.Contains(gotBody, audio) {
		t.Fatalf("uploaded body missing audio payload")
	}
	if !bytes.Contains(gotBody, []byte("leah-voice.ogg")) {
		t.Fatalf("uploaded body missing leah-voice.ogg filename: %s", gotBody)
	}
	if len(audit.rows) != 1 || !audit.rows[0].Success {
		t.Fatalf("audit rows = %+v", audit.rows)
	}
	if audit.rows[0].VoiceSHA == "" {
		t.Fatalf("audit row missing VoiceSHA")
	}
}

// TestPostVoice_AttestationDenied_NoCall: HTTP MUST NOT fire on deny;
// token never loaded — denial happens before secret materialization.
func TestPostVoice_AttestationDenied_NoCall(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("HTTP MUST NOT fire on attestation deny")
	}))
	defer srv.Close()
	att := &fakeAttestor{err: errors.New("denied")}
	ts := &fakeTokenSource{}
	audit := &fakeAudit{}
	a := newTestAdapter(t, att, ts, srv, nil, audit)

	err := a.PostVoice(context.Background(), "chan-1", []byte("audio"))
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if ts.called != 0 {
		t.Fatalf("token loaded %d times on deny", ts.called)
	}
	if len(audit.rows) != 1 || audit.rows[0].Success {
		t.Fatalf("audit rows = %+v", audit.rows)
	}
}

// TestPostVoice_RateLimit_RejectsBurst: the rate window is shared with
// PostMessage. 10 mixed sends saturate the window; the 11th — voice — is
// rejected without attesting or hitting HTTP.
func TestPostVoice_RateLimit_RejectsBurst(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer srv.Close()

	start := time.Unix(1700000000, 0)
	tick := start
	now := func() time.Time { return tick }

	att := &fakeAttestor{}
	a := newTestAdapter(t, att, &fakeTokenSource{}, srv, now, nil)
	for i := 0; i < 10; i++ {
		if err := a.PostMessage(context.Background(), "chan-A", "msg"); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
		tick = tick.Add(3 * time.Second)
	}
	attBefore := att.called
	if err := a.PostVoice(context.Background(), "chan-A", []byte("audio")); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("11th err = %v, want ErrRateLimited", err)
	}
	if att.called != attBefore {
		t.Fatalf("attestor invoked on rate-limited voice send")
	}
}

// TestPostVoice_AudioBoundedSize: 9MB rejected pre-attestation; 7MB passes.
// Size check sits BEFORE rate limit + attestation so an obviously-bad
// buffer wastes neither a quota slot nor an operator prompt.
func TestPostVoice_AudioBoundedSize(t *testing.T) {
	t.Parallel()
	var httpCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&httpCalls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	}))
	defer srv.Close()

	att := &fakeAttestor{}
	a := newTestAdapter(t, att, &fakeTokenSource{}, srv, nil, nil)

	big := make([]byte, 9*1024*1024)
	if err := a.PostVoice(context.Background(), "chan-1", big); !errors.Is(err, ErrAttachmentTooLarge) {
		t.Fatalf("9MB err = %v, want ErrAttachmentTooLarge", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor invoked for oversized audio")
	}
	if atomic.LoadInt32(&httpCalls) != 0 {
		t.Fatalf("HTTP invoked for oversized audio")
	}

	ok := make([]byte, 7*1024*1024)
	if err := a.PostVoice(context.Background(), "chan-1", ok); err != nil {
		t.Fatalf("7MB PostVoice: %v", err)
	}
	if atomic.LoadInt32(&httpCalls) != 1 {
		t.Fatalf("HTTP calls = %d, want 1", atomic.LoadInt32(&httpCalls))
	}
}

// TestPostVoice_EmptyAudio_NoAttestation: validation precedes attest —
// callers wiring up empty TTS buffers must not burn operator prompts.
func TestPostVoice_EmptyAudio_NoAttestation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("HTTP MUST NOT fire on empty-audio validation")
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, &fakeTokenSource{}, srv, nil, nil)
	if err := a.PostVoice(context.Background(), "chan-x", nil); !errors.Is(err, ErrEmptyAudio) {
		t.Fatalf("err = %v, want ErrEmptyAudio", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor invoked on empty audio")
	}
}

// TestPostVoice_AuditRowNoPlaintext: audit row stores hashed channel +
// hashed voice digest, never the channel ID or the audio bytes.
func TestPostVoice_AuditRowNoPlaintext(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer srv.Close()
	audit := &fakeAudit{}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{}, srv, nil, audit)
	const ch = "channel-secret-voice"
	audio := []byte("super-secret-audio-bytes")
	if err := a.PostVoice(context.Background(), ch, audio); err != nil {
		t.Fatalf("PostVoice: %v", err)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("audit rows = %d", len(audit.rows))
	}
	row := audit.rows[0]
	sum := sha256.Sum256([]byte(ch))
	wantCh := hex.EncodeToString(sum[:])[:8]
	if row.ChannelHash != wantCh {
		t.Fatalf("ChannelHash = %q, want %q", row.ChannelHash, wantCh)
	}
	vsum := sha256.Sum256(audio)
	wantV := hex.EncodeToString(vsum[:])[:8]
	if row.VoiceSHA != wantV {
		t.Fatalf("VoiceSHA = %q, want %q", row.VoiceSHA, wantV)
	}
	blob, _ := json.Marshal(row)
	if strings.Contains(string(blob), ch) {
		t.Fatalf("plaintext channel id leaked: %s", blob)
	}
	if strings.Contains(string(blob), string(audio)) {
		t.Fatalf("plaintext audio leaked: %s", blob)
	}
}

// TestSubscribe_InboundVoiceAttachmentFetched: a MESSAGE_CREATE with a
// voice attachment causes the dispatch goroutine to fetch the CDN bytes
// and populate Message.Voice — fetch is bot-token authenticated.
func TestSubscribe_InboundVoiceAttachmentFetched(t *testing.T) {
	t.Parallel()
	const voicePayload = "OggS-INBOUND-VOICE-BYTES"
	var fetchAuth string
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(voicePayload))
	}))
	defer cdn.Close()

	frame := voiceMessageFrame(t, "chan-1", "guild-A", "user-1", cdn.URL+"/voice-message.ogg")
	conn := newFakeConn([][]byte{frame})
	dialer := &fakeDialer{conn: conn}

	att := &fakeAttestor{}
	ts := &fakeTokenSource{}
	audit := &fakeAudit{}
	a, err := New(Config{
		Attestor:        att,
		TokenSource:     ts,
		HTTPClient:      cdn.Client(),
		WebSocketDialer: dialer,
		GuildAllowlist:  []string{"guild-A"},
		Audit:           audit,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var got Message
	var gotOnce sync.Once
	done := make(chan struct{})
	handler := func(m Message) {
		gotOnce.Do(func() {
			got = m
			close(done)
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Subscribe(ctx, []string{"chan-1"}, handler); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	})
	if string(got.Voice) != voicePayload {
		t.Fatalf("Voice = %q, want %q", got.Voice, voicePayload)
	}
	if fetchAuth != "Bot bot-token-xyz" {
		t.Fatalf("CDN Authorization = %q", fetchAuth)
	}
}

// voiceMessageFrame builds a gateway MESSAGE_CREATE payload with a
// single audio/ogg attachment — mirrors Discord's voice-message shape.
func voiceMessageFrame(t *testing.T, channelID, guildID, authorID, url string) []byte {
	t.Helper()
	frame := map[string]any{
		"op": 0,
		"t":  "MESSAGE_CREATE",
		"d": map[string]any{
			"channel_id": channelID,
			"guild_id":   guildID,
			"content":    "",
			"author":     map[string]any{"id": authorID},
			"timestamp":  "2026-06-10T12:00:00Z",
			"attachments": []map[string]any{{
				"url":          url,
				"filename":     "voice-message.ogg",
				"content_type": "audio/ogg",
				"size":         32,
			}},
		},
	}
	b, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
