---
slug: whatsapp-adapter
status: draft
phase: self-host
owner: leah
---

# WhatsApp adapter (MVP)

## 1. Goal

Operator-attested bidirectional WhatsApp messaging via the **WhatsApp
Business Cloud API** (official, Meta-provided REST surface). This is the
SMS-like reach channel — operator texts Leah from any phone with
WhatsApp installed; Leah pushes daily briefs and accept/reject
recommendation prompts into the operator's WhatsApp conversation.

**Explicit non-goal**: NOT WhatsApp Web automation. Reverse-engineered
Web-protocol libraries (e.g. `tulir/whatsmeow`) carry account-ban risk
under Meta's ToS and break on protocol updates. This adapter uses only
the documented Cloud API.

Shape mirrors `internal/adapters/gmail` (per-RPC `Scope*` + `Attestor`
+ load-bearing `gateAndExec` ordering) so the dispatcher reuses one
consent path across all comms adapters.

## 2. Dependency (adopt-over-build)

- API: WhatsApp Business Cloud API (Meta Graph API).
- Pinned Graph API version: `v22.0` (latest stable as of 2026-06-10).
- Library: NO official Go SDK exists. Adapter uses direct REST via
  `net/http` + `encoding/json` — no new third-party module needed.
- Rationale: a thin REST client (~150 LOC) beats wrapping someone
  else's unmaintained wrapper. The Graph API surface this adapter
  needs is small (send text, send media, fetch media, webhook
  receive) and stable.

**Alternatives considered**:

- `tulir/whatsmeow` (MIT) — unofficial Web-protocol Go library. Free,
  no per-conversation fees, but **carries account-ban risk** (Meta
  routinely revokes accounts that automate via Web). Documented for
  the record; **recommend against for production**.
- `twilio-go` WhatsApp channel — Twilio resells the official Cloud
  API with their own pricing on top. Adds a vendor; we can hit Meta
  directly.

No `go.mod` edit in this wave — `net/http` and `encoding/json` are
stdlib. The webhook receiver (Section 7) reuses the daemon's existing
HTTPS muxer.

## 3. Setup prereqs (operator steps, executed by `leah connect whatsapp` in the wiring wave)

1. Operator creates a Meta Business account at
   https://business.facebook.com and adds a WhatsApp Business app.
2. Gets a **phone-number ID** + **permanent access token** from the
   Meta App Dashboard -> WhatsApp -> API Setup.
3. Sets a **webhook URL** in the App Dashboard pointing to the
   operator's Leah daemon. Local dev: cloudflared / ngrok tunnel.
   Production: regatta-Docker (per PR #74) hosts the webhook receiver
   with a stable public URL.
4. Picks a **verify token** (any opaque string); Meta sends it as
   `hub.verify_token` on subscribe — the daemon must echo
   `hub.challenge` to confirm ownership of the URL.
5. `leah connect whatsapp` reads the access token + phone-number ID +
   verify token via stdin (never argv) and writes
   `$HOME/.leah-state/secrets/whatsapp-token.json` mode `0600`.
6. Operator subscribes the app to the `messages` webhook field for
   inbound message + voice delivery.
7. Operator pre-approves any **template messages** via Meta's
   template console (required for outbound messages outside the 24h
   conversation window).

The adapter itself does NOT perform browser-based onboarding — those
clicks happen in the Meta dashboard. The token file is the credential
surface the adapter touches.

## 4. Surface

```go
package whatsapp

const (
    ScopeSendText     = "whatsapp:send_text"
    ScopeSendVoice    = "whatsapp:send_voice"
    ScopeSubscribe    = "whatsapp:subscribe"
    ScopeSendTemplate = "whatsapp:send_template"
)

var (
    ErrAttestationDenied   = errors.New("whatsapp: attestation denied")
    ErrRecipientNotAllowed = errors.New("whatsapp: recipient not on allowlist")
    ErrTemplateNotApproved = errors.New("whatsapp: template not in approved set")
    ErrRateLimited         = errors.New("whatsapp: rate limit exceeded")
    ErrCostCapExceeded     = errors.New("whatsapp: monthly conversation cap exceeded")
    ErrWebhookHMACInvalid  = errors.New("whatsapp: webhook signature invalid")
    ErrSendFailed          = errors.New("whatsapp: send failed")
)

type MessageType string

const (
    TypeText  MessageType = "text"
    TypeAudio MessageType = "audio"
    TypeImage MessageType = "image"
)

type Message struct {
    From      string      // E.164 phone, e.g. "+14155551234"
    Body      string
    Voice     []byte      // populated when Type == TypeAudio (.ogg/opus)
    Type      MessageType
    Timestamp time.Time
}

type Attestor interface {
    Attest(ctx context.Context, scope string) error
}

type TokenSource interface {
    Token(ctx context.Context) (string, error)
    VerifyToken(ctx context.Context) (string, error)
    AppSecret(ctx context.Context) (string, error)  // for HMAC validation
    PhoneNumberID(ctx context.Context) (string, error)
}

// HTTPClient is the net/http seam.
type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
}

// OpusEncoder converts TTS PCM/wav to .ogg/opus for outbound voice.
// Production wires an ffmpeg subprocess; tests inject a fake.
type OpusEncoder interface {
    Encode(ctx context.Context, audio []byte) ([]byte, error)
}

type Config struct {
    Attestor          Attestor
    TokenSource       TokenSource
    HTTP              HTTPClient
    OpusEncoder       OpusEncoder
    RecipientAllowlist []string         // E.164 numbers; empty rejects all outbound
    ApprovedTemplates  []string         // template names operator pre-approved with Meta
    MonthlyCap         int              // conversation count cap; 0 = no cap
    Now               func() time.Time
}

type Adapter struct { /* unexported */ }

func New(cfg Config) (*Adapter, error)
func (*Adapter) SendText(ctx context.Context, to, body string) error
func (*Adapter) SendVoice(ctx context.Context, to string, audio []byte) error
func (*Adapter) SendTemplate(ctx context.Context, to, templateName string, params []string) error
func (*Adapter) Subscribe(ctx context.Context, handler func(Message)) error
// HandleWebhook is the http.Handler shape the daemon's muxer mounts at
// the operator-configured webhook URL. Performs HMAC verification +
// challenge response + payload parse + dispatch to the Subscribe handler.
func (*Adapter) HandleWebhook(w http.ResponseWriter, r *http.Request)
```

`New` fail-closes on nil `Attestor`, nil `TokenSource`, nil `HTTP`,
nil `OpusEncoder`. `Now` defaults to `time.Now` when nil.
`RecipientAllowlist` empty is **not** a permissive default — it means
no outbound message will be sent.

## 5. Operator-attestation flow

Per-RPC. Ordering carries from gmail / imessage / discord:

1. Argument validation (`to` matches E.164 regex, `body != ""`, audio
   ≤ 16MB per Meta's media-upload cap). Bad input MUST NOT consume
   attestation.
2. `RecipientAllowlist` check. Off-list send returns
   `ErrRecipientNotAllowed` **before** attestation.
3. For `SendTemplate`: the named template MUST be in
   `ApprovedTemplates`; otherwise `ErrTemplateNotApproved` before
   attestation.
4. Rate-limit + cost-cap check (Section 8). Cap-exceeded outbound
   returns `ErrCostCapExceeded` before attestation.
5. `Attestor.Attest(ctx, Scope*)` runs. Non-nil aborts with
   `ErrAttestationDenied` (wrapped). Token NOT loaded yet.
6. On consent, `TokenSource.Token(ctx)` materializes the bearer; the
   token never appears in errors, logs, or panic traces.
7. Each operation writes an audit row (Section 8 schema) with
   `success bool`.

Inbound webhook events attest **once** at `Subscribe` time, not per
message — the operator consents to the inbound stream at subscription
grain (same rationale as discord).

## 6. Voice handling

- **Inbound voice**: WhatsApp voice messages arrive as `audio` type
  with a `media_id`. The adapter fetches the media via the Graph API
  (`GET /{media_id}` then `GET {media_url}`), decodes to bytes,
  populates `Message.Voice`. WhatsApp serves `.ogg`/opus. Adapter
  hands bytes to the STT pipeline scaffolded in `voice-comm` (W11+);
  decoupled — adapter does not call STT directly.
- **Outbound voice**: TTS via `ChainTTS` (PR #49) produces PCM/wav.
  `OpusEncoder.Encode` (ffmpeg subprocess in production) converts to
  `.ogg`/opus. Adapter uploads via the Graph media endpoint, captures
  the returned `media_id`, then sends a `messages` payload of type
  `audio` referencing the `media_id`.
- **Format conversion**: ffmpeg is the only viable Go ecosystem
  option for opus encode without CGo bindings; the subprocess
  pattern aligns with imessage's `OSExec` seam. ffmpeg's presence is
  checked by `leah connect whatsapp` (probe `ffmpeg -version`).

## 7. Webhook architecture

- **Local dev**: cloudflared tunnel
  (`cloudflared tunnel --url http://localhost:8443`) gives a stable
  HTTPS URL bound to a local port.
- **Production**: regatta-Docker (per PR #74) hosts the daemon with
  a public ingress. Meta posts to that URL.
- **Verification handshake**: on `GET /webhook?hub.mode=subscribe&hub.verify_token=...&hub.challenge=...`,
  the adapter compares against `TokenSource.VerifyToken` and echoes
  `hub.challenge` on match. Mismatch returns 403.
- **Signature verification**: every `POST /webhook` carries
  `X-Hub-Signature-256: sha256=<hex>`. The adapter computes
  `hmac-sha256(body, AppSecret)` and rejects with 401 +
  `ErrWebhookHMACInvalid` on mismatch. HMAC verify runs **before**
  JSON parse to keep parser CPU off untrusted inputs.
- **Idempotency**: Meta retries on non-2xx. Adapter dedups by
  message `id` in a bounded in-memory ring (size 1024); duplicate
  IDs are 2xx-acked without redispatching to the handler.

## 8. Threat model

| Surface | Mitigation |
| --- | --- |
| Access token leak in logs / panic traces | `gateAndExec` loads token only after attestation; never logged; token file `0600`. |
| Webhook spoofing | `X-Hub-Signature-256` HMAC verified against `AppSecret` BEFORE JSON parse + handler dispatch. Invalid signature -> 401, audit row `whatsapp_webhook_hmac_invalid`. |
| Cross-recipient leak | `RecipientAllowlist` (E.164 strings) MUST be non-empty for any outbound. Off-list send returns `ErrRecipientNotAllowed` before attestation. |
| Voice-message bytes in audit | Audit row stores `voice_sha256[:8]` + `voice_duration_ms`, never the bytes. |
| Recipient leak in audit | `recipient_hash = sha256(to)[:8]`; plaintext numbers NEVER in audit JSON. |
| Template abuse | Only operator-pre-approved template names accepted. Adding a template requires (a) Meta approval, (b) appending to `ApprovedTemplates` config. |
| Cost blowup (Meta charges per conversation) | `MonthlyCap` config; in-memory counter persisted to `~/.leah-state/whatsapp-conv-counter.json`; cap-exceeded returns `ErrCostCapExceeded` before attestation. Operator alerted via daemon dashboard at 80% of cap. |
| Meta account ban via Web automation | Strictly use official Cloud API. WhatsApp Web automation (e.g. `whatsmeow`) is an **explicit non-goal**. |
| Token rotation | Cloud API tokens expire (24h temporary, 60-day system-user). Daemon-driven refresh path deferred to wiring wave; mirrors gmail OAuth-refresh pattern. |
| Webhook DoS | Daemon's HTTPS muxer enforces request size cap (256KB) and `X-Hub-Signature-256` requirement before any handler work. |
| Attestor bypass | `New` refuses to construct without non-nil `Attestor`, `TokenSource`, `HTTP`, `OpusEncoder`. |
| HTTPS-only | Webhook URL MUST be `https://`; Meta refuses HTTP. Daemon serves TLS via the cert path the operator configured for the dashboard. |

Audit row schema (consumed by `internal/audit`):

```
{
  "kind": "whatsapp_send_text" | "whatsapp_send_voice" | "whatsapp_send_template" | "whatsapp_inbound" | "whatsapp_webhook_hmac_invalid",
  "ts": "2026-06-10T...",
  "success": true,
  "recipient_hash": "a1b2c3d4",     // sha256(to)[:8]
  "body_len": 42,
  "voice_sha256": "...",            // first 8 hex chars, "" if no voice
  "voice_duration_ms": 0,
  "template_name": "",              // populated only for send_template
  "conversation_cap_remaining": 873,
  "reason": ""
}
```

## 9. Test plan (all hermetic; no real Meta network, no real ffmpeg)

- `TestSendText_HappyPath` — fake HTTP captures the POST; assert body
  JSON shape matches Cloud API `messages` schema (to, type, text).
- `TestSendText_AttestationDenied_NoCall` — fake Attestor rejects;
  HTTP invocation counter MUST stay zero; returns wrapped
  `ErrAttestationDenied`.
- `TestSendText_AllowlistRejects` — `to` not in `RecipientAllowlist`;
  returns `ErrRecipientNotAllowed`; attestor + HTTP counters zero.
- `TestSendText_RateLimit_RejectsBurst` — burst exceeds limit;
  returns `ErrRateLimited`; attestor + HTTP counters zero on the
  rejected send.
- `TestSendText_CostCapExceeded` — `MonthlyCap` reached; returns
  `ErrCostCapExceeded`; no attestation prompt; no HTTP call.
- `TestSendTemplate_NotInApprovedSet` — unknown template name;
  returns `ErrTemplateNotApproved` before attestation.
- `TestSendVoice_OpusConversion` — fake `OpusEncoder` records the
  PCM input bytes; outbound HTTP body references the resulting
  `media_id` from the (faked) upload response.
- `TestSendVoice_OversizedAudio_Rejected` — 17MB buffer returns
  validation error before attestation.
- `TestWebhook_VerifyHandshake_HappyPath` — `GET` with matching
  `hub.verify_token` echoes `hub.challenge` and returns 200.
- `TestWebhook_VerifyHandshake_TokenMismatch` — mismatched token
  returns 403; no challenge echoed.
- `TestWebhook_HMACInvalid_Rejects` — `POST` with bad
  `X-Hub-Signature-256` returns 401; handler NOT dispatched; audit
  row `whatsapp_webhook_hmac_invalid` written.
- `TestWebhook_HMACValid_DispatchesHandler` — correct HMAC; payload
  parsed; handler receives `Message` with expected fields.
- `TestWebhook_DuplicateMessageID_Dedup` — same `id` twice in 1s;
  handler invoked once; both POSTs return 200.
- `TestWebhook_InboundVoice_FetchesMedia` — synthetic `audio` payload
  triggers fake HTTP fetch of the media URL; `Message.Voice` is
  populated with the fetched bytes.
- `TestAuditRowHashed` — outbound + inbound audit rows contain only
  hashed recipient identifiers; plaintext phone numbers NEVER appear
  in audit-row JSON.
- `TestNewRejectsMissingDeps` — nil `Attestor` / nil `TokenSource` /
  nil `HTTP` / nil `OpusEncoder` each fail construction with a
  descriptive error.

## 10. Trade-offs (per README.md UX > performance > long-term)

- **UX**: WhatsApp reaches the operator anywhere they get push
  notifications. Worth the Meta-dashboard setup tax. ~1000 free
  conversations/mo (Meta's free tier) covers a single operator
  generously.
- **Performance**: webhook-driven inbound has lower CPU + battery
  cost than polling. Outbound REST is unremarkable.
- **Long-term**: rolling our own HTTP client (vs. depending on a
  third-party SDK) means we own the Graph-API-version bump cost.
  Acceptable — the surface area we touch is small and the Cloud API
  versioning policy gives us 2-year deprecation windows.

## 11. Out of scope (MVP)

- WhatsApp Group messaging (1:1 conversations only MVP).
- Reactions, link previews, location messages, contact cards, polls.
- WhatsApp Pay / payments / commerce.
- WhatsApp Business catalog + product messages.
- Multi-number per operator (one phone-number ID per install MVP).
- Web-protocol automation (explicit non-goal; ToS risk).
- Token refresh / rotation automation (deferred to wiring wave;
  mirrors gmail's pattern).
- Operator-configurable rate limit (issue filed alongside this spec).
