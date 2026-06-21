---
title: Inbound-reply router + per-action consent contract (F3)
status: proposed
owner: tri
created: 2026-06-21
tracker: MAY-267
---

# Inbound-reply router + per-action consent contract

## 1. Goal

Close the proactive loop's return leg: the operator replies to a pushed
recommendation from a remote surface ("approve" / "no" / "later") and the daemon
acts — `recommend.Accept` then `Apply`, or `Reject`. F2 already pushes the prompt
out (comms notifier, MAY-264). F3 carries the reply back in.

This is the highest-risk wave in the roadmap: it opens an **inbound channel that
makes Leah mutate state**. Every prior mutation path (HUD accept/reject) is gated
by an attested *loopback* endpoint the operator physically owns. A remote reply
arrives over a channel Leah does not own (Discord's gateway, Meta's webhook), so
the safety can no longer come from "the request reached 127.0.0.1." It must come
from a **per-action consent gate**. Section 4 is the load-bearing half of this
spec; sections 3 and 5 are the plumbing around it.

## 2. Producers it depends on (verified present on main @ dd6fa00)

- `internal/adapters/discord/subscribe.go` —
  `Subscribe(ctx, channelIDs []string, handler func(Message)) error`. Opens an
  **outbound** gateway websocket and routes inbound `MESSAGE_CREATE`. **Present**
  but never wired: requires `Config.WebSocketDialer` (a `WebSocketDialer`
  interface), which `cmd/leah-daemon/comms.go` never sets (the daemon constructs
  discord with no dialer because it only posts). `Message{ChannelID, GuildID,
  AuthorID, Body, Voice, Timestamp}` is the inbound surface. Attests once at
  `ScopeSubscribe` ("discord:subscribe") at subscription grain.
- `internal/adapters/whatsapp/whatsapp.go` —
  `WebhookHandle(ctx, payload []byte, signature string) ([]Message, error)`.
  **Present**, HMAC-verified before JSON parse. But there is **no HTTP route**
  wiring it, and a webhook needs a *public* endpoint Meta can POST to — see §6.
- `internal/recommend/engine.go` — `MemoryEngine.Accept/Reject/Apply`. `Apply`
  honours the tier rubric (`TierConfirm` requires a prior `Accept`;
  `TierBlocked` never fires). **Present** — the reachable mutation.
- `internal/hud/recommendations.go` — `RecommendSeam{Propose, Accept, Reject}`.
  Today the **only** caller of the mutation, behind the loopback HUD endpoint.
- `internal/attestation` — `Pool.Pick(scope)`, `Load(path, scopes...)`,
  `contracts.Attestor.Attest(ctx, scope) error`. Existing per-action gates:
  `self-build`, `self-build-a2a`, `cost_override`, `self-upgrade`
  (`scopes.go`). The `self-build` vs `self-build-a2a` split is the precedent
  this spec extends: a distinct scope per *trust origin* so habituation on one
  cannot satisfy another.
- `cmd/leah-daemon/main.go:111` — `recommend.NewMemoryEngine(a)`, the daemon-
  owned singleton. The router wires here, not in the HUD process.

## 3. Inbound-reply router

### 3.1 Shape

One new package `internal/inbound` owns the channel-agnostic router; the two
transports stay in their adapter packages and feed it normalized messages.

```go
// internal/inbound/router.go
type Reply struct {
    Channel   string // "discord" | "whatsapp"
    PeerID    string // discord AuthorID / whatsapp sender — the consent subject
    ConvID    string // discord ChannelID / whatsapp thread — pending-rec correlation key
    Text      string
    Voice     []byte // STT deferred (see §6); router ignores until text path lands
    Received  time.Time
}

// Router maps a Reply to a pending recommendation, clears the consent gate,
// classifies intent, and dispatches to the engine. It owns NO transport.
type Router struct {
    Pending  PendingStore   // conversation state: ConvID -> pending rec id
    Consent  ConsentGate    // §4 — the load-bearing gate
    Classify Classifier     // §5
    Engine   recommend.Engine
    Audit    func(AuditRow)
}
func (r *Router) Handle(ctx context.Context, reply Reply) error
```

### 3.2 Conversation state (reply -> pending rec)

When F2 pushes a recommendation to a channel, it records the correlation so the
reply can find its target:

```go
// internal/inbound/pending.go
type Pending struct {
    RecID    string
    Channel  string
    ConvID   string
    PeerID   string // who the prompt was sent to == who may answer it
    SentAt   time.Time
}
type PendingStore interface {
    Put(Pending) error
    Take(channel, convID string) (Pending, bool) // single-use: consumed on first matching reply
}
```

- Keyed by `(channel, convID)` — a reply in the thread the prompt landed in is
  matched to that prompt's pending rec. `Take` is **single-use**: a reply
  consumes the pending entry so a duplicate/replayed message cannot re-fire.
- An in-memory `map` store is sufficient for v1 (single operator, daemon-
  lifetime state; a missed reply after a daemon restart simply re-prompts on the
  next cron — no durability requirement). SQLite is out of scope (§6).
- `PeerID` mismatch (a reply from someone who is **not** the prompt's recipient
  in a shared channel) is dropped before the consent gate — the consent subject
  must equal the prompt's addressee.

### 3.3 Where it wires (`cmd/leah-daemon`)

- **Discord (primary, loopback-safe).** New `cmd/leah-daemon/inbound.go`
  constructs a discord adapter **with** `Config.WebSocketDialer` set (the real
  gorilla dialer — the seam already exists), then calls `Subscribe(ctx,
  channelIDs, handler)` where `handler` adapts `discord.Message` -> `Reply` and
  calls `Router.Handle`. Gated behind `LEAH_INBOUND_DISCORD=1` + a connected
  discord token + a configured channel allowlist (same connected-and-configured
  silent-absence pattern as the F2 notifier). The gateway is an **outbound**
  websocket — no bound public port — so the loopback invariant holds (§4.4).
- **WhatsApp (deferred).** `WebhookHandle` would wire under a new daemon HTTP
  route, but that route must be reachable by Meta = a public endpoint or tunnel.
  The operator has **not** authorized public-endpoint/tunnel infra. So this
  variant is **deferred**, mirroring MAY-126's reasoning: ship the loopback-safe
  transport now, defer the one that needs an inbound public port until tunnel
  infra is explicitly authorized. The `internal/inbound` router is transport-
  agnostic by design, so the whatsapp feed is purely additive later.

## 4. Per-action consent contract (load-bearing)

### 4.1 The hole this closes

Channel HMAC (whatsapp) and once-at-subscription attestation (discord
`ScopeSubscribe`) prove **the channel is authentic** and **the operator allowed
Leah to read the channel**. Neither proves **the operator consents to this
specific mutation right now**. Without a per-action gate, anyone who can post the
text "approve" into a subscribed channel (a compromised Discord account, a
shared thread, a spoofed forward) would drive `Apply` — including a
`self-build` merge. Read-consent must not imply act-consent.

### 4.2 Two-layer gate

```go
// internal/inbound/consent.go
type ConsentGate interface {
    // Enrolled reports whether the operator one-time-authorized this channel+peer
    // to answer recommendations at all (layer 1). Fail-closed.
    Enrolled(channel, peerID string) (bool, error)
    // Authorize clears the PER-ACTION gate for a reply that will mutate state
    // (layer 2). Attests against a scope chosen by the rec's blast radius.
    Authorize(ctx context.Context, p Pending, intent Intent) error
}
```

**Layer 1 — channel enrollment (one-time).** Before any remote reply can act,
the operator runs a local CLI: `leah inbound enroll discord <channelID>
<peerID>`. This attests **once** at a new scope `inbound-enroll`
(register in `attestation/scopes.go`; `Pool.Pick` surfaces the question) and
persists the `(channel, peerID)` pair to `~/.leah-state/`. Enrollment happens on
the **loopback CLI**, not over the remote channel — the act of granting a remote
surface the right to act is itself an attested **local** action. An un-enrolled
reply is dropped at the router with an audit row; it never reaches layer 2.

**Layer 2 — per-action attestation, scaled to blast radius.** A reply that maps
to a pending rec must clear an attestation keyed to *what the rec does*:

| Rec tier / action class            | Per-action gate                                                |
|------------------------------------|---------------------------------------------------------------|
| `TierAuto`, non-destructive        | enrollment only (layer 1) — no per-reply prompt                |
| `TierConfirm` (state mutation)     | attest scope `inbound-apply` on **this** Accept                |
| `self-build` / `self-upgrade` rec  | attest the rec's **own** existing scope (`self-build`, etc.) — remote origin does NOT downgrade the gate |
| `TierBlocked`                      | never fires regardless of reply (engine already enforces)      |

The crucial rule: **a remote reply never clears a weaker gate than the same
action would face locally.** A `self-build` accepted from the couch attests the
identical `self-build` scope a local accept would. Remote origin can only *add*
friction (enrollment), never remove it. This is why §4.1's spoof cannot merge:
even an enrolled, authentic "approve" must still clear the action's own
attestation, which prompts the operator on a trusted local surface.

### 4.3 When the gate fires (sequence)

```
remote msg → adapter normalize → Reply
  → PeerID == Pending.PeerID?           (else drop+audit)
  → ConsentGate.Enrolled(channel,peer)? (layer 1; else drop+audit)
  → Pending.Take(channel,convID)        (single-use; else drop "no pending")
  → Classify(text) → accept|reject|defer|unknown
  → reject/defer: Engine.Reject — NO per-action gate (rejecting is safe)
  → accept: ConsentGate.Authorize(ctx,pending,accept)  ← layer 2 attest
             → Engine.Accept(id); Engine.Apply(rec)
  → audit row at every branch
```

Reject and defer do **not** attest — declining an action is always safe and a
per-reply prompt there would only train the operator to dismiss prompts. The gate
sits exclusively on the **act** path.

### 4.4 Loopback invariant reconciliation

- Discord-gateway primary is an **outbound** websocket: Leah dials out, no
  inbound port bound. The loopback invariant ("no bound public port") holds
  unchanged.
- Enrollment (the authorization to let a remote surface act) is a **local
  loopback CLI** attestation — the trust grant never crosses the network.
- Per-action attestation for mutating accepts also prompts on the **local**
  attestation surface (the same one HUD/self-build use). The remote channel
  carries the *intent*; the *consent* is always collected locally. No remote
  message is ever itself the attestation.
- WhatsApp webhook (needs an inbound public port) is therefore the only variant
  that touches the invariant, and it is deferred until tunnel infra is
  authorized.

## 5. Classifier (the easy half)

Bridges normalized text to `accept | reject | defer | unknown` **after** the
consent gate, so a misclassification can never escalate privilege (the gate
already cleared on intent class, and `unknown` is fail-safe).

```go
// internal/inbound/classify.go
type Intent int // IntentAccept | IntentReject | IntentDefer | IntentUnknown
type Classifier interface {
    Classify(ctx context.Context, text string) (Intent, error)
}
```

- **Fast path — regex/keyword.** Anchored, case-folded match:
  accept = `^(y|yes|ok|okay|approve|do it|go|ship it|👍|✅)\b`;
  reject = `^(n|no|nope|reject|stop|don'?t|skip|👎|❌)\b`;
  defer = `^(later|snooze|not now|defer|tomorrow)\b`. Covers the overwhelming
  majority of one-word replies with zero LLM latency/cost.
- **LLM fallback** only when the fast path returns no match, behind the existing
  LLM seam. Prompt: classify into the four intents, **default `unknown`** on any
  ambiguity. `unknown` → no action, audit, and a one-line clarifying reply back
  to the channel ("reply yes / no / later"). Fail-safe: ambiguity never acts.
- Defer records a re-prompt marker so the next brief re-surfaces the rec; it does
  not delete the pending rec.

## 6. Out of scope

- **WhatsApp webhook inbound** — deferred (needs public endpoint/tunnel;
  loopback invariant). Router is transport-agnostic so it is additive later.
- **Voice replies** — `Reply.Voice` is carried but ignored until the STT inbound
  path lands; v1 is text-only intent. (Discord voice attachments already arrive
  as bytes on `discord.Message`; decoding is a separate wave.)
- **Multi-operator** — enrollment is single-peer per channel; no roles, no
  delegation, no quorum. Single-operator (owner == user) per the ship-path.
- **Durable pending store (SQLite)** — in-memory is sufficient; a daemon restart
  re-prompts on the next cron.
- **Free-form commands** — only accept/reject/defer of an *already-pushed*
  recommendation. "Leah, do X" from a channel is a separate, much larger
  trust-surface wave and is explicitly not this spec.

## 7. Test plan (TDD — failing test first)

Consent contract (the load-bearing tests, write first):

- `Authorize` for a `self-build` rec attests the **`self-build`** scope, NOT a
  weaker `inbound-apply` (assert the scope passed to a fake Attestor) — proves
  remote origin does not downgrade the gate.
- A reply from a peer ≠ `Pending.PeerID` is dropped before `Enrolled` is called
  (fake gate records zero calls) — proves addressee binding.
- An un-enrolled `(channel,peer)` is dropped before `Take`/attest — fail-closed.
- A denied per-action attestation leaves the rec **unapplied** and emits an audit
  row (Engine.Apply never called).
- `Take` is single-use: a second identical reply finds no pending rec (replay
  defense).

Router + classifier:

- Regex table test: each accept/reject/defer keyword → correct `Intent`;
  gibberish → `IntentUnknown` → no engine call + clarifying reply.
- `unknown` and `reject`/`defer` paths never call `ConsentGate.Authorize`
  (gate only on the act path).
- discord `Message` → `Reply` adapter maps fields correctly; voice bytes ignored.

Wiring:

- daemon constructs discord **with** a dialer only when `LEAH_INBOUND_DISCORD=1`
  + connected token + channel allowlist; omits the subscription otherwise
  (silent absence, no error).

## 8. Risk

- **Privilege escalation via read channel** — the entire §4 design exists for
  this. Mitigation: two-layer gate, action-own-scope rule, local-only consent
  collection. Highest-severity item; every test in §7's first block targets it.
- **Spoofed/forwarded "approve"** — addressee binding (`PeerID`) + per-action
  local attestation. An attacker who posts "approve" still cannot clear the
  local attestation prompt.
- **Replay** — single-use `Take` + the engine's existing idempotent
  pending-removal.
- **Operator habituation on per-action prompts** — mitigated by gating ONLY the
  act path (reject/defer never prompt) and by enrollment removing the prompt for
  non-destructive `TierAuto` recs. Destructive recs deliberately keep the
  prompt; that friction is the feature.
- **Attestation pool not loaded with new scopes** — `Pool.Pick` fails closed on
  an unregistered scope (`ErrUnknownScope`); a forgotten `Load` registration
  blocks the act rather than bypassing it. Add `inbound-enroll` + `inbound-apply`
  to the daemon's `Load(...)` call (covered by a registration test, the
  `CostOverrideScope` precedent).

## 9. Dependency order

1. **Router + pending store** (`internal/inbound/router.go`, `pending.go`) —
   transport-agnostic, no consent yet; pure mapping + state, fully unit-testable.
2. **Consent contract** (`consent.go` + `attestation/scopes.go` new scopes +
   `leah inbound enroll` CLI) — the gate. Nothing acts until this lands.
3. **Classifier** (`classify.go`) — regex + LLM fallback, bridges to the engine
   behind the cleared gate.
4. **Discord wiring** (`cmd/leah-daemon/inbound.go` with `WebSocketDialer`) —
   the loopback-safe transport feeding the router.
5. **WhatsApp webhook** — **deferred** (loopback invariant; needs tunnel infra).

Steps 1–4 are the shippable F3. Step 5 unblocks only when public-endpoint infra
is explicitly authorized.
