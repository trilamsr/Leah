# Wave-9 — Velocity + Responsiveness brief

Synthesizes 4 proposer reports (operational responsiveness, feedback loops, dev velocity, product responsiveness) against 4 adversarial verdicts. Keeps only adversarial-verified findings. Rejected proposals are listed explicitly so they do not re-surface.

Proposer agents: `af05ba78` (op), `ab083481` (fb), `a76c79f2` (vel), `a8b74b22` (prod).
Verdicts: `a8b4f14d` ACCEPT-WITH-AMENDMENTS, `ac7cc5a4` ACCEPT-WITH-AMENDMENTS, `a22adf88` REJECT (heavy), `a43f957d` ACCEPT-WITH-AMENDMENTS.

## 1. Goal

Make Leah responsive on both axes:

- **Dev**: PR throughput — file-disjoint cap-6 stays the rule; cut local `check.sh` wall-clock, sweep worktrees, catch stale-base + placeholder PRs before reviewer cost.
- **Op**: Voice turn p95, HUD ambient freshness, anticipatory recommend — but **measure first**. The dev-velocity verdict and the responsiveness verdict converge on one demand: no histograms = no baseline = no claim. V1 instruments before V10 optimizes.

## 2. Adversarial-verified findings (kept items only)

| # | Finding | Source | Verdict confirms |
|---|---|---|---|
| F1 | Voice path uninstrumented — `voice_turn_seconds` histogram + per-stage breakdowns missing. Current "1.5s p95" is **spec target**, not measurement. | `af05ba78` §voice | `a8b4f14d`: "p95 1.5s is the BUDGET in voice-comm spec §5, not an observed number" |
| F2 | HUD ambient still polls `/api/state` (`ambient.js:54,65`, 5s setInterval) and recommendations widget polls `/hud/recommendations` (`recommendations.js:100`, 15s setInterval) despite W77 SSE shipped. | `af05ba78` §HUD | `a8b4f14d`: verified — `ambient.js:44` opens SSE for events but `pollMetrics` at `:52` still runs alongside; `recommendations.js` has no SSE wiring |
| F3 | `mirror.go DefaultTickInterval = 60s` for Calendar/Contacts/Notes/Mail/Messages while EventKit + NSWorkspace are push APIs. | `af05ba78` §mirror | `a8b4f14d`: VERIFIED structurally; EventKit push ≤1s realistic, FSEvents has 1-5s coalescence (not "<1s" universally) |
| F4 | `cache_control` zero hits in repo — Anthropic prompt-cache not implemented. | `af05ba78` §cache | `a8b4f14d`: VERIFIED; but savings only materialize if system-prompt > ~1024 tokens. Cost = MEASURE first |
| F5 | Reasoner non-streaming — zero hits for `Stream|delta|chunk` in `internal/reasoner/`. | `af05ba78` §stream | `a8b4f14d`: VERIFIED; Anthropic Go SDK streaming surface exists in 2026 |
| F6 | Pre-PR base-staleness gate would have caught PR #151-class (built off #144 not main → 2🔴). | `ab083481` P5 | `ac7cc5a4`: VERIFIED need; 30-LOC estimate OVERSTATED-LOW (real ≈60-90 LOC incl. file-overlap + tests) |
| F7 | `check.sh` is sequentially vet/lint/comment-density/no-bare-sleep/doc-links AFTER build+test; safe to parallelize post-build stages. | `a76c79f2` P1 | `a22adf88`: VERIFIED structurally; **savings inflated** (real ≈1.5s warm, not 4-10s) |
| F8 | 134 worktrees, 129 locked, 354MB on disk. Janitor needed. | `a76c79f2` P7 | `a22adf88`: VERIFIED exactly (134 not 131; locks confirmed) |
| F9 | Implementer subagents shipped placeholder code that passed compile (PR #146 no audio uplink, PR #148 dead Session field). Catchable pre-PR via grep gate. | `ab083481` P6 | `ac7cc5a4`: VERIFIED — pattern-match for `TODO`/`func _ =`/unused-field in `feat/` PRs is cheap |
| F10 | `Engine.Propose` is interval-driven (60s daemon tick), not signal-driven. SSE substrate from W75/W77 lets it become event-driven. | `a8b74b22` Engine.OnSignal | `a43f957d`: VERIFIED architecturally; needs per-pattern debounce or NSWorkspace push will saturate downstream |
| F11 | `activeapp.Query()` pulls via osascript every tick; NSWorkspace push is native. | `a8b74b22` §activeapp | `a43f957d`: VERIFIED at `internal/macos/activeapp/activeapp.go`; CGO bridge cost real; EventKit defer already in `macos-ecosystem-integration.md:158` |
| F12 | Operator-trust "voice trust phrase" is privacy-hostile in public; HUD banner default is correct. | `a8b74b22` §trust | `a43f957d`: VERIFIED concern — invert default: HUD banner default, voice opt-in |
| F13 | Lifelog composer would spend Anthropic budget on empty days (operator on vacation). Gate on non-empty audit/mirror/feed window. | `a8b74b22` §lifelog | `a43f957d`: VERIFIED gap; mitigation: skip if observation-count < threshold |
| F14 | HUD-armed voice should go via Wails IPC, not WebRTC — browser mic = consent prompt each session. | `a8b74b22` §HUD-arm | `a43f957d`: VERIFIED; cost-of-IPC underrated, defer to Wails v3 per `docs/engineer/specs/2026-06-10-wails-decision.md` |

## 3. P0 — wave-9a (this wave, file-disjoint, parallelizable to cap-6)

Each item is a single PR, single owning package(s), no shared roots.

**V1 — voice instrumentation FIRST.** `internal/voice/instrumentation.go` exists; add `voice_turn_seconds` histogram + per-stage (wake_to_stt_seconds, stt_seconds, reasoner_seconds, tts_first_audio_seconds). Per `a8b4f14d`: "no histograms = no baseline = unfalsifiable claims." Blocks V10. ~80 LOC + tests. Owner: `internal/voice/`.

**V2 — HUD ambient SSE migration.** Drop `/api/state` 5s poll in `ambient.js:52-65` (pollMetrics + setInterval); drop `/hud/recommendations` 15s poll in `recommendations.js:99-100` (load + setInterval). Both subscribe to `/api/events` SSE (W77 substrate; ambient already has the EventSource open at `ambient.js:44`). ~80-120 LOC, mostly JS deletion. Owner: `internal/hud/static/`.

**V3 — parallel `check.sh` post-build stages.** After `go build && go test`, run vet+lint+comment-density+no-bare-sleep+doc-links in parallel (`wait` at end). ~1.5s warm save (corrected from inflated 4-10s claim per `a22adf88`). 30 LOC. Owner: `scripts/check.sh`.

**V4 — worktree janitor.** launchd plist + sweep script: prune `.claude/worktrees/agent-*` with no live agent + merged branch. 354MB/129 locks confirmed. ~50 LOC bash + plist. Owner: `scripts/worktree-janitor.sh` + `.claude/launchd/`.

**V5 — pre-PR base-staleness gate.** `gh pr create` wrapper or pre-push hook: `git fetch && git merge-base --is-ancestor origin/main HEAD || reject`. Plus file-overlap detection vs other open PRs (catches sub-disjoint conflicts). 60-90 LOC bash + tests (corrected from `ab083481`'s "~30 LOC" per `ac7cc5a4`). Catches PR #151-class. Owner: `scripts/check-pr-base.sh`.

**V6 — placeholder-detection script for `feat/` branches.** Grep gate: `func _ =`, unused struct fields, `TODO(impl)` patterns in non-test Go files of feat/ PRs. ~30 LOC. Catches PR #146/#148 class. Owner: `scripts/check-placeholders.sh`.

**V7 — Anthropic prompt-cache** (sequential after V1; needs cost measurement). Measure `internal/reasoner/` system-prompt token count first; if >1024 tokens, add `cache_control: {type: "ephemeral"}` on the system block per Anthropic 2026 cache semantics. Cache-read = 10% input cost, cache-write = 125% — savings only material above the 1024-token threshold. Range-quantify before merge. Owner: `internal/reasoner/`.

## 4. P1 — wave-9b (next wave)

**V8 — Engine.OnSignal event-driven refactor.** On the W75/W77 SSE substrate, `recommend.Engine` subscribes to context_transition events instead of polling each daemon tick. **Two-layer debounce required** (per `a43f957d`: NSWorkspace fires 10-30×/min): (a) upstream NSWorkspace 250ms coalescing window at the producer; (b) per-pattern debounce — min 30s between same-pattern `Propose` calls downstream of the NSWorkspace 250ms coalesce, so a single recurring app-switch pattern cannot saturate the proposer. Owner: `internal/recommend/`.

**V9 — NSWorkspace push for activeapp.** Replace osascript pull with `NSWorkspaceDidActivateApplicationNotification` via CGO bridge. EventKit calendar push **defer to wave-10** — `docs/engineer/specs/2026-06-10-macos-ecosystem-integration.md:158` already marks it Defer. Owner: `internal/macos/activeapp/`.

**V10 — Streaming Reasoner → Streaming TTS.** 250-400 LOC across 6-8 files per `a8b4f14d`: requires Client interface streaming + budget-attribution refactor + Anthropic SDK Go stream API + sentence-boundary buffer + Kokoro stdin streaming. **Gate on V1 baseline measurement** — without histograms there is no way to validate the claimed 600ms reasoner-stall kill. Owner: `internal/reasoner/`, `internal/voice/tts.go`, `internal/voice/session/`.

## 5. P2 — REJECTED proposals (do not re-propose)

Listed so the next wave's brainstorming does not re-derive them:

- **Reviewer GH App / bot identity laundering.** Contradicts `CLAUDE.md:24-25` "Never self-approve" structurally. A different `gh` identity is still not an independent human review. `a22adf88` REJECT.
- **Spec-PR sub-disjoint exemption.** Root cause of the serialize rule (`CLAUDE.md:14-16`) is stale-base regression, not content overlap. Sub-disjoint check is moot. `a22adf88` REJECT.
- **P14 instant `/admin/forget` HTTP endpoint.** Daemon admin endpoint doesn't exist; mechanism fictional. `ac7cc5a4` WRONG.
- **P6 dropping `-count=1` in CI.** Conflates Go build-cache (already on via `setup-go@v5`) with test-result-cache (intentionally disabled for hermeticity). `a22adf88` WRONG.
- **P10 reviewer SHA cache.** Cache key never hits — rebase changes SHA. Internally broken. `a22adf88` WRONG.
- **Reflexion 40% catch / 20% cost claim.** Invented numbers; Shinn et al. 2023 numbers vary by task and don't transfer to Go PR review. `ac7cc5a4` UNSOURCED. Re-propose only with a real local measurement.
- **Kokoro warm-pool "20ms" claim.** Real floor is afplay-bound 50-80ms, not synth-bound. `a8b4f14d` OVERSTATED. Warm-pool still worth doing, but quote the right number.
- **Attestation cache for `read_*` routes.** `read_*` is BR=0, no friction to cache. The friction lives on `send_*`/`pay_*`/`delete_*` (BR≥3) and those MUST NOT be cached per voice-comm spec §7. Inverts where value lives. `a43f957d` WRONG.

## 5a. ROI revision (adversary-corrected)

Proposer headline numbers were inflated. Adversarial verdicts trimmed them — use these revised figures in any downstream planning:

- **Dev-velocity savings**: ~5 min/day, not ~30 min/day (per `a22adf88` REJECT-heavy verdict on `a76c79f2`: `check.sh` parallel-stage real savings ≈1.5s warm, worktree janitor is one-time disk reclaim, and the proposer summed best-case wall-clock across stages that already overlap with build+test).
- **Feedback-loop ops cost saved**: 4-8 operator-hr/wk, not 12-18 op-hr/wk (per `ac7cc5a4` ACCEPT-WITH-AMENDMENTS verdict on `ab083481`: base-staleness gate + placeholder grep are real but the proposer's 12-18 hr figure assumed every PR #151/#146/#148 class regression cost ~2 op-hr; verdict trimmed to the observed ≤1 op-hr-per-regression rate over the last fortnight).
- Operational-responsiveness and product-responsiveness proposers (`af05ba78`, `a8b74b22`) made no headline ROI claim; their verdicts (`a8b4f14d`, `a43f957d`) ACCEPT-WITH-AMENDMENTS at the per-finding level only.

## 6. Critical fix-FIRST gates

1. **V1 before V10.** Streaming Reasoner+TTS optimization without a baseline histogram is unverifiable. Hard sequence.
2. **Operator-trust default = HUD banner**, not voice phrase (privacy in public space). V12-class change goes into V2 PR.
3. **Lifelog composer gated on non-empty day.** Skip composition if audit+mirror+feed observation-count over the window is below threshold — protects Anthropic budget on vacation days.
4. **HUD-armed voice via Wails IPC, not WebRTC.** Defer until Wails v3 adoption per `docs/engineer/specs/2026-06-10-wails-decision.md`.

## 7. Sequencing

**Wave-9a (this wave)**: V1, V2, V3, V4, V5, V6 dispatch in parallel — six file-disjoint PRs, cap-6 saturation. V7 dispatches after V1 lands (needs system-prompt token count from V1's measurement scaffold).

**Wave-9b (next wave)**: V8, V9 parallel. V10 single-PR, gated on V1's baseline being live in production for ≥3 voice turns so optimization claims are falsifiable.

## 8. Source citations (per kept item)

- F1 ← `af05ba78` voice section + `a8b4f14d` finding "p95 1.5s is SPEC TARGET not measurement".
- F2 ← `af05ba78` HUD section + `a8b4f14d` finding "ambient.js still polls /api/state alongside its SSE; recommendations.js has no SSE" (`ambient.js:54,65`, `recommendations.js:100`).
- F3 ← `af05ba78` mirror section + `a8b4f14d` "FSEvents 1-5s coalescence, not universal <1s".
- F4 ← `af05ba78` cache section + `a8b4f14d` "cache-write 125%, cache-read 10%; savings require >1024 token system prompt".
- F5 ← `af05ba78` streaming section + `a8b4f14d` "Anthropic Go SDK stream API exists".
- F6 ← `ab083481` P5 + `ac7cc5a4` "LOC OVERSTATED-LOW, real 60-90".
- F7 ← `a76c79f2` P1 + `a22adf88` "savings OVERSTATED, real ≈1.5s warm".
- F8 ← `a76c79f2` P7 + `a22adf88` "VERIFIED exactly 134 worktrees".
- F9 ← `ab083481` P6 + `ac7cc5a4` "placeholder grep gate cheap".
- F10 ← `a8b74b22` Engine.OnSignal + `a43f957d` "requires debounce".
- F11 ← `a8b74b22` activeapp + `a43f957d` "CGO real, EventKit defer per spec:158".
- F12 ← `a8b74b22` trust panel + `a43f957d` "invert default".
- F13 ← `a8b74b22` lifelog + `a43f957d` "gate on non-empty day".
- F14 ← `a8b74b22` HUD-arm + `a43f957d` "Wails IPC not WebRTC".
