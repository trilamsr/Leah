---
title: Leah v3 spec — adversarial consistency + traceability audit
date: 2026-06-21
reviewer: senior tech editor (Apple HIG editor lineage; Stripe/Linear DS audits)
scope: docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md (2330 lines)
folds: docs/research/2026-06-21-{ux-review-nielsen-a11y, ui-review-craft-hig, workflow-review-daily-use, perf-review-render-memory}.md
stance: refute, not validate
status: BLOCKING — 7 BLOCKERs found before any SwiftUI work
---

# Summary

| Bucket                  | Count |
|-------------------------|-------|
| Self-contradictions     |   7   |
| Silent reviewer drops   |   3   |
| Token / value drift     |   4   |
| Wireframe mismatches    |   5   |
| Cosmetic / typo         |   3   |
| **Total findings**      | **22** |

Severity totals: 🔴 BLOCKER 8 · 🟡 INCONSISTENCY 11 · ⚪ COSMETIC 3.

The v3 spec is internally rigorous at the decision-log + changelog level — folds are well-traced and most reviewer-CRITICALs land in named §-paragraphs. The failure pattern is **the spec body lagging the decisions**: § 4.1 still says "max 3 pins," § 5.5 still draws "idle 90 s" arrows, § 14 still carries a v1-era decision row that contradicts its own v2 successor row, and Tiempos italic — declared "ONE location only" — is still referenced in widget-eyebrow descriptions across § 10.1 (silently practiced anti-pattern).

# Top 5 blockers

1. **§ 4.1 HUD pin cap contradicts § 10.3.** L239: "max 3 pins → 336 px tall"; L1346/L1349/decision #40: "max 2 pinned." Reviewer who reads § 4 first will build a 3-pin HUD layout.
2. **§ 5.5 state diagram still shows `idle 90s` transition** (L340) despite § 6.3 (L380) + decision #36 (L1958) killing 90 s auto-dismiss in favor of "≥ 5 min → ambient pill."
3. **§ 13.8 wireframe titled "Ambient HUD with 3 pinned widgets"** (L1835) — explicitly draws a state that decision #40 (max 2 pinned) forbids.
4. **§ 14 decision row 7 contradicts decision row 28.** Row 7 (L1929) still says Tiempos italic "used in dashboard header AND every widget eyebrow"; row 28 (L1950) enforces "ONE location only — Dashboard 'Today' header." Both rows are live in the same table.
5. **Tiempos italic still practiced across § 10.1 widget descriptions** at L902, L1084, L1121, L1145, L1232, L1290, L1389, L1679 — body text describes "Tiempos italic eyebrow / Tiempos italic column names / Tiempos italic label" on Market, Table, Chart, Image, Stat, Diff, Gallery, and Widget-tile empty states. The "ONE location only" rule (decision #28, changelog v2) is silently violated by spec prose.

---

# Traceability matrix — CRITICAL + MEDIUM reviewer findings → spec location

Reviewer ID legend: `ux-N` = ux-review-nielsen-a11y; `ui-N` (or `ui:section`) = ui-review-craft-hig; `wf-N` (or `wf:topic`) = workflow-review-daily-use; `perf-N` = perf-review-render-memory. Status: FOLDED (spec section cited) · REJECTED (§14/§15/§18 rationale cited) · DEFERRED (§18 deferred list) · SILENT-DROP.

## UX (Nielsen + a11y) — 15 CRITICAL, 22 MEDIUM

| ID    | Sev | Status   | Spec location / disposition                                                                    |
|-------|-----|----------|------------------------------------------------------------------------------------------------|
| ux-1  | 🔴  | FOLDED   | § 3.1 L117 + § 11.1 + §18 v2 changelog "contrast recomputed; `--text-dim` → `#8A8478`"          |
| ux-2  | 🔴  | FOLDED   | § 3.1 L108 + §18 v2 "`--red-alert` → `#D75A66` 5.26:1"                                          |
| ux-3  | 🟡  | FOLDED   | §18 v3 changelog L2215 "hover-rows freeze placeholder color"                                    |
| ux-4  | 🟡  | FOLDED   | §18 v3 L2215 "HUD captions pinned to `--text-muted` not `--text-dim`"                           |
| ux-5  | 🟡  | FOLDED   | Decision #52 (L1974) "Dividers tiered — decorative 8 % vs structural ≥20 %"                    |
| ux-6  | 🟡  | SILENT-DROP — tile-frame 20 % ivory + 12 px corner → recompute under reduce-transparency. No bump-to-30 % rule found in § 3.1, § 10.0, or § 12. |
| ux-7  | 🟡  | SILENT-DROP — `--gold-muted` `#8A7340` AA-edge audit ("never used for text < 18 pt") not enforced anywhere in § 3 or § 11. |
| ux-8  | ⚪  | FOLDED   | §18 v2 "regen contrast table with script" — § 11.1 L1582+ has the recomputed table; satisfies. |
| ux-9  | 🔴  | FOLDED   | Decision #2 (L1924) + §15 anti-pattern (L2046) "pre-checked privacy-cost defaults killed"      |
| ux-10 | 🔴  | FOLDED   | § 6.7 L449 + decision #48 "wake-word armed → ambient HUD `LISTENING` text + filled-hex menubar"|
| ux-11 | 🔴  | FOLDED   | § 4.2 L252 "Shape, not color — idle outlined / listening filled / error inner ●"; §18 v2       |
| ux-12 | 🔴  | FOLDED   | § 6.1 L355 — switched to `⌥Space` keydown; decision #5 (L1927)                                   |
| ux-13 | 🟡  | FOLDED   | Decision #93 (L2015) "real-time conflict feedback as keys pressed, not on save"                 |
| ux-14 | 🟡  | FOLDED   | § 6.1 L361 "conflict detection runs against curated list of Apple-documented shortcuts"; HIToolbox removed |
| ux-15 | 🟡  | FOLDED   | § 6.3 L378 "click-outside → input pulses gold-glow 1 s, draft preserved on next summon"        |
| ux-16 | 🟡  | FOLDED-DIFFERENTLY — § 6.3 L380 says "no timed auto-dismiss; idle ≥ 5 min → ambient pill"; warning-at-75 s recommendation rendered moot by removing the 90 s timer entirely. Acceptable. |
| ux-17 | 🔴  | FOLDED   | § 3.1 + decision-log + § 6.6 L412+; `--focus-ring` token defined dark + light                  |
| ux-18 | 🔴  | FOLDED   | § 6.6 L417–L424 widget-tile Tab semantics + Pin/X aria; § 11.3 VoiceOver                       |
| ux-19 | 🔴  | FOLDED   | § 6.6 keyboard model + § 11.3 (`P` to pin, `X` to dismiss, `⌘C` copy)                          |
| ux-20 | 🔴  | FOLDED   | § 10.1 maps adapter — long-press replaced; cursor-keyboard model in § 6.6                       |
| ux-21 | 🔴  | FOLDED   | § 6.6 arrow-key range selection cited                                                           |
| ux-22 | 🟡  | FOLDED   | Decision #99 (L2021) + § 6.6 L426 "first focus = search; Esc returns to chamber input"         |
| ux-23 | 🔴  | FOLDED   | § 3.4 L155 24×24 hit-area rule + § 10.0 L884 + decision #34                                    |
| ux-24 | 🟡  | FOLDED   | Decision #98 (L2020); code-widget line-number-click affordance removed                          |
| ux-25 | 🟡  | FOLDED   | § 3.4 L155 globalized                                                                           |
| ux-26 | 🟡  | FOLDED   | § 4.2 L249 "NSStatusItem hit-area system-padded to ≥24×24"                                      |
| ux-27 | 🔴  | FOLDED   | § 6.1 L359 + § 7.3 L509 inline ghost-chamber on hotkey-press during daemon-down                |
| ux-28 | 🟡  | FOLDED   | §18 v3 L2223 "sensitive-content blur reveals pattern + Mark-safe + Always-allow + per-message" |
| ux-29 | 🟡  | FOLDED   | § 10.5 L1425 "secondary line ≤ 40 char with HTTP/timeout/auth context"                          |
| ux-30 | 🟡  | FOLDED   | §18 v3 L2231 "widget→prose citation on schema-fail"                                              |
| ux-31 | 🟡  | FOLDED   | Decision #64 (L1986) "Sigil animation frequency cap ≤ 1 per 500 ms"                             |
| ux-32 | 🟡  | FOLDED   | Decision #60 (L1982) "Listening pulse 30–60 %; 0.5 Hz under reduced-motion"                    |
| ux-33 | 🟡  | FOLDED   | §18 v3 L2232 "row-by-row loading sweeps → static text under reduced-motion"                    |
| ux-34 | ⚪  | FOLDED   | § 6.5 L399 SCShareableContent observers; restore on capture-end implied (NOT explicit — see ui-21) |
| ux-35 | 🔴  | FOLDED   | §18 v3 L2234 "200 % zoom reflow without horizontal scroll" + § 11.4                            |
| ux-36 | 🟡  | FOLDED   | § 11.4 L1663 "Dynamic Type ≥ 130 % drops Pulse row; ≥ 150 % collapses to Mini"                 |
| ux-37 | 🟡  | DEFERRED | §18 L2273 "sigil cold-eye user-test — research task, not spec change"                          |
| ux-38 | 🟡  | FOLDED   | §18 v3 L2224 "status-glyph tooltip + section-header micro-legend"                              |
| ux-39 | 🟡  | REJECTED | §18 L2276 — rejected; operator decision #14 Calendar pre-select is load-bearing for HUD "Now"  |
| ux-40 | 🟡  | SILENT-DROP — `⌘/` help-overlay is spec'd at § 6.6 L412 and decision-log mentions discoverability footer, but the "1-line ⌘/ shortcuts footer in chamber empty-state" plus "menubar dropdown 'Keyboard shortcuts ⌘/' entry" combo from ux-40 is NOT in § 7.2 empty-state spec NOR in § 13.6 menubar wireframe (L1800-L1818). |

## UI craft + HIG — 11 CRITICAL, 22 MEDIUM

| ID                         | Sev | Status   | Spec location                                                                              |
|----------------------------|-----|----------|--------------------------------------------------------------------------------------------|
| ui:oxblood-vs-alert        | 🔴  | FOLDED   | § 3.1 `--red-alert` `#D75A66` repositioned; ●-glyph paired (decision #61 / §18 v2)         |
| ui:gold-vs-oxblood         | 🔴  | FOLDED   | Decision #61 (L1983) color-not-alone; deuteranopia simulation note in § 11.1                |
| ui:hairline-non-retina     | 🔴  | FOLDED   | § 3.1 L118 + L133 backingScaleFactor branch: 1 px @ 20 % → 1.5 px @ 28 %                   |
| ui:tiempos-11pt            | 🔴  | FOLDED-INCOMPLETE — decision #28 + §18 enforce one-location-only; **§ 10.1 widget descriptions still cite Tiempos italic in 8+ places (L902, L1084, L1121, L1145, L1232, L1290, L1389, L1679).** See BLOCKER #5. |
| ui:gold-fatigue            | 🔴  | FOLDED   | § 10.0 L887 "Max 3 visible `--gold-primary` per render; auto-demote" enforced render-time   |
| ui:sigil-hexagon-recog     | 🟡  | DEFERRED | §18 L2273 (10-user cold-eye test)                                                            |
| ui:custom-gold-system-accent | 🔴 | FOLDED   | § 3.0 (L69) tint policy + decision #26 (L1948)                                              |
| ui:glass-blur-material     | 🔴  | FOLDED   | § 3.2 L130 `material: .hudWindow`, `.behindWindow`, `.active`                                |
| ui:cmd-ctrl-chord          | 🔴  | FOLDED   | § 6.1 + decision #5; `⌘⌃` killed                                                              |
| ui:nonactivating-panel     | 🔴  | FOLDED   | Decision #10 (L1932); §15 anti-pattern (L2061); spec accepts focus-steal on summon          |
| ui:wizard-titlebar         | 🟡  | SILENT-DROP — wizard spec § 8 L517 still says "hidden titlebar, traffic lights top-left at 20 pt inset." Recommendation was "show a thin titlebar." Not addressed; not rejected; not deferred. |
| ui:dynamic-type            | 🔴  | FOLDED   | § 11.4 + § 3.3 L150 + decision rows; preferredFont(forTextStyle) cited                       |
| ui:no-light-mode           | 🔴  | FOLDED   | § 3.6 (L170+) full light palette + auto-switch via NSApp.effectiveAppearance KVO            |
| ui:flourish-2-fatigue      | 🟡  | FOLDED   | Decision #58 (L1980) first-ack-per-5min + border-flash subsequent                            |
| ui:flourish-1-fatigue      | 🟡  | FOLDED   | §18 v3 L2219 "first-summon-per-session only for full ceremony; warm = 160 ms cross-fade"   |
| ui:pinned-ambient-stack    | 🟡  | FOLDED   | Decision #40 (L1962) "max 2 pinned" — see BLOCKER #1 (spec body still says 3 in places)    |
| ui:candlestick-density     | 🟡  | FOLDED   | § 10.1 L898 + decision; "candlestick = `hero` only"                                          |
| ui:flights-gold-clutter    | 🟡  | FOLDED   | §18 v3 L2227 "global-min only filled-gold; row mins use 1.5 pt left-edge hairline"          |
| ui:letterpress             | 🟡  | FOLDED-WITH-SCOPE — §18 v3 L2265 confined to light-mode sigil + Dashboard "Today" header only |
| ui:lucide-stroke           | 🟡  | FOLDED   | § 3.4 L158 SF-Symbols-first; decision #54                                                    |
| ui:blur-text-bleed         | 🟡  | FOLDED   | § 3.2 L131 degraded-blur state + § 12 L1688 occlusion-sampling rule                          |
| ui:canjoinallspaces        | 🟡  | DEFERRED | §18 L2269 — contradicts operator decision #21                                                |
| ui:menubar-template        | 🟡  | FOLDED   | § 4.2 L252 + §18 v2 "pure-alpha template; state via shape; gold dot removed"                 |
| ui:grain-reduce-transp     | 🟡  | FOLDED   | § 12 L1689 "tint becomes fully opaque; grain disabled" + §18 v3 L2236                       |
| ui:permissions-accumulate  | 🟡  | FOLDED   | § 8.3 L591 + § 8.7 L714 Accessibility breakage-warning copy                                  |
| ui:dashboard-1180          | 🟡  | DEFERRED | §18 L2268 — Dashboard is non-MVP-5; cosmetic; lands when Dashboard implementation begins   |
| ui:status-glyph-toggle     | 🟡  | FOLDED   | §18 v3 L2224 "per-row Permissions toggle where OS allows + deep-link CTA otherwise"          |
| ui:cgscreenIsCaptured      | 🟡  | FOLDED   | § 6.5 L399 + §18 v2 "SCShareableContent observers"                                           |
| ui:cache-path              | 🟡  | FOLDED   | §18 v2 "`~/.leah-state/` → `~/Library/Application Support/Leah/`; cache → `~/Library/Caches/com.leah.daemon/`" |
| ui:conflict-detection-API  | 🟡  | FOLDED   | § 6.1 L361 "curated list of Apple-documented shortcuts; HIToolbox removed"                  |
| ui:tiered-disconnect       | 🟡  | FOLDED   | §18 v3 L2225 "low-data vs data-bearing; keep-index / delete-index disambig"                  |
| ui:weather-emoji           | 🟡  | FOLDED   | § 10.1 L1020 + decision #74 (L1996) "SF Symbols, never emoji"                                |
| ui:hotkey-vs-wizard-doc    | 🟡  | FOLDED   | Single source: § 6.1; wizard cross-references                                                |
| ui:palette-doc-diverge     | 🟡  | FOLDED   | § 3.1 reconciled; pal.§8 superseded — §18 L2278 notes pal §6 type rejection                  |
| ui:proto-vs-v1-tile-cap    | 🟡  | FOLDED   | § 10.0 L886 enforced; widget #5 in turn → `tool_error` (decision-log implied)                |
| ui:listening-pulse-1400ms  | ⚪  | FOLDED-DIFFERENTLY — duration kept; opacity + frequency clamped (decision #60)               |
| ui:ascii-vs-production     | ⚪  | SILENT-DROP — no statement that ASCII wireframes are sketches and Figma frames will follow.  |
| ui:mic-fake-waveform       | ⚪  | FOLDED   | § 8.4 L625 + decision-log "honest waveform" pattern                                          |
| ui:weather-emoji-ascii     | ⚪  | INCONSISTENT — § 13.8 ambient wireframe + § 10.1 L1043 ASCII still uses ☀ ⛅ 🌧 emoji glyphs in code blocks even though the body text says "SF Symbol stroke set." Wireframe lies about the spec. |

## Workflow (daily-use) — 12 CRITICAL, 27 MEDIUM, 5 LOW

| ID                                       | Sev | Status   | Spec location                                                                          |
|------------------------------------------|-----|----------|----------------------------------------------------------------------------------------|
| wf:chord-collides-rectangle              | 🔴  | FOLDED   | § 6.1 + decision #5 (⌘⌃ killed)                                                          |
| wf:wake-word-false-trigger               | 🔴  | FOLDED   | § 6.7 entire section (VAD-gate, per-app suppression, ignore-30 s, learning loop)        |
| wf:90s-dismiss-destroys-loop             | 🔴  | FOLDED   | § 6.3 L380 + decision #36 ("≥ 5 min → ambient pill, preserve 24 h")                     |
| wf:abort-two-verbs                       | 🔴  | FOLDED-DIFFERENTLY — § 6.3 L376 keeps Esc + ⌘. as **deliberately distinct** ("Esc = dismiss UI free; ⌘. = cancel backend save money"); reviewer asked to unify. Decision-log row #57 (implicit) + decision #57 cites the dual-verb intent. NOT a silent drop — explicit rejection of unification. |
| wf:widget-discoverability                | 🔴  | FOLDED   | Decision #46 (chamber `+` button) + §18 v2 "widget gallery add chamber-resident `+`"   |
| wf:corner-real-estate                    | 🔴  | FOLDED   | Decisions #38 + #40 (toast cap 2; pinned cap 2)                                          |
| wf:gold-aesthetic-overload               | 🔴  | FOLDED   | § 10.0 gold-budget invariant + decision #49 "Reduced ornament toggle"                   |
| wf:hotkey-cmd-shift-space-claw-grip      | 🔴  | FOLDED   | § 6.1 + decision #5 (`⌥Space` single-thumb-shift)                                        |
| wf:cmd-ctrl-chord-rectangle              | 🔴  | FOLDED   | Same fix as above                                                                        |
| wf:hud-multitasking-occlusion            | 🔴  | DEFERRED | §18 L2270 — HUD edge-dodge requires NSWindow occlusion sampling; v3.1                  |
| wf:hud-pinned-toast-stack-height         | 🔴  | FOLDED   | Decisions #38 + #40                                                                      |
| wf:wake-word-default-ON                  | 🔴  | FOLDED   | § 6.7 + decision #2 + §15 anti-pattern                                                   |
| wf:refresh-during-interaction            | 🔴  | FOLDED   | § 10.3 + decision #76 "pinned tiles static-until-glanced"                                |
| wf:widget-density-max-4                  | 🔴  | FOLDED   | § 10.0 L886 "Max 2 widget tiles per turn"                                                |
| wf:history-different-window              | 🔴  | FOLDED-PARTIALLY — § 6.6 L411 adds `⌘[`/`⌘]` prior-turn navigation in chamber; §18 L2279 acknowledges full cross-conversation search is deferred. Reviewer-intent partially satisfied. |
| wf:pin-3-eviction-fight                  | 🔴  | FOLDED-INVERSE — reviewer asked to raise to 5; spec lowered to 2 with explicit "Ambient is full · unpin one to add" copy. Deliberate; satisfies the corner-real-estate priority. |
| wf:post-wizard-reonboarding              | 🔴  | FOLDED   | § 8.6 + decision-log; `⌘/` discoverability footer per spec                              |
| wf:cmd-slash-discoverability             | 🔴  | SILENT-DROP — see ux-40. Decision-log mentions "⌘/" but **no chamber empty-state footer or menubar entry is drawn in § 13.4 (L1748-L1764) or § 13.6 (L1800-L1818)**. The spec adopts the verb but never surfaces it. |
| wf:voice-summon-screen-center            | 🟡  | FOLDED   | Decision #56 (L1978) "voice-summon = 400×280 corner frame"                              |
| wf:flourish-1-50-summons-day             | 🟡  | FOLDED   | §18 v3 L2219 "first-summon-per-session only"                                             |
| wf:widget-chrome-4-layer                 | 🟡  | FOLDED   | Decision #70 (L1992) "drops gold rule under eyebrow; Tiempos out of eyebrow"             |
| wf:hud-row3-pulse-3-metrics              | 🟡  | FOLDED   | § 7.1 L465 + decision #66 (1 primary metric, hover rotates secondary)                   |
| wf:sigil-rotation-false-pos-loop         | 🟡  | FOLDED   | Decision #59 (L1981) "F2 ack gated on VAD pass + 600 ms transcribed-token window"        |
| wf:tiempos-everywhere                    | 🟡  | FOLDED-INCOMPLETE — see BLOCKER #5 & ui:tiempos-11pt                                     |
| wf:code-block-wrap                       | 🟡  | FOLDED   | §18 v3 L2234 "chamber default 860 × 480"                                                 |
| wf:space-as-PTT                          | 🟡  | FOLDED   | § 6.4 + decision #55 + §15 anti-pattern L2073 "Fn (or ⌥) never Space"                   |
| wf:show-more-undefined                   | 🟡  | FOLDED   | § 10.0 L886 "truncation order: stat → chart → table"                                     |
| wf:pinned-delta-blink                    | 🟡  | FOLDED   | Decision #76 "static-until-glanced"                                                       |
| wf:wake-word-pre-checked-copy            | 🟡  | FOLDED   | § 8.4 + decision-log "honest battery cost copy"                                          |
| wf:permissions-glyph-collapse-3-states   | 🟡  | FOLDED   | §18 v3 L2224 "collapse-to-3-states visual" (granted / needs-action / denied)            |
| wf:settings-search-auto-focus            | 🟡  | FOLDED   | § 9.1 L731 "auto-focused on Settings open (Things pattern)"                              |
| wf:daemon-restart-fetch-fan-out          | 🟡  | FOLDED   | § 10.3 L1350 + decision #77 "stagger 250 ms; paint from cache first"                    |
| wf:fullscreen-hud-hide                   | 🟡  | FOLDED   | § 6.5 L395 corner-orb 0.6 opacity (raised from 0.3)                                      |
| wf:corner-orb-0.3-invisible              | 🟡  | FOLDED   | Decision #97 (L2019) raised to 0.6                                                       |
| wf:placeholder-rotation                  | 🟡  | FOLDED   | Decision #67 (L1989) fixed to "Ask Leah anything…"                                       |
| wf:weather-emoji                         | 🟡  | FOLDED   | Decision #74 — see also ui:weather-emoji-ascii wireframe lag                            |
| wf:maps-illegible                        | 🟡  | FOLDED   | Decision #73 + §18 L2277 "citation card for routing-intent"                              |
| wf:stream-pause-on-widget                | 🟡  | FOLDED   | Decision #13 (L1935 area) "stagger widget reveals; reserve tile height" §18 L2228       |
| wf:widget-mount-empty-reflow             | 🟡  | FOLDED   | Same — "reserve tile height from props"                                                  |
| wf:cli-parity                            | 🟡  | SILENT-DROP — no section addresses CLI parity for widgets. Spec is GUI-only. The reviewer's "TUI tables/sparklines OR doc GUI-only widgets clearly" choice — neither was made. |
| wf:purge-export-flow                     | 🟡  | FOLDED   | § 9.5 L845 "Export, then purge" + §18 L2264                                              |
| wf:hud-long-press-undiscoverable         | 🟡  | FOLDED   | § 4.1 L242 long-press REMOVED                                                            |
| wf:spawn-chips-disappear                 | 🟡  | FOLDED   | §18 v3 L2230 "quick-spawn chips persist across session via O(1) rolling top-3 file"     |
| wf:sensitive-content-show-scope          | 🟡  | FOLDED   | §18 v3 L2223 "per-message scope + Always-allow-for-this-app"                             |
| wf:wizard-step3-cram                     | 🟡  | FOLDED   | § 8.4 L640 "two stacked sub-cards" + decision #91                                        |
| wf:hud-row2-fallback-types               | 🟡  | FOLDED   | §18 v3 L2222 "Row 2 time-of-day-gated"                                                   |
| wf:gold-fatigue-validation               | 🟡  | DEFERRED-IMPLICITLY — decision #49 (Reduced-ornament toggle) provides the escape valve; the "validate by mocking 8h-of-use screenshots" research task is NOT in §18 deferred list. |
| wf:waveform-30Hz                         | 🟡  | FOLDED   | §18 v3 L2218 "Speaking waveform" decision (Metal shader / variableValue)                |
| wf:post-wizard-toast-self-summon         | 🟡  | FOLDED   | Decision #92 (L2014) "hotkey is hot only after the toast dismisses"                     |
| wf:sigil-rotate-after-week-LOW           | ⚪  | FOLDED-DIFFERENTLY — decision #58 caps acks to 1/5-min instead of "off-after-1-week"    |
| wf:greeting-time-aware-LOW               | ⚪  | SILENT-DROP — § 7.3 + § 13.4 still show "Good morning, Tri." with no statement on whether the greeting is fixed or rotates by time of day. Reviewer wf-7.4 asked: pick one or drop. |
| wf:sigil-hex-favicon-LOW                 | ⚪  | DEFERRED | §18 L2273 sigil cold-eye test                                                            |
| wf:settings-mock-preview-LOW             | ⚪  | FOLDED   | §18 v2 L2257 "Settings preview 50 % scale + 10 fps"                                      |
| wf:image-svg-allowlist-LOW               | ⚪  | SILENT-DROP — proto §6 image MIME allowlist is referenced in spec § 10.8 but svg-with-sanitization is not added. Minor. |

## Perf (render + memory + battery) — 11 CRITICAL, 19 MEDIUM, 10 LOW

| ID       | Sev | Status   | Spec location                                                                              |
|----------|-----|----------|--------------------------------------------------------------------------------------------|
| perf-1   | 🔴  | FOLDED   | § 6.7 wake-word OFF default + decision #2 + §15 anti-pattern                                |
| perf-2   | 🔴  | FOLDED   | § 3.2 L132 "static texture loaded ONCE; opacity locks to 2.5 %"; per-frame composite killed |
| perf-3   | 🔴  | FOLDED   | § 10.3 L1347 NSBackgroundActivityScheduler + decision #43                                   |
| perf-4   | 🔴  | FOLDED   | § 3.2 L131 + § 12 L1688 degraded-blur state                                                  |
| perf-5   | 🔴  | FOLDED   | § 10.7 + decision-log + perf-test plan §16.5; 200 ms fsnotify debounce; decision #44        |
| perf-6   | 🟡  | FOLDED   | § 8.4 L625 + §18 v2 "wizard preview" decisions                                               |
| perf-7   | 🟡  | FOLDED   | §18 v2 L2257 "Settings preview 50 % scale + 10 fps"                                          |
| perf-8   | 🟡  | FOLDED   | § 16.6 / § 12 idle-cold-launch decisions                                                     |
| perf-9   | 🔴  | FOLDED   | Decision #60 + decision #62 (animation-halt-when-idle)                                       |
| perf-10  | 🟡  | FOLDED   | §18 v3 L2218 "20-frame sprite-sheet"                                                          |
| perf-11  | 🟡  | FOLDED   | §18 v3 L2218 "Speaking waveform = Metal shader OR `.variableValue`"                           |
| perf-12  | ⚪  | FOLDED   | §18 v3 L2219 "transform.scale.y anchored at seam center"                                      |
| perf-13  | 🟡  | FOLDED   | §18 v3 L2228 "stagger widget reveals 80 ms"                                                   |
| perf-14  | ⚪  | SILENT-DROP — §3 still spec'd `--dur-instant` at 80 ms for hover color changes; reviewer-perf #14 asked for 1-frame swap. Decision-log #14 area not addressed; minor. |
| perf-15  | 🔴  | FOLDED   | § 3.2 + § 12 degraded-blur state                                                              |
| perf-16  | 🟡  | FOLDED-INCOMPLETE — §18 L2254 says "blur radius locked 18 px ambient / 24 px chamber"; **§ 3.2 L130 still says `blur(<radius>)` with no number; perf-prompt cited 16 px max for sub-frame budget. Spec uses 18 + 24 — within the band, but reviewer-perf #16 explicitly recommended ≤ 16 px**. Difference noted, not a contradiction but a defended decision (DEFERRED-IMPLICITLY). |
| perf-17  | ⚪  | FOLDED   | § 12 L1689 explicit reduce-transparency state                                                 |
| perf-18  | ⚪  | FOLDED   | § 10.0 L883 "single global static texture; not per-tile composite"                            |
| perf-19  | 🔴  | FOLDED   | § 10.6 L1569 lazy adapter registration (`Adapter.init()` on first render_widget); decision  |
| perf-20  | 🔴  | FOLDED   | § 10.3 + decision #43 NSBackgroundActivityScheduler + isLowPowerModeEnabled gate             |
| perf-21  | 🔴  | FOLDED   | § 10.3 + decision #44; 200 ms fsnotify debounce                                              |
| perf-22  | 🟡  | FOLDED   | §18 v3 L2228 "In-memory LRU + 50 MB cache cap + 5-min flush"                                 |
| perf-23  | 🟡  | FOLDED   | §18 v3 L2234 "chamber resize hysteresis 850-870 px"                                          |
| perf-24  | 🟡  | FOLDED   | § 10.7 L1549 "msgpack hot-path fallback" + §18 v3 L2231                                      |
| perf-25  | ⚪  | FOLDED   | §18 v3 L2231 envelope cap 256 KB                                                              |
| perf-26  | 🔴  | FOLDED   | § 6.7 + § 8.4 honest battery copy ("Adds ~ 2-4 % to daily battery drain")                   |
| perf-27  | 🟡  | FOLDED   | §18 L2266 + decision #48 XPC-budget separation                                                |
| perf-28  | ⚪  | FOLDED   | § 6.4 PTT-empty-input check kept (negligible cost)                                            |
| perf-29  | 🔴  | FOLDED   | §17 / §18 v2 surface-collapsing recommendations cited                                          |
| perf-30  | 🟡  | FOLDED   | Decision #90 (L2012) "history paged from disk via NSFetchedResultsController"                |
| perf-31  | 🟡  | FOLDED   | §18 v3 L2228 LRU cap 50 MB cache dir size                                                     |
| perf-32  | ⚪  | FOLDED   | §18 v3 L2230 "O(1) rolling top-3 file" — no unindexed scan                                    |
| perf-33  | 🔴  | FOLDED   | § 12 L1691 "system-idle ≥ 10 min → animations halt" + decision-log + §15 anti-pattern L2070  |
| perf-34  | ⚪  | FOLDED   | § 6.5 L399 SCShareableContent observers (push, not poll)                                      |
| perf-35  | 🟡  | FOLDED   | Decision #82 (L2004) "single coalesced NSTimer for all visible toasts"                       |
| perf-36  | 🟡  | FOLDED   | § 10.3 L1350 + decision #77 "paint from cache; refresh in background"                        |
| perf-37  | 🟡  | FOLDED   | § 16.5 / §18 v3 L2232 "frame-count parity check spec'd for sub-200 ms animations"            |
| perf-38  | ⚪  | FOLDED   | §18 v3 L2232 "reduced-motion = true 0 ms swap"                                                |
| perf-39  | 🟡  | DEFERRED | §18 L2271 "HUD/toast Stage Manager `.stationary + .fullScreenAuxiliary`" — coupled to wf-5  |
| perf-40  | ⚪  | SILENT-DROP — wizard sigil PNG @1x/@2x/@3x: §11.5 L1668 still says "PNG hero only for wizard, served @1x/@2x/@3x"; reviewer-perf #40 asked for SVG/PDF vector. Not addressed. Minor (~150 KB binary). |

---

# Findings (severity-tagged, ≤60 entries)

§4.1:L239: 🔴 BLOCKER: HUD "max 3 pins → 336 px tall" contradicts §10.3 L1349 "Max 2 pinned" + decision #40. Update §4.1 to "max 2 pins → 252 px tall" and recompute the 84-px increment math (3 base + 2 pinned = 5 × 84 = 420 px wait — current "84-px increments per pinned widget (max 3 pins → 336 px tall)" is actually 4 × 84 = 336 px which encodes 1 base + 3 pinned, so an editor needs to confirm both base-row count + max-pin and rewrite the math sentence end-to-end).

§5.5:L340: 🔴 BLOCKER: State-machine diagram shows `[ambient HUD] <--idle 90s--` arrow despite §6.3 L380 + decision #36 + §15 anti-pattern killing 90-s auto-destroy in favor of "≥ 5 min → ambient pill." Update the arrow to `idle ≥ 5min → ambient pill` (and rename "dismiss timer armed" → "ambient-pill shrink armed").

§13.8:L1835: 🔴 BLOCKER: Wireframe titled "Ambient HUD with 3 pinned widgets" + body shows MARKET / WEATHER / PRS pinned stack (L1843-L1849). Decision #40 caps pinned at 2. Rename + redraw with 2 pins.

§14:L1929 (decision #7) vs L1950 (decision #28): 🔴 BLOCKER: Row 7 says Tiempos "used in dashboard header AND every widget eyebrow"; row 28 says "ONE location only — Dashboard 'Today' header." Two live decisions contradict; delete row 7 or rewrite to point at row 28 as the supersede.

§10.1:L902, L1084, L1121, L1145, L1232, L1290, L1389, L1679: 🔴 BLOCKER: Widget descriptions still cite Tiempos italic in eyebrows / column names / annotation labels / gallery categories / empty-states, violating decision #28's "ONE location only." Rewrite each citation to use Söhne small-caps tracking +80 (the v3 default for eyebrows per craft-HIG #4 fix).

§4.1:L242 vs §1.1 (intro): 🔴 BLOCKER-LITE: §4.1 says HUD "Persists across Spaces (NSPanel `canJoinAllSpaces`)" but §18 L2269 defers `.moveToActiveSpace` per-Space pinning. Operator decision #21 keeps canJoinAllSpaces. Acceptable, BUT decision #21 is not back-referenced from §4.1 L241. Add the back-ref so a reader of §4.1 doesn't think it's an unguarded default.

§6.3:L376 vs Workflow-CRITICAL "unify Esc and ⌘.": 🔴 BLOCKER-LITE: Spec explicitly keeps two abort verbs (Esc dismisses UI free; ⌘. cancels backend save money). This is a deliberate rejection of a reviewer-CRITICAL but the rationale is buried in §6.3 footer (L383). Move the rationale into §15 anti-patterns OR add a decision row in §14 calling out "Esc-unification rejected" so future reviewers don't re-raise.

§14:L1929 zombie row: 🟡 INCONSISTENCY: Decision-log row 7 is a v1-era statement that wasn't deleted when v2 decision #28 superseded it. The pattern (zombie decision rows) is likely repeated — recommend a sweep where any row marked "(v1)" or pre-v2 is either deleted or marked `[superseded by #N]`.

§10.1:L1043 weather ASCII: 🟡 INCONSISTENCY: Wireframe code-block shows `[☀] [⛅] [⛅] [🌧]` — bare emoji glyphs — while body text L1020 says "SF Symbol stroke set (sun.max.fill, cloud.sun.fill...)." Wireframe lies about the spec. Replace ASCII emoji with bracketed SF Symbol names (`[sun]` `[cloud.sun]`) or pure stroke glyphs (e.g., `[ ○ ]` `[ ◐ ]`).

§13 wireframes (broad): 🟡 INCONSISTENCY: None of the wireframes 13.1–13.8 render `⌥Space` — only §13.9 (Settings) shows the glyph. Wireframes 13.4 (chamber empty) and 13.6 (menubar dropdown) are exactly where a "Press ⌥Space to ask" footer SHOULD appear per ux-40 + wf:cmd-slash-discoverability. The wireframes show neither the hotkey nor the `⌘/` cheatsheet pointer.

§13.4:L1748-L1764: 🟡 INCONSISTENCY: Empty-state wireframe shows "Good morning, Tri." with no statement of whether the greeting is time-of-day-aware or fixed. Reviewer wf-7.4 (LOW) flagged this exact ambiguity. Either pin the greeting to a single string or note the time-of-day rotation explicitly.

§13.4-light:L1786 vs §3.6 L187: 🟡 INCONSISTENCY: Light-mode wireframe footnote says sigil L is `--gold-primary-light` `#7A6332` which matches §3.6 L187. Good. BUT §3.6 L215 "letterpress emboss" two-direction text-shadow values (`#FFFFFF66, #0000001A`) differ from §18 L2265's scoping which lists `#FFFFFF08, #00000080`. Two different emboss specs in the same document. Pick one.

§3.2:L130 blur radius: 🟡 INCONSISTENCY: §3.2 L130 cites `backdrop-filter: blur(<radius>) saturate(...)` with `<radius>` left as a placeholder. §18 v2 L2254 locks "18 px ambient / 24 px chamber." Add the literal numbers to §3.2 so an implementer reading §3.2 in isolation has the locked value.

§4.1:L241 + §6.5: 🟡 INCONSISTENCY: HUD "Floats above normal windows; below fullscreen apps unless `always-on-top` toggled" (§4.1) vs §6.5 L395 "Fullscreen app (default) → corner orb @ 0.6 opacity." The two rules read as different system states. Reconcile: under fullscreen, does HUD become a 0.6-opacity corner orb (visible) OR is it "below the fullscreen app" (hidden)? Pick one and delete the other.

§7.1:L463-L465 vs §13.1 wireframe: 🟡 INCONSISTENCY: §7.1 describes 3 rows (Sigil+state / Now / Pulse=ONE primary metric). §13.1 wireframe Row 3 still shows the multi-metric `3 briefs · 12 arxiv · 5 PRs` (the very pattern decision #66 killed). Replace with `◇ 5 PRs` per decision #66.

§3.4:L154 vs §3.4 L158: 🟡 INCONSISTENCY: L154 "1.5 pt at 24×24; 1 pt at 16×16" (Lucide stroke spec) vs L158 "SF Symbols first" priority. The two coexist but L154 isn't qualified as "Lucide-only" — an implementer might apply 1.5 pt stroke to SF Symbols and break system-icon weight. Add qualifier: "1.5 pt at 24×24 applies to **restyled Lucide** only; SF Symbols inherit system stroke."

§8.4:L640 wizard split: 🟡 INCONSISTENCY: Step 3 body text describes "two stacked sub-cards" but §13.11 (L1889) just says "See §8.2-8.6 wireframes" without explicit step-3 wireframe. The "two stacked sub-cards" choice has no visual contract anywhere in §13.

§10.0:L883 grain texture sourcing: 🟡 INCONSISTENCY: "grain is the same single static texture loaded once globally" + §3.2 L132 "static texture loaded ONCE at app start (procedurally generated 128×128 tile, cached as a single CGImage)" + §3.6 L213 "1.5 % opacity (vs 2.5 % dark) — light surfaces tolerate less grain." Three places say similar-but-different things; the procedural-generation + opacity-by-mode logic should live in ONE place referenced by the others.

§11.4:L1659 "Expanded 280 × 168 px (+agenda strip) | 960 × 640 px" vs §4.1 L239 "Expanded: 280 × 168 px (adds agenda strip)": 🟡 INCONSISTENCY: The 960 × 640 column in §11.4 conflates HUD-expanded size with chamber size. Add a column header that clearly separates "ambient HUD expanded" from "chamber default." Reader can't tell which dimension belongs to which surface.

§6.6:L411 + §7.2 L498: 🟡 INCONSISTENCY: `⌘[`/`⌘]` for prior-turn navigation is mentioned but the empty-state wireframe doesn't surface the affordance. Add a `⌘[ prior turn` chip OR a discoverability footer ("⌘[ ⌘] navigate turns · ⌘/ shortcuts").

§9.1:L731 vs wireframe §13.9: 🟡 INCONSISTENCY: "Search: auto-focused on Settings open" — no visual representation in §13.9 wireframe shows the cursor / focused state on the search field. Minor but the wireframe should show the focused-state visually.

§11.5:L1668: 🟡 INCONSISTENCY (perf-40 silent drop): "PNG hero only for wizard, served @1x/@2x/@3x." Spec keeps PNG triplet; reviewer perf-40 recommended SVG/PDF vector (zero raster cost, single asset). Either accept the 150 KB binary cost explicitly OR fold the SVG/PDF recommendation.

§8:L517 wizard frameless: 🟡 INCONSISTENCY (ui:wizard-titlebar silent drop): Spec still says "hidden titlebar, traffic lights top-left at 20 pt inset." Reviewer craft-HIG asked for a thin titlebar so users can grab + identify the window. Not addressed; not rejected. Add to §18 deferred or fold.

§3.1:L98 — `--gold-primary` hex value commentary: 🟡 INCONSISTENCY: Comment "midpoint of `#C5A572` / `#CDB370` / `#B89968` / `#D4AF37`" — three of those four hexes appear nowhere else in spec; reads as palette-derivation note. Move to §3.1 footer or convert to a small "calibration" subsection so the table isn't cluttered.

§3.1:L98 vs §15 L2030: 🟡 INCONSISTENCY: `--gold-primary` `#C9A961` — §15 anti-pattern explicitly forbids `#FFD700` (Vegas gold). Good. BUT §3.1 L98 commentary cites `#D4AF37` (also Vegas-gold-adjacent) as a calibration point. Either drop the citation or note it as "specifically rejected as too saturated."

§10.0:L884 hit-target: 🟡 INCONSISTENCY: "12 px visual + 24 × 24 padded hit-target; 8 px gap." If both pin and dismiss are 24 × 24 with an 8 px gap, the cluster is 56 px wide. None of the §10.1 ASCII wireframes (e.g., L920, L959, L997, L1038) reflect this width — they just show `◆ × ` (3 chars). Wireframes don't enforce the 56-px hit-cluster visually. Acceptable but should be noted in §13 caption.

§10.7:L1549 hot-path framing: 🟡 INCONSISTENCY: "if bursty load is measured (>200 widget.update frames/sec across pinned widgets), upgrade the hot path." Threshold is a number but no measurement instrumentation is spec'd in §16. Add a perf-test that emits the metric.

§3.6:L215 emboss + §18 L2265: 🟡 INCONSISTENCY: Two different emboss formulas in spec. §3.6 L215 says `text-shadow: 0 1px 0 #FFFFFF66, 0 -1px 0 #0000001A` for light-mode sigil. §18 L2265 says `text-shadow: 0 -1px 0 #FFFFFF08, 0 1px 0 #00000080` per palette-doc reconciliation. Direction inverted + opacity values differ. The §18 version is described as "per palette-doc" — pick the canonical one.

§3.3:L139-L150 Tiempos role: 🟡 INCONSISTENCY: §3.3 L146 "ONE location only (v2): Dashboard 'Today' header. Stripped from widget eyebrows, gallery categories, empty-state placeholders." Then §3.3 L150 (Dynamic Type) talks about HUD reflow — not Tiempos-relevant but the Tiempos role-section ends abruptly without enumerating where it's NOT used (Search, About, etc.). Decision #28 enforces, but body prose could enumerate the "stripped from" list more clearly.

§9.5:L845 export-then-purge: ⚪ COSMETIC: Three-button flow "Export, then purge / Purge / Cancel" cited inline; no ASCII wireframe in §13.13 (which just says "See §9.5 wireframe" — circular).

§13.11:L1889 wizard wireframes: ⚪ COSMETIC: "(See §8.2-8.6 wireframes)" — content elsewhere. §13.11 has no actual visual; an editor expects either the wireframe OR a clear note "wireframes embedded inline in §8."

§7.1:L469 "weather chrome (just a glyph if shown)": ⚪ COSMETIC: Weather-glyph rule (SF Symbol) lives in §10.1 L1020 + decision #74. §7.1's brief mention could cite "See §10.1 for glyph set" so an editor finds the canonical rule.

---

# Notes for the spec owner

- **The decision-log + changelog discipline is genuinely strong.** Every reviewer-CRITICAL has an explicit fold OR an explicit rejection with rationale. That is exceptional for a 3-version 2330-line spec.
- **The failure mode is mechanical lag.** Body text + ASCII wireframes + state diagrams + the decision-log itself were not swept to reflect post-v2 changes (Tiempos one-location, pinned-cap 2, 90-s killed). A single grep pass over the spec body for `90 s|max 3 pin|Tiempos italic` would catch most BLOCKERs.
- **Suggested gate:** add a §16.7 "Cross-section parity check" rule that runs as part of `make check` — fail the build if `90 s` appears anywhere outside §15 / §18 historical, if `max 3 pin` appears anywhere, or if `Tiempos italic` appears outside the single declared dashboard-header citation.
- **Two reviewer-CRITICALs are intentionally rejected**, not silent-dropped (Esc/⌘. unification, drop-maps-widget). Surface the rejections in §15 OR §14 — currently the rejection lives only in §18 changelog footer which an implementer is unlikely to read.

— end —
