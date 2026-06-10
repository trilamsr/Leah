# Leah MCP server + A2A publish endpoint

Status: design draft (Wave-8 S11)
Owner: operator (tri@maydow.com)
Scope: single operator, single laptop, 127.0.0.1-only. No remote bind in v1.
Source brief: `docs/engineer/briefs/2026-06-10-wave-8-aiml-upgrade.md` §S11.

Leah today is MCP-client-only and Claude-Code-orchestrator-only. Other agents
(Claude Desktop, a peer assistant, a CI bot) cannot read Leah's audit/memory
state or delegate a SelfBuild task to her. This spec turns Leah into an
addressable MCP server + A2A 1.0 peer — read-only by default, write-capable
only through the existing attestation gate.

## 1. Goal + non-goals

**Goal.** A peer agent on the same machine can:
- query Leah's memory rules, audit trail, dispatcher state, SelfBuild queue
  via MCP read-only tools, AND
- delegate a SelfBuild task via an A2A agent-card, gated by per-call
  operator attestation (Touch ID / TTY confirm).

**Non-goals.**
- Remote (non-loopback) bind. Tunnel/relay infra is a wave-12+ paired spec.
- Multi-operator auth. Single-operator-per-machine matches the rest of Leah.
- Exposing write-capable surfaces beyond SelfBuild (no `leah_dispatch_ship`,
  no `leah_memory_write`). Operator-scoped writes stay CLI-only in v1.
- New LLM calls. MCP tools are CRUD over existing on-disk state; cost = $0.

## 2. S11.0 PREREQ — auth + attestation gate

Exposing SelfBuild over an open protocol without auth = remote feature-request
RCE: any peer agent on loopback could trigger `leah self-build <arbitrary
spec>` against the operator's machine. The gate below is **blocking** — no
S11.1 surface lands until §2 is green in CI.

### 2.0 Threat model

Same-UID is trusted (mirrors `ssh-agent`, `gpg-agent`, Keychain): any
process running as the operator can `cat $LEAH_STATE_DIR/mcp_token`.
Bearer auth gates *peer-agent* delegation, not host compromise.
Mitigations: token rotation (paste-leak recovery), one-shot per-peer
tokens (wave-12+), macOS native-messaging UID-scoped bridge (wave-12+).

### 2.1 Bind invariant

Default and only v1 bind: `127.0.0.1:9876`. The listener refuses to bind on
any other interface; `--listen 0.0.0.0` flag is **absent** in v1 (not just
gated behind a warning — the code path does not exist). Loopback-only is
enforced at the `net.Listen` call site with a startup check:

```go
if !host.IsLoopback() { return ErrNonLoopbackBindRefused }
```

Audit row `mcp_server_start` records the bound address. A unit test
(`TestMCPServer_RefusesNonLoopback`, §10) pins the regression — no selflearn
detector is added, the bind check + test is the gate.

### 2.2 Caller identity

Bearer token, operator-managed:

- Token file: `$LEAH_STATE_DIR/mcp_token` (0600, generated at first server
  start, 32 bytes hex). Peer agents read the file (same UID) or are handed
  the token by the operator via `leah mcp token --print`.
- Rotation: `leah mcp rotate-token` writes a new value + appends a
  `mcp_token_rotated` audit row. Old token rejected immediately (no grace).
- Header: every inbound request MUST carry
  `Authorization: Bearer <token>` (MCP) or the equivalent A2A
  `auth.bearer.token` field. Missing/wrong → 401 + `mcp_auth_fail` audit row.

Rejected alternatives:
- mTLS: cert lifecycle on a single laptop is operator-hostile (renewals,
  trust-store wrangling) for zero added security over a loopback bearer.
- Operator-attestation-per-call for **read** tools: would make routine queries
  ("what's my dispatcher status?") block on Touch ID and habituate the
  operator into rubber-stamping. Read = bearer; write = bearer + attestation.

### 2.3 Per-call attestation (write tools only)

Write-capable tools (currently only SelfBuild delegation via A2A; no
MCP-server write tool exists in v1) require fresh operator attestation per
call:

1. Inbound write call → server queues a pending task, returns
   `202 Accepted` with a `task_id`.
2. **TTY prompt** on the operator's active `/dev/tty` displays: peer
   identity, requested tool, args summary, one question drawn from
   `prompts/self-build-attestations.txt`
   (`attestation.Pool.Pick("self-build-a2a")`). HUD-popup confirm UX does
   not exist in `internal/hud/` today (`config`/`focus`/`state`/`widgets`/
   `recommendations`/`ipc` only) — deferred to W140.5 follow-up. v1 ships
   TTY-only. Voice surface excluded from v1 (off-mic hot-mic attack risk).
3. Operator types `Attestation: <answer>`.
4. Accept → dequeue + execute. Reject/timeout (60s) → discard +
   `mcp_attestation_denied` audit row + `403 attestation_failed`.

Scope `"self-build-a2a"` is distinct from the existing `"self-build"`
(PR-merge gate) so habituation on one cannot satisfy the other and audit
rows stay separable. One-line registry add, no new module.

**Anti-DoS** (60s window invites flood-the-operator):
- Per-peer rate limit: 1 attestation prompt/min per peer-id (bearer hash);
  excess → `429 attestation_rate_limited`.
- Global pending cap: 5 in-flight `self_build` tasks; 6th → `503` +
  `mcp_attestation_queue_full` (cap=global).
- Per-peer pending cap: 2/peer; 3rd → `503` (cap=peer).

### 2.4 Blast-radius floor

Every write-tool call records BR ≥ 3 (matches Ship's BR=3 / SelfBuild's BR=4
in `internal/dispatcher/selfbuild.go`). Read tools record BR=0. Selflearn's
existing BR-aware filters apply unchanged.

## 3. Read-only MCP tool surface

All tools are pure CRUD over existing on-disk state. Zero LLM cost. Schemas
follow MCP 2025-09 tool-spec JSON-Schema.

### 3.1 `leah_get_memory_rule(name)`

- Input: `{ name: string }`.
- Output: `{ name, body_md, sha256, mtime_rfc3339 }`.
- Source: `$LEAH_STATE_DIR/memory/<name>.md`. Symlink + path-traversal hardening:
  `filepath.Clean(name)` MUST be a leaf basename (no `/`, no `..`) — fail
  with `400 invalid_name` otherwise. Closes the obvious LFI hole.
- Not found → `404 unknown_rule`.

### 3.2 `leah_search_audit(query, since)`

- Input: `{ query: string, since: rfc3339 }`. `query` is FTS-style; matches
  against `Kind`, `Outcome`, `Detail`, `Workspace` substring (case-fold).
- Output: `{ rows: [Entry...], truncated: bool }`. Hard cap 200 rows; older
  matches dropped first; `truncated=true` signals more available.
- Source: `audit.jsonl` reader reused from `internal/audit/`. Egress
  redaction lint (see §3.2.1) drops any row whose `Detail`/`Workspace`
  matches a PII pattern + emits `mcp_redact_drop` with the offending row's
  args_hash. No `internal/redact` package exists today — lint set is
  defined inline below and lives in `internal/mcp/redact.go`.

### 3.2.1 Redaction lint set (inline, v1)

Run on `Entry.Detail` and `Entry.Workspace` only (`Kind`/`Outcome` are
enum-shape). Drop-the-row, not redact-in-place ("redacted but shape
visible" still leaks). Seven patterns, all case-insensitive:

| Lint name        | Pattern                                                  |
|------------------|----------------------------------------------------------|
| `email`          | `[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`                  |
| `bearer_token`   | `(bearer|token|api[_-]?key|secret)\s*[=:]\s*[a-z0-9_\-]{16,}` |
| `ssh_private_key`| `-----BEGIN [A-Z ]*PRIVATE KEY-----`                     |
| `home_path`      | `/Users/[a-z0-9._-]+/` (and Linux `/home/<u>/`)          |
| `phone_us`       | `\b(\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b`  |
| `cc_number`      | `\b(?:\d[ -]?){13,19}\b` + Luhn check                    |
| `aws_access_key` | `AKIA[0-9A-Z]{16}`                                       |

~50 LOC in `internal/mcp/redact.go`, one regex slice. Minimal v1 floor;
slips surface in weekly retro via existing audit corpus query and are
added as new lints, not by widening these seven.

### 3.3 `leah_dispatch_status()`

- Input: `{}`.
- Output: `{ in_flight: [{kind, args_hash, started_ts, br}], recent_completed: [...] }`.
- Source: tail of `audit.jsonl` filtered to `kind ∈ {"ship","self-build","ask"}`
  joined against outcome rows (existing `dispatcher.Status` already does
  this — wrap it).

### 3.4 `leah_self_build_status()`

- Input: `{}`.
- Output: `{ queue: [...], in_flight: [...], dangling: [...] }`.
  `dangling` = `selflearn.DetectDanglingSelfBuild(auditPath, time.Now)`
  (`[]DanglingSelfBuild`; dispatched without outcome > 7d).
- Source: `internal/selflearn/dangling_selfbuild.go` reused.

### 3.5 What is NOT exposed

- No `leah_recommend_*` (operator-personal predictions leak preference
  surface to peer agents).
- No `leah_knowledge_query` (knowledge graph contains 3rd-party PII).
- No write tools at the MCP layer in v1; SelfBuild lives behind A2A
  (§4–§5) so the attestation gate is unambiguous.

## 4. A2A 1.0 agent-card

Served at `127.0.0.1:9876/.well-known/agent-card.json` per A2A 1.0 §8.2
(IANA-registered URI). Schema (generated at first start,
operator-editable):

```json
{
  "name": "leah",
  "description": "Operator's local agent. Audit, memory, SelfBuild.",
  "version": "0.1.0",
  "supportedInterfaces": [
    { "url": "http://127.0.0.1:9876/a2a", "protocolBinding": "JSONRPC", "protocolVersion": "1.0" }
  ],
  "capabilities": { "streaming": false, "extendedAgentCard": false },
  "defaultInputModes": ["text/plain"],
  "defaultOutputModes": ["text/plain"],
  "securitySchemes": { "bearer": { "type": "http", "scheme": "bearer" } },
  "securityRequirements": [ { "bearer": [] } ],
  "skills": [
    { "id": "self_build", "name": "Self-build",
      "description": "Delegate spec → PR. Per-call operator attestation.",
      "tags": ["write", "operator-attested"],
      "x-leah": { "auth_scope": "self-build-a2a", "blast_radius": 4,
                  "requires_attestation": true } }
  ]
}
```

A2A 1.0 has no first-class `requires_attestation` field; Leah gate
metadata lives under the `x-leah` extension namespace (A2A §10).

The card is the **only** advertised skill surface — read-only MCP tools are
NOT listed (peers discover them via MCP `tools/list`). A peer that submits a
task delegation for a skill flagged `x-leah.requires_attestation=true`
without a fresh attestation handshake is rejected at the protocol layer; the
gate is structural, not policy.

Operator overrides: `agent_card.json` lives at `$LEAH_STATE_DIR/agent_card.json`.
Operator may edit `description` / `name` / `version`. The
`skills[*].x-leah.*` and `securitySchemes` fields are regenerated from code
on every server start — operator-edits to those fields are overwritten +
`mcp_card_reset` audit row appended. Closes the foot-gun where operator-edit
silently downgrades the gate.

## 5. SelfBuild over A2A

End-to-end flow for an inbound `self_build` task:

```
peer ──POST /a2a/tasks {skill:"self_build", intent:"..."}──> Leah
                                                              │
                                                  bearer check│  fail → 401 + audit
                                                              ▼
                                              queue pending task (task_id)
                                                              │
                                                  202 Accepted│
                                                              ▼
                                      TTY: "peer X wants
                                      self_build('...'); answer: <Q>"
                                                              │
                                            timeout 60s / deny│ → 403 + audit
                                                              ▼
                                      attestation OK → reasoner.Ask(spec
                                      system prompt + intent) → emits
                                      `## leah-selfbuild-spec-v1` or
                                      clarify-questions
                                                              │
                                            clarify → return clarify text
                                            spec    → run existing
                                                      dispatcher.SelfBuild
                                                              ▼
                                                  audit row + task result
                                                  posted to peer via A2A
                                                  task-result callback
```

Key reuses (zero net-new business logic):
- `dispatcher.SelfBuild.Run` — unchanged. A2A wrapper marshals `intent` in,
  marshals `LastURL` / err out.
- `attestation.Pool.Pick("self-build-a2a")` — new scope, same pool file.
- `audit.Logger.Append` — write `mcp_call` row at every entry/exit.

Failure semantics:
- Reasoner clarify → A2A `task.status = "clarify-required"`, body =
  clarify text. No PR filed (matches CLI behavior, `ErrSelfBuildClarify`).
- Reasoner error / budget exceeded → `task.status = "failed"`,
  `mcp_call` audit row with `outcome=failed`.
- Operator-deny → `task.status = "denied"`, `outcome=denied`.

## 6. Audit row schema

New `Kind` values appended to `audit.jsonl` (additive — backward
compatible with `audit.Entry`'s existing JSON tags):

| Kind                    | BR | When                                    | Detail                       |
|-------------------------|----|-----------------------------------------|------------------------------|
| `mcp_server_start`      | 0  | listener bound                           | `addr=127.0.0.1:9876`       |
| `mcp_token_rotated`     | 0  | `leah mcp rotate-token`                  | `old_sha=<8>`               |
| `mcp_call`              | 0+ | every inbound tool/task call (per-call) | `peer=<ua> tool=<name> attest=<bool>` |
| `mcp_auth_fail`         | 0  | bearer mismatch                          | `peer=<ua>`                  |
| `mcp_attestation_denied`| 3  | operator denied / 60s timeout            | `peer=<ua> task=<id>`       |
| `mcp_redact_drop`       | 0  | row dropped by redact lint pre-egress    | `args_hash=<8>`             |
| `mcp_card_reset`        | 0  | operator-edited gate field overwritten   | `field=<name>`              |
| `mcp_malformed_task`    | 0  | A2A body unparseable / schema-violating  | `peer=<ua> err=<short>`     |
| `mcp_attestation_rate_limited` | 0  | per-peer 1/min rate limit hit            | `peer=<ua>`                  |
| `mcp_attestation_queue_full`   | 0  | global 5-pending or per-peer 2-pending cap hit | `peer=<ua> cap=<global\|peer>` |

`mcp_call` BR mirrors the tool's intrinsic BR (read = 0, self_build = 4).
Selflearn's existing `(Kind, ArgsHash)` queries scale unchanged.

## 7. Operator overrides

- `LEAH_MCP_SERVER=0` env var → server does not start; CLI `leah mcp serve`
  exits 0 with `disabled by LEAH_MCP_SERVER=0`. Single kill-switch.
- `leah mcp token --print` → reveal current token (TTY confirmation
  required: `[y/N]`).
- `leah mcp rotate-token` → rotate; old token invalidated immediately.
- `leah mcp tail` → `tail -f` the `mcp_call` audit rows.

## 8. Remote enablement (deferred)

Wave-12+ paired spec, blocked on: tunnel/relay identity (Tailscale-style),
multi-operator auth model, non-loopback adversarial threat model.

## 9. Wave plan (W137-W140, file-disjoint)

| Wave | Files                                       | Deliverable                                  |
|------|---------------------------------------------|----------------------------------------------|
| W137 | `internal/mcp/server.go` + `_test.go`        | Loopback HTTP listener + bearer auth + token file + per-peer rate-limit + pending-task caps (§2.3). |
| W138 | `internal/mcp/tools.go`, `internal/mcp/redact.go` + `_test.go` | Read-only tools §3.1–§3.4 + inline redact lint set §3.2.1. |
| W139 | `internal/mcp/a2a.go` + `_test.go`           | A2A agent-card (§4) + SelfBuild wrapper §5 (TTY-only attestation). |
| W140 | `cmd/leah/mcp.go` + `prompts/` updates       | CLI `leah mcp {serve,token,rotate-token,tail}` + scope registration. |
| W140.5 (follow-up) | `internal/hud/popup.go` + IPC + listener wiring | HUD popup-confirm path (replaces TTY default once shipped). |

Each wave is one PR. W137 must merge before W138; W138 and W139 are
file-disjoint after W137 and can parallelize. W140 is single-owner against
`cmd/leah/`. W140.5 is non-blocking — TTY attestation is the v1 shipped
default.

## 10. Test plan (TDD, per wave)

W137:
- `TestMCPServer_RefusesNonLoopback` (load-bearing — startup check).
- `TestMCPServer_BearerMissing_401`.
- `TestMCPServer_BearerWrong_401_AuditRow`.
- `TestMCPServer_TokenRotate_OldTokenRejected_NewAccepted`.
- `TestMCPServer_KillSwitch_LEAH_MCP_SERVER_0`.
- `TestMCPServer_PerPeerRateLimit_429`.
- `TestMCPServer_GlobalPendingCap_503`.
- `TestMCPServer_PerPeerPendingCap_503`.

W138:
- `TestGetMemoryRule_PathTraversalRejected`.
- `TestGetMemoryRule_UnknownReturns404`.
- `TestSearchAudit_RedactDropDropsAndAudits`.
- `TestSearchAudit_TruncationFlag`.
- `TestDispatchStatus_InFlightAndCompleted`.
- `TestSelfBuildStatus_DanglingIncluded`.
- `TestRedact_EachLintDropsAndAudits` (all §3.2.1 lints).

W139:
- `TestAgentCard_GateFieldsOverwriteOperatorEdit`.
- `TestA2A_TaskWithoutAttestation_403`.
- `TestA2A_OperatorDeny_403_AndAudit`.
- `TestA2A_AttestationOK_DispatchesSelfBuild`.
- `TestA2A_ReasonerClarify_Returns_ClarifyRequired_NoPR`.
- `TestA2A_MalformedTask_RejectedAndAudited`.

W140:
- `TestCLI_MCPServe_ChecksLoopbackOnly`.
- `TestCLI_MCPRotateToken_AppendsAudit`.
- `TestCLI_MCPTokenPrint_RequiresTTYConfirm`.

Failing test FIRST per CLAUDE.md TDD discipline; failing output captured in
each PR body.

## 11. Privacy

MCP responses leave Leah's process. Three layers protect operator data:

1. **Bind invariant** — loopback-only; no LAN/WAN egress in v1.
2. **Redact lints** — the inline §3.2.1 lint set (lives in
   `internal/mcp/redact.go`) runs over every `leah_search_audit` row
   pre-egress; PII matches drop the row + `mcp_redact_drop` audit. No
   `internal/redact` package and no `leah audit export` command exist
   today — both were spec-author hallucinations, struck.
3. **Surface allow-list** — only the 4 tools in §3 are exposed; no
   `tools/list` reflection over memory file paths or env vars.

Recommend / knowledge are explicitly NOT exposed (§3.5).

## 12. Cost

Zero LLM calls in the MCP read path. A2A `self_build` path reuses
`dispatcher.SelfBuild` and its existing budget accounting — no new cost
surface. `budget.DefaultCeiling = 5.0` per-process gate applies unchanged.

## 13. Failure modes

- Malformed A2A body → `mcp_malformed_task` reject (never reaches
  reasoner/budget). Closes budget-DoS vector.
- Bearer leak (operator pastes token) → `leah mcp rotate-token`, old token
  invalidated immediately.
- Redact lint false-negative → `mcp_redact_drop` miss surfaces in weekly
  retro; operator sees egressed rows (not silent).
- Attestation habituation → reused pool rotates questions per-call;
  selflearn habituation flag from GH SelfBuild path covers A2A too.

## 14. Open questions (resolve before W137)

- Token discovery: file read only (env var `LEAH_MCP_TOKEN` leaks via
  `ps -ef`). Tentative — confirm in W137.
- A2A version pin: const `mcp.A2AProtocolVersion = "1.0"`; bump =
  wave-spec, not silent upgrade.
