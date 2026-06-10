---
slug: imessage-adapter
status: draft
phase: self-host
owner: leah
---

# iMessage adapter (MVP)

## 1. Goal

Give Leah an operator-attested outbound iMessage seam so the dispatcher can
ship operator-authored messages from the host Mac. MVP is **outbound text
only** — group chats, attachments, reactions, edit/recall, read-receipt
observation, and SMS fallback all defer until a concrete caller files the
gap. Shape mirrors `internal/adapters/gmail` (Attestor + per-RPC scope +
`gateAndExec` ordering) so wiring code reuses one consent path.

## 2. Dependency (adopt-over-build)

macOS-local automation via AppleScript:

```
osascript -e 'tell application "Messages" ...'
```

- No new Go module. Subprocess invocation goes through `os/exec`.
- No `go.mod` edit in this wave — the adapter only introduces an `OSExec`
  interface and a small AppleScript template.
- The break with gmail/gcal: there is **no OAuth and no token file**. macOS
  Messages.app holds the operator's signed-in iMessage account; the Automation
  permission grant (System Settings -> Privacy & Security -> Automation ->
  `leah` -> `Messages`) is the only credential surface and is operator-granted
  out-of-band.

## 3. Surface

```go
package imessage

const ScopeSend = "imessage:send"

var (
    ErrAttestationDenied = errors.New("imessage: attestation denied")
    ErrPermissionDenied  = errors.New("imessage: automation permission denied")
    ErrSendFailed        = errors.New("imessage: send failed")
    ErrRateLimited       = errors.New("imessage: rate limit exceeded")
    ErrInvalidRecipient  = errors.New("imessage: invalid recipient")
)

type Message struct {
    To   string // phone number or iMessage email; validated before exec
    Body string
}

type Attestor interface {
    Attest(ctx context.Context, scope string) error
}

// OSExec is the subprocess seam. Production wires os/exec.CommandContext;
// tests inject a fake that records invocations and returns canned errors.
type OSExec interface {
    Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

type Config struct {
    Attestor Attestor
    OSExec   OSExec
    // Now is injectable so the rate limiter is testable without sleeping.
    Now func() time.Time
}

type Adapter struct { /* unexported */ }

func New(cfg Config) (*Adapter, error)
func (*Adapter) Send(ctx context.Context, msg Message) error
```

`New` fail-closes on `nil` Attestor or `nil` OSExec — there is no silent
default. `Now` defaults to `time.Now` when nil.

## 4. Operator-attestation flow

Mirror of gmail's `gateAndToken`, renamed `gateAndExec`. Ordering is
load-bearing:

1. `Send` validates `msg.To != ""` and that the recipient matches a
   conservative regex (`^[+\d\w@.-]+$`). A typo MUST NOT consume an
   attestation prompt.
2. Rate-limit check (Section 6) — a burst-rejected send MUST NOT consume an
   attestation prompt either.
3. `Attestor.Attest(ctx, ScopeSend)` runs. A non-nil return aborts with
   `ErrAttestationDenied` (wrapped). **No subprocess is spawned.**
4. Only on consent does the adapter build the AppleScript and invoke
   `OSExec.Run`.
5. Each send writes an audit row (Section 6 schema) with `success bool` so
   denied/failed/succeeded sends are all observable post-hoc.

Mirroring gmail's `Send-to-attacker` mitigation: validation runs before
attestation so an invalid recipient does not get a consent prompt.

## 5. AppleScript prereqs (out-of-scope-for-this-MVP, documented for W22)

The wiring wave's `leah connect imessage` subcommand (Wave 22 in the
combined plan) will need to:

1. Verify Messages.app is launched and signed-in:
   `osascript -e 'tell application "Messages" to get name of accounts'`
2. Document the Automation grant path: System Settings -> Privacy & Security
   -> Automation -> `leah` -> `Messages`.
3. Write an audit row `Kind: "connect_imessage"` on success.

The adapter itself does NOT probe Messages.app at construction. If the grant
is absent, the first `Send` surfaces `ErrPermissionDenied` (mapped from the
osascript exit code / stderr substring `"not authorized to send Apple
events"`). Graceful degrade — no panic, no retry loop.

## 6. Threat model

| Surface | Mitigation |
| --- | --- |
| Spam vector if Leah is compromised | Hardcoded MVP rate limit: max **10 sends per rolling 60s**. 11th send returns `ErrRateLimited`. Operator-configurable limit deferred to a follow-up issue. |
| Recipient leak in audit log | Audit row stores `recipient_hash = sha256(to)[:8]`, never plaintext. Hash is stable per-operator so dedupe / spam-pattern detection still works. |
| Body leak in audit log | Audit row stores `body_len int` (rune count), NOT body bytes. Operator-toggle for full-body capture deferred to a follow-up. |
| Subprocess / shell injection via recipient or body | NEVER `fmt.Sprintf` into the shell. AppleScript is delivered via `osascript -` with the script on stdin and the recipient / body interpolated through AppleScript `text` literals using a single escape pass (double-quote and backslash). `OSExec.Run` takes `name`, `args []string`, and `stdin []byte` — no shell parsing. |
| AppleScript injection via body content | The body is wrapped in an AppleScript `text` literal with `"` -> `\"` and `\` -> `\\` substitution. Tests cover quote and backslash payloads. |
| macOS Sequoia / Sonoma sandboxing changes | If Automation TCC blocks the call, osascript returns a recognizable error; adapter maps to `ErrPermissionDenied` and returns. No private TCC API touched. |
| Unsigned binary blocked by Apple | Explicit non-goal. Documented: a packaged Leah requires a Developer ID signature for Messages Automation; unsigned runs surface `ErrPermissionDenied` and that is acceptable for MVP. |
| Private API (IMCore) temptation | Explicit non-goal. Adapter MUST only call public `osascript`. Adding any IMCore / private-framework surface requires a separate design review. |
| Attestor bypass | `New` refuses to construct an `Adapter` without a non-nil `Attestor`. No nil-Attestor fallback. |
| Recipient-allowlist bypass | Out of scope for MVP; tracked alongside operator-configurable rate limit. |

Audit row schema (consumed by `internal/audit`):

```
{
  "kind": "imessage_send",
  "ts": "2026-06-10T...",
  "success": true,
  "recipient_hash": "a1b2c3d4",  // sha256(to)[:8]
  "body_len": 42,
  "reason": ""                   // populated on failure
}
```

## 7. Test plan (all hermetic; no real osascript, no real Messages.app)

- `TestSend_HappyPath` — fake `OSExec` captures `name`, `args`, `stdin`;
  assert `name == "osascript"`, args contain `-`, stdin contains the
  recipient + body and is a syntactically-plausible AppleScript snippet.
- `TestSend_AttestationDenied_NoExec` — fake `Attestor` returns an error;
  fake `OSExec` increments an invocation counter that MUST stay zero.
  Returns wrapped `ErrAttestationDenied`.
- `TestSend_PermissionDenied_PassThrough` — fake `OSExec` returns stderr
  `"not authorized to send Apple events"` + non-zero exit; adapter maps to
  `ErrPermissionDenied` (wrapped, preserving the underlying message).
- `TestSend_RecipientEmpty_NoExec` — `msg.To == ""`; returns
  `ErrInvalidRecipient` BEFORE attestation runs. Both attestor and OSExec
  invocation counters MUST stay zero.
- `TestSend_RecipientShellSpecials_Rejected` — `msg.To = "$(rm -rf ~)"`
  fails the regex; returns `ErrInvalidRecipient`; OSExec not invoked.
- `TestSend_BodyWithQuotesAndBackslashes_Escaped` — body
  `He said "hi" \ bye`; assert the script on stdin contains the escaped
  forms and not the raw characters in literal positions that would break
  the AppleScript parse.
- `TestSend_AuditRowHashed` — recipient field in the emitted audit row is
  the 8-char sha256 prefix; plaintext recipient does NOT appear anywhere
  in the audit row JSON; `body_len` matches `utf8.RuneCountInString(body)`.
- `TestSend_RateLimit_RejectsBurst` — with injected `Now`, 10 sends in 30s
  succeed; the 11th returns `ErrRateLimited` and does NOT invoke OSExec or
  Attestor. After advancing `Now` past the window, sends resume.
- `TestNewRejectsMissingDeps` — nil `Attestor` and nil `OSExec` each fail
  construction with a descriptive error.

## 8. Trade-offs (per CLAUDE.md UX > performance > long-term)

- **UX**: AppleScript subprocess adds ~80ms latency per send vs. a
  hypothetical native binding. Accepted — operator-facing flow is "type the
  message, confirm, see it sent"; 80ms is invisible.
- **Performance**: spawning `osascript` per-send is not amortized. A pooled
  long-lived `osascript -i` REPL would cut latency but multiplies the
  surface area (REPL lifecycle, error recovery, signal handling). Defer
  until a caller observes the cost.
- **Long-term**: a future native-binding adapter (signed app + IMCore) is
  intentionally NOT scaffolded here. Three similar lines beat a premature
  abstraction.

## 9. Out of scope (MVP)

- Group chats / multi-recipient sends.
- Attachments (images, files, links with previews).
- Read-receipt observation, typing indicators, delivery confirmation.
- Edit / recall / unsend (iMessage 16+ feature).
- Reactions (tapbacks).
- SMS fallback when iMessage is unavailable for the recipient.
- Inbound listening (handled by a separate `imessage_listen` adapter if
  ever needed).
- Operator-configurable rate limit (issue filed alongside this spec).
- Recipient allow-listing.
- Full-body audit capture (operator-toggle deferred).
