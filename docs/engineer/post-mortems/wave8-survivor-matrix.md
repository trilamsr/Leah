---
title: Wave-8 survivor decision matrix — operator review 2026-06-23
audience: operator
purpose: rubber-stamp the survivor set (KILL / FOLD / KEEP / SHIPPED) for Wave-8 W121-W142 before any code agent loads
input: issue #249 unshipped set + brief 2026-06-10-wave-8-aiml-upgrade.md + tree state on main@8a69180
companion: docs/engineer/post-mortems/2026-06-23-wave-8-survivor-decision-matrix.md (long-form rationale)
---

# Wave-8 survivor decision matrix — operator review 2026-06-23

Issue #249 closed Wave-8 against 9 impl waves marked NOT YET shipped: **W121, W122, W123, W125, W126, W127, W133, W134, W136, W142, W143** (11 ids; W125/W127/W142/W143 verified shipped post-#249, see ALREADY SHIPPED below). This page is the operator pre-spec — pick the survivor set, sign off, then code agents may load.

## Matrix

| W#   | Title                                              | Status              | Classification     | Reason (1-line)                                                                                  | Action                                            |
|------|----------------------------------------------------|---------------------|--------------------|--------------------------------------------------------------------------------------------------|---------------------------------------------------|
| W121 | `Graph.QueryLive` + cap                            | 0% (spec only)      | WAVE-8.1 KEEP      | W120 demotion column landed but no ranked-live reader; W120 weight-filter is half-wired.         | Dispatch against knowledge-graph-wiring §3+§7     |
| W122 | TTL + demotion logic (`internal/knowledge/ttl.go`) | 0% (spec only)      | WAVE-8.1 KEEP      | Without time-decay update path, the "Sarah is ex-coworker" use case in S8 §3 stays broken.       | Dispatch after W121 (additive file, file-disjoint)|
| W123 | `OnSignal` consumes Graph + `leah forget --relation` | 0% (spec only)    | WAVE-8.1 KEEP      | Operator-facing closeout: no command exists to forget a relation; S8 "flip-day wave" never flipped.| Dispatch after W122 (touches dispatcher + cmd/leah)|
| W125 | `ConsolidatePass` nightly pipeline                 | SHIPPED PR #219     | ALREADY SHIPPED    | `internal/operatormodel/consolidate.go` + 7 tests live; happy-path covered.                      | Mark complete in #249; file edge-case test backfill separately |
| W126 | Dual-read Update + `consolidated_loader.go`        | 0% (spec only)      | WAVE-8.1 KEEP      | W125+W127 write to `operator_profile_consolidated` nightly but no reader exists → write-only.    | Dispatch against memory-consolidation §8 (W126 row L291) |
| W127 | Daemon Daily-task wiring + replay verb             | SHIPPED             | ALREADY SHIPPED    | `cmd/leah-daemon/consolidate.go` + `cmd/leah/suggest.go runSuggestReplay` end-to-end.            | Mark complete in #249 (note spec→tree path drift) |
| W133 | `LEAH_LOCAL_ONLY=1` + egress verification          | 0% (spec only)      | KILL               | Trust-moats.md:122 itself flags M4 blocked on V10 Ollama reasoner (not shipped); ships a stub.   | Drop; revisit only after V10 lands                |
| W134 | `docs/operator/data-flow.svg`                      | 0% (file absent)    | KILL               | "An afternoon of work" doc labor; zero code; Phase-4 trust surface is continuous attestation not static SVG. | Drop; absorb into README + `leah whoami` if needed |
| W136 | Per-category memory attestation at first sight    | 0% (spec only)      | KILL               | S10 §11 says M7 ships "dormant — operator sees no behavior change until tagging lands"; upstream tagging wave never authored. | Drop; gate on tagging wave first                  |
| W142 | Sparkle EdDSA appcast generator + verify + rollback| SHIPPED PR #353+#421 | ALREADY SHIPPED    | Swift impl in `app/Leah/Sources/LeahUpdate/` covers generator, verify-on-install, rollback channel.| Mark complete in #249; key custody + hosting URL are operator-side |
| W143 | Brew formula auto-PR script                        | PARTIAL PR #263     | PARTIAL            | Tarball publish wired in `release.yml:75-76`; `scripts/release/update-brew-formula.sh` auto-PR is courtesy gap. Spec itself says human-gate is intentional. | Discharge as shipped-enough; file new ticket if auto-PR ever bites |

## Recommended Wave-8.1 batch — 4 KEEP items, sequenced

Two coherent closeouts. File-disjoint cap honored: W121+W122 touch `internal/knowledge/`; W123 touches `cmd/leah/` + dispatcher; W126 touches `internal/operatormodel/` — all disjoint, dispatchable in parallel only where dependency-clean.

1. **W121** — `Graph.QueryLive` type + cap on `internal/knowledge/graph.go` + `storage.go`. (no dep)
2. **W122** — `internal/knowledge/ttl.go` + test + one-line hook into `graph.Update`. (dep: W121)
3. **W123** — Dispatcher `OnSignal` consumes `Graph.QueryLive` + `--relation` flag on `cmd/leah/forget.go`. (dep: W121+W122; operator-facing closeout)
4. **W126** — Dual-read Update path + `consolidated_loader.go` in `internal/operatormodel/`. (dep: W125 shipped; parallel with W121/W122 — file-disjoint)

Dispatch order: W121 + W126 in parallel (file-disjoint, no shared dep), then W122 (waits on W121), then W123 (waits on W122). Total: 2 waves, ≤4 PRs.

No new spec PR needed — surviving waves are spec'd in `docs/engineer/specs/2026-06-10-knowledge-graph-wiring.md §7` (W121-W123) and `2026-06-10-memory-consolidation.md §8` (W126 row L291).

## Why these survive vs the rest

- **Survivors discharge shipped-but-inert debt.** W120 (knowledge-graph) and W125+W127 (consolidation) already landed write paths — the survivors complete the read path that makes those waves observable. Closing them is regressed-feature repair, not net-new scope.
- **Kills are S10 trust-moat polish with no pull signal.** W133/W134/W136 are not in the operator's "path-a-full personal-use" memory, not on the Phase-4 critical path, and W133/W136 are blocked on upstream waves (V10 Ollama, category-tagging) that have not been authored.
- **W125/W127/W142 are post-#249 ships.** Reissue #249 closing with these references.

## Operator decision

- [ ] Approve **KILL** set: W133, W134, W136 (drop from backlog; do not dispatch)
- [ ] Approve **ALREADY SHIPPED** set: W125, W127, W142 (close #249 against these; file test-backfill ticket separately if desired)
- [ ] Approve **PARTIAL** discharge: W143 (mark shipped-enough; auto-PR courtesy only)
- [ ] Approve **Wave-8.1 KEEP batch**: W121 → W122 → W123 closeout + W126 closeout (dispatch order above)
- [ ] Or: redirect (sequence differently, add/drop items, defer Wave-8.1 to post-Phase-4)

## Notes

1. Existing long-form audit at `docs/engineer/post-mortems/2026-06-23-wave-8-survivor-decision-matrix.md` carries the file-by-file tree-state evidence (49 lines of prose); this page is the operator-facing decision surface. The two are consistent — same survivor set, same kill set.
2. Spec-vs-tree drift to record on #249 close: W127 replay shipped inline in `cmd/leah/suggest.go` (not `internal/operatormodel/replay.go`); W142 shipped in Swift (not Go). Both functionally complete.
3. Residual test-coverage debt on W125 (9 spec'd test cases, ~4 covered) + W127 (`TestReplay_ReproducesProfile` golden-replay) is a backfill ticket, NOT a Wave-8 survivor.
4. No Phase 5 spec exists on disk as of `main@8a69180`. The "FOLD INTO PHASE 5" classification was not used because no Phase 5 scope is defined to fold into; if a sibling agent authors one, W121-W123 + W126 could move there instead of a Wave-8.1 batch — operator call.
