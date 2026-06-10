---
slug: regatta-integration-plan
status: draft
phase: self-host
owner: leah
covers: W38–W43
---

# Regatta integration delivery plan (W38–W43) — Docker-first

Pairs with `docs/engineer/specs/2026-06-10-regatta-integration.md`.

Each wave is sized to a single PR (~500 LOC budget) with a green test plan
and a concrete `unblocks-what`. Primary topology is local-Docker; cloud is
deliberately at the **end** of the wave list and gated on explicit operator
opt-in.

Trade-offs surface at the head of each wave (priority per `CLAUDE.md`:
UX > performance > long-term). Default simpler. Three similar lines beat a
premature abstraction.

---

## W38 — `internal/regattaclient/` skeleton + Endpoint interface + Docker transport scaffolding

**Goal.** Land the package skeleton: `Endpoint` interface, `Status` value,
sentinel errors, `Config`, and the `dockerTransport` struct (HTTP-over-
loopback). No `connect` CLI yet — this wave is the seam every later wave
plugs into.

**Trade-off.** A single `Endpoint` interface with one transport
implementation looks like premature abstraction. It is not — W39's connect,
W40's auto-detect, and W43's cloud branch all need the same seam, and
landing it once now avoids a refactor in W43.

**Files touched (≤500 LOC).**
- `internal/regattaclient/doc.go` — package overview citing the spec.
- `internal/regattaclient/endpoint.go` — `Endpoint`, `Status`, sentinels.
- `internal/regattaclient/config.go` — `Config`, `OSExec`, `Attestor`.
- `internal/regattaclient/image.go` — pinned image digest constant +
  digest-format regex test.
- `internal/regattaclient/transport_docker.go` — HTTP client targeting
  `127.0.0.1:9090`, `Status()` implementation.
- `internal/regattaclient/transport_docker_test.go` — `httptest.Server`
  spins up locally; assert `Status()` round-trip.
- `internal/regattaclient/forbidden_grep_test.go` — package-level grep
  guard against the `scripts/forbidden-grep.sh` pattern.

**Risk.** Premature interface lock-in. Mitigation: `Endpoint` exposes only
`Status` at W38; later waves add methods as their owning RPCs land.

**Size.** ~350 LOC including tests.

**Test plan.** Hermetic. `httptest.Server` stands in for the regatta
container. `make check` green.

**Unblocks.** W39 has an `Endpoint` to wire `connect` into.

---

## W39 — `leah connect regatta` Docker branch (primary path)

**Goal.** Operator-facing `leah connect regatta` runs the eight-step Docker
flow (spec §5): attest → `docker info` → `docker pull @sha256:...` →
`docker run` (loopback bind) → health probe → mode-file write → status
sanity → audit row.

**Trade-off.** We drive the Docker daemon by shelling out to the `docker`
CLI rather than linking the Engine SDK. Three similar lines beat a
premature abstraction; the CLI surface is stable and the SDK would add a
multi-MB dep for ~6 commands.

**Files touched.**
- `internal/regattaclient/connect_docker.go` — the eight-step flow.
- `internal/regattaclient/connect_docker_test.go` — fake `OSExec` records
  argv; assert ordering, digest pin, loopback bind, mode-file `0600`,
  teardown-on-failure.
- `cmd/leah/connect_regatta.go` — CLI subcommand; default branch is
  `--docker` (implicit when `docker` is available).
- `internal/audit/kinds.go` — register `connect_regatta_docker` kind.

**Risk.** Partial-start leaks. Mitigation: explicit `defer` teardown if
any post-`docker run` step fails; test
`TestConnect_Docker_HealthTimeout_TeardownRuns` asserts the cleanup argv
runs.

**Size.** ~450 LOC including tests.

**Test plan.** Hermetic per spec §10: fake `OSExec`, fake `Attestor`,
`httptest.Server` for the health probe. No real `docker` invocation in
CI. `make check` green.

**Unblocks.** W40 has a real `regatta-mode.json` writer to detect against.

---

## W40 — Auto-detect at daemon boot

**Goal.** Daemon resolves the active regatta mode at boot per spec §7
priority (env override → docker → socket → cloud → not-connected). Emits
the one-shot `regatta_mode_cloud_active` warning audit row when cloud mode
is active.

**Trade-off.** Auto-detect is run-once at boot, not per-call. A
hot-swap (operator runs `leah connect regatta --cloud` while the daemon is
running) requires a daemon restart. Accepted — operators connect once and
the cost of a restart is a few seconds.

**Files touched.**
- `internal/regattaclient/detect.go` — five-rule resolver.
- `internal/regattaclient/detect_test.go` — table-driven across all five
  branches; asserts `LEAH_REGATTA_MODE=off` returns `nil, nil` (caller
  treats as not-connected).
- `cmd/leahd/build_regatta.go` — wiring at daemon boot.
- `cmd/leahd/run.go` — register the resolved Endpoint into the daemon
  graph (gracefully no-op if not-connected).

**Risk.** A misconfigured `regatta-mode.json` panics the daemon at boot.
Mitigation: detect.go returns `ErrNotConnected` on any parse failure and
logs an audit row; the daemon continues without a regatta Endpoint.

**Size.** ~300 LOC.

**Test plan.** Table-driven. Hermetic `httptest.Server` simulates a
healthy / unhealthy regatta. `make check` green.

**Unblocks.** W41 has a resolved Endpoint to gate RPCs through.

---

## W41 — Attestation gating + per-RPC audit rows

**Goal.** Every regatta RPC (currently just `Status`; later waves add
write paths) runs `Attestor.Attest(ctx, scope)` before transport. Audit
row is emitted for both denied and succeeded calls.

**Trade-off.** Attestation per-RPC vs. per-session. Per-RPC is more
prompts but matches the gmail / imessage pattern (PR #67) — operators
build muscle memory once. Per-session would shave prompts but creates a
"forgotten consent window" foot-gun.

**Files touched.**
- `internal/regattaclient/gate.go` — `gateAndCall` helper mirroring
  gmail's `gateAndToken` and imessage's `gateAndExec`.
- `internal/regattaclient/gate_test.go` — denied-before-transport,
  succeeded-emits-audit, recipient-validation-before-attestation patterns.
- `internal/regattaclient/transport_docker.go` — `Status` now routes
  through `gateAndCall`.
- `internal/audit/kinds.go` — register `regatta_status_call` kind.

**Risk.** Audit-row spam if `Status` is polled. Mitigation: `Status` uses
a coarse-grained audit row (one per minute window) — the regulatory
question is "was this surface exercised", not "how often".

**Size.** ~250 LOC.

**Test plan.** Hermetic. Fake `Attestor` toggles deny/allow; asserts no
transport call on deny and exactly one audit row per call. `make check`
green.

**Unblocks.** W42 disconnect can rely on the same gate.

---

## W42 — `leah disconnect regatta` Docker teardown

**Goal.** Implement spec §9: attest → resolve mode → `docker stop && rm`
→ optional `--purge-data` behind a second attestation → delete
`regatta-mode.json` → audit row. Mirrors PR #66.

**Trade-off.** Default keeps `regatta-data/` on disk so reconnect is
cheap. Per spec, purge is opt-in AND requires a fresh attestation. UX win
> long-term storage win.

**Files touched.**
- `internal/regattaclient/disconnect.go` — the seven-step flow.
- `internal/regattaclient/disconnect_test.go` — fake OSExec sees
  `stop`+`rm`; `--purge-data` without second attestation fails closed;
  idempotent re-run is OK.
- `cmd/leah/disconnect_regatta.go` — CLI subcommand.
- `internal/audit/kinds.go` — register `disconnect_regatta` kind.

**Risk.** Operator double-clicks disconnect and second run errors. Test
`TestDisconnect_Idempotent` asserts a missing container is not an error.

**Size.** ~300 LOC.

**Test plan.** Hermetic. `make check` green.

**Unblocks.** Clean handoff to W43; primary-path topology is complete.

---

## W43 — Cloud branch (DEFERRED — opt-in only)

**Goal.** Implement spec §6: `leah connect regatta --cloud`,
`cloudTransport`, token-file separation from the mode file, one-shot
`regatta_mode_cloud_active` warning at daemon boot.

**Gating.** This wave does **not** auto-schedule. It lands only after
explicit operator request that cost is no longer a concern. The CLI flag
existed in W39's parser to surface a "deferred — see W43" error; this
wave wires the actual transport.

**Trade-off.** Shipping cloud before operator demand creates an
unmonitored payment surface. UX > performance > long-term: a deferred
flag with a clear error message is the right default until the operator
asks for it.

**Files touched (sketch — final scope decided at activation).**
- `internal/regattaclient/transport_cloud.go` — HTTPS + bearer.
- `internal/regattaclient/connect_cloud.go` — five-step flow per spec §6.
- `internal/regattaclient/transport_cloud_test.go` — `httptest.Server`
  with TLS; assert bearer header redacted from audit rows.
- `cmd/leah/connect_regatta.go` — replace the "deferred" error with the
  real wiring.

**Risk.** Token leak into logs. Mitigation: `cloudTransport` wraps the
HTTP client with a request-scrubber that redacts `Authorization` headers
before any log line.

**Size.** ~400 LOC at activation.

**Test plan.** Hermetic via TLS `httptest.Server`. `make check` green.

**Unblocks.** Cross-machine regatta sync becomes a wiring problem, not a
redesign.

---

## Cross-wave conventions

- **No AI signatures, no automerge, no self-APPROVE** (per `CLAUDE.md`).
- Every PR runs the forbidden-grep test
  (pattern in `scripts/forbidden-grep.sh` — 0 hits).
- Every PR answers "what got smaller?" — for this stream the dominant
  win is **zero cloud spend on the primary path**.
- Every PR ships its failing test FIRST in the body, then impl, then
  green output.
