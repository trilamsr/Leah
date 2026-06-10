---
slug: regatta-integration-plan
status: draft
phase: self-host
owner: leah
---

# Leah ↔ Regatta integration delivery plan (Waves 38-42)

Companion to `docs/engineer/specs/2026-06-10-regatta-integration.md`.
Five waves, each ≤500 LOC including tests, each independently revertable.
Cloud-only happy path lands first so we can stop blocking on the
sibling-binary `ShellExec` early; local + auto-detect + attestation
gating + integration tests follow once the shape is proven.

## Sequencing principle

The `Endpoint` interface is the load-bearing seam — until it exists the
other waves cannot land cleanly. So W38 introduces the type
(Config, Detect stub, cloudEndpoint with Status only) and migrates
`Client.List` to forward through it without changing the public API.
Existing callers (`brief`, `dispatcher`, `daemonloop`, `web`) are
untouched in W38 — they keep using `Client.List`. Local transport (W39)
and `Ship/Review` RPCs come once cloud is real, so each wave has a
working comparison point.

## Wave 38 — Endpoint interface + cloud Status RPC

**Goal**: Land the `Endpoint` interface, `Config`, sentinel errors, and a
working `cloudEndpoint.Status` over httptest. `Client.List` keeps its
current `ShellExec` path; the interface exists but only `Status` flows
through it.

**Files touched**:

- `internal/regattaclient/regattaclient.go` (extend: add types,
  sentinels; keep `Client` + `Agent` + `List` unchanged)
- `internal/regattaclient/cloud.go` (new, ~140 LOC)
- `internal/regattaclient/cloud_test.go` (new, ~160 LOC)
- `docs/engineer/specs/2026-06-10-regatta-integration.md` (status →
  `in-progress`)

**Test plan** (failing-first, capture red output in PR body):

- `TestNew_RejectsMissingAttestor`
- `TestCloud_Status_HappyPath`
- `TestCloud_Status_Unreachable`
- `TestCloud_Auth401`

**Risk**: Low. Pure additive — no existing caller changes.

**Size**: M (~300 LOC).

**Unblocks**: W39 (local transport reuses the `Endpoint` shape),
W41 (Ship/Review RPCs hang off the same interface).

## Wave 39 — Local-socket transport + Detect()

**Goal**: Implement `localEndpoint` over `net.UnixConn` and the `Detect`
function with all four topology cases from §6 of the spec. No CLI yet;
no boot-path migration yet.

**Files touched**:

- `internal/regattaclient/local.go` (new, ~120 LOC)
- `internal/regattaclient/local_test.go` (new, ~180 LOC)
- `internal/regattaclient/detect.go` (new, ~80 LOC)
- `internal/regattaclient/detect_test.go` (new, ~140 LOC)

**Test plan**:

- `TestLocal_Status_HappyPath` via in-process `net.Pipe` dialer
- `TestLocal_RejectsForeignUidSocket` (Statter test seam)
- `TestDetect_TableDriven` — all 4 cases
- `TestDetect_EnvOverridesEverything`

**Risk**: Medium. Unix-socket permissions vary by host; the Statter seam
keeps tests hermetic.

**Size**: M (~450 LOC).

**Unblocks**: W40 (`connect regatta` probe needs `Detect`), W42 (boot
path consumes `Detect`).

## Wave 40 — `leah connect regatta` subcommand

**Goal**: Extend `connect.DefaultRegistry` with a Regatta provider and
land the `cmd/leah/connect.go` flow for topology selection, probe, and
mode-file writing. Local + cloud branches both work end-to-end.

**Files touched**:

- `internal/connect/regatta.go` (new, ~140 LOC) — implements
  `connect.Provider` for cloud, plus a sibling local-only path; reuses
  `runDeviceCodeFlow` for the OAuth sub-branch
- `internal/connect/regatta_test.go` (new, ~200 LOC)
- `internal/connect/registry.go` (1-line edit: add `NewRegatta(...)` to
  `DefaultRegistry`)
- `cmd/leah/connect.go` (extend `summarizeConnectErr` for regatta
  sentinels)
- `cmd/leah/connect_test.go` (extend table)

**Test plan**:

- `TestConnectRegatta_CloudHappyPath` (httptest + temp `LEAH_STATE_DIR`)
- `TestConnectRegatta_LocalHappyPath` (net.Pipe + temp socket)
- `TestConnectRegatta_AttestDenied_NoFilesWritten`
- `TestConnectRegatta_UnreachableProbe_NoFilesWritten`
- `TestConnectRegatta_LocalSocketMissing_PrintsBrewHint`

**Risk**: Medium. Registry edit ripples to `connect --list` output —
golden tests need an update; trivial.

**Size**: M (~450 LOC).

**Unblocks**: W41 (Ship/Review need a real token on disk for tests),
W42 (boot path needs the mode file).

## Wave 41 — Ship + Review RPCs with attestation gating

**Goal**: Land `Endpoint.Ship` and `Endpoint.Review` on both cloud and
local. Each call gates through `Attestor.Attest(ctx, "regatta:<op>")`
before any I/O. Audit row per call.

**Files touched**:

- `internal/regattaclient/cloud.go` (extend: Ship, Review)
- `internal/regattaclient/local.go` (extend: Ship, Review)
- `internal/regattaclient/audit.go` (new, ~40 LOC) — single helper that
  appends `Kind: "regatta_<op>"` rows
- `internal/regattaclient/cloud_test.go`,
  `internal/regattaclient/local_test.go` (extend)

**Test plan**:

- `TestCloud_Ship_HappyPath`, `TestCloud_Ship_AttestDenied`
- `TestCloud_Review_HappyPath`
- `TestLocal_Ship_HappyPath`, `TestLocal_Ship_AttestDenied`
- `TestAuditRow_OnEveryCall` — payload bytes never on disk

**Risk**: Medium. Audit rows are read by `patterns.Detect`,
`selflearn.Resolver`, etc. — `Kind` strings must match the spec's
`regatta_<op>` shape exactly. Cross-checked by a golden test in this
wave.

**Size**: M (~400 LOC).

**Unblocks**: W42 (the dispatcher migration needs Ship/Review to be
real), and the future dispatcher consolidation.

## Wave 42 — Boot-path auto-detect + env override + integration suite

**Goal**: `cmd/leah-daemon/main.go` switches to `regattaclient.New(
regattaclient.Detect())` and surfaces `ErrTopologyMissing` with a hint
pointing at `leah connect regatta`. End-to-end integration tests cover
the four topology cases.

**Files touched**:

- `cmd/leah-daemon/main.go` (~20 LOC delta)
- `cmd/leah/main.go` (~10 LOC delta — `regattaclient.New()` becomes
  `regattaclient.New(regattaclient.Detect())`)
- `cmd/leah-daemon/integration_test.go` (new, ~250 LOC) — four-case
  table covering missing transport, ambiguous, cloud-only, local-only

**Test plan**:

- Daemon refuses to boot when neither transport is configured (clean
  error, exit 2, hint in stderr).
- Daemon prefers local when both are configured + env unset → returns
  `ErrTopologyAmbiguous` and refuses to boot (matches spec §6).
- `LEAH_REGATTA_MODE=cloud` env overrides a local socket on disk.
- Cloud-only flow boots and serves one `List` call end-to-end.

**Risk**: Higher — touches daemon boot path. Mitigated by keeping
behavior identical (`Detect()` returns the same `ModeCloud` config the
zero-arg `New()` implicitly used before, when `regatta` was the only
binary). A feature flag `LEAH_REGATTA_DETECT=1` gates the new path for
one release if reviewer requests it.

**Size**: L (~500 LOC including tests).

**Unblocks**: The future dispatcher migration off `ShellExec`, which is
**not** part of this plan and lands in a follow-up batch once W38-W42
have settled.

## Cross-wave constraints

- Each PR is failing-test-first per CLAUDE.md; captured red output goes
  in the PR body.
- Each PR ships an independent reviewer subagent immediately after
  `gh pr create`; no self-APPROVE.
- No `go.mod` edits across W38-W42 — all transports use the stdlib
  (`net/http`, `net`, `net/http/httptest`, `net.Pipe`).
- No AI signatures, no automerge.
- Deletion-default: W38 adds ~300 LOC; W41 sets up the deletion target
  (the post-W42 follow-up that removes `ShellExec` from
  `dispatcher/ship.go`).

## What this plan does NOT do

- Does not migrate `dispatcher/ship.go`, `brief.Gather`, or `daemonloop`
  off `Client.List` — that consolidation belongs in a follow-up batch
  once `Endpoint` is battle-tested.
- Does not implement OAuth token refresh — operator re-runs
  `leah connect regatta` on expiry; deferred per spec §10.
- Does not pin a default cloud host — the cloud URL is operator-supplied
  until a hosted Regatta endpoint exists.
