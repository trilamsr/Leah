# Wave-8 S10 — Operator-trust moat artifacts

Date: 2026-06-10
Status: design
Brief: `docs/engineer/briefs/2026-06-10-wave-8-aiml-upgrade.md` §S10
Decision priority (wave-8 override): long-term-benefit > root-cause > performance.

## 1. Goal

Seven trust-moat artifacts that make Leah's data handling auditable, portable, revocable, and reproducible. One spec; impl waves split file-disjoint (W130–W136). Each moat is independently kill-switchable and adds zero new metrics — `audit.jsonl` is the single observability surface.

The moats answer the four operator-trust questions a 2026 assistant must answer in <60 seconds without reading source: (a) what do you know about me? (b) how do I take it back? (c) can I prove you didn't phone home? (d) can I prove this binary is the one you published?

## 2. M1 — `leah whoami --full`

### 2.1 CLI surface

`cmd/leah/whoami.go` (new). Default `leah whoami` prints the existing short form (operator email, active workspace, authorized integrations). `--full` walks every persisted store and emits one row per source.

Output format: JSON-lines on stdout (machine-readable; pipe to `jq`). One line per (source, count) tuple plus one tail row per source listing the on-disk path. Schema:

```
{"source":"memory","table":"operator_profile","rows":143,"path":"~/.leah-state/memory.db"}
{"source":"audit","kind":"connect_gmail","rows":4,"path":"~/.leah-state/audit.jsonl"}
{"source":"knowledge","entity_kind":"person","rows":27,"path":"~/.leah-state/knowledge.db"}
{"source":"recommend","table":"recommend_pattern_state","rows":61,"path":"~/.leah-state/memory.db"}
{"source":"mirror","stream":"gmail.messages","rows":12041,"path":"~/.leah-state/mirror/gmail.db"}
{"source":"events","kind":"calendar.event","rows":892,"path":"~/.leah-state/event.db"}
{"source":"oauth","provider":"gmail","authorized":true,"path":"~/.leah-state/connect/gmail.json"}
```

Sources enumerated (closed set; adding a new persistent store requires extending `whoami.Sources` — gate test fails otherwise):

- `memory` — operator_profile, operator_profile_meta tables.
- `recommend` — recommend_pattern_state.
- `knowledge` — `internal/knowledge/` graph entities + edges.
- `mirror` — adapter mirror tables (gmail, gcal, slack, etc.) grouped by stream.
- `events` — event-timeline event store.
- `audit` — `audit.jsonl` grouped by `kind`.
- `oauth` — `internal/connect/` token files per provider.

### 2.2 Tests

`TestWhoamiFull_ListsKnownStores` — seed one row per source in a tempdir state-dir, assert each source appears in JSON output. `TestWhoamiFull_EmptyStateClean` — first-run operator (no state dir) emits zero rows + exit 0. `TestWhoamiFull_StableSchema` — golden-file the field set per source to catch silent schema drift.

### 2.3 Privacy gate

`--full` reveals every persisted row count. Treat as operator-attested: BR=2, kind `whoami_full`, audit row records source list + total row count (not row contents).

## 3. M2 — `leah purge --everything`

### 3.1 CLI surface

`cmd/leah/purge.go` (new). Three-stage destructive flow gated by BR=4 attestation (highest in repo; matches operator-irreversible classification from `feedback_no_pause_on_backlog_drain.md`).

Stage 1 — OAuth revoke at provider. For each authorized provider in `connect.DefaultRegistry().List()` where `Authorized=true`, call provider-side revoke endpoint (Google revoke URL for gmail/gcal; Slack auth.revoke; etc.). Reuses the `connect.Disconnect` path that already deletes the local token file. Failure to reach provider does NOT block local deletion — emit `purge_oauth_remote_revoke` audit row with `outcome=remote_unreachable` and continue.

Stage 2 — Local state delete. `rm -rf ~/.leah-state/` after operator confirmation. Final audit row written to stdout (not to file — the file is about to be deleted): a tar-streamed last-row record gets printed for operator archival.

Stage 3 — Distribution cleanup hints (no auto-execution). Print:

```
Leah binary still on disk. To remove:
  brew uninstall leah        # if installed via brew
  rm $(which leah)            # otherwise
PATH entry in ~/.zshrc / ~/.bashrc may reference leah; review and remove manually.
```

Auto-uninstall is rejected — purge MUST stop short of touching files outside `~/.leah-state/` to remain a single-blast-radius operation. Hints are documentation, not execution.

### 3.2 Attestation gate (BR=4)

Reuse existing `forgetAttest` pattern (operator types phrase, not y/N). Phrase: `purge everything`. Env override `LEAH_PURGE_AUTO_ATTEST=1` for tests only; production attestation cannot be bypassed via flag. Audit row pre-deletion: `kind=purge_everything`, `outcome=attested`, `blast_radius=4`, `detail=<provider list + row count>`.

### 3.3 Tests

`TestPurge_RevokesEveryAuthorizedProvider` (stub provider with revoke recorder). `TestPurge_StateDirDeletedAfterConfirm` (tempdir + auto-attest env). `TestPurge_RemoteRevokeFailureContinues` (5xx from provider → local delete still happens, audit row records remote failure). `TestPurge_RejectsWithoutAttestation` (no env, no stdin → returns exit 1, state dir intact).

## 4. M3 — `leah export --all` / `leah import`

### 4.1 Archive format

TAR (gzip) of `~/.leah-state/` containing: every `.db` file, `audit.jsonl`, every `connect/*.json` OAuth token, memory dir. Encrypted with AES-256-GCM via operator passphrase (PBKDF2-SHA256, 600k iterations — OWASP 2024 baseline). Single file: `leah-export-<unix-ts>.tar.gz.enc`.

Header (cleartext, prepended): JSON `{"version":1,"leah_version":"<git-sha>","created":"<rfc3339>","pbkdf2_iter":600000,"salt":"<base64>"}` followed by `\n--ENCRYPTED--\n` separator. Header is signed (HMAC-SHA256 with key derived from same passphrase) so tamper detection happens before decrypt.

### 4.2 CLI surfaces

`leah export --all --out <path>` — prompts for passphrase twice (confirm), writes archive, audit row `kind=export_all`, BR=3 (high — bundle contains every secret). Default `--out` is `./leah-export-<ts>.tar.gz.enc` (operator-visible, not buried in state dir).

`leah import <path>` — prompts for passphrase, verifies header HMAC, decrypts, refuses if `~/.leah-state/` already exists unless `--overwrite` (BR=4 attestation phrase: `overwrite my leah state`). Audit row `kind=import_all`.

### 4.3 Tests

`TestExportImport_RoundTrip` — seed state dir, export, wipe, import, every file byte-identical. `TestImport_WrongPassphrase` — returns `ErrBadPassphrase`, state dir unchanged. `TestImport_TamperedHeader` — flip one byte in cleartext header → HMAC fails before decrypt attempt. `TestExport_HeaderClearReadable` — `head -c 1024 archive.enc | grep version` works (operator can inspect export provenance without decrypting).

## 5. M4 — `LEAH_LOCAL_ONLY=1`

### 5.1 Egress block

`internal/reasoner/anthropic.go` Reasoner path today instantiates `anthropic.NewClient` with default http.Client (default transport, public DNS). Wrap with `reasoner.NewClient` switching layer:

- `LEAH_LOCAL_ONLY=1` → return Ollama-backed reasoner (paired with V10 streaming spec's local fallback wiring).
- Unset → existing `AnthropicClient`.

The switch belongs at the `reasoner.NewClient` factory, not inside `AnthropicClient` — that keeps AnthropicClient's HTTP transport untouched (no per-request env-check hotpath) and makes the contract explicit at construction.

Additionally, wrap the `http.Transport` used by every adapter (gmail, gcal, slack, etc.) with `localOnlyTransport` that returns `ErrEgressBlocked` if `LEAH_LOCAL_ONLY=1` and `req.URL.Host` does not resolve to a loopback address (127.0.0.0/8, ::1, localhost). Belongs in `internal/connect/instrumentation.go` next to the existing instrumentation wrapper.

### 5.2 Audit row "0 egress bytes" verification

Extend `audit.Entry` with one optional field `EgressBytes int64 json:"egress_bytes,omitempty"` (additive — back-compat, matches S2 LLM-dim observability extension already underway). Every reasoner call appends an entry with EgressBytes set. Under `LEAH_LOCAL_ONLY=1`, EgressBytes MUST be 0 for every row in the session — `leah whoami --full` (M1) gets a verification mode `--verify-local-only` that scans audit.jsonl and asserts EgressBytes=0 for every row since LEAH_LOCAL_ONLY was set.

The marker for "LEAH_LOCAL_ONLY active" is itself an audit row `kind=local_only_enter` at daemon start (when env=1) and `kind=local_only_exit` at daemon stop. Verification = "find latest local_only_enter, assert every subsequent row has egress_bytes=0 or missing".

### 5.3 Tests

`TestLocalOnly_BlocksNonLoopback` (http roundtrip to example.com returns ErrEgressBlocked). `TestLocalOnly_AllowsLoopback` (roundtrip to 127.0.0.1:11434 — Ollama default — succeeds). `TestLocalOnly_ReasonerSwitches` (factory returns Ollama-backed when env=1). `TestVerifyLocalOnly_FlagsViolations` (synthetic audit row with egress_bytes>0 after local_only_enter → exit 1).

### 5.4 Dependency on V10

Ollama-backed reasoner is paired with V10 streaming spec — M4 lands behind a feature flag (`LEAH_LOCAL_ONLY=1` errors with "ollama reasoner not yet shipped, blocking on V10") until V10 lands. This is acceptable; the moat is the wiring + audit verification, not the Ollama integration itself.

## 6. M5 — Data-flow SVG

One hand-authored `docs/operator/data-flow.svg`. Shows every persistent store (M1's seven sources), every network egress (Anthropic, OpenAI realtime, every adapter provider, ollama loopback, OAuth provider endpoints), and the LEAH_LOCAL_ONLY cut line. Distinct visual treatment for "always local" vs "egress" arrows.

Acceptance: 1 afternoon of dev cost. Not generated — hand-authored so it stays accurate where auto-generation would silently drift. Re-author cadence: each new adapter spec PR MUST update the SVG (added to spec-PR checklist). CI gate: `make check` greps every adapter package name in `docs/operator/data-flow.svg`; missing name → fail.

## 7. M6 — Reproducible build attestation

### 7.1 GHA workflow

`.github/workflows/release-attest.yml` (new). Trigger: tag push matching `v*`. Steps:

1. Build with `CGO_ENABLED=0 GOFLAGS="-trimpath -mod=readonly" go build -ldflags="-s -w -buildid="` for reproducibility.
2. Run `syft .` → SBOM (SPDX-JSON) attached to release.
3. Generate SLSA L2 provenance via `slsa-framework/slsa-github-generator` (its `generator_generic_slsa3.yml` reusable workflow — currently the canonical Go binary path).
4. Sign with `cosign sign-blob --keyless` (Sigstore OIDC, no key management overhead).
5. Attach `leah-<ver>-<os>-<arch>`, `leah-<ver>.spdx.json`, `leah-<ver>.intoto.jsonl`, `leah-<ver>.sig` to the GitHub release.

### 7.2 Verify-checksums target

`Makefile` adds `verify-checksums` target: downloads release artifacts for `LEAH_VERIFY_VERSION`, runs `cosign verify-blob` + `slsa-verifier verify-artifact`, prints pass/fail. Operator-facing: ship this in the brew formula's post-install hint.

### 7.3 Tests

GHA workflow `act`-runnable smoke test in `scripts/test-release-attest.sh` (CI-skipped — runs only on tag-staging PRs to avoid burning Actions minutes). Repro-build check: build twice in CI matrix, assert byte-identical binaries (`sha256sum`).

## 8. M7 — Per-category memory attestation

### 8.1 Schema extension

`internal/memory/schema.sql` adds `category TEXT NOT NULL DEFAULT 'general'` to operator_profile. Allowed values (closed enum, enforced at write site): `general | relationship | finance | health`. Adding a new category requires schema migration + spec edit — friction is the point.

Three sensitive categories (`relationship`, `finance`, `health`) trigger a first-sight attestation prompt: "Leah is about to start tracking <category> data. Continue? [type 'I consent to <category> tracking']". Subsequent writes in the same category are silent. Operator can revoke per-category via `leah forget --category <name>` (extends existing forget surface).

### 8.2 Detection

The classification of an incoming memory write into a sensitive category is upstream of M7 — `internal/operatormodel/` is responsible for tagging at observation time. M7 ships the schema, the attestation gate, and the `--category` forget surface. Upstream tagging is a separate spec (folds into S3 memory-dispatch injection — the same keyword/embedding work already in flight). M7 is gated behind tagging landing; until then the category column is populated but every row is `general` and the prompt never fires.

### 8.3 Tests

`TestMemory_FirstFinanceWriteAttests` (mock attestor, asserts prompt fires once). `TestMemory_SecondFinanceWriteSilent` (no prompt after first consent). `TestForgetCategory_DeletesOnlyThatCategory` (seed 3 categories, forget one, other two intact).

## 9. Wave plan W130–W136 (file-disjoint)

Each wave is one moat, one PR, one reviewer. File-disjoint by package — parallelizable up to 6 per README.md dispatch rule (M5 SVG is the singleton outlier — docs/ only).

| Wave | Moat | Primary paths |
| --- | --- | --- |
| W130 | M1 whoami | `cmd/leah/whoami.go`, `internal/whoami/` |
| W131 | M2 purge | `cmd/leah/purge.go`, `internal/purge/` |
| W132 | M3 export/import | `cmd/leah/export.go`, `cmd/leah/import.go`, `internal/archive/` |
| W133 | M4 local-only | `internal/reasoner/local_only.go`, `internal/connect/instrumentation.go`, `internal/audit/audit.go` (EgressBytes field) |
| W134 | M5 data-flow SVG | `docs/operator/data-flow.svg`, `Makefile` (grep gate) |
| W135 | M6 reproducible build | `.github/workflows/release-attest.yml`, `Makefile` (verify-checksums) |
| W136 | M7 category attestation | `internal/memory/schema.sql`, `internal/memory/memory.go`, `cmd/leah/forget.go` (--category) |

W133's audit-schema edit is the only cross-wave coupling — lands first, others rebase. W130 + W131 + W132 + W134 + W135 + W136 are otherwise parallel.

## 10. Test plan per wave

Every wave ships:

- Failing test first (README.md TDD rule).
- Golden-file test where output schema is stable (M1 JSON, M3 archive header, M7 schema).
- Negative test (M2 without attestation, M3 wrong passphrase, M4 with egress attempted).
- Audit-row assertion (every CLI path emits one row with the documented kind + BR).

Cross-cutting: each wave's PR body MUST include `leah whoami --full` before/after diff demonstrating the new source row appears (or, for M4/M6/M7, demonstrating the new audit kind appears).

## 11. Privacy gates

M1 + M3 expose every persisted operator row in one shot. Both gated by attestation:

- M1 `--full` is BR=2 attested (read-only but full enumeration).
- M3 `export --all` is BR=3 attested (read-only but archive leaves the machine).
- M3 `import --overwrite` is BR=4 attested (destroys current state).
- M2 `purge --everything` is BR=4 attested.

No moat is opt-out without operator action — each ships with a default that errs toward operator control over discoverability.

## 12. Operator override / kill-switches

Each moat is independently disable-able via env. The kill-switch's job is to let operators opt out of NEW surfaces, never to disable existing privacy guarantees:

- `LEAH_DISABLE_WHOAMI_FULL=1` → `--full` returns "disabled by operator config".
- `LEAH_DISABLE_PURGE=1` → `purge --everything` returns exit 1 + msg "disabled; remove flag to re-enable".
- `LEAH_DISABLE_EXPORT=1` → `export --all` + `import` both return exit 1.
- `LEAH_LOCAL_ONLY=1` IS the M4 switch — no separate disable (operator just unsets).
- M5/M6 are build artifacts — not runtime kill-switchable.
- `LEAH_DISABLE_CATEGORY_ATTEST=1` → M7 prompt never fires (but category column still populated, audit row still written). Visible degradation, not silent.

Every disable env emits an audit row `kind=<moat>_disabled` on daemon start so operator forensics can detect a tampered config.

## 13. Cardinality / observability

Zero new metrics. `audit.jsonl` carries every signal these moats need:

- M1 row count → derivable from `wc -l` per audit kind.
- M2 purge events → kind=purge_everything.
- M3 export/import → kind=export_all, kind=import_all.
- M4 egress verification → existing `egress_bytes` field (S2 schema).
- M7 attestation prompts → kind=memory_category_attested.

This is consistent with the wave-8 brief P0 sequencing — S2 LLM-dim observability extends `audit.Entry` with InputTokens/OutputTokens/LatencyMS/EgressBytes; M4 reuses EgressBytes, M7 reuses the existing Detail field. No metric proliferation.

## 14. Out of scope

- Hardware-attested local-only (TPM-backed key sealing) — defer until v2; current scope is software-attested.
- Encrypted-at-rest state dir — M3's archive is encrypted but `~/.leah-state/` itself is FS-default. macOS FileVault is the operator's job.
- Multi-operator namespacing (P1 in brief) — every moat assumes single-operator state dir.
- Notarized macOS binary — that's S12 (`signed-distribution.md`), distinct wave.

## 15. Risks

1. M4's egress block depends on every HTTP client routing through `localOnlyTransport`. A library import that constructs its own transport bypasses the gate. Mitigation: lint rule + reviewer-required check that any new `http.Client{}` literal is wrapped, plus the audit-side EgressBytes assertion catches violations post-hoc.
2. M6's slsa-github-generator reusable workflow API may drift. Mitigation: pin to a specific tag, not `@main`; rev-bump explicit.
3. M7's category tagging being upstream-of-M7 means M7 ships dormant — operator sees no behavior change until tagging lands. Mitigation: ship M7 + tagging in the same wave-window (W136 + a follow-on) so the dormant period is <1 wave.
4. M3 passphrase loss = data unrecoverable by design. Operator-facing doc MUST surface this; brew formula post-install message and `leah export --all` first-run prompt both print the warning.
