---
slug: facetime-adapter
status: draft
phase: self-host
owner: leah
---

# FaceTime adapter (MVP)

## 1. Goal

Give Leah an operator-attested seam that **initiates** outbound FaceTime
calls from the host Mac. "Initiate" means "open the system call UI so the
operator can press the call button"; Apple blocks fully-unattended outbound
FaceTime by design, and Leah does not work around that. MVP is **video +
audio call initiation only** — recording, group calls, screen sharing,
hang-up control, and in-call mute defer until a concrete caller asks.

Shape mirrors the iMessage adapter (Attestor + per-RPC scope + `gateAndExec`
ordering); wiring code reuses one consent path across both macOS-native
adapters.

## 2. Dependency (adopt-over-build)

macOS URL-scheme launch via `open(1)`:

```
open facetime://<callee>           // video
open facetime-audio://<callee>     // audio
```

- No new Go module. Subprocess invocation goes through `os/exec`.
- No `go.mod` edit in this wave.
- No AppleScript needed — URL schemes are handled directly by the OS.
- The break with gmail/gcal: there is **no OAuth and no token file**. The
  FaceTime UI confirmation **is** the credential surface — Apple requires
  the operator to confirm in-app before the call places. We rely on that.

## 3. Surface

```go
package facetime

const (
    ScopeInitiateVideo = "facetime:video"
    ScopeInitiateAudio = "facetime:audio"
)

var (
    ErrAttestationDenied = errors.New("facetime: attestation denied")
    ErrInvalidCallee     = errors.New("facetime: invalid callee")
    ErrRateLimited       = errors.New("facetime: rate limit exceeded")
    ErrLaunchFailed      = errors.New("facetime: open(1) failed")
)

type Attestor interface {
    Attest(ctx context.Context, scope string) error
}

type OSExec interface {
    Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

type Config struct {
    Attestor Attestor
    OSExec   OSExec
    Now      func() time.Time
}

type Adapter struct { /* unexported */ }

func New(cfg Config) (*Adapter, error)
func (*Adapter) InitiateVideo(ctx context.Context, callee string) error
func (*Adapter) InitiateAudio(ctx context.Context, callee string) error
```

`New` fail-closes on `nil` Attestor or `nil` OSExec. `Now` defaults to
`time.Now` when nil.

## 4. Operator-attestation flow

Same `gateAndExec` ordering as iMessage:

1. Validate `callee` against `^[+\d\w@.-]+$`. Reject otherwise with
   `ErrInvalidCallee` — no attestation prompt for shell-special input.
2. Rate-limit check (Section 6). 6th call within 60s -> `ErrRateLimited`,
   no attestation prompt.
3. `Attestor.Attest(ctx, ScopeInitiateVideo | ScopeInitiateAudio)` runs.
   Non-nil return -> `ErrAttestationDenied`. **No URL is launched.**
4. Only on consent does the adapter invoke
   `OSExec.Run(ctx, "open", []string{url}, nil)` where
   `url = "facetime://" + callee` or `"facetime-audio://" + callee`.
5. Audit row written per call (Section 6 schema) with `success bool`.

## 5. Calling contract (load-bearing UX clarification)

`InitiateVideo` / `InitiateAudio` return `nil` once `open(1)` exits 0.
That means **the FaceTime UI has appeared, not that the call has been
placed**. The operator still has to press the call button in the FaceTime
window. This is intentional and aligned with Apple's platform contract:

- Apple does NOT expose a public API to place a FaceTime call without
  operator UI confirmation.
- Private API workarounds (IOKit / IMCore poking) are an explicit non-goal.
- The UI confirmation is the human-in-the-loop that prevents robo-dialing
  even if Leah is compromised.

Callers MUST document this in user-facing copy: `leah call alice@example.com`
opens the call UI; the operator still presses dial.

## 6. Threat model

| Surface | Mitigation |
| --- | --- |
| Robo-dialing if Leah is compromised | Hardcoded MVP rate limit: max **5 initiations per rolling 60s**. 6th call returns `ErrRateLimited`. Apple's UI confirmation is the second line of defense. Operator-configurable limit deferred. |
| Callee leak in audit log | Audit row stores `callee_hash = sha256(callee)[:8]`, never plaintext. Mirrors iMessage. |
| URL-scheme injection via callee | Regex `^[+\d\w@.-]+$` rejects shell-special and URL-special characters (`;`, `&`, `|`, `'`, `"`, space, `?`, `#`, `/`). `OSExec.Run` takes args as `[]string` — no shell parsing. Tests cover `;`, `&`, ``backtick``, `$()`, and unicode-spoof payloads. |
| Spam-via-URL-scheme | Apple's pre-call UI confirmation IS the mitigation; explicitly documented. No additional Leah-side block beyond the rate limit. |
| Phone-tree / DTMF automation | Out of scope. No keypad-injection surface is exposed. |
| Attestor bypass | `New` refuses construction without a non-nil `Attestor`. |
| Audio-vs-video scope confusion | Two distinct scope constants (`facetime:video` vs `facetime:audio`) so the operator-attestation log records which call type was requested; the audit row also records it (`kind` field). |
| Unsigned binary | `open(1)` does not require TCC; URL-scheme launch works for unsigned Leah. This is a deliberate accessibility win vs. iMessage. |

Audit row schema:

```
{
  "kind": "facetime_initiate",
  "ts": "2026-06-10T...",
  "mode": "video",                // or "audio"
  "success": true,
  "callee_hash": "a1b2c3d4",      // sha256(callee)[:8]
  "reason": ""                    // populated on failure
}
```

## 7. Test plan (all hermetic; no real `open(1)`, no real FaceTime UI)

- `TestInitiateVideo_HappyPath` — fake `OSExec` captures the invocation;
  assert `name == "open"` and `args == []string{"facetime://alice@example.com"}`.
- `TestInitiateAudio_UsesAudioScheme` — assert the URL prefix is
  `facetime-audio://`, not `facetime://`. Pins the audio/video divergence.
- `TestInitiateVideo_AttestationDenied_NoOpen` — fake Attestor rejects;
  fake OSExec invocation counter MUST stay zero. Returns wrapped
  `ErrAttestationDenied`.
- `TestInitiateAudio_AttestationDenied_NoOpen` — same, audio scope.
- `TestInitiate_InvalidCallee_NoExec` — table covers `";rm -rf"`, `"a&b"`,
  `"$(whoami)"`, `"\` + "`" + `id\` + "`" + `"`, `"a b"`, `"a/b"`, empty
  string. Each returns `ErrInvalidCallee`; neither attestor nor OSExec is
  invoked.
- `TestInitiate_AuditRowHashed` — callee field in audit row is the 8-char
  sha256 prefix; plaintext callee does NOT appear in the audit row JSON;
  `mode` field is `"video"` or `"audio"` as appropriate.
- `TestInitiate_RateLimit_RejectsBurst` — with injected `Now`, 5
  initiations in 30s succeed; 6th returns `ErrRateLimited` without
  invoking OSExec or Attestor. Limit is shared across video + audio
  (compromised Leah cannot evade by alternating modes).
- `TestInitiate_OpenExitNonZero_LaunchFailed` — fake OSExec returns
  exit-1; adapter maps to `ErrLaunchFailed` (wrapped). Audit row records
  `success: false`.
- `TestNewRejectsMissingDeps` — nil Attestor and nil OSExec each fail
  construction with a descriptive error.

## 8. Trade-offs (per CLAUDE.md UX > performance > long-term)

- **UX**: Operator still has to press the call button. This is a worse UX
  than fully-automatic dialing, but it is the only Apple-blessed path. The
  alternative (private APIs) trades operator UX for a permanent
  signing / notarization fragility tail.
- **Performance**: `open(1)` is sub-100ms; not a hotspot.
- **Long-term**: rate-limit middleware is intentionally inlined per-adapter
  in MVP and extracted to shared middleware in Wave 24 once we have two
  adapters using it. Premature shared-package was rejected.

## 9. Out of scope (MVP)

- Call recording.
- Group FaceTime (>1 callee).
- Screen sharing / SharePlay.
- Links-only mode (`facetime.apple.com/link` invite generation).
- Hang-up / mute / camera-flip control.
- In-call DTMF / phone-tree navigation.
- Inbound-call observation / answer automation.
- Operator-configurable rate limit (issue filed alongside this spec).
- Callee allow-listing.
