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
	"strings"
	"testing"
)

type fakeAttestor struct {
	called int
	err    error
}

func (f *fakeAttestor) Attest(_ context.Context, scope string) error {
	f.called++
	if !strings.HasPrefix(scope, "whatsapp:") {
		return errors.New("unexpected scope: " + scope)
	}
	return f.err
}

type fakeTokenSource struct {
	tok, verify, secret, phoneID string
}

func (f *fakeTokenSource) Token(context.Context) (string, error)         { return f.tok, nil }
func (f *fakeTokenSource) VerifyToken(context.Context) (string, error)   { return f.verify, nil }
func (f *fakeTokenSource) AppSecret(context.Context) (string, error)     { return f.secret, nil }
func (f *fakeTokenSource) PhoneNumberID(context.Context) (string, error) { return f.phoneID, nil }

type fakeHTTP struct {
	calls    int
	lastReq  *http.Request
	lastBody []byte
	status   int
	respBody string
}

func (f *fakeHTTP) Do(req *http.Request) (*http.Response, error) {
	f.calls++
	f.lastReq = req
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		f.lastBody = b
	}
	status := f.status
	if status == 0 {
		status = 200
	}
	body := f.respBody
	if body == "" {
		body = `{"messages":[{"id":"wamid.abc"}]}`
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader([]byte(body))), Header: http.Header{}}, nil
}

type recordingSink struct{ rows []AuditRow }

func (r *recordingSink) Record(row AuditRow) { r.rows = append(r.rows, row) }

func newTestAdapter(t *testing.T, att *fakeAttestor, ts *fakeTokenSource, h *fakeHTTP, sink *recordingSink, allow []string) *Adapter {
	t.Helper()
	a, err := New(Config{
		Attestor:           att,
		TokenSource:        ts,
		HTTP:               h,
		Audit:              sink,
		RecipientAllowlist: allow,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestSendText_HappyPath(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	ts := &fakeTokenSource{tok: "bearer", phoneID: "PNID"}
	h := &fakeHTTP{}
	sink := &recordingSink{}
	a := newTestAdapter(t, att, ts, h, sink, []string{"+14155551234"})

	if err := a.SendText(context.Background(), "+14155551234", "hi"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if att.called != 1 {
		t.Fatalf("attestor called=%d want 1", att.called)
	}
	if h.calls != 1 {
		t.Fatalf("http calls=%d want 1", h.calls)
	}
	if !strings.Contains(h.lastReq.URL.String(), "PNID/messages") {
		t.Fatalf("URL=%q missing phone-number-id path segment", h.lastReq.URL.String())
	}
	if h.lastReq.Header.Get("Authorization") != "Bearer bearer" {
		t.Fatalf("auth header=%q", h.lastReq.Header.Get("Authorization"))
	}
	body := string(h.lastBody)
	if !strings.Contains(body, `"to":"+14155551234"`) || !strings.Contains(body, `"type":"text"`) || !strings.Contains(body, `"body":"hi"`) {
		t.Fatalf("payload missing required fields: %s", body)
	}
}

func TestSendText_AttestationDenied_NoCall(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{err: errors.New("denied")}
	ts := &fakeTokenSource{tok: "bearer", phoneID: "PNID"}
	h := &fakeHTTP{}
	a := newTestAdapter(t, att, ts, h, &recordingSink{}, []string{"+14155551234"})

	err := a.SendText(context.Background(), "+14155551234", "hi")
	if !errors.Is(err, ErrAttestationDenied) {
		t.Fatalf("err=%v want ErrAttestationDenied", err)
	}
	if h.calls != 0 {
		t.Fatalf("http called %d times on denied attestation", h.calls)
	}
}

func TestSendText_AllowlistRejects(t *testing.T) {
	t.Parallel()
	att := &fakeAttestor{}
	ts := &fakeTokenSource{tok: "bearer", phoneID: "PNID"}
	h := &fakeHTTP{}
	a := newTestAdapter(t, att, ts, h, &recordingSink{}, []string{"+14155550000"})

	err := a.SendText(context.Background(), "+14155551234", "hi")
	if !errors.Is(err, ErrRecipientNotAllowed) {
		t.Fatalf("err=%v want ErrRecipientNotAllowed", err)
	}
	if att.called != 0 {
		t.Fatalf("attestor consulted before allowlist check (called=%d)", att.called)
	}
	if h.calls != 0 {
		t.Fatalf("http called on off-list recipient")
	}
}

func TestWebhookVerify_HappyPath(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{verify: "secret-verify"}, &fakeHTTP{}, &recordingSink{}, []string{"+1"})

	got, err := a.WebhookVerify(context.Background(), "challenge-xyz", "secret-verify")
	if err != nil {
		t.Fatalf("WebhookVerify: %v", err)
	}
	if got != "challenge-xyz" {
		t.Fatalf("challenge=%q want challenge-xyz", got)
	}

	if _, err := a.WebhookVerify(context.Background(), "challenge-xyz", "wrong"); err == nil {
		t.Fatal("WebhookVerify accepted mismatched token")
	}
}

func TestWebhookHandle_HMACInvalid_Rejects(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{secret: "app-secret"}, &fakeHTTP{}, sink, []string{"+1"})

	payload := []byte(`{"entry":[]}`)
	_, err := a.WebhookHandle(payload, "sha256=deadbeef")
	if !errors.Is(err, ErrWebhookHMACInvalid) {
		t.Fatalf("err=%v want ErrWebhookHMACInvalid", err)
	}
	if len(sink.rows) == 0 || sink.rows[0].Kind != "whatsapp_webhook_hmac_invalid" {
		t.Fatalf("expected hmac_invalid audit row, got %+v", sink.rows)
	}
}

func TestWebhookHandle_HMACValid_ParsesMessages(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{secret: "app-secret"}, &fakeHTTP{}, &recordingSink{}, []string{"+1"})

	payload := []byte(`{"entry":[{"changes":[{"value":{"messages":[{"from":"14155551234","timestamp":"1700000000","type":"text","text":{"body":"hello"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte("app-secret"))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	msgs, err := a.WebhookHandle(payload, sig)
	if err != nil {
		t.Fatalf("WebhookHandle: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Body != "hello" || msgs[0].From != "14155551234" {
		t.Fatalf("messages=%+v", msgs)
	}
}

func TestSendText_AuditRowHashesRecipient(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	a := newTestAdapter(t, &fakeAttestor{}, &fakeTokenSource{tok: "bearer", phoneID: "PNID"}, &fakeHTTP{}, sink, []string{"+14155551234"})

	if err := a.SendText(context.Background(), "+14155551234", "hi"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if len(sink.rows) != 1 {
		t.Fatalf("rows=%d want 1", len(sink.rows))
	}
	r := sink.rows[0]
	if r.Kind != "whatsapp_send_text" || !r.Success {
		t.Fatalf("row=%+v", r)
	}
	if strings.Contains(r.RecipientHash, "415") || r.RecipientHash == "+14155551234" || len(r.RecipientHash) != 8 {
		t.Fatalf("recipient leaked or wrong-shape hash: %q", r.RecipientHash)
	}
	if r.BodyLen != 2 {
		t.Fatalf("body_len=%d want 2", r.BodyLen)
	}
}

func TestNewRejectsMissingDeps(t *testing.T) {
	t.Parallel()
	full := Config{
		Attestor:    &fakeAttestor{},
		TokenSource: &fakeTokenSource{},
		HTTP:        &fakeHTTP{},
	}
	cases := []struct {
		name string
		mut  func(c *Config)
	}{
		{"no attestor", func(c *Config) { c.Attestor = nil }},
		{"no token source", func(c *Config) { c.TokenSource = nil }},
		{"no http", func(c *Config) { c.HTTP = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := full
			tc.mut(&c)
			if _, err := New(c); err == nil {
				t.Fatal("expected error for missing dep")
			}
		})
	}
}
