package discord

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeAttestor records scope so tests can assert per-RPC gate routing.
type fakeAttestor struct {
	called    int
	lastScope string
	err       error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	f.lastScope = scope
	if !strings.HasPrefix(scope, "discord:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

type fakeTokenSource struct {
	called int
	tok    string
	err    error
}

func (f *fakeTokenSource) Token(_ context.Context) (string, error) {
	f.called++
	if f.tok == "" {
		f.tok = "bot-token-xyz"
	}
	return f.tok, f.err
}

type fakeAudit struct {
	rows []AuditRow
}

func (f *fakeAudit) Record(r AuditRow) { f.rows = append(f.rows, r) }

func newTestAdapter(t *testing.T, att Attestor, ts TokenSource, srv *httptest.Server, now func() time.Time, audit AuditSink) *Adapter {
	t.Helper()
	a, err := New(Config{
		Attestor:    att,
		TokenSource: ts,
		HTTPClient:  srv.Client(),
		BaseURL:     srv.URL,
		Now:         now,
		Audit:       audit,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// TestPostMessage_HappyPath: body+channel reach Discord REST after attestation+token load.
func TestPostMessage_HappyPath(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		var payload struct{ Content string }
		_ = json.NewDecoder(r.Body).Decode(&payload)
		gotBody = payload.Content
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg-1"}`))
	}))
	defer srv.Close()

	att := &fakeAttestor{}
	ts := &fakeTokenSource{}
	audit := &fakeAudit{}
	a := newTestAdapter(t, att, ts, srv, nil, audit)

	if err := a.PostMessage(context.Background(), "chan-123", "hello"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if att.lastScope != ScopePost {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopePost)
	}
	if ts.called != 1 {
		t.Fatalf("token loaded %d times, want 1", ts.called)
	}
	if gotAuth != "Bot bot-token-xyz" {
		t.Fatalf("Authorization = %q, want Bot bot-token-xyz", gotAuth)
	}
	if !strings.Contains(gotPath, "/channels/chan-123/messages") {
		t.Fatalf("path = %q, want channels/chan-123/messages", gotPath)
	}
	if gotBody != "hello" {
		t.Fatalf("body = %q, want hello", gotBody)
	}
	if len(audit.rows) != 1 || !audit.rows[0].Success {
		t.Fatalf("audit rows = %+v", audit.rows)
	}
}

// TestPostMessage_AttestationDenied: HTTP MUST NOT fire on deny; token never loaded.
func TestPostMessage_AttestationDenied(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("HTTP MUST NOT be called on attestation deny")
	}))
	defer srv.Close()
	att := &fakeAttestor{err: errors.New("denied")}
	ts := &fakeTokenSource{}
	audit := &fakeAudit{}
	a := newTestAdapter(t, att, ts, srv, nil, audit)

	err := a.PostMessage(context.Background(), "chan-1", "hi")
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err = %v, want ErrAttestationDenied", err)
	}
	if ts.called != 0 {
		t.Fatalf("token loaded %d times on deny — leak risk", ts.called)
	}
	if len(audit.rows) != 1 || audit.rows[0].Success {
		t.Fatalf("audit rows = %+v", audit.rows)
	}
}

// TestPostMessage_EmptyChannelID_NoAttestation: validation runs before attest.
func TestPostMessage_EmptyChannelID_NoAttestation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("HTTP MUST NOT fire on validation failure")
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, &fakeTokenSource{}, srv, nil, nil)
	if err := a.PostMessage(context.Background(), "", "hi"); err == nil {
		t.Fatal("want validation error on empty channelID")
	}
	if att.called != 0 {
		t.Fatalf("attestor called %d times on validation failure", att.called)
	}
}

// TestPostMessage_RateLimit_RejectsBurst: 10 in 30s pass, 11th rejected; 11th MUST NOT attest.
func TestPostMessage_RateLimit_RejectsBurst(t *testing.T) {
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
	if err := a.PostMessage(context.Background(), "chan-A", "msg"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("11th err = %v, want ErrRateLimited", err)
	}
	if att.called != attBefore {
		t.Fatalf("attestor invoked on rate-limited send (before=%d after=%d)", attBefore, att.called)
	}
}

// TestListChannels_HappyPath: guild channels returned after attestation.
func TestListChannels_HappyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/guilds/guild-1/channels") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"id":"c1","name":"general","type":0},
			{"id":"c2","name":"voice-room","type":2},
			{"id":"c3","name":"thread-x","type":11}
		]`))
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, &fakeTokenSource{}, srv, nil, nil)
	a.guildAllowlist = []string{"guild-1"}

	chans, err := a.ListChannels(context.Background(), "guild-1")
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(chans) != 3 {
		t.Fatalf("len = %d, want 3", len(chans))
	}
	if chans[0].ID != "c1" || chans[0].Name != "general" || chans[0].Type != "text" {
		t.Fatalf("c1 = %+v", chans[0])
	}
	if att.lastScope != ScopeListChannels {
		t.Fatalf("scope = %q, want %q", att.lastScope, ScopeListChannels)
	}
}

// TestListChannels_GuildAllowlistRejects: non-allowlisted guild rejected pre-attestation.
func TestListChannels_GuildAllowlistRejects(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("HTTP MUST NOT fire on allowlist rejection")
	}))
	defer srv.Close()
	att := &fakeAttestor{}
	a := newTestAdapter(t, att, &fakeTokenSource{}, srv, nil, nil)
	a.guildAllowlist = []string{"guild-allowed"}

	_, err := a.ListChannels(context.Background(), "guild-evil")
	if !errors.Is(err, ErrGuildNotAllowed) {
		t.Fatalf("err = %v, want ErrGuildNotAllowed", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor invoked on disallowed guild")
	}
}

// TestPostMessage_AuditRowHashesChannel: audit row stores sha256(channel)[:8], never plaintext.
func TestPostMessage_AuditRowHashesChannel(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer srv.Close()
	audit := &fakeAudit{}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{}, srv, nil, audit)
	const ch = "channel-secret-id"
	if err := a.PostMessage(context.Background(), ch, "body"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("audit rows = %d", len(audit.rows))
	}
	row := audit.rows[0]
	sum := sha256.Sum256([]byte(ch))
	want := hex.EncodeToString(sum[:])[:8]
	if row.ChannelHash != want {
		t.Fatalf("ChannelHash = %q, want %q", row.ChannelHash, want)
	}
	blob, _ := json.Marshal(row)
	if strings.Contains(string(blob), ch) {
		t.Fatalf("plaintext channel id leaked in audit row: %s", blob)
	}
}

// TestNewRejectsMissingDeps: nil collaborators fail construction.
func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{TokenSource: &fakeTokenSource{}}); err == nil {
		t.Fatal("nil Attestor MUST fail construction")
	}
	if _, err := New(Config{Attestor: &fakeAttestor{}}); err == nil {
		t.Fatal("nil TokenSource MUST fail construction")
	}
}
