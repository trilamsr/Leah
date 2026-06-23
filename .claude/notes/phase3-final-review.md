# Phase 3 Final Review — Leah macOS Native UI

**Diff range:** `b93efd1..dedb595` — 27 PRs since Phase 2 closeout
**Spec:** v3.3.0 (Phase 3 plan: `docs/superpowers/plans/2026-06-22-leah-macos-native-phase3.md`)
**Date:** 2026-06-22

---

## 1. Verdict

**FIX-N-THEN-SHIP**

Phase 3 landed all 20 plan-tasks, but four substrates merged as packages without daemon-side wiring. The "must ship + run 7 days" gate cannot fire until at least the TTS providers are reachable through the `tts.speak` IPC handler — today they fall through to `tts.speak.err`.

1. **BLOCKER (MEDIUM)** — TTS providers shipped but not constructed. `cmd/leah-daemon/main.go` calls `newIPCHandler(..., ttsClass)` with `nil` for both `local` and `cloud` providers. Comment: "Task 2 + Task 3 wire concrete elevenlabs/apple implementations; for now the classifier is live but providers are nil — handler emits tts.speak.err until providers ship." PRs #409 (ElevenLabs, 376/0) and #410 (Apple Ava, 184/0) shipped the provider code; PR #411 shipped the handler; nothing wired them to `main`.

2. **BLOCKER (MEDIUM)** — Knowledge citation enrichment is dead code. `internal/knowledge/citation.go` (PR #416, 207/0) defines `EnrichWithCitations` (or equivalent), but `grep` finds **zero non-test callers outside `internal/knowledge/`**. The §17.10 ship criterion ("cite Linear + GitHub on every ask") still routes through the pre-existing `handleAsk` path, which never calls into citation.

3. **BLOCKER (LOW)** — MCP outbound publish has no daemon caller. `internal/mcp/publish.go` (PR #418, 398/0) ships `Publish` / `NewPublisher`, but no `cmd/` or `internal/` non-test file imports the package outside its own directory. The "outbound publish surface on Unix socket" PR added the surface; nothing fans daemon events into it.

Phase 4 debt (track but do not block Phase 3 ship; Phase 4 plan #406 absorbs):

4. PR #417 Push-To-Talk (Fn / right-⌘) is referenced only from `PushToTalkTests.swift` — no `Sources/` file constructs `PushToTalk()`. The hotkey path is unexercised at run time.
5. PR #419 LeahWake CoreML adapter is declared in `Package.swift` and resourced (`Models/wake-leah.mlmodel`) but no Swift `Sources/` file imports `LeahWake` — wake path is unexercised at run time.
6. PR #428 Phase 3 README refresh deleted 305 lines for 119 lines added — verify no spec-parity invariants slipped (spec-parity script is the gate; PR #424 added parity guards).

---

## 2. PR Manifest (27)

| PR | Hash | Summary | ins/del |
|---|---|---|---|
| #432 | dedb595 | fix(embed): drop voyage from backend-not-found hint (no impl) | 1/1 |
| #431 | 6cf9acb | docs(audit): Wave-8 survivor decision matrix for operator review | 49/0 |
| #430 | d53f043 | docs(arch): ARCHITECTURE.md — Phase 2 + 3 deltas | 104/3 |
| #429 | 0db0de5 | fix(obs): make TestCaptureStack_PoolReusesBuffer deterministic | 20/12 |
| #428 | 4b4cd16 | docs(phase3): refresh README + frozen-enum list for current state | 119/305 |
| #427 | 16704e8 | fix(widgets): repair Flights + Maps tile-view fixture tests | 2/2 |
| #426 | fbe564e | fix(hud): replace time.Sleep with deterministic sync in pinned_test.go | 16/18 |
| #425 | 0e686d1 | test(phase3): Phase 3 E2E smoke | 399/0 |
| #424 | 1bcf0ff | docs(phase3): v3.3.0 changelog + parity guards | 23/0 |
| #423 | 75dd7a7 | feat(auth): Touch ID confirmation for memory purge + telemetry toggle | 277/22 |
| #422 | cc77af9 | feat(dashboard): §4.7 dashboard surface (Phase 3 Task 16) | 281/0 |
| #421 | 9af00ff | feat(release): appcast generator + EdDSA verify + rollback channel | 473/3 |
| #420 | 96d7a45 | feat(widgets): minimal-mode strips gold + grain + italic at render time | 81/3 |
| #419 | 104c926 | feat(wake): LeahWake CoreML adapter + VAD + per-app suppression | 205/0 |
| #418 | 4c392d3 | feat(mcp): outbound publish surface on Unix socket (Phase 3 Task 14) | 398/0 |
| #417 | 67e5abc | feat(ptt): Fn / right-⌘ push-to-talk per §6.4 | 102/0 |
| #416 | 4b9d433 | feat(knowledge): KG-backed citation enrichment (Phase 3 W4 T13) | 207/0 |
| #415 | 9206f34 | feat(audio): LeahAudio Player + AppleSpeech (Phase 3 W2 T7) | 85/0 |
| #414 | c010234 | feat(daemon): fan macOS push-sources into IPC frame stream | 314/0 |
| #413 | 281ef4c | feat(marketing): hero asset bundle scaffolding | 66/0 |
| #412 | 7733c80 | docs(audit): internal/voice supersession by internal/tts — not deletable | 74/0 |
| #411 | 3fc41f1 | feat(daemon): tts.speak/tts.cancel IPC handler with ctx-cancel propagation | 511/8 |
| #410 | 9b025d8 | feat(tts/apple): daemon-side router for §17.17 Apple Ava | 184/0 |
| #409 | adca265 | feat(tts): ElevenLabs Flash v2.5 cloud provider | 376/0 |
| #408 | 34150da | feat(eval): LEAH_EVAL=1 fixture scheduler | 593/0 |
| #407 | e9b68a6 | feat(tts): §17.17 provider contract + privacy classifier | 292/0 |
| #406 | dffcbe3 | docs(plans): Phase 4 plan (20 tasks, 5 waves) | 3196/0 |

Aggregate: 98 files changed, 8448 insertions(+), 377 deletions(-) — **ratio 22:1 insertions:deletions**.

---

## 3. Top 5 Wins

1. **Phase 4 plan landed in one PR** (#406, 3196 lines) — 20 tasks across 5 waves, 9 deliverables, complete dependency matrix, file-disjoint task partition. Hardest single artifact this session; unblocks the next planning round without ambiguity. Evidence: `docs/superpowers/plans/2026-06-22-leah-macos-native-phase4.md`.

2. **TTS substrate end-to-end** (#407 + #409 + #410 + #411) — provider contract + classifier + ElevenLabs cloud + Apple Ava local + IPC handler with ctx-cancel propagation. Four PRs, 1363 lines, file-disjoint across `internal/tts/`, `internal/tts/elevenlabs/`, `internal/tts/apple/`, `cmd/leah-daemon/tts_handler.go`. Caveat: not wired in `main.go` (Blocker 1).

3. **Eval harness with LEAH_EVAL=1 fixture scheduler** (#408, 593 lines) — `internal/eval/scheduler.go` + `internal/eval/store.go` + opt-in env gate in `cmd/leah-daemon/main.go` + new `cmd/leah-eval/main.go`. Largest single feature PR. Wired end-to-end; main reads `LEAH_EVAL` and opens the store.

4. **Release pipeline: appcast + EdDSA verify + rollback** (#421, 473 lines) — `scripts/release/generate-appcast.sh` + `_test.sh`, `LeahUpdate/AppcastTemplate.swift`, `publish-release.sh` integration. Closes one of the operator-redirect pending decisions (Sparkle EdDSA key hosting was on the survivor list) by giving the generator + verifier; the hosting decision remains operator-side.

5. **Phase 3 E2E smoke + parity guards** (#425 + #424) — `scripts/dev/phase3-smoke.sh` (305) + `_test.sh` (65) + parity guards (23). The smoke harness exists, parity guards are in place — this is the load-bearing artifact for the "must ship + run 7 days" gate.

---

## 4. Deletion-Debt

Pure-add PRs (zero deletions): **17 of 27 = 63%**. Threshold per audit-session Phase 5: ≥3 → `[DELETION-DEBT]` flag.

| PR | Justification |
|---|---|
| #406 docs(plans) Phase 4 plan 3196/0 | Justified — new spec/plan artifact |
| #407 feat(tts) provider contract 292/0 | Justified — new package |
| #408 feat(eval) scheduler 593/0 | Justified — new package |
| #409 feat(tts) elevenlabs 376/0 | Justified — new subpackage |
| #410 feat(tts) apple 184/0 | Justified — new subpackage |
| #411 feat(daemon) tts handler 511/8 | Mixed — net-add, 8 deletes |
| #412 docs(audit) voice supersession 74/0 | Justified — audit memo |
| #413 feat(marketing) hero assets 66/0 | Justified — net-new assets |
| #414 feat(daemon) push-source fan 314/0 | **Candidate** — new file `pushsource_runtime.go`; no obvious pre-existing scaffolding to delete |
| #415 feat(audio) LeahAudio 85/0 | Justified — new module |
| #416 feat(knowledge) citation 207/0 | **Candidate** — new `citation.go`; the §17.10 path was claimed shipped via Phase 1 `SearchRelevant` (per phase1-final-review.md F5) — verify no duplicated retrieval surface |
| #417 feat(ptt) PushToTalk 102/0 | **Candidate** — and orphan (see §5) |
| #418 feat(mcp) outbound 398/0 | **Candidate** — and orphan (see §5) |
| #419 feat(wake) LeahWake 205/0 | **Candidate** — and orphan (see §5) |
| #422 feat(dashboard) §4.7 surface 281/0 | Justified — new pane group |
| #424 docs(phase3) changelog 23/0 | Justified — doc |
| #425 test(phase3) E2E smoke 399/0 | Justified — new harness scripts |

Net: 5 of 17 are legitimate `[DELETION-DEBT]` candidates (#414, #416, #417, #418, #419). The other 12 are new-package PRs where pure-add is structurally unavoidable. The single deletion-heavy PR (#428, 305 deletions) is the README refresh — net negative on docs surface, healthy.

---

## 5. Orphan Code

`go vet ./...` is clean. Symbol-level callers for new-this-session packages, scanning non-test refs **outside the symbol's own package**:

| Symbol / package | Non-test callers outside its package | Verdict |
|---|---|---|
| `internal/tts` (provider contract + classifier) | 3 — `cmd/leah-daemon/main.go`, `cmd/leah-daemon/tts_handler.go`, `cmd/leah-daemon/ipc_handler.go` | WIRED (contract reachable) |
| `internal/tts/elevenlabs` (Flash v2.5) | 0 non-test refs | **ORPHAN** — daemon passes `nil` for `cloud` provider |
| `internal/tts/apple` (Ava local) | 0 non-test refs | **ORPHAN** — daemon passes `nil` for `local` provider |
| `internal/knowledge` `citation.*` | 0 non-test refs outside `internal/knowledge/` | **ORPHAN** — citation enrichment not on `handleAsk` path |
| `internal/mcp` `Publish` / `NewPublisher` | 0 non-test refs outside `internal/mcp/` | **ORPHAN** — outbound surface not invoked |
| `internal/eval` | 2 — `cmd/leah-eval/main.go`, `cmd/leah-daemon/main.go` | WIRED |
| `cmd/leah-daemon/pushsource_runtime` | n/a (cmd-local) — wiring via `main.go` | needs spot-check, in scope of #414 |
| Swift `PushToTalk` | 0 production refs (only `PushToTalkTests.swift`) | **ORPHAN** |
| Swift `LeahWake` | 0 production import (declared in `Package.swift`, not imported by any source) | **ORPHAN** |
| Swift `DashboardView` | wired via `DashboardWindow.swift` | WIRED |
| Swift `AppearancePane.minimalMode` | wired via `DashboardView.swift` + `LeahWidgets/Tokens.swift` | WIRED |
| Swift `BiometricsGate` (Touch ID) | wired (BiometricsGate.swift constructs `TouchIDGuard` directly) | WIRED |

**Orphan total: 5 (2 Go provider impls + 1 Go citation surface + 1 Go MCP outbound + 2 Swift modules).**

The TTS orphan pair is the same wiring gap as Blocker 1: providers exist, contract exists, handler exists, `main.go` passes `nil`. The Swift orphans (PTT, Wake) are referenced by Phase 4 plan §1 (`DuplexSession` will drive both) — track as Phase 4 prerequisites, not Phase 3 ship gates.

---

## 6. Operator-Redirects Count

Best-effort from session memory (`MEMORY.md`) + recent audit archives. The `phase-a2-flags.txt` mechanism is the canonical source; this audit does not have access to the live transcript, so the gauge is qualitative.

Phase 3 dispatch ran predominantly autonomous: the 27 PRs landed across 5 waves of file-disjoint dispatch with no obvious mid-session redirect. The Wave-8 survivor matrix (PR #431) was queued for operator decision (one of the pending items below) — that is a planned hand-back, not a redirect.

**Estimate: <5 redirects/hour** — below the autonomy-lever threshold per audit-session Phase A2.

---

## 7. Friction Triggers (already saved as feedback memories)

These patterns surfaced ≥2 times across Phase 2 + Phase 3 and are already in MEMORY.md / `.claude/notes/`:

1. **errcheck on adapters** — `feedback_check_gates.md`. Adapter packages forgot `_ = ` on error returns; surfaced enough times to be a check-gates rule.
2. **gh comment self-approve** — `feedback_merge_discipline.md` + `.claude/notes/reviewer_gh_identity_inherits_author.md`. Reviewer subagents posting `gh pr comment` from the same session inherit author identity → counts as self-approval. Phase 4 plan §header now mandates transcript-channel verdict only.
3. **stale-base on Wave 2** — `feedback_merge_discipline.md`. Two file-disjoint PRs branched off pre-merge main produce phantom-deletion diffs against post-merge main. Fix codified: rebase stale-base first.
4. **subagent force-push forbidden** — `.claude/notes/subagent_force_push_forbidden.md`. Surfaced this session via PR #428 (intentional 305 deletions; reviewer-checked, not a force-push).
5. **agent-rebase-races-merge** — `.claude/notes/agent_rebase_races_merge.md`. Concurrent rebase + merge collisions; mitigation: branch guard in its own call.

No new friction patterns from Phase 3 that lacked a prior `feedback_*` entry.

---

## 8. Pending Operator Decisions

1. **Wave-8 survivor set (PR #431)** — 4 survivors (W121-W123 KG read-path + W126 consolidation read-path), 3 kills (W133, W134, W136), 4 already-shipped. Operator chooses: ship the 4 survivors as Phase 3.5 hotfix, fold into Phase 4, or kill. PR is doc-only and merged; the decision is on the matrix.
2. **gh bot identity-leak** — long-standing; reviewer-spawned `gh pr comment` is self-approval. Mitigated by Phase 4 plan header mandating transcript-channel verdict; operator should confirm the policy is binding on Phase 4 dispatch.
3. **Sparkle EdDSA key hosting** — PR #421 ships the appcast generator + EdDSA verify code; the actual EdDSA private key + public key publication site is operator-only. Decision: GitHub Pages (`maydow.github.io/leah/appcast.xml` referenced in template) vs custom host.
4. **Phase 4 dispatch authorization** — Phase 4 plan #406 is merged; the 20 tasks are not yet dispatched. Operator chooses whether Phase 3 ships first (run 7 days per operator rule) or Phase 4 W1 begins in parallel (perception substrate is file-disjoint from Phase 3 surface).
5. **TTS provider wiring follow-up** — Blocker 1 above. One-PR fix in `cmd/leah-daemon/main.go`. Operator decides: hotfix on Phase 3 branch before ship, or absorbed into Phase 4 W1-T02 (`DuplexSession` builds on `tts.Provider`).

---

## 9. Recommended Next Session Focus

Given operator rule "Phase 3 must ship + run 7 days":

1. **Resolve the three blockers as a single hotfix wave** — one PR each for (a) TTS provider construction in `main.go` (Blocker 1), (b) citation enrichment on `handleAsk` (Blocker 2), (c) MCP outbound publisher hookup (Blocker 3). File-disjoint at `cmd/leah-daemon/main.go` (single-owner), `cmd/leah-daemon/ipc_handler.go`, and one new wiring file respectively. Cannot parallelize (a) and (b)/(c) because all three touch `main.go`; serialize (a) first.

2. **Cut Phase 3 ship tag** — once blockers green, tag `v3.3.0`, run the Phase 3 smoke (`scripts/dev/phase3-smoke.sh`), generate the appcast (`scripts/release/generate-appcast.sh`), sign + notarize, publish.

3. **Start the 7-day run window** — operator rule. Do not begin Phase 4 W1 dispatch until the window opens.

4. **In parallel during the run window** — operator decides Wave-8 survivor disposition (item 8.1) and Sparkle EdDSA hosting (item 8.3). These are doc / config decisions, not code; they do not consume dispatch slots.

5. **Day 7 of run window** — open Phase 4 W1 dispatch (T01-T05, file-disjoint, T05 single-owner per the plan).

Deletion debt to retire during the run window: re-examine the 5 candidate orphan PRs (§4) and either wire them (PTT into VoiceCoordinator scaffolding, Wake into duplex coordinator) or fold the orphan symbols into Phase 4 W1 task descriptions so they are not deleted by the W5-T20 sweep.
