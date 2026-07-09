---
slug: regatta-integration
status: draft
phase: self-host
owner: leah
---

# Regatta integration (MVP) — Docker-first topology

## 1. Goal

Give Leah a single `regattaclient.Endpoint` seam that talks to a regatta
instance the operator runs **as a Docker container on their own Mac**. Cloud
hosting is documented but explicitly DEFERRED until the operator decides cost
is no longer a concern.

Primary topology is local-Docker. Auto-detect picks it without operator
configuration when Docker is healthy. Cloud and sibling-binary modes are
spec'd so the future cost-no-longer-an-issue switch is wiring, not redesign.

## 2. Dependency (adopt-over-build)

- **Docker** as the regatta runtime — Docker Desktop or colima provide the
  daemon. Operator installs out-of-band; Leah only requires `docker info` to
  succeed.
- **Regatta container image** — `ghcr.io/trilamsr/regatta`. Pinned by
  **sha256 digest** in `internal/regattaclient/image.go`, never by `:latest`
  at runtime (supply-chain mitigation). The pin moves only via a deliberate
  PR.
- **No new Go module** — Leah talks to the container over loopback HTTP
  (`http://127.0.0.1:9090`). The Docker daemon itself is driven via
  `os/exec` against the `docker` CLI; we do not link the Docker Engine SDK.
  Three similar lines beat a premature abstraction.

## 3. Topology decision matrix

Ranked by current priority (cost-conscious operator on their own Mac):

| Rank | Topology | When | Status |
| --- | --- | --- | --- |
| **A** | **Docker on Mac (PRIMARY)** | Default. Regatta runs as `leah-regatta` container; Leah talks to `http://127.0.0.1:9090`. Zero cloud cost. Self-contained per Mac. | **Shipped MVP** |
| B | Cloud (hosted regatta) | Operator opts in once cross-machine sync or hosted SLA is worth paying for. | **Deferred** — wiring spec'd, branch dark until W43 |
| C | Sibling local binary (no Docker) | Advanced operator who prefers a Homebrew-installed `regatta` daemon over Docker. Same loopback HTTP / unix-socket surface. | Documented fallback |
| D | Embedded library (in-process) | Co-locates regatta into the daemon binary. Couples release cadence and OOMs together. | **Non-goal** — explicitly rejected |

Decision rule: **A wins by default. B is an opt-in escape valve, not a
default.** C is the "I already have it installed" path. D is closed.

## 4. Surface

```go
package regattaclient

const (
    ScopeStatus = "regatta:status"
    ScopeWrite  = "regatta:write"
)

var (
    ErrAttestationDenied = errors.New("regatta: attestation denied")
    ErrNotConnected      = errors.New("regatta: not connected (run `leah connect regatta`)")
    ErrTransport         = errors.New("regatta: transport failure")
    ErrDockerUnavailable = errors.New("regatta: docker daemon unavailable")
)

// Endpoint is the single seam every caller uses. Implementations:
//   - dockerTransport (primary)   — HTTP over 127.0.0.1
//   - socketTransport (fallback)  — HTTP over ~/.leah-state/regatta.sock
//   - cloudTransport  (deferred)  — HTTPS + bearer token
type Endpoint interface {
    Status(ctx context.Context) (Status, error)
    // Write / Read RPCs added by their owning waves; the interface is
    // intentionally small at W38.
}

type Status struct {
    Mode       string // "docker" | "socket" | "cloud"
    Healthy    bool
    Version    string
    LatencyMS  int64
}

type Config struct {
    Attestor Attestor
    Now      func() time.Time
    // Exec is the subprocess seam used by the Docker transport to drive the
    // `docker` CLI. Production wires os/exec.CommandContext; tests inject a
    // fake.
    Exec OSExec
}

func New(cfg Config) (Endpoint, error)
```

`New` resolves the active mode from disk (Section 7) and constructs the
matching transport. Nil `Attestor` and nil `Exec` fail construction with a
descriptive error — no silent default.

## 5. `leah connect regatta` — Docker branch (primary)

Ordering is load-bearing (mirrors the gmail / imessage `gateAndExec`
pattern):

1. `Attestor.Attest(ctx, "connect:regatta")` — operator consent. A non-nil
   return aborts before any docker call.
2. `docker info` via `OSExec.Run` — probes the daemon. Non-zero exit returns
   `ErrDockerUnavailable` with a remediation hint ("install Docker Desktop
   or `brew install colima && colima start`"). NO subsequent step runs.
3. `docker pull ghcr.io/trilamsr/regatta@sha256:<digest>` — pin by digest,
   never by `:latest`. The digest is a `const` in
   `internal/regattaclient/image.go`.
4. `docker run -d --name leah-regatta -p 127.0.0.1:9090:9090 -v
   ~/.leah-state/regatta-data:/data ghcr.io/trilamsr/regatta@sha256:<digest>`
   — bind to **127.0.0.1 only**, never `0.0.0.0`. Volume mount mode is
   `0700` and chown'd to the operator UID before `docker run`.
5. Health probe: poll `http://127.0.0.1:9090/healthz` every 250ms up to a
   **5s budget**. First 2xx wins. Budget exhaustion is `ErrTransport` and
   the partial container is **torn down** before returning (no leaked
   `leah-regatta`).
6. Write `~/.leah-state/secrets/regatta-mode.json` with mode `0600`:
   ```json
   {
     "mode": "docker",
     "url": "http://127.0.0.1:9090",
     "container": "leah-regatta",
     "image_digest": "sha256:...",
     "connected_at": "2026-06-10T..."
   }
   ```
7. Sanity call `Endpoint.Status(ctx)` — must succeed. On failure, run the
   same teardown as step 5.
8. Audit row `Kind: "connect_regatta_docker"` with `success bool` and (on
   failure) a `reason` string.

## 6. `leah connect regatta --cloud` (DEFERRED)

Document the path so W43 is wiring, not design:

1. Operator-attestation `connect:regatta:cloud` — a distinct scope so the
   consent prompt names the cost implication.
2. Validate `--url` and `--token` flags; reject if either is missing.
3. Single round-trip `GET <url>/healthz` with the bearer token; 2xx is
   success.
4. Write `regatta-mode.json` with `{mode: "cloud", url, token_ref}` —
   `token_ref` points to a separate `0600` file holding the bearer token
   (never inline the token into the mode file).
5. Audit row `Kind: "connect_regatta_cloud"`.

W43 is gated on **explicit operator request**. The flag exists in the CLI
parser earlier (to give a clear "deferred — see W43" error) but the
transport implementation lands only in W43.

## 7. Auto-detect rules (daemon boot)

Resolved in this order, first match wins:

1. `LEAH_REGATTA_MODE` env override — forces `docker | socket | cloud | off`.
   `off` means "do not construct an Endpoint" (test scaffolding, CI).
2. Docker mode: `docker inspect leah-regatta` returns running AND
   `GET http://127.0.0.1:9090/healthz` returns 2xx within 1s.
3. Socket mode: `~/.leah-state/regatta.sock` exists, is a unix socket, and
   `GET /healthz` over it returns 2xx within 1s.
4. Cloud mode: `regatta-mode.json` `mode == "cloud"` AND token file
   present. Emit a one-time **warning audit row**
   `Kind: "regatta_mode_cloud_active"` so the cost path is observable.
5. Otherwise return `ErrNotConnected` — callers surface the
   `leah connect regatta` remediation string.

The auto-detect lives in `internal/regattaclient/detect.go` and is
covered by table-driven tests with all five branches.

## 8. Threat model

| Surface | Mitigation |
| --- | --- |
| Container escape | Out of scope — Docker's threat model, not Leah's. Documented in spec so the boundary is explicit. |
| Supply-chain swap of regatta image | Pin by **sha256 digest** in `image.go`. `docker pull` is rejected by Docker itself if the digest does not match. `:latest` never appears in runtime paths. |
| Port exposed to LAN | `docker run -p 127.0.0.1:9090:9090` binds to loopback only. Spec MUST NOT recommend `-p 9090:9090` (which binds `0.0.0.0`). A test asserts the argv contains the `127.0.0.1:` prefix. |
| Volume contents on disk | `~/.leah-state/regatta-data` is `0700` and owned by the operator UID. Created by `connect` before `docker run`; `connect` fails closed if the chown does not stick. |
| Bearer-token leak (cloud path) | Token in a separate `0600` file referenced by `token_ref`, never inlined into `regatta-mode.json` (which is read by more code paths). |
| Stale container on crashed `connect` | `connect` runs `docker stop && docker rm` in a deferred teardown when any step after `docker run` fails. Test asserts no `leah-regatta` survives a forced health-probe timeout. |
| `disconnect` leaving data | `leah disconnect regatta` stops + removes the container by default. Volume removal requires `--purge-data` AND a fresh attestation `disconnect:regatta:purge`. Mirrors the PR #66 `leah disconnect` pattern. |
| Docker not installed | `docker info` failure surfaces `ErrDockerUnavailable` with a remediation hint. No silent fallback to cloud — falling back to a paid path without consent is a UX violation. |
| Forbidden brand names in audit kinds / errors | Hard-coded grep gate `make check` (pattern lives in `scripts/forbidden-grep.sh`) MUST return 0 hits in this package. |
| Attestor bypass | `New` refuses construction without a non-nil `Attestor`. |

Audit row schemas:

```
{ "kind": "connect_regatta_docker",  "success": bool, "image_digest": "sha256:...", "reason": "" }
{ "kind": "connect_regatta_cloud",   "success": bool, "url_host": "regatta.example", "reason": "" }
{ "kind": "regatta_mode_cloud_active", "ts": "..." }   // one per daemon boot under cloud mode
{ "kind": "disconnect_regatta",      "purged_data": bool, "success": bool }
```

## 9. `leah disconnect regatta`

Mirror of PR #66 disconnect pattern:

1. Operator-attestation `disconnect:regatta`.
2. Resolve current mode from `regatta-mode.json`.
3. Docker mode: `docker stop leah-regatta` (10s grace) then `docker rm
   leah-regatta`. Idempotent — missing container is not an error.
4. `--purge-data` flag + second attestation `disconnect:regatta:purge`:
   delete `~/.leah-state/regatta-data/`. Without the flag, data survives so
   reconnect is cheap.
5. Cloud mode: delete the token file; leave the remote service alone (we do
   not own its lifecycle).
6. Delete `regatta-mode.json`.
7. Audit row `Kind: "disconnect_regatta"`.

## 10. Test plan (all hermetic; no real docker, no real regatta)

- `TestNew_RejectsMissingDeps` — nil `Attestor` / nil `Exec` each fail.
- `TestConnect_Docker_HappyPath` — fake Exec records the argv sequence;
  assert ordering: `info`, then `pull`, then `run`, then health-probe
  HTTP. Assert `run` argv contains `127.0.0.1:9090:9090` and the digest
  form `@sha256:`.
- `TestConnect_Docker_AttestationDenied_NoExec` — fake Attestor returns
  error; Exec invocation counter MUST stay zero. No mode file written.
- `TestConnect_Docker_DockerInfoFails_NoPull` — `docker info` exit !=0;
  returns `ErrDockerUnavailable`; `pull` and `run` MUST NOT be invoked.
- `TestConnect_Docker_HealthTimeout_TeardownRuns` — health probe never
  returns 2xx; assert subsequent fake-Exec calls include `docker stop` and
  `docker rm leah-regatta`. No `regatta-mode.json` left on disk.
- `TestConnect_Docker_RejectsLatestTag` — if `image.go` ever drifts to
  `:latest`, the build fails: a regex test asserts the digest constant
  matches `^sha256:[0-9a-f]{64}$`.
- `TestConnect_Docker_PortBindingLoopbackOnly` — argv contains
  `127.0.0.1:9090:9090` and does NOT contain a bare `:9090` or
  `0.0.0.0:9090`.
- `TestAutoDetect_PriorityOrder` — table-driven across the five rules.
- `TestAutoDetect_CloudActive_EmitsWarningOnce` — cloud mode boot emits
  one `regatta_mode_cloud_active` audit row per daemon process lifetime.
- `TestConnect_Cloud_Deferred` — `leah connect regatta --cloud` returns a
  "deferred — see W43" error in MVP; no token file written.
- `TestDisconnect_Docker_RemovesContainer_KeepsData` — fake Exec sees
  `stop` + `rm`; data directory still exists.
- `TestDisconnect_Docker_PurgeData_RequiresSecondAttestation` — without
  the second attestation, `--purge-data` returns
  `ErrAttestationDenied`; data directory survives.
- `TestForbiddenBrandGrep` — package-level grep test against the
  `scripts/forbidden-grep.sh` pattern. 0 hits required.

## 11. Trade-offs (per README.md UX > performance > long-term)

- **UX**: Docker-first is the cost-conscious operator's path — `leah
  connect regatta` with no flags pulls + starts a container in seconds and
  costs $0. Cloud requires an explicit `--cloud` flag and a separate
  attestation so the cost implication is impossible to miss.
- **Performance**: loopback HTTP adds ~0.2ms per call vs. an embedded
  library. Accepted — a self-hosted regatta on the same Mac is far below
  any operator-visible threshold, and the process isolation prevents one
  side from OOM-killing the other.
- **Long-term**: cloud-mode wiring is fully spec'd (Section 6) so flipping
  it on in W43 is a transport implementation and a flag, not a redesign.
  Embedded library mode is intentionally NOT scaffolded.

## 12. Out of scope (MVP)

- Cloud transport implementation (W43, deferred).
- Multi-container regatta clusters.
- TLS on the loopback HTTP — accepted for Docker mode since the bind is
  127.0.0.1. Cloud mode is HTTPS by construction.
- Auto-update of the pinned image digest. Operator runs `leah upgrade
  regatta` (separate command, separate spec) when ready.
- Cross-Mac sync of regatta state (the whole point of deferring cloud).
- Regatta-side authentication / multi-tenant setup — out-of-process
  concern.
- Embedded / in-process regatta — see Section 3 row D.
