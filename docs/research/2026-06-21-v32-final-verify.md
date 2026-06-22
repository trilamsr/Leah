# v3.2 Spec Final Verification

**Spec:** `docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md` (2724 lines)
**Date:** 2026-06-21
**Verdict:** **SPEC-READY-FOR-IMPL** (with 1 nit — see Finding F1).

---

## 1. Parity script execution

- **Exit code:** `0` (clean — "no forbidden phrases outside the allow-list").
- **Allow-list logic:** sound, but moderately liberal. Whitelist lines containing `(formerly`, `v1:|v2:|v3:|v3.1|v3.2`, `was`, `formerly`, `Killed by`, `Reviewer fix`, `SCShareableContent <stream|observer|enumerate|...>`, `<N pin`. These markers ARE present on every flagged line outside §14/§15/§16/§18, and each flagged line is genuinely a historical citation — no false-negatives observed in the manual sweep.
- ✅ PASS.

## 2. Grep sweep (residual regressions)

Run against each forbidden token. All hits cross-checked against allow-list rules (§14/§15/§16/§18 OR `v1`/`was`/`formerly` markers OR §3.5 single heraldic paragraph):

| Token | Hits | Outside-allow-list & unjustified |
|-------|------|----------------------------------|
| `\bchamber\b` (i) | 31 | 0 (only L279 header `(formerly "focus chamber")`) |
| `\bsigil\b` (i) | 19 | 0 (L183 header `(formerly "the sigil")`; L187 §3.5 heraldic paragraph — explicitly permitted) |
| `Flourish [12]` | 3 | 0 (all §14/§18) |
| `aesthetic-reduced` | 4 | 0 (all §14/§15/§18) |
| `gold seam` | 1 | 0 (§18 rename narration) |
| `#0A0A0C` | 1 | 0 (§18 palette-lock narration) |
| `⌘⌃` | 5 | 0 (L46 §1 v1-row, L418 §6 "Why ⌥Space and not ⌘⌃ (v1)", rest in §14/§18) |
| `\b90 ?s\b` / `idle 90` | 7 | 0 (L287 §4 reviewer-fix, L441 §6 reviewer-fix, rest in §14/§18) |
| `max 3 pin` / `3 pinned` | 7 | 0 (L264 §4 + L1466 §10 + L1948 §13 all carry `was "3"` / `v3.1 fix` markers) |
| `SCShareableContent` | 11 | 0 (L57 §1 v1-row, L460 §6 with `v3.1 — implementability fix A-3`, L1804 §10 enumerate-only-use case, rest in §14/§15/§18) |
| `wake-word ON` | 1 | 0 (§18 narration) |

- ✅ PASS — every hit either lives in the allow-list or carries a parity-script-recognized historical marker.

## 3. Cross-section invariants

- ✅ Hotkey = `⌥Space` everywhere normative (42 occurrences; `⌘⌃` only as v1 citation).
- ✅ Wake-word OFF by default — L5/L18/L47/L431/L751/L862 normative; L2080/L2089/L2125 decision-log.
- ✅ Light + dark mode parity claimed + specced — §3 palette tokens for both; §13.4 + §13.4-light wireframes; §11 contrast tables for both.
- ✅ Gold budget = max 3 instances per surface (decision #112, L2189). Decision #39 explicitly marked `[SUPERSEDED by #112]` at L2116.
- ✅ Type stack = Inter + New York Italic primary (decision #107 L2184; §3.3 L166-L169).
- ✅ Pin cap = 2 widgets (decision #40 referenced at L264, L580, L1466, L1948).
- ✅ Distribution = Developer ID + Sparkle via GitHub Releases (decision #101 L2178; §17.1 L2404; §19 L2649).
- ✅ macOS minimum = 14.0 Sonoma (decision #104 L2181; §17.5 L2460).
- ✅ IPC = JSON over `~/Library/Caches/Leah/leah.sock` (decision #105 L2182; §17.2 L2439). Note: L1613 §10 still says `Application Support/Leah/leah.sock` — confirmed superseded by decision #105 wording at L2182 and §17.2 normative path at L2439.
- ✅ JSON Schema = qri-io draft-07 (decision #106 L2183; §17.2 L2440).
- ✅ TTS canon = ElevenLabs Leah voice; Samantha offline-only fallback (decision #109 L2186; §2.7 L81-L84). **See Finding F1 below.**
- ✅ Wake-word `.mlmodel` at `Leah.app/Contents/Resources/Models/wake-leah.mlmodel` (L513 §6.7; L2444 §17.2).

### Finding F1 — Phase 3 TTS ordering contradicts §2.7 canon (NIT)

**Line 2674 (§19 Phase 3):**
> **TTS subsystem** — Samantha first (offline-safe default), then ElevenLabs Leah voice per §2.7.

This phrasing reads as "ship Samantha as default first, swap in ElevenLabs later," which contradicts §2.7 (L81-L84) and decision #109 (L2186) — both state ElevenLabs Leah voice IS the canonical default, Samantha is offline-fallback only. Build-order intent is likely "implement Samantha plumbing first because it's offline, then layer in ElevenLabs cloud," but the wording inverts the canon.

- **Fix:** rewrite L2674 as
  `**TTS subsystem** — ElevenLabs Leah voice as canonical default (cloud, pre-warmed on launch), with Samantha as offline fallback per §2.7. Implementation order: Samantha adapter first (offline-safe), then ElevenLabs integration once API key flow ships.`

Non-blocking. Plan author can read intent correctly; downstream reviewer would catch it. Suggest fix before Phase 3 plan dispatch.

## 4. Section completeness — §19 deliverables ↔ §3-§13 spec backing

Walked all 23 Phase 1/2/3 deliverables. Each maps to a specced surface/feature:

- Phase 1 (12 items): all backed (focus panel §4.3, hotkey §1+§6, NSPanel §17.2, menubar §6.4, wizard §8, Settings §9, Developer ID §17.1, dark palette §3, widget primitives §10.1, knowledge §17.10, daemon LLM §6+§17.10, memory §17.10).
- Phase 2 (8 items): all backed (ambient HUD §4.1/§7.1, notification §10/§13.7, 10 widgets §10.4-10.13, gallery §13.10, pin §10.3, light §3+§13.4-light, Settings §9, wizard step 4 §8).
- Phase 3 (8 items): all backed (TTS §2.7, wake-word §6.7, VAD §6.7, PTT §6.6, minimal mode §3+decision #110, dashboard §4.4, Touch ID §17.13, hero §17.12).
- ✅ PASS — no orphan deliverable.

## 5. Wireframe sanity (§13)

- ✅ `⌥Space` chrome in §13.3, §13.4, §13.4-light, §13.5, §13.6, §13.9 (no `⌘⌃`).
- ✅ §13.11 wizard step 3 shows `☐  Listen for "Hey Leah" wake-word  (opt-in)` (L738) — UNCHECKED.
- ✅ §13.8 renders exactly 2 pinned widgets (MARKET + PRS MERGED · 7d); title updated to "with 2 pinned widgets" (L1948).
- ✅ §13.4 (L1858) + §13.4-light (L1878) both present.
- ✅ §13.14 marketing-hero composition present at L2027.

## 6. New gap-fill sections present (all 19)

- ✅ §5.6 Timezone + DST (L406).
- ✅ §6.9-§6.15: Force-quit recovery, Sleep/wake, Low Power Mode, Low Data Mode, AirPods route change, External keyboard variance, VPN+proxy (L540-L564) — 7 sections.
- ✅ §17.3-§17.13: Localization, Multi-user FUS, compat matrix, Telemetry, iCloud, Crash recovery, Logging, Knowledge store, Settings persistence, Marketing-hero, Touch ID (L2450-L2495) — 11 sections.

Total: 1 + 7 + 11 = 19. All exist; spot-checked are ≥1 paragraph, not stubs.

## 7. Decision log integrity

- ✅ §14 row 7 deleted — table jumps 6 → 8 (L2085, no row 7).
- ✅ Decision #39 marked `[SUPERSEDED by #112 — gold-budget visible-surface cap]` (L2116).
- ✅ Decision #112 binding for gold budget (L2189).
- ✅ Spot-check on 6 decisions (#2, #5, #28, #40, #101, #109): no placeholder rationale; each cites the originating review channel and the workflow/perf/craft fix path.

---

## Final verdict

**SPEC-READY-FOR-IMPL**

One nit (F1, Phase 3 TTS ordering wording at L2674) — non-blocking, can be fixed in the implementation-plan dispatch or as a 1-line spec edit before Phase 3 starts. All other invariants hold; parity script is green; all 19 new gap-fill sections present; wireframes consistent; decision log clean.
