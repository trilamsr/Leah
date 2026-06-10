# Memory-as-dispatch-input (wave-8 S3)

Date: 2026-06-10
Brief: `docs/engineer/briefs/2026-06-10-wave-8-aiml-upgrade.md` §S3
Sibling specs: `2026-06-10-eval-pipeline.md` (S1), `2026-06-10-llm-ops.md` (S2)

## 1. Goal + non-goals

### Goal
Promote operator memory entries under `~/.claude/projects/-Users-treedesk-Desktop-Projects-leah/memory/` from operator-readable markdown into a live input on every subagent dispatch. Collapse the prose-trap accretion in `docs/engineer/dispatch-templates/implementer.md` (now 6+1 meta-learned traps — 1, 1a, 2, 3, 4, 5, 6; ~200 LOC, growing every session) into executable gates fed by topic-matched memory rules + mechanical pre-Write hooks.

### Non-goals
- Editing memory content from inside dispatch (memory is operator-write-only; subagents read).
- Cross-operator memory sharing (single-operator architecture per CLAUDE.md).
- Replacing CLAUDE.md / dispatch templates (memory layers ON TOP; templates remain canonical for invariants).
- Online-learning the topic-match model (model is loaded once per process; retraining is operator-explicit).

## 2. Topic-match algorithm

Two-stage pipeline. Stage 2 is opt-in via env flag — keyword-only is the deterministic default.

### Stage 1 — keyword candidates (BM25)
- Tokenize task description + dispatch payload (`<TASK-ID>`, `<SPEC-PATH>` title, `<FILE-SCOPE>` package names) → bag-of-tokens after lower-casing, splitting on `[^a-z0-9]`, dropping stopwords (`go`, `the`, `a`, `pr`, `wave`, `task`).
- For each memory file: tokenize title (first `# ` line) + body the same way.
- BM25 score (k1=1.5, b=0.75) over the memory-dir corpus; keep top `2N` candidates (default `2*3=6`).
- Tie-break by file mtime descending (newer ops feedback wins).

### Stage 2 — embedding rerank (optional)
- Behind `LEAH_MEMORY_RERANK=1` env (default off).
- Model: local `sentence-transformers/all-MiniLM-L6-v2` via Ollama (`ollama run nomic-embed-text` as fallback). Adds 50-150ms per dispatch — acceptable budget given dispatch is human-initiated.
- Cosine-rank the `2N` candidates → top `N`.
- Hard fallback: if model unreachable, return Stage-1 top-`N` and log `audit.Entry{Kind: "memory_inject_fallback"}`.

### Why two stages
BM25 alone misclassifies semantic-but-non-lexical matches (e.g. task description "subagent commits stray to primary" vs memory titled "Worktree discipline"). Pure embedding alone is non-deterministic across model versions. Stage 1 narrows determinism-preserving; Stage 2 adds recall when explicitly enabled.

## 3. Prompt assembly

Memory entries are injected into a single `<memory-rules>` XML block, prepended BEFORE the dispatch template body, AFTER the task header:

```
<task-id>W96-G1</task-id>
<memory-rules count="3" rerank="off" injection-sha="<sha256>">
<rule path="feedback_autonomous_handoff_2026-06-10.md" sha="abcd1234" score="9.21">
# Autonomous handoff operating profile
- priority long-term > root-cause > perf ...
</rule>
<rule path="leah_first_launch_integration_auth.md" sha="ef567890" score="7.44">
...
</rule>
</memory-rules>
<dispatch-template>
... (existing implementer.md body) ...
</dispatch-template>
```

Per-rule fields:
- `path` — basename only, never absolute (privacy: no operator-home leak in subagent logs).
- `sha` — first 8 chars of SHA-256 of the file content at injection time.
- `score` — BM25 or cosine score; informational, never load-bearing.

Total `<memory-rules>` block hard-capped at 4096 tokens (≈16KB). Overflow → truncate longest entry first; emit `audit.Entry{Kind: "memory_inject_truncated"}` AND inject `<memory-truncated count="N" reason="token-budget"/>` as the first child of `<memory-rules>` so the subagent sees the signal in-band.

### Subagent contract on truncation
A "safety-class" rule is any rule whose content contains the keywords `never`, `always`, or `MUST` (case-insensitive, whole-word). If `<memory-truncated/>` is present AND any retained rule is safety-class AND ANY safety-class rule was truncated (recorded as `truncated_safety_class_count > 0` in the marker attributes), the subagent MUST refuse-dispatch with `error: memory_safety_class_truncated` and surface the marker back to the parent dispatcher. Non-safety truncations proceed; the subagent treats the marker as informational.

### Cold edges
- **Empty memory dir**: dispatch proceeds with NO `<memory-rules>` block (skip insertion entirely); emit `audit.Entry{Kind: "memory_inject"}` with `detail.memory_dir_empty=true` and `n=0`. Subagents see only the dispatch template; no soft-failure mode.
- **`<memory-pin path="X">` not found**: dispatch FAILS with explicit error `memory_pin_not_found: <X>` returned to the operator (no silent skip — a missing pin is almost always an operator typo, and silently dropping it would defeat the override).

## 4. N selection

| Mode | N | Trigger |
| --- | --- | --- |
| Default | 3 | Every dispatch without override |
| Compact | 1 | Token-budget pressure (parent context >80% full) |
| Wide | 8 | Operator override via `<wide-memory-injection>` tag in dispatch payload |
| Off | 0 | `<no-memory-injection>` tag (see §12) |

`N=8` is the hard ceiling. The dispatcher MUST refuse `N>8` requests with `error: memory_injection_cap_exceeded`. Rationale: with 8 entries × ~400 tokens avg, the block alone consumes ~3.2K tokens — pushing 25% of a 16K-context subagent before the dispatch body lands.

## 5. Audit trail

Every dispatch with memory injection writes ONE row to `audit.jsonl`:

```json
{
  "ts": "2026-06-10T11:58:00Z",
  "kind": "memory_inject",
  "args_hash": "<sha256 of task description>",
  "blast_radius": 0,
  "outcome": "ok",
  "detail": "{\"prompt_sha\":\"<sha256 of assembled prompt>\",\"injected\":[{\"path\":\"feedback_autonomous_handoff_2026-06-10.md\",\"sha\":\"abcd1234\"},{\"path\":\"leah_first_launch_integration_auth.md\",\"sha\":\"ef567890\"}],\"n\":3,\"rerank\":false,\"memory_dir_sha\":\"<sha256 of sorted (path,content-sha) list>\"}"
}
```

Schema reuses existing `audit.Entry` (no migration); the structured payload lives in `Detail` as JSON-encoded string to stay back-compat with downstream readers (`patterns.Detect`, `selflearn.Resolver`, `web.Snapshot`).

`memory_dir_sha` is the SHA over `(path, content_sha)` pairs sorted by path — the exact state of the memory dir at injection. Together with `prompt_sha`, this row is sufficient to replay the dispatch.

## 6. Non-determinism guard

Memory injection MUST be reproducible for a fixed `(task_description, memory_dir_state)` tuple. Mechanism:

### Cache key
`SHA256(prompt_template_path || prompt_template_sha || memory_dir_sha || task_topic_tokens_sorted || N || rerank_enabled || model_sha)`

`model_sha` is the pinned Ollama embedding model SHA (zero-valued string when `rerank_enabled=false`). Without it in the key, a `ollama pull` of a newer `all-MiniLM-L6-v2` revision would silently serve stale rerank results from cache — same precedent as `audit.Entry.Detail.model_sha` already records on the row (§5).

### Cache store
In-memory `map[string][]InjectedRule` on the dispatcher process; persisted to `~/.leah/cache/memory-inject.jsonl` (append-only; cap 10MB rolling). On dispatcher restart, replay the file into the map.

### Hit semantics
Cache hit → return the same `[]InjectedRule` set in the same order. Cache miss → run §2 algorithm, write the result, return.

### Replay
`leah dispatch-replay <prompt_sha>`:
- Reads the `memory_inject` audit row matching `prompt_sha`.
- Re-fetches memory files at the recorded `sha`s (requires they still exist locally; archived to `~/.leah/memory-history/` on operator edits — see §11).
- Re-assembles the exact prompt; pipes to stdout. Operator can re-feed to a fresh subagent.

### Embedding non-determinism mitigation
When `LEAH_MEMORY_RERANK=1`:
- Pin model SHA at dispatcher start; record in `audit.Entry.Detail` as `model_sha`.
- Mismatch on replay (operator pulled a new model) → emit `memory_replay_model_drift` audit row; replay still proceeds but flags the divergence risk.

## 7. Pre-Write absolute-path hook (companion gate)

Standalone file: `scripts/check-write-paths.sh`. Invoked by the dispatcher's pre-tool hook (registered in `.claude/settings.json` under `hooks.PreToolUse`).

### Behavior
- Input: `$1 = tool name`, `$2 = JSON args (stdin or arg)`.
- If `$1 != "Write"` and `$1 != "Edit"` → exit 0.
- Parse `file_path` from JSON.
- If `file_path` is absolute (`/...`) AND does NOT contain `.claude/worktrees/agent-` → emit error to stderr and exit 1.
- Error message: `pre-write: rejected absolute path outside agent worktree: <path>; use worktree-relative path or include .claude/worktrees/agent-<id>/ in absolute path. See implementer.md trap #1a.`
- Exit 1 from a `PreToolUse` hook in Claude Code aborts the tool call.

### Test plan (lives next to script)
- `scripts/check-write-paths.bats` (or equivalent shellcheck-clean go test):
  - reject `/Users/treedesk/Desktop/Projects/leah/internal/foo.go`
  - accept `/Users/treedesk/Desktop/Projects/leah/.claude/worktrees/agent-abc123/internal/foo.go`
  - accept `internal/foo.go` (relative)
  - skip on `Read` / `Bash` tool names
  - reject `/tmp/leah-out/foo.go` (any absolute outside worktrees)

### Coverage carve-out
`/tmp/cicheck.log`, `/tmp/pr-*.md`, `/tmp/review-*.md` — these are the harness-blessed ephemeral paths from `implementer.md` PR-BODY-HYGIENE; the hook MUST allow them. Carve-out: absolute paths matching `^/tmp/(cicheck|pr|review)[^/]*$` pass.

## 8. Migration path: prose-trap → executable gate

Audit of `implementer.md` recurring traps (lines 131-192) → mechanical gate disposition:

| Trap | Today | After S3 |
| --- | --- | --- |
| 1a. Write absolute path lands in primary worktree | prose warning + recovery snippet | `check-write-paths.sh` rejects pre-Write (§7) |
| 1. `cd` non-persistence | prose warning | Memory-injected rule (operator-owned `memory/dispatch_cd_discipline.md`) prepended to every implementer dispatch |
| 2. Implementer over-self-review | prose warning | Memory-injected rule + reviewer-template lens already flags self-APPROVE |
| 3. Stale-base rebase-at-two-points | prose warning | `scripts/check-base-fresh.sh` exists; memory-inject reinforces. No new gate. |
| 4. `defer X.Close()` errcheck on SQL | prose warning | `errcheck` already CI-gated. Memory-inject keeps it top-of-mind for new adapters. |
| 5. ≤5% comment-density on tiny new pkgs | prose warning | `check-comment-density.sh` handles + `<!-- comment-density-justified -->` marker. No new gate. |
| 6. Worktree-held branches block cleanup | prose warning | Memory-injected rule for cleanup-phase dispatches. |

Net: trap 1a becomes mechanical (§7). Traps 1, 2, 6 become memory-injected rules (operator writes `memory/dispatch_*.md`; dispatcher injects on topic match). Traps 3, 4, 5 already had executable gates; memory injection adds belt-and-suspenders.

Implementer.md §"Recurring traps (meta-learned)" SHRINKS from 6 rules to a 2-line pointer: "Recurring traps are now memory-injected per dispatch. See `memory/dispatch_*.md`. Operator-edit the memory file; new dispatches pick it up on next run." Prose growth halts.

## 9. Wave plan W96-W99 (file-disjoint impl)

| Wave | Package | Output |
| --- | --- | --- |
| W96 | `internal/memoryinject/` (new) | `Selector` type: `Select(ctx, taskDesc, n int) []Rule`; BM25 impl; tests against fixture memory dir |
| W97 | `internal/memoryinject/rerank.go` | Embedding rerank behind `LEAH_MEMORY_RERANK` env; Ollama client wrap; fallback path |
| W98 | `internal/dispatcher/` | Wire `Selector` into the dispatch-prompt assembler; emit `memory_inject` audit row; cache replay |
| W99 | `scripts/check-write-paths.sh` + `.claude/settings.json` hook | Pre-Write gate; bats tests; implementer.md migration commit (delete prose traps that became gates) |

File-disjoint check:
- W96, W97 — new package, no overlap with anything.
- W98 — touches `internal/dispatcher/` only (composition root sibling-touch flagged by §SHARED-PRIMITIVE OWNERSHIP — single-owner per dispatch).
- W99 — touches `scripts/` + `.claude/settings.json` + `docs/engineer/dispatch-templates/implementer.md` (single-owner per CLAUDE.md root-file ownership rule).

Parallelism: W96 + W97 can dispatch in parallel (same new package, file-disjoint). W98 serializes after W96 lands. W99 serializes after W98 (implementer.md edit is the migration commit).

## 10. Test plan per wave

### W96 — `internal/memoryinject/`
- `selector_bm25_test.go`: fixture `testdata/memory/{a,b,c}.md`; assert top-1 result for known token queries; assert mtime tie-break.
- `selector_stopword_test.go`: assert "go", "pr", "wave" tokens drop.
- `selector_n_cap_test.go`: `N=9` returns error `memory_injection_cap_exceeded`.
- `selector_truncate_test.go`: synthetic 5KB memory file; assert `memory_inject_truncated` audit hook fires.

### W97 — rerank
- `rerank_offline_test.go`: stub Ollama 503 → returns Stage-1 result + emits fallback audit (verified via `audit.Logger` mock).
- `rerank_pinned_model_test.go`: assert model SHA recorded in returned `[]Rule` metadata.

### W98 — dispatcher wiring
- `dispatcher_inject_test.go`: golden-file dispatch prompt assembly; assert `<memory-rules count="3" injection-sha="...">` block present; assert injection-sha is deterministic across two runs with same memory state.
- `dispatcher_cache_test.go`: two consecutive `Select`s with same key → second hits cache (verified via call counter on stubbed Selector).
- `dispatcher_replay_test.go`: round-trip — assemble, write audit row, parse audit row, re-assemble, byte-compare.

### W99 — hook
- `scripts/check-write-paths.bats`: 5 cases from §7 test plan.
- E2E: `make check` runs the hook in dry-run mode against a known-bad fixture; expect exit 1.

## 11. Privacy + audit

### Content boundary
Memory file content is operator-private. The injected block IS shown to subagents (they ARE the operator's agent), but:
- Audit `Detail` records `path` + `sha` ONLY — never content.
- Replay reads from the local memory dir; archived old versions go to `~/.leah/memory-history/<sha-prefix>/<basename>` (operator-readable, 0600).
- No cross-session leak: a subagent dispatched in session A cannot read session B's injection content directly — they only see what their parent dispatcher injected into THEIR prompt.

### Network egress
BM25 selection runs LOCAL ONLY (zero egress) — file reads from the operator's memory dir, scoring in-process, no socket touched. The assembled prompt — including selected memory content — is then sent to the reasoner, which is operator-configured (Anthropic by default, Ollama with `LEAH_LOCAL_ONLY=1`). The operator must trust the reasoner with selected memory content per their reasoner-of-choice; this is the same trust boundary as CLAUDE.md content reaching the reasoner today (no new exposure surface).

`LEAH_MEMORY_RERANK=1` → cosine call goes to local Ollama only (127.0.0.1). Hard rule: rerank MUST NOT hit external embedding APIs (no OpenAI, no Anthropic embeddings). Enforced by `internal/budget` egress check on the dispatcher process. `LEAH_LOCAL_ONLY=1` (per S10) forces rerank-off regardless of `LEAH_MEMORY_RERANK`, AND routes the reasoner itself through local Ollama (zero egress end-to-end).

### Audit retention
`memory_inject` rows participate in the standard `audit.jsonl` retention (>30d per wave-8 P1 flag). Replay possible for any unpruned row. Consolidation (S9 nightly dream pass) MAY summarize `memory_inject` rows into per-week aggregates after 14d, but original `prompt_sha` + `memory_dir_sha` are preserved in the summary so replay still resolves.

## 12. Operator overrides

### Per-dispatch opt-out
Operator (or a parent dispatcher) may add `<no-memory-injection>` to the task payload. Effect: N=0, no audit row emitted with `kind="memory_inject"`, instead emits `kind="memory_inject_skipped"` with `detail: "operator opt-out"`.

Use cases:
- Replay testing (avoid double-injection).
- Tournament review (designer-arbitrator dispatches from S5 — already see the implementer's full prompt; second injection is noise).
- Spec-only PR dispatches (the dispatch payload IS the spec; memory adds nothing).

### Per-dispatch widening
`<wide-memory-injection>` → N=8. Use for cross-cutting refactors where multiple memory rules apply (e.g. macOS adapter that touches integration auth + first-launch UX + audit schema).

### Per-dispatch keyword pin
`<memory-pin path="dispatch_cd_discipline.md">` (repeatable) → force-include a specific memory file in the top-N, regardless of BM25 score. Counts against N. Use when operator KNOWS a specific rule applies but the topic-match might miss it (rare; default to letting selection run).

### Memory file opt-out
Memory file with frontmatter `dispatch-inject: false` is excluded from the corpus entirely. Use for memory files that are operator-only notes (the three `research_*.md` files in current memory dir — too long, too niche to inject by default).

## 13. Risks + mitigations

| Risk | Mitigation |
| --- | --- |
| BM25 misses semantic matches | Stage 2 embedding rerank behind explicit flag |
| Embedding rerank introduces non-determinism in human-auditable surface | `memory_inject` audit row records `prompt_sha` + every injected file's `sha` + `model_sha` → exact replay possible |
| Prompt bloat from N=8 mode | 4096-token hard cap on `<memory-rules>` block; truncate longest entry first |
| Operator edits memory mid-session changes future dispatches | Each `memory_inject` row records `memory_dir_sha` → replay still resolves; archive history dir keeps prior versions |
| `/tmp/leah-out/` cargo-cult by future operators | Pre-Write hook carve-out is exhaustive — only blessed `/tmp/(cicheck\|pr\|review)*` paths pass |
| Memory file containing secrets gets injected | Memory dir is operator-private; subagent context is operator-trusted (same trust boundary as CLAUDE.md content). Same gate as today. |
| Cache poisoning if `~/.leah/cache/memory-inject.jsonl` corrupted | Cache is advisory — on parse error, log + skip cache, recompute. Worst case: 1 extra dispatch's worth of BM25 work. |

## 14. Out of scope (explicit defer)

- Memory editing UX (operator already uses `$EDITOR` on memory files directly).
- Memory file versioning beyond the archive-on-edit history dir (no git inside `~/.leah/memory-history/`; flat sha-prefix dirs suffice).
- Reranker model selection UX — first impl pins `all-MiniLM-L6-v2`; later wave (TBD) may add operator-config.
- Cross-dispatch memory bundling (e.g. share one injection across a 6-fan-out batch). Each dispatch re-runs §2 independently; cache may hit when payloads share tokens.

### Deferred follow-ups (tracked, not blocking)
- **Multi-process cache flock**: `~/.leah/cache/memory-inject.jsonl` is currently single-writer-safe by virtue of single-dispatcher-process. If a future fan-out spawns parallel dispatcher processes (not planned in v1), the append needs `flock(2)`. Track when fan-out parallelism lands.
- **`memory-dir-sha` on `<memory-rules>` tag**: today the SHA lives in the audit row only. Adding `memory-dir-sha="..."` as an attribute on the `<memory-rules>` open tag would let the subagent self-verify against the parent's claim without reading audit. Deferred — needs subagent-side verification logic that doesn't exist yet.
- **Compact-mode actor**: §4 says N=1 triggers when "parent context >80% full" but doesn't name who measures. Decision deferred to W98: dispatcher reads parent context-usage from the latest `audit.Entry{Kind: "context_usage"}` row (already emitted by §S2 LLM-dim observability) and selects N at assembly time. Locking this here would couple S3 to S2's exact row schema before S2 lands.

## 15. Acceptance criteria (per wave gate)

- W96: `go test ./internal/memoryinject/...` green; selector deterministic across 100-run loop on fixture corpus.
- W97: rerank fallback path covered (kill `ollama` mid-test → audit row emitted, dispatch continues).
- W98: dispatch prompt assembler emits `<memory-rules>` block; `audit.jsonl` contains `kind="memory_inject"` row after every memory-using dispatch; `leah dispatch-replay <prompt_sha>` round-trips byte-identical.
- W99: pre-Write hook installed; bats tests green; `implementer.md` recurring-traps section shrinks from 6 rules to a 2-line pointer; CLAUDE.md unchanged (no new rule — the gate IS the rule).

## 16. References

- Wave-8 brief §S3: `docs/engineer/briefs/2026-06-10-wave-8-aiml-upgrade.md`
- Sibling S1: `docs/engineer/specs/2026-06-10-eval-pipeline.md`
- Sibling S2: `docs/engineer/specs/2026-06-10-llm-ops.md`
- Implementer trap surface: `docs/engineer/dispatch-templates/implementer.md` lines 131-192
- Reviewer template (for the second-pass review per S5): `docs/engineer/dispatch-templates/reviewer.md`
- Audit schema: `internal/audit/audit.go` — reuses `Entry.Detail` JSON-string convention
- BM25 reference: Robertson & Zaragoza, "The Probabilistic Relevance Framework: BM25 and Beyond" (2009)
- Sentence-transformer model: `sentence-transformers/all-MiniLM-L6-v2` (Apache-2.0)
