// Package whatsapp is the WhatsApp Business Cloud API adapter (W68 skeleton).
// Direct REST to graph.facebook.com — explicit non-goal is reverse-engineered
// Web-protocol automation (Meta ToS / account-ban risk; see spec §1).
package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const graphAPIVersion = "v22.0"

const (
	ScopeSendText       = "whatsapp:send_text"
	ScopeWebhookVerify  = "whatsapp:webhook_verify"
	ScopeWebhookHandle  = "whatsapp:webhook_handle"
)

var (
	ErrAttestationDenied   = errors.New("whatsapp: attestation denied")
	ErrRecipientNotAllowed = errors.New("whatsapp: recipient not on allowlist")
	ErrWebhookHMACInvalid  = errors.New("whatsapp: webhook signature invalid")
	ErrSendFailed          = errors.New("whatsapp: send failed")
)

type Message struct {
	From      string
	Body      string
	Type      string
	Timestamp time.Time
}

type Attestor interface {
	Attest(ctx context.Context, scope string) error
}

// TokenSource yields the four secrets the adapter needs. Splitting them keeps
// the access-token surface separate from webhook-verification material so a
// leaked bearer cannot also forge inbound webhooks.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
	VerifyToken(ctx context.Context) (string, error)
	AppSecret(ctx context.Context) (string, error)
	PhoneNumberID(ctx context.Context) (string, error)
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// AuditRow omits plaintext recipient and body by design — recipient is hashed,
// body recorded by length only.
type AuditRow struct {
	Kind          string
	Success       bool
	RecipientHash string
	BodyLen       int
	Reason        string
}

type AuditSink interface {
	Record(row AuditRow)
}

type Config struct {
	Attestor           Attestor
	TokenSource        TokenSource
	HTTP               HTTPClient
	Audit              AuditSink
	RecipientAllowlist []string
}

type Adapter struct {
	att     Attestor
	ts      TokenSource
	http    HTTPClient
	audit   AuditSink
	allow   map[string]struct{}
}

func New(cfg Config) (*Adapter, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("whatsapp: Config.Attestor required (operator-attestation gate)")
	}
	if cfg.TokenSource == nil {
		return nil, errors.New("whatsapp: Config.TokenSource required")
	}
	if cfg.HTTP == nil {
		return nil, errors.New("whatsapp: Config.HTTP required")
	}
	allow := make(map[string]struct{}, len(cfg.RecipientAllowlist))
	for _, r := range cfg.RecipientAllowlist {
		allow[r] = struct{}{}
	}
	return &Adapter{
		att:   cfg.Attestor,
		ts:    cfg.TokenSource,
		http:  cfg.HTTP,
		audit: cfg.Audit,
		allow: allow,
	}, nil
}

// SendText posts a text message via the Cloud API messages endpoint. Order is
// load-bearing (spec §5): validate → allowlist → attest → token → HTTP. A
// rejection at any earlier stage MUST NOT advance to the next.
func (a *Adapter) SendText(ctx context.Context, to, body string) error {
	if to == "" || body == "" {
		return fmt.Errorf("%w: missing recipient or body", ErrSendFailed)
	}
	if _, ok := a.allow[to]; !ok {
		return ErrRecipientNotAllowed
	}
	if err := a.att.Attest(ctx, ScopeSendText); err != nil {
		return fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.Token(ctx)
	if err != nil {
		return fmt.Errorf("whatsapp: token load: %w", err)
	}
	pnid, err := a.ts.PhoneNumberID(ctx)
	if err != nil {
		return fmt.Errorf("whatsapp: phone-number-id load: %w", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]string{"body": body},
	})
	url := "https://graph.facebook.com/" + graphAPIVersion + "/" + pnid + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("whatsapp: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		a.record(AuditRow{Kind: "whatsapp_send_text", Success: false, RecipientHash: hashRecipient(to), BodyLen: len(body), Reason: err.Error()})
		return fmt.Errorf("%w: %v", ErrSendFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		respBody, _ := io.ReadAll(resp.Body)
		a.record(AuditRow{Kind: "whatsapp_send_text", Success: false, RecipientHash: hashRecipient(to), BodyLen: len(body), Reason: "http " + strconv.Itoa(resp.StatusCode)})
		return fmt.Errorf("%w: status %d: %s", ErrSendFailed, resp.StatusCode, string(respBody))
	}
	a.record(AuditRow{Kind: "whatsapp_send_text", Success: true, RecipientHash: hashRecipient(to), BodyLen: len(body)})
	return nil
}

// WebhookVerify implements the Meta GET-subscribe handshake: echo challenge
// when the operator-configured verify token matches.
func (a *Adapter) WebhookVerify(ctx context.Context, challenge, presentedToken string) (string, error) {
	if err := a.att.Attest(ctx, ScopeWebhookVerify); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	want, err := a.ts.VerifyToken(ctx)
	if err != nil {
		return "", fmt.Errorf("whatsapp: verify token load: %w", err)
	}
	if !hmac.Equal([]byte(want), []byte(presentedToken)) {
		return "", errors.New("whatsapp: verify token mismatch")
	}
	return challenge, nil
}

// WebhookHandle validates the X-Hub-Signature-256 HMAC against AppSecret BEFORE
// JSON parse (spec §7) — invalid signatures must not consume parser CPU on
// untrusted bytes. Returns parsed inbound messages on success.
func (a *Adapter) WebhookHandle(payload []byte, signature string) ([]Message, error) {
	secret, err := a.ts.AppSecret(context.Background())
	if err != nil {
		return nil, fmt.Errorf("whatsapp: app secret load: %w", err)
	}
	if !verifyHMAC(payload, signature, secret) {
		a.record(AuditRow{Kind: "whatsapp_webhook_hmac_invalid", Success: false, Reason: "signature mismatch"})
		return nil, ErrWebhookHMACInvalid
	}
	return parseInbound(payload)
}

func verifyHMAC(payload []byte, signature, secret string) bool {
	const prefix = "sha256="
	if len(signature) <= len(prefix) || signature[:len(prefix)] != prefix {
		return false
	}
	got, err := hex.DecodeString(signature[len(prefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hmac.Equal(got, mac.Sum(nil))
}

type webhookEnvelope struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Messages []struct {
					From      string `json:"from"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func parseInbound(payload []byte) ([]Message, error) {
	var env webhookEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, fmt.Errorf("whatsapp: parse webhook: %w", err)
	}
	var out []Message
	for _, e := range env.Entry {
		for _, c := range e.Changes {
			for _, m := range c.Value.Messages {
				ts := time.Time{}
				if n, err := strconv.ParseInt(m.Timestamp, 10, 64); err == nil {
					ts = time.Unix(n, 0).UTC()
				}
				out = append(out, Message{
					From:      m.From,
					Body:      m.Text.Body,
					Type:      m.Type,
					Timestamp: ts,
				})
			}
		}
	}
	return out, nil
}

func hashRecipient(to string) string {
	sum := sha256.Sum256([]byte(to))
	return hex.EncodeToString(sum[:])[:8]
}

func (a *Adapter) record(row AuditRow) {
	if a.audit != nil {
		a.audit.Record(row)
	}
}
