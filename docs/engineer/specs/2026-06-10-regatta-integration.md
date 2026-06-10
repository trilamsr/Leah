---
slug: regatta-integration
status: draft
phase: self-host
owner: leah
---

# Leah ↔ Regatta integration topology + `leah connect regatta`

## 1. Goal

Leah is a client of Regatta. The `regatta` binary owns agent dispatch
(`ship`, `review`, `agents list`); Leah owns operator UX, audit, and the
brief/dashboard. Today `internal/regattaclient` shells out to a sibling
`regatta` binary on `$PATH` — a single transport with no topology
negotiation. This spec gives Leah a typed `Endpoint` seam that supports
**cloud Regatta (A)** and **local sibling daemon (B)** at the same code
path, picks one at `leah connect regatta` time, and auto-detects at boot.
Embedded library mode (C) is an explicit non-goal.

## 2. Existing surface

`internal/regattaclient/regattaclient.go` (single file, 69 LOC) exposes:

- `Executor` interface: `Run(ctx, args []string) (string, error)`.
- `ShellExec`: production `Executor`; runs `exec.CommandContext` on
  `args[0]` and surfaces `ExitError.Stderr` on failure.
- `Client`: holds an `Executor`; constructed with `New()` which defaults
  to `ShellExec{}`.
- `Agent`: JSON shape `{id, branch, state, pr}` mirroring
  `regatta agents list --json`. State string is consumed by
  `daemonloop.isTerminal`.
- `Client.List(ctx) ([]Agent, error)`: shells `regatta agents list --json`
  and decodes the JSON result.

Callers (production):

- `cmd/leah/backlog.go` (`filterActive`)
- `cmd/leah/brief.go` (passes the client into `brief.Gather`)
- `cmd/leah/selfbuild.go`, `cmd/leah/main.go` (constructs the dispatcher
  with `Regatta: regattaclient.New()`)
- `cmd/leah-daemon/main.go`, `cmd/leah-daemon/dashboard.go` (long-running
  loop + dashboard read path)
- `internal/dispatcher/ship.go`, `internal/brief/brief.go`,
  `internal/web/state.go`, `internal/daemonloop/loop.go` — each defines a
  local `RegattaLister`-shaped subset interface

Gaps vs. the integration target:

- **No transport abstraction** beyond `exec.Cmd`. Cloud HTTP/gRPC and
  local Unix-socket transports both require a new seam.
- **No auth surface**. No token storage, no OAuth flow, no API-key path.
- **No `Status` RPC**. Boot-time reachability check needs one.
- **No `Ship` / `Review` calls**. Today the dispatcher composes these
  itself by shelling out to other binaries; consolidating under
  `Endpoint` is a wave-level move (W41, see plan).
- **No `Detect` / topology config**. `New()` is a zero-arg constructor.
- **No sentinel errors** beyond ad-hoc `fmt.Errorf` wraps.
- **No attestation gate**. Every caller bypasses `internal/connect`'s
  `Attestor` contract.

## 3. Topology decision matrix

| Topology | Latency | Dev-loop | Setup cost | Failure mode | Recommended for |
| --- | --- | --- | --- | --- | --- |
| A — Cloud HTTP/gRPC | 80-200 ms | needs net | OAuth or API-key flow at `connect` time | network partition → `ErrEndpointUnreachable` | shared / multi-machine ops, CI |
| B — Local sibling daemon (UDS) | <5 ms | offline OK | `brew install regatta && regatta daemon start` | regatta crash → reconnect on next call | solo / dev / `leah-daemon` boot |
| C — Embedded library | <1 ms | N/A | very high (vendoring + lockstep releases) | one-binary-bug kills both processes | **not pursued** |

Why **C is rejected** (root cause, not symptom): Leah's deletion-default
rule says every dependency must answer "what got smaller". Vendoring
regatta in-process would couple release cycles, double the surface area
of every Leah PR (the regatta diff lands too), and erase the
process-boundary that the Attestor was designed around. The two-binary
boundary is the audit boundary — collapsing it loses the operator-consent
seam.

## 4. Surface (Go API)

```go
package regattaclient

type TransportMode string

const (
    ModeCloud TransportMode = "cloud"
    ModeLocal TransportMode = "local"
)

// Endpoint is the typed seam Leah holds. Concrete implementations are
// cloudEndpoint (HTTPS to a Regatta server) and localEndpoint
// (Unix-socket to a sibling daemon). Both honor per-call attestation.
type Endpoint interface {
    Ship(ctx context.Context, req ShipReq) (ShipResp, error)
    Review(ctx context.Context, req ReviewReq) (ReviewResp, error)
    Status(ctx context.Context) (Status, error)
    List(ctx context.Context) ([]Agent, error)
}

type Config struct {
    Mode       TransportMode
    URL        string        // cloud: https://regatta.example.com
    SocketPath string        // local: $HOME/.leah-state/regatta.sock
    Token      string        // cloud auth bearer
    Attestor   Attestor      // mirrors internal/connect.Attestor shape
    HTTPClient *http.Client  // test seam (cloud)
    Dialer     func(ctx context.Context) (net.Conn, error) // test seam (local)
}

type Status struct {
    Version string
    Mode    TransportMode
    Healthy bool
}

type ShipReq struct{ Branch, Title, Body string }
type ShipResp struct{ AgentID string; PR int }
type ReviewReq struct{ PR int; Verdict string }
type ReviewResp struct{ Acked bool }

func New(cfg Config) (Endpoint, error)
func Detect() Config // auto-detect local-vs-cloud at boot; pure, no I/O on disk write
```

Sentinel errors (top of file, exported):

```go
var (
    ErrAttestationDenied  = errors.New("regattaclient: attestation denied")
    ErrEndpointUnreachable = errors.New("regattaclient: endpoint unreachable")
    ErrAuthFailed         = errors.New("regattaclient: auth failed")
    ErrTopologyAmbiguous  = errors.New("regattaclient: both cloud token and local socket present; set LEAH_REGATTA_MODE")
    ErrTopologyMissing    = errors.New("regattaclient: no transport configured; run leah connect regatta")
)
```

Compatibility note. The existing `Client.List` and `Agent` type survive
unchanged — `Client` becomes a thin wrapper that constructs an `Endpoint`
via `New(Detect())` and forwards `List`. Existing callers
(`brief.Gather`, `dispatcher/ship.go`, `daemonloop`, `web/state.go`)
continue to typecheck. The `RegattaLister` mini-interfaces in those
packages stay valid because `Endpoint` is a superset.

## 5. `leah connect regatta` flow

Subcommand extends the registry in `internal/connect/registry.go` by
adding a third provider alongside `NewGmail` / `NewGcal`. Regatta differs
from the OAuth pair in two ways: (1) topology choice precedes auth, and
(2) local mode has no token — only a mode file.

Sub-flow:

1. **Choose topology**. Prompt `cloud or local? [cloud]`. Environment
   override `LEAH_REGATTA_MODE` short-circuits the prompt.
2. **Cloud branch**:
   - Prompt URL (default `https://regatta.example.com`).
   - Prompt auth method: `api-key` (paste) or `oauth` (device-code, same
     `runDeviceCodeFlow` helper as gmail/gcal).
   - `Attestor.Attest(ctx, "connect:regatta")` BEFORE any token I/O —
     mirrors `connect.Authorize` ordering.
   - Probe: construct `cloudEndpoint`, call `Status(ctx)` with a 5-second
     deadline. Non-nil error → `ErrEndpointUnreachable`, abort without
     writing.
   - Write `~/.leah-state/secrets/regatta-token.json` 0600
     (`connect.WriteToken` already does the mkdir-0700 + chmod-0600
     dance).
   - Write `~/.leah-state/secrets/regatta-mode.json` containing
     `{"mode":"cloud","url":"<u>"}` 0600.
3. **Local branch**:
   - Probe `~/.leah-state/regatta.sock`: must exist, be a socket, be
     owned by `os.Geteuid()`, and have mode 0600.
   - Missing → print remediation: `brew install regatta && regatta daemon
     start` (and exit non-zero so scripts halt).
   - `Attestor.Attest(ctx, "connect:regatta")` BEFORE writing the mode
     file.
   - Probe: dial the socket, call `Status(ctx)` with a 1-second deadline.
   - Write `~/.leah-state/secrets/regatta-mode.json` containing
     `{"mode":"local","socket":"<p>"}` 0600.
4. **Audit row** on success: `audit.Entry{Kind: "connect_regatta",
   BlastRadius: 2, Outcome: "success", Detail: "<mode>"}`.
5. **Failure paths** emit `Kind: "connect_regatta"`,
   `Outcome: "failed"`, `Detail` from a `summarizeRegattaErr` analog of
   `summarizeConnectErr` in `cmd/leah/connect.go`.

## 6. Auto-detect rules (boot path)

Order, first match wins:

1. `LEAH_REGATTA_MODE=cloud|local` env var — explicit operator override.
2. Read `~/.leah-state/secrets/regatta-mode.json`. If present and parses,
   use its `mode`.
3. Probe local socket (`~/.leah-state/regatta.sock`): if exists +
   writable + owned-by-uid + `Status` returns healthy in <1 s, choose
   local.
4. Probe cloud token (`~/.leah-state/secrets/regatta-token.json`): if
   present, choose cloud.
5. Both present, no env, no mode file → return `ErrTopologyAmbiguous`
   (refuse to silently pick a side).
6. Neither present → return `ErrTopologyMissing` with a hint pointing at
   `leah connect regatta`.

The probe in step 3 is bounded by a 1-second deadline so a stale socket
(file exists, daemon dead) cannot block daemon boot; failure falls
through to step 4. This is the only piece of I/O `Detect` performs;
callers can wrap it in a test seam by setting `LEAH_REGATTA_MODE`.

## 7. Operator-attestation

Per-call attestation, scoped at the action grain so the audit row
attributes consent the way the operator sees the action.

| RPC | Scope | Attest? | Rationale |
| --- | --- | --- | --- |
| `Status` | `regatta:status` | no | read-only liveness probe; daemon polls it every 30 s |
| `List` | `regatta:list` | no | read-only; backs morning brief + dashboard |
| `Ship` | `regatta:ship` | yes | creates a PR / mutates remote state |
| `Review` | `regatta:review` | yes | merges or escalates a PR |

Ordering inside the cloud/local endpoint mirrors
`internal/connect/connect.go`'s `Authorize`: `Attestor.Attest` runs
**before** the bearer token is loaded into memory for cloud, and before
the socket dial for local. Same load-bearing rule as the gmail / gcal
adapters — a denied attestation must not materialize the secret nor open
the connection.

`connect:regatta` is the scope passed at first-launch (Section 5).

## 8. Threat model

- **Token leak (cloud)**. Token file `0600` enforced by
  `connect.WriteToken`. Token never logged; `summarizeRegattaErr` returns
  only sentinel names. Errors wrapping `ErrAuthFailed` MUST NOT carry
  the bearer.
- **Socket hijack (local, multi-user Mac)**. At construction,
  `localEndpoint` `os.Stat`s the socket and rejects if
  `Sys().(*syscall.Stat_t).Uid != os.Geteuid()` or mode is not `0600`.
  Rejection returns `ErrEndpointUnreachable` with detail
  `"socket_owner_mismatch"` — the path itself is logged once, the file
  contents never are.
- **Cloud MITM**. TLS required: `cloudEndpoint` rejects non-`https://`
  URLs at construction. Optional allowlist via
  `LEAH_REGATTA_ALLOWED_HOSTS` (comma-separated); default empty = any
  HTTPS host is acceptable. Tightening this default defers to a
  follow-up wave once a hosted Regatta endpoint is published.
- **Cross-topology poisoning**. Both a cloud token AND a local socket
  present, with no `LEAH_REGATTA_MODE` env and no mode file ->
  `ErrTopologyAmbiguous`. We refuse rather than silently choose; the
  operator's `connect` flow always writes the mode file, so this state
  only occurs when files were placed by another path.
- **Replay attacks**. The wire-format guarantees are Regatta's
  responsibility (it owns the server); Leah does not attempt
  client-side nonces. Documented as out of scope.
- **Audit-row leak**. Every RPC appends `Kind: "regatta_<op>",
  Outcome: success|failed, Detail: "<short>"`. No payload bytes, no
  branch contents, no PR bodies — just the verb. `ShipReq.Body` is never
  written to disk by Leah.
- **Stale-attestation reuse**. `Attestor.Attest` is called per RPC; the
  scope is the action verb, not a session token. There is no cache.

## 9. Test plan

Hermetic, no real network, no real `regatta` binary.

- `TestNew_RejectsMissingAttestor` — fail-closed wiring contract,
  mirrors gmail/gcal.
- `TestDetect_TableDriven` covers the four cases:
  - only cloud token + no env → `ModeCloud`
  - only mode file `{mode:local}` + no env → `ModeLocal`
  - both present, no env → `ErrTopologyAmbiguous`
  - neither present → `ErrTopologyMissing`
- `TestDetect_EnvOverridesEverything` — `LEAH_REGATTA_MODE=cloud` with
  only a local socket on disk returns `ModeCloud`.
- `TestCloud_Status_HappyPath` — `httptest.Server` returns `{healthy:
  true}`; client returns `Status{Healthy: true}`.
- `TestCloud_Status_Unreachable` — server closed; expect
  `ErrEndpointUnreachable`.
- `TestCloud_Ship_AttestDenied` — failing `Attestor`; expect
  `ErrAttestationDenied`; httptest server records zero requests
  (proves the gate ran before the network call).
- `TestCloud_Auth401` — server returns 401; expect `ErrAuthFailed`.
- `TestLocal_Status_HappyPath` — in-process `net.Pipe` dialer; expect
  healthy.
- `TestLocal_RejectsForeignUidSocket` — set up a tempfile owned by a
  different uid (use a `Statter` test seam since CI cannot chown);
  expect `ErrEndpointUnreachable`.
- `TestLocal_Ship_AttestDenied` — denied gate; socket dialer records
  zero calls.
- `TestAuditRow_OnEveryCall` — every RPC writes a single audit row with
  no payload bytes; `Detail` length ≤ 32.

## 10. Out of scope (MVP)

- **Embedded library mode (Option C)** — rejected in §3.
- **Bidirectional sync** — Regatta calling back into Leah (e.g., to push
  a review request). Today Leah is strictly a client.
- **Multi-account Regatta** — one token per topology; multi-tenant
  defers until a real operator asks.
- **Cross-machine local sockets (UDS-over-SSH)** — operators who want
  remote-machine Regatta use cloud mode.
- **Regatta server-side contract changes** — schema, wire format, and
  auth endpoints belong in the Regatta repo. This spec pins only the
  client side.
- **Token refresh** — cloud OAuth refresh defers to a follow-up wave;
  MVP requires the operator to re-run `leah connect regatta` when the
  token expires.
- **Migration of existing `ShellExec` callers** — the wave plan
  consolidates `dispatcher/ship.go` and friends onto `Endpoint`, but
  that lands wave-by-wave so each PR stays ≤500 LOC.
