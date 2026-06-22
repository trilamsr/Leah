# Wave-8 — AI/ML upgrade brief

Date: 2026-06-10
Source: 4 parallel adversarial critique agents (AI/ML feedback loop, voice+multimodal, agent-loop+meta-learning, observability+product moats).
Goal: convert "ambitious 2024-era assistant" → "calibrated 2026-frontier assistant with measurable moats."

## Decision priority for this wave

long-term-benefit > root-cause > performance (operator handoff window).

CLAUDE.md baseline is `UX > performance > long-term`. The inversion here is the autonomous-handoff operating profile override documented in `memory/feedback_autonomous_handoff_2026-06-10.md` (operator handed off 2026-06-10; for this wave only, durability work outranks UX polish).

## Synthesis — five durability gaps span all four reports

1. **No calibration**: no SWE-bench-style internal benchmark, no LLM-eval pipeline, no prompt-version metric — every change is faith-based.
2. **No closed-loop learning**: dispatch templates calcify as prose; `SQLiteEngine.RecordFeedback` is deferred-by-design wiring (godoc: "the propose-time blender lands in W18") — owed work, not a bug; memory dir is operator-readable, never injected at dispatch.
3. **Static ranking + no exploration**: Thompson sampling missing; cold-start = silence; concept drift undetected.
4. **No protocol surface**: Leah is MCP-client-only and Claude-Code-orchestrator-only — no MCP server, no A2A endpoint.
5. **No defensible moat artifacts**: no `leah whoami`, no portable export, no verifiable local-only mode, no reproducible build.

## P0 spec PRs (serialize — file under docs/engineer/specs/)

### S1 — Leah internal eval benchmark
Spec: `2026-06-10-eval-pipeline.md`
- `evals/<feature>.jsonl` golden traces (reasoner, recommend, voice-intent, brief).
- `make eval` runs LLM-as-judge harness; posts delta table to PR.
- GH check required on `prompts/`, `internal/reasoner/`, `internal/intent/`, `internal/recommend/`.
- Fail if pass-rate drops >2% vs main.
- Budget: ≤$0.10 per-eval enforced via existing per-process `budget.DefaultCeiling = 5.0` (this is per-process, not weekly). Weekly aggregate ≤$5 enforced by separate persistent counter (sqlite row `eval_cost_week_dollars`) so multi-process runs don't blow the cap.
- ROI: HIGH. Without this every other improvement is uncalibrated. Equal-priority with S8 (regressed-feature repair — currently shipped vapor).

### S2 — Prompt-version + LLM-dim observability
Spec: `2026-06-10-llm-ops.md`
- Hash every prompt at load → `prompt_sha` label on `leah_reasoner_call_total`.
- Adopt OpenLLMetry `gen_ai.*` OTel semconv.
- Extend `audit.Entry` with Model, PromptSHA, InputTokens, OutputTokens, LatencyMS, EgressBytes.
- Per-operator monthly cost gauge `leah_cost_month_dollars{kind}` persisted across restarts.
- Cost circuit breaker w/ graceful degrade (80% cap → Sonnet→Haiku for non-merge work).

### S3 — Memory-as-dispatch-input
Spec: CARVED (S3 deferred — see stale-specs.md)
- Dispatcher greps `memory/` at dispatch time, prepends N most-relevant entries to subagent prompt by topic-match (keyword + cheap embedding).
- Promotes memory rules from operator-readable markdown → live dispatch enforcement.
- Companion: pre-Write hook rejects absolute-paths-outside-worktree (mechanically enforces trap #1a from implementer.md).
- Goal: stop the prose-trap accretion in implementer.md; collapse traps into executable gates.
- **Tradeoff**: embedding-based ranking introduces non-determinism into a human-auditable surface. Memory dir today is fully operator-readable and grep-replayable; once embeddings are in the dispatch path, injection ordering becomes model-dependent and reproducibility regresses. Mitigation: log the injected entries' SHAs into `audit.jsonl` so any subagent run can be replayed with the exact prompt window.

### S4 — Thompson-sampling recommender + change-point detection
Spec: `2026-06-10-bandit-recommender.md`
- Replace `RankedPropose` greedy sort with Beta posterior sampling per pattern.
- Accept→α+=1; Reject→β+=1; Ignore→β+=0.1.
- Add BOCPD on daily ProfileRow.Weight deltas; emit `operator_changepoint_detected` audit row.
- Cold-start: ship `cold_start_prior.json` (≈20 hand-curated patterns); blend with α=days_observed/14.
- Phase out static halflives via nightly meta-learning grid-search (PRM-cheap scorecard pattern).
- **W18 owed-wiring (not a bug)**: `SQLiteEngine.RecordFeedback` is deferred-by-design — the godoc explicitly states the propose-time blender lands in W18. This wave delivers that owed wiring: route accept/reject/ignore counts into the Beta posterior at propose time. No regression to repair; closing planned scope.

### S5 — Reflexion + tournament review
Spec: `2026-06-10-reflexion-loop.md`
- Reviewer `block-on-findings` → dispatch second reviewer with first's findings attached, asking "what did reviewer-1 miss?"
- Tournament review for load-bearing PRs (spec PRs, CLAUDE.md edits): 2 designer subagents + 1 arbitrator.
- Per-stage scorecard rows in `audit.jsonl` (TDD-order Y/N, comment-density Y/N, reviewer-clean-first-pass Y/N).
- Selflearn aggregates per-template → exposes which dispatch template degrades over time.

### S6 — Voice frontier upgrade
Spec: `2026-06-10-voice-frontier.md`
- Sentence-boundary streaming Reasoner→TTS (kills 600ms first-audio latency).
- Porcupine wake-word backend behind feature flag (parallel to existing energy detector).
- OpenAI Realtime-2 (gpt-realtime) opt-in single-stage path (250-500ms total latency).
- Picovoice Eagle speaker-ID enrollment → can drop "yes leah" phrase for rightful operator.
- Voice Isolation mic mode integration (cuts cough/TV barge-in DoS ~80%).
- Phoneme-distance wake-phrase fuzzy match (3x true-positive rate).
- Rolling 30s energy-floor (vs single-shot calibration).
- Per-backend 2s context timeout in ChainTTS.
- Local Ollama Reasoner fallback when network down.

### S7 — Eval+memory infrastructure for SelfBuild trustworthiness
Spec: `2026-06-10-selfbuild-attestation-risk.md`
- Per-PR risk score from (BR × historical-failure-rate × diff-LOC).
- High-risk → harder attestation questions, 2-of-3 reviewers.
- Routine PRs → easier attestation (reduce operator habituation risk).
- Sub-PR retro: capture per-subagent-turn observations in audit.jsonl.

### S8 — Knowledge graph wiring into engine (regressed-feature repair)
Status: spec landed on main; `internal/knowledge/` only present in worktrees, no caller in `engine.go`.
- Implement `Graph.Query`; let pattern adapters accept KnowledgeQuery result.
- Pattern key: `(pattern_name, entity_key)` not just `pattern_name`.
- Entity TTL + demotion handles "Sarah is now ex-coworker" stale problem.
- ROI: HIGH — equal-priority with S1. Regressed-feature repair: knowledge graph is currently shipped vapor (spec on main, no caller), so user-visible behavior already regressed. Repairing a regression outranks net-new calibration work in ordering.

### S9 — Nightly consolidation pass ("dreaming")
Spec: `2026-06-10-memory-consolidation.md`
- 3am cron: any (kind, slot) cell with stable decayed weight ≥14d → write durable summary row, prune underlying audit older than 14d to `consolidated.jsonl`.
- Episodic → semantic abstraction.
- Pattern source: OpenAI Dreaming V3 + SCM paper 2026.

### S10 — Operator-trust moat artifacts
Spec: `2026-06-10-trust-moats.md`
Each is ~1 spec + ~1 PR:
- `leah whoami --full`: enumerate every persisted row by source (memory, recommend, knowledge, mirror, events).
- `leah purge --everything`: revoke OAuth at provider + delete local state + brew/PATH cleanup.
- `leah export --all` / `leah import`: portable encrypted archive.
- `LEAH_LOCAL_ONLY=1`: force every Reasoner call through Ollama; audit row "0 egress bytes" verifies.
- Data-flow SVG: one diagram, every store, every network egress — afternoon of work.
- Reproducible build attestation: GHA + SBOM + SLSA L2.
- Per-category memory attestation (relationship/finance/health) at first sight.

### S11 — MCP server + A2A endpoint (interop)
Spec: `2026-06-10-mcp-a2a-publish.md`

**S11.0 PREREQ — auth/attestation gate (blocking, MUST land first)**
Exposing `SelfBuild` (or any write-capable surface) over MCP/A2A without auth = remote feature-request RCE: any peer agent could trigger `leah self-build <arbitrary spec>` against the operator's machine. Before S11.1 lands:
- Per-tool auth scope on the MCP server: read-only tools (`leah_search_audit`, `leah_dispatch_status`) allowed under operator-presigned token; write-capable tools (`leah_self_build_*`) require fresh operator attestation (touch ID / TTY confirm) per call.
- A2A agent-card MUST advertise required `auth_scheme=oauth2+operator_attestation` for SelfBuild delegation; reject task delegations without it.
- Localhost-only bind by default (`127.0.0.1`); explicit `--listen 0.0.0.0` opt-in with warning + audit row.
- Audit row per inbound MCP/A2A call: peer identity, tool, args-hash, attestation status.

**S11.1 — surface (only after S11.0)**
- Expose `leah_get_memory_rule`, `leah_search_audit`, `leah_dispatch_status`, `leah_self_build_status` as MCP tools (read-only — no PREREQ block).
- Wrap SelfBuild in A2A 1.0 agent-card (task delegation + verification via attestation gate from S11.0).
- Cost: ~few hundred LOC wrapping `internal/audit`, `internal/selflearn`, `internal/dispatcher/selfbuild`.

### S12 — Distribution: signed/notarized binary
Spec: `2026-06-10-signed-distribution.md`
- $99/yr Apple Developer ID + 2 dev-days work.
- `codesign --options runtime` + `notarytool submit --wait` in GHA.
- Sparkle EdDSA appcast.
- First non-developer operator unlocked.
- Bump priority above Discord/WhatsApp adapter waves.

## P1 — Lower-ROI but flagged risks

- Multi-operator state-dir namespacing (single-operator architecture = moat illusion).
- OAuth refresh-rotation (W10-2 is now load-bearing tech debt across 10 adapters).
- Anthropic prompt caching (`cache_control` on system prompt → 60-80% monthly Anthropic cost drop).
- Event store retention >30d (forensics gone too fast).
- Per-package SelfCheck failure → docs URL map (current error string isn't actionable).
- HUD reviewer-subagent tile ("12 reviews, 3 blocks, 9 approves this week").
- Stub OllamaClient reasoner (makes provider-agnostic dependency arrow real).

## P2 — Defer / reject

- DSPy programmatic prompt optimization: HIGH cost, payoff requires benchmark first, and compiled prompts are opaque to operator (conflicts with CLAUDE.md single-source-of-truth).
- Wails native HUD: ADR already deferred until v3 beta — hold.
- Hardware wearable (glasses/pendant): vendor lock-in dead-ends. iPhone companion is the only viable form-factor extension.
- App Store distribution: sandbox kills cross-app SQLite pattern. Brew + direct download is correct.

## Sequencing

Phase 1 (this week — regressed-feature repair + eval/obs scaffolding, parallel where file-disjoint):
- S1 eval pipeline
- S8 knowledge-graph wiring (regressed-feature repair, equal-priority with S1, depends on nothing)
- S2 LLM-dim obs (audit schema additive — back-compat)
- S3 memory-dispatch injection
- S10a `leah whoami` + `leah export` (file-disjoint within cmd/leah/)

Phase 2 (next — calibrated improvements):
- S4 Thompson sampling + change-point (depends on S1 for eval; folds in W18 owed `RecordFeedback` wiring)
- S5 reflexion + tournament (depends on S2 for scorecard rows)

Phase 3 (frontier voice + moats):
- S6 voice frontier
- S9 consolidation pass
- S11 MCP server + A2A (S11.0 auth PREREQ blocks S11.1)
- S12 signed distribution

## Constraints inherited

- spec PRs serialize (CLAUDE.md rule)
- code PRs fan out to 6 (file-disjoint internal/<pkg>/)
- no AI signatures
- no self-approve
- adversarial reviewer per PR
- worktree discipline
- root cause > symptom

## Source critiques (this session, 2026-06-10)

- AI/ML feedback loop critique (agent aeba2c31): static α-blend ranking, no consolidation, `SQLiteEngine.RecordFeedback` W18-deferred-by-design wiring (owed work, not a bug — see S4), no knowledge-graph wiring, no exploration.
- Voice + multimodal critique (agent a8fb8649): cascaded pipeline loses 600-1500ms vs native S2S, energy-threshold wake is 2015-era, no streaming Reasoner→TTS, no speaker-ID, hardware form-factor gap.
- Agent loop + meta-learning critique (agent ab12f297): no benchmark → uncalibrated meta-learning, templates calcify, closed ecosystem (no MCP server, no A2A), single-vendor Claude-Code lock-in.
- Obs + product critique (agent a1d62293): 2018-classical obs ported to 2026 (no prompt-version, no eval, no LLM dim on audit), per-process budget hides monthly cost, distribution gap blocks non-dev operators, no defensible moat artifacts.

## Wave-8 closeout status (2026-06-10)

**Spec chain S1-S12: ALL MERGED ON MAIN**

| Spec | PR | Status |
|---|---|---|
| S1 eval pipeline | #154 | merged |
| S2 LLM-dim observability | #166 | merged |
| S3 memory-as-dispatch-input | #168 | merged |
| S4 Thompson-sampling recommender | #171 | merged |
| S5 reflexion loop + tournament review | #172 | merged |
| S6 voice frontier upgrade | #173 | merged |
| S7 SelfBuild attestation risk-tiering | #175 | merged |
| S8 knowledge-graph wiring | #177 | merged |
| S9 memory consolidation pass | #185 | merged |
| S10 operator-trust moat artifacts | #188 | merged |
| S11 MCP server + A2A endpoint | #189 | merged |
| S12 signed + notarized distribution | #190 | merged |

**Impl waves shipped:**
- W82 eval harness (#195)
- W92+W93 LLM-dim audit schema + reasoner instrumentation (#196)
- W94 CostMonth + circuit breaker (#215)
- W100 SQLite propose-time blender (#181)
- W101 Beta posterior + Thompson sampling (#216)
- W109 streaming Reasoner→TTS (#217)
- W116 risk-score + tier (#207)
- W120 knowledge-graph TTL+demotion (#192)
- W124 nightly consolidation pass (#219)
- W130 leah whoami --full (#194)
- W131 leah purge --everything (#208)
- W132 leah export/import (#218)
- W135 reproducible build + SLSA L2 (#206)
- W137 MCP server scaffold (#193)
- W138 MCP tools + redact (#199)
- W139 A2A endpoint + SelfBuild over A2A (#209)
- W141 GHA signed release workflow (#205)

**Impl waves NOT YET shipped:** W121 W122-W123 W125-W127 W133-W134 W136 W142 W143 — see #249.

**Prevention bundle landed:** PR #236 — `scripts/check-{reviewer-verdict,tdd-evidence}.sh` gates + GHA `pr-gates` job + `docs/operator/ci-gates.md`. Operator must promote to required-check (see #246).

**Audit issues filed:** #242 (self-approve token leaks), #243 (self-approve-after-amend), #244 (TDD-evidence misses), #246 (branch protection), #247 (worktree janitor install), #248 (memory codification), #250 (UX-audit B1/B5 still open).
