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

### 2.1 Bind invariant

Default and only v1 bind: `127.0.0.1:9876`. The listener refuses to bind on
any other interface; `--listen 0.0.0.0` flag is **absent** in v1 (not just
gated behind a warning — the code path does not exist). Loopback-only is
enforced at the `net.Listen` call site with a startup check:

```go
if !host.IsLoopback() { return ErrNonLoopbackBindRefused }
```

Audit row `mcp_server_start` records the bound address; selflearn flags any
non-loopback string as a CVE-class regression.

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
2. HUD popup (or, if HUD daemon not running, TTY prompt on the operator's
   active terminal) displays: peer identity, requested tool, args summary,
   one question drawn from `prompts/self-build-attestations.txt`
   (`attestation.Pool.Pick("self-build")`).
3. Operator answers (Touch ID for HUD; typed `Attestation: <answer>` for
   TTY). The voice surface is **out of scope for v1** — explicitly excluded
   so an off-mic hot-mic attack can't impersonate the operator.
4. Answer accepted → task dequeued + executed. Rejected/timeout (default
   60s) → task discarded, `mcp_attestation_denied` audit row, peer gets
   `403 attestation_failed`.

Pool reuse: the same `internal/attestation.Pool` that gates GH SelfBuild
PR-merge attestations is reused, scoped `"self-build-a2a"`. Adding the scope
is a one-line registry change, not a new module.

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
- Source: `$MEMORY_DIR/<name>.md`. Symlink + path-traversal hardening:
  `filepath.Clean(name)` MUST be a leaf basename (no `/`, no `..`) — fail
  with `400 invalid_name` otherwise. Closes the obvious LFI hole.
- Not found → `404 unknown_rule`.

### 3.2 `leah_search_audit(query, since)`

- Input: `{ query: string, since: rfc3339 }`. `query` is FTS-style; matches
  against `Kind`, `Outcome`, `Detail`, `Workspace` substring (case-fold).
- Output: `{ rows: [Entry...], truncated: bool }`. Hard cap 200 rows; older
  matches dropped first; `truncated=true` signals more available.
- Source: `audit.jsonl` reader reused from `internal/audit/`. **Privacy
  hook**: each row passes through the existing redaction lints
  (`internal/redact.Apply`) before egress; any row that fails redaction
  (i.e. still matches a PII pattern) is dropped + `mcp_redact_drop` audit
  row recorded with the offending row's args_hash.

### 3.3 `leah_dispatch_status()`

- Input: `{}`.
- Output: `{ in_flight: [{kind, args_hash, started_ts, br}], recent_completed: [...] }`.
- Source: tail of `audit.jsonl` filtered to `kind ∈ {"ship","self-build","ask"}`
  joined against outcome rows (existing `dispatcher.Status` already does
  this — wrap it).

### 3.4 `leah_self_build_status()`

- Input: `{}`.
- Output: `{ queue: [...], in_flight: [...], dangling: [...] }`.
  `dangling` = `selflearn.DanglingSelfBuild` (dispatched without outcome >
  N days).
- Source: `internal/selflearn/dangling_selfbuild.go` reused.

### 3.5 What is NOT exposed

- No `leah_recommend_*` (operator-personal predictions leak preference
  surface to peer agents).
- No `leah_knowledge_query` (knowledge graph contains 3rd-party PII).
- No write tools at the MCP layer in v1; SelfBuild lives behind A2A
  (§4–§5) so the attestation gate is unambiguous.

## 4. A2A 1.0 agent-card

Served at `127.0.0.1:9876/.well-known/agent.json` per A2A 1.0 §3.2. Schema
(stable on-disk, generated at first server start, operator-editable):

```json
{
  "schema_version": "a2a/1.0",
  "name": "leah",
  "description": "Operator's local agent. Audit, memory, SelfBuild.",
  "endpoint": "http://127.0.0.1:9876/a2a",
  "auth_scheme": "bearer+operator_attestation",
  "skills": [
    { "id": "self_build", "auth_scope": "self-build-a2a",
      "blast_radius": 4, "requires_attestation": true,
      "input_schema": { "intent": "string" } }
  ],
  "audit_trail": "127.0.0.1:9876/.well-known/audit.jsonl (bearer-gated)"
}
```

The card is the **only** advertised skill surface — read-only MCP tools are
NOT listed (peers discover them via MCP `tools/list`). A peer that submits a
task delegation without `auth_scheme=bearer+operator_attestation` is rejected
at the protocol layer; the gate is structural, not policy.

Operator overrides: `agent_card.json` lives at `$LEAH_STATE_DIR/agent_card.json`.
Operator may edit `description` / `name`. The `skills[*].requires_attestation`
and `auth_scheme` fields are regenerated from code on every server start —
operator-edits to those fields are overwritten + `mcp_card_reset` audit row
appended. Closes the foot-gun where operator-edit silently downgrades the
gate.

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
                                      HUD popup / TTY: "peer X wants
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

`mcp_call` BR mirrors the tool's intrinsic BR (read = 0, self_build = 4).
Selflearn's existing `(Kind, ArgsHash)` queries scale unchanged.

## 7. Operator overrides

- `LEAH_MCP_SERVER=0` env var → server does not start; CLI `leah mcp serve`
  exits 0 with `disabled by LEAH_MCP_SERVER=0`. Single kill-switch.
- `leah mcp token --print` → reveal current token (TTY confirmation
  required: `[y/N]`).
- `leah mcp rotate-token` → rotate; old token invalidated immediately.
- `leah mcp tail` → `tail -f` the `mcp_call` audit rows.

## 8. Bind invariant (recap, load-bearing)

127.0.0.1 only in v1. The string literal `"127.0.0.1"` is the bind host;
`"0.0.0.0"` and `"::"` appear nowhere in `internal/mcp/`. A startup check
+ unit test pins this:

```go
// TestMCPServer_RefusesNonLoopback ensures a regression here is caught
// before merge, not after a peer scans the LAN.
```

Remote enablement = wave-12+ paired spec, blocked on:
1. Tunnel/relay infra (Tailscale-style identity).
2. Multi-operator auth model.
3. Adversarial threat model for non-loopback peers.

## 9. Wave plan (W137-W140, file-disjoint)

| Wave | Files                                       | Deliverable                                  |
|------|---------------------------------------------|----------------------------------------------|
| W137 | `internal/mcp/server.go` + `_test.go`        | Loopback HTTP listener + bearer auth + token file. |
| W138 | `internal/mcp/tools.go` + `_test.go`         | Read-only tools §3.1–§3.4 wrapped over audit/memory/dispatcher. |
| W139 | `internal/mcp/a2a.go` + `_test.go`           | A2A agent-card + SelfBuild wrapper §4–§5.   |
| W140 | `cmd/leah/mcp.go` + `prompts/` updates       | CLI `leah mcp {serve,token,rotate-token,tail}` + scope registration. |

Each wave is one PR. W137 must merge before W138; W138 and W139 are
file-disjoint after W137 and can parallelize. W140 is single-owner against
`cmd/leah/`.

## 10. Test plan (TDD, per wave)

W137:
- `TestMCPServer_RefusesNonLoopback` (load-bearing — startup check).
- `TestMCPServer_BearerMissing_401`.
- `TestMCPServer_BearerWrong_401_AuditRow`.
- `TestMCPServer_TokenRotate_OldTokenRejected_NewAccepted`.
- `TestMCPServer_KillSwitch_LEAH_MCP_SERVER_0`.

W138:
- `TestGetMemoryRule_PathTraversalRejected`.
- `TestGetMemoryRule_UnknownReturns404`.
- `TestSearchAudit_RedactDropDropsAndAudits`.
- `TestSearchAudit_TruncationFlag`.
- `TestDispatchStatus_InFlightAndCompleted`.
- `TestSelfBuildStatus_DanglingIncluded`.

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
2. **Redact lints** — `internal/redact.Apply` runs over every
   `leah_search_audit` row pre-egress; PII matches drop the row +
   `mcp_redact_drop` audit. Reuses the same lint set as the existing
   `leah audit export` path.
3. **Surface allow-list** — only the 4 tools in §3 are exposed; no
   `tools/list` reflection over memory file paths or env vars.

Recommend / knowledge are explicitly NOT exposed (§3.5).

## 12. Cost

Zero LLM calls in the MCP read path. A2A `self_build` path reuses
`dispatcher.SelfBuild` and its existing budget accounting — no new cost
surface. `budget.DefaultCeiling = 5.0` per-process gate applies unchanged.

## 13. Failure modes

- **Malformed A2A task body** (missing fields, schema violation) →
  protocol-level reject + `mcp_malformed_task` audit row. Never reaches the
  reasoner or budget. Closes the obvious budget-DoS vector.
- **Bearer leak** (operator pastes token in chat) → `leah mcp rotate-token`
  is the documented recovery; old token invalidated immediately.
- **Redact lint false-negative** (PII slips through) → `mcp_redact_drop`
  miss surfaces in weekly retro via existing audit-row corpus query; not a
  silent failure — operator sees the rows that egressed.
- **Operator habituation on attestation** — reused pool already rotates
  questions per-call; A2A path reuses the same retro flag from selflearn
  that catches habituation in the GH SelfBuild path.

## 14. Open questions (resolve before W137)

- **Token discovery for peer agents**: env var `LEAH_MCP_TOKEN` vs file
  read — file read is more auditable (no token in `ps -ef`). Tentative:
  file read only; env var unsupported in v1.
- **A2A version pin**: A2A 1.0 is stable; pin via constant
  `mcp.A2AVersion = "a2a/1.0"`. Bump = wave-spec, not a silent upgrade.
- **HUD vs TTY fallback ordering**: if HUD daemon is running, popup wins;
  otherwise TTY prompt on the operator's `/dev/tty`. No headless path —
  no operator-present = no attestation = no write.
