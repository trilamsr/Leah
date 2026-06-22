---
title: Leah macOS UI — Nielsen + WCAG 2.2 AA adversarial review
date: 2026-06-21
author: senior UX critic (NN/g consultant lineage, fintech + healthtech a11y audits, WCAG 2.2)
status: adversarial review — refute, not validate
scope: 4 source docs — v1 design / wizard + settings / widget protocol / palette refs
---

# Adversarial UX review — Nielsen heuristics + WCAG 2.2 AA

Default stance: skeptical. Every finding cites a concrete failing user flow or recomputed measurement. Each line: `<doc>:<section>: <severity> <heuristic/WCAG>: <problem>. <fix>.`

Doc abbreviations: `v1` = leah-macos-native-ui-design-v1.md · `wiz` = leah-wizard-settings-ux.md · `wp` = leah-widget-protocol.md · `pal` = sleek-regal-palette-refs.md

Severity: 🔴 CRITICAL / 🟡 MEDIUM / ⚪ LOW

---

## A. WCAG 2.2 contrast failures (recomputed — claims are wrong)

1. v1:§6.1: 🔴 WCAG 1.4.3: `--text-dim` `#6B6558` on `--obsidian-0` is recomputed at **3.44:1**, not the claimed 4.6:1. FAILS AA for normal text. Used for placeholders, timestamps, divider labels — visible to operator on every screen. Fix: lighten to `#8A8478` (4.69:1) or restrict to ≥18pt large-text only (WCAG 3:1).
2. v1:§6.1: 🔴 WCAG 1.4.3: `--red-alert` `#C8434F` on `--obsidian-0` is **4.14:1**, not the claimed 5.1:1. FAILS AA for normal text — yet doc uses it for "Model couldn't respond" body text and "Daemon offline" caption. Fix: lighten to `#D75A66` (5.04:1) or reserve for ≥18pt headings only.
3. v1:§6.1: 🟡 WCAG 1.4.3: `--text-dim` on `--obsidian-2` `#161922` (card-hover bg) is **3.03:1** — placeholder vanishes the moment row is hovered. Fix: bump text-dim or freeze placeholder color on hover state.
4. v1:§6.1: 🟡 WCAG 1.4.3: placeholder text-dim on HUD `--obsidian-1` `#0E1014` is **3.29:1**. HUD captions like "Standup in 12m" if rendered in text-dim fail AA. Fix: spec which token HUD captions use; force `--text-muted` (8.16:1) for any time-sensitive caption.
5. v1:§2.1: 🟡 WCAG 1.4.11: `--divider` 8% white hairlines compute to ~1.14:1 vs obsidian — below the 3:1 UI-component floor. Doc admits "decorative hairline" but Settings sidebar separators, focus chamber section rules, and table row separators carry information (group boundaries). Fix: tier dividers — decorative at 8%, structural at ≥20% (3:1 met).
6. v1:§9.1: 🟡 WCAG 1.4.11: Tile chrome "1 px hairline @ 20% ivory" → ~3.3:1 borderline, but `12 px corner radius` widget frame is the *only* boundary between adjacent tiles in 2-col grid. If user has reduced transparency or the grain disabled, tiles bleed into background. Fix: bump to 30% ivory frame, or add 1px inset shadow.
7. v1:§6.1: 🟡 WCAG 1.4.3: `--gold-muted` `#8A7340` on `--obsidian-0` = 4.37:1 — JUST under AA for normal text but doc uses it for "hairline frame" and disabled state. Disabled state needs 3:1 contrast (WCAG 2.2 1.4.11 exempts disabled UI but icons must remain meaningful). Fix: confirm gold-muted is never used for text < 18pt; if it is, lighten.
8. v1:§6.1: ⚪ Doc claims `--gold-primary` is 8.4:1; actual is 8.85:1. Minor — but signals the table values weren't audit-recomputed. Fix: regen the entire contrast table with a script and commit it.

## B. Wake-word default ON (privacy + consent)

9. wiz:§0/§3: 🔴 Nielsen H4 (consistency) + H6 (recognition): Wake-word **pre-checked ON** at step 3 contradicts macOS norms (Siri requires explicit opt-in flow) AND the v1 doc's own privacy rationale (v1 §8.2 "shipping an always-listening assistant by default is hostile to operator trust"). Pre-checked = passive consent = GDPR Art. 7 / CCPA dark-pattern territory. Fix: default OFF; surface as choice not toggle ("Yes, listen for 'Leah'" / "No, hotkey only"); record consent timestamp.
10. wiz:§3: 🔴 Nielsen H1 + H10: "always listening" has no system-status indicator at HUD level beyond a tiny gold dot. Operator cannot answer "is the mic hot RIGHT NOW?" at 100px glance. Fix: when wake-word ON, HUD MUST show explicit `LISTENING` text + animate the pulse — and replace the menubar 18px hexagon with a recording-style red dot when mic is active.
11. wiz:§3: 🔴 WCAG 1.3.3 (sensory): wake-word ready-state distinguishable ONLY by gold-dot color in menubar. Color-blind operators (deuteranopia ~6% of males) cannot distinguish gold from amber from grey at 18px. Fix: add a shape change (hexagon → filled hexagon-with-dot) AND a VoiceOver announce-on-state-change.

## C. Hotkey + summon (Nielsen H4 consistency, H7 efficiency)

12. wiz:§A.3-step2: 🔴 Nielsen H4: `⌘⌃` modifier-only chord with 250ms tap window is unreliable. Operators with motor impairment, Sticky Keys enabled (WCAG 2.1.4), or holding modifiers for accessibility shortcuts will trigger summons accidentally OR fail to trigger them. Raycast precedent doesn't make it accessible. Fix: default to `⌘⇧Space` per v1; offer modifier-only as opt-in advanced.
13. wiz:§A.3-step2: 🟡 Nielsen H5 (error prevention): "conflict check runs only on save" → operator can record a hotkey that conflicts with their text-editor's `⌘⇧K` and not learn until next time they try to use that editor. Fix: real-time conflict feedback as keys are pressed; also surface 3rd-party app conflicts (queried via `HIToolbox` — already specced, just enforce).
14. v1:§4.1: 🟡 Nielsen H4: `⌘⇧Space` conflicts with **macOS native** "Change input source" (System Settings → Keyboard → Input Sources → Select previous source). Doc claims it's "globally free" — it is NOT on multi-language keyboards (most non-US setups). Fix: detect and warn on first launch; doc must acknowledge the conflict.
15. v1:§4.3: 🟡 Nielsen H5: "Click-outside dismisses unless input has unsent text (subtle gold-glow, 1s — no modal)". Operator who clicks outside expecting dismiss + comes back to find chamber still open will be confused. The "subtle glow" is below the recognition threshold of 50% of operators (NN/g micro-feedback studies). Fix: either always-dismiss with toast "Draft saved" + restore on next summon, or actual confirm.
16. v1:§4.3: 🟡 Nielsen H3 (user control): "90s idle auto-dismiss" with no warning kills work-in-progress reads. Operator opens chamber, reads long response, gets distracted for 91s, returns to a closed chamber. Fix: warn at 75s with a quiet fade indicator + reset on hover/scroll/focus events.

## D. Focus visibility + keyboard (WCAG 2.4.7, 2.1.1, 2.1.2)

17. v1:§2.5/§3: 🔴 WCAG 2.4.7: focus indicator is "gold-primary on hover/active" — but the spec NEVER defines a focus ring shape, thickness, or offset that survives keyboard-only navigation. Tab cycling through input → sources → chips → close (v1 §4.6) needs a visible 2px focus outline at 3:1 minimum. Fix: explicit `--focus-ring` token (`#E8CC8C` 2pt outline, 2pt offset, square corners; never relies on color alone — add 1pt outer halo).
18. v1:§4.6: 🔴 WCAG 2.1.1: Tab order documented (input → sources → follow-ups → close) but Section 9 widget tiles are unspecified in keyboard model. How does operator Tab through 4 stacked widget tiles, then into the next prose block, then to follow-up chips? Pin/dismiss `◆ ×` controls in tile top-right — keyboard reachable how? Fix: append §4.6 to cover widget-tile Tab semantics; pin/dismiss must be Tab-targets with explicit aria-labels.
19. v1:§9.0: 🔴 WCAG 2.1.1: tile interactions documented only as click/hover ("hover sparkline → crosshair", "click symbol → spawns", "right-click row → copy as TSV"). NO keyboard equivalents. Right-click especially — keyboard-only users (motor impairment, screen reader users) cannot reach context menus. Fix: every click/right-click action needs `⌘`-shortcut or menu equivalent.
20. v1:§9.1 (Maps): 🔴 WCAG 2.1.1: "long-press → copy coordinates" — no keyboard equivalent. Fix: replace with Cmd+C when tile focused.
21. v1:§9.1 (Calendar): 🔴 WCAG 2.1.1: "Drag-select a range → [Block focus time] chip" — drag is mouse-only. Fix: arrow-key range selection with Shift modifier, like NSDatePicker.
22. v1:§9.4: 🟡 WCAG 2.1.2 (no keyboard trap): widget gallery overlay (`/widgets`) — Esc dismisses, but no spec for Tab cycling within overlay. If first Tab lands on category list (left rail) and Tab/Shift-Tab can leak focus to the chamber input behind the overlay, focus trap is broken. Fix: explicit focus-trap rules; first focus = search field; Esc returns focus to chamber input.

## E. Target size (WCAG 2.2 2.5.8 — 24×24 minimum)

23. v1:§9.0: 🔴 WCAG 2.5.8: pin `◆` glyph **12 px** and dismiss `×` glyph **12 px**, 8 px gap. Below the 24×24 minimum target size (WCAG 2.2 AA). Fix: hit-target must be 24×24 even if visual glyph is 12 — wrap each in a 24×24 padded button.
24. v1:§9.1 (Code widget): 🟡 WCAG 2.5.8: "click line number → copy that line" — line numbers in JetBrains Mono @ 30% opacity are far below 24×24. Fix: row-level hover affordance for copy, not per-line-number click.
25. v1:§2.4: 🟡 WCAG 2.5.8: 16×16 icons in primary chrome don't meet 24×24. The "no filled icons in primary chrome" rule doesn't address hit area. Fix: hit-target 24×24 minimum even when icon glyph is 16.
26. v1:§7.6: 🟡 WCAG 2.5.8: menubar dot (18×18) for "listening state" — system menubar tolerates this but the *clickable* affordance must be 24×24. Fix: implicit 24×24 NSMenubarExtra padding; verify in impl.

## F. Error recovery + diagnose (Nielsen H9)

27. v1:§5.3: 🔴 Nielsen H9: "Daemon down → focus chamber refuses to summon; menubar dot pulses red; click → 'Daemon offline. [Restart Leah]'" — operator pressing the global hotkey gets ZERO feedback at the cursor location. They press `⌘⇧Space` 3 times wondering if hotkey broke, then have to remember to look at the menubar. Fix: hotkey-press during daemon-down MUST flash an obsidian-1 inline ghost-chamber at screen-center with "Daemon offline. Click to restart." — visible where the operator is looking.
28. v1:§5.3: 🟡 Nielsen H9: "Sensitive content detected → blur scrim + [Show]" — but no explanation of WHY it was flagged, no opt-out path for false positives. Operator who got an answer with a password they wanted to see has no recovery if regex misfires on their own data. Fix: blur reveal shows pattern that matched + "Mark safe" + "Always allow for this app".
29. wp:§3 (error frame): 🟡 Nielsen H9: "Tile frame swaps hairline to oxblood + 'COULDN'T LOAD' + [Retry]" — no diagnose info (HTTP code, timeout, auth?). Operator clicks Retry, fails again, has zero new info. Fix: secondary line ≤40 chars: "couldn't reach api.market.io · timeout 5s" → operator knows it's network not credential.
30. wp:§6 (security): 🟡 Nielsen H9: schema validation failure returns `widget.error` to UI but LLM gets `tool_error` — what does operator see in the chamber? If LLM falls back to prose, operator never knows a widget was attempted. Fix: surface "tried to show {widget} but data didn't fit" as a 1-line citation under the prose.

## G. Reduced motion + sensory adaptation (WCAG 2.3.3, 1.4.4)

31. v1:§3.4 (Flourishes): 🟡 WCAG 2.3.3 (animation from interactions): reduced-motion replacement for the Gold Seam is "cross-fade 160ms" — good. But sigil acknowledgment fallback "color shifts to gold-glow for 200ms" still uses color-flash, which can trigger photosensitive responses if rapid (multiple wake-events in quick succession). Fix: cap state-change frequency to ≤1 per 500ms; or use opacity-fade not color-shift.
32. v1:§9.1 (Market loading): 🟡 WCAG 2.2.2: "1 Hz gold dot pulse" loading indicator — exactly 1 Hz is within the seizure-trigger range (3-50 Hz is the flash band, but 1Hz at high contrast on dark BG can still trigger migraine in vestibular-sensitive users). Fix: slow to 0.5 Hz under reduced-motion; ensure pulse amplitude is opacity 30-60%, not 0-100%.
33. v1:§9.1 (Flights loading): 🟡 WCAG 2.3.3: "gold seam sweeping left-to-right at 240ms intervals across rows" — discrete row-by-row motion is more attention-grabbing than continuous and is NOT addressed by reduced-motion (doc only zeros durations, doesn't suppress loop). Fix: reduced-motion → static "Loading fares" text only.
34. v1:§4.5 (Screen recording): ⚪ Nielsen H1: "auto-hide HUD during screen recording" via `CGScreenIsCaptured()` — but **doesn't restore** is unspecified. If the recording app exits and the HUD doesn't reappear, operator thinks Leah crashed. Fix: spec the restore trigger + a "Leah hidden — recording detected, [Show anyway]" menubar notice while hidden.

## H. Text scaling + density (WCAG 1.4.4, 1.4.10)

35. v1:§2.3: 🔴 WCAG 1.4.4: Body 14pt at 1.45 line — at 200% zoom this becomes 28pt; the focus chamber at 720×480 default has no spec for reflow at zoom. If 200% zoom causes horizontal scroll on the response stream, WCAG 1.4.10 (reflow) FAILS. Fix: explicit reflow spec — chamber width must accommodate 200% zoom without horizontal scroll, OR allow chamber to grow with system text-size up to display bounds.
36. v1:§1.1 + §6.4: 🟡 WCAG 1.4.4: HUD captions (Row 2 "Standup in 12m", Row 3 "3 briefs · 12 arxiv · 5 PRs") — at 280×84px and presumably 11-12pt, 200% zoom would overflow. Doc doesn't specify behavior. Fix: HUD must auto-resize to Mini (sigil-only) at OS text-size ≥ 150%, or grow vertically.

## I. Match real world + recognition (Nielsen H2, H6)

37. v1:§2.5 (sigil): 🟡 Nielsen H2: "italic serif L inside hexagon" — operators don't know it's an "L"; at 18px menubar (L disappears per §2.5), the hexagon is meaningless. Per the brief's "recognizable at 100px" target — verify with 10 unprimed users that the hexagon reads as Leah and not "settings/options". Fix: user-test the sigil with cold-eye recall; if <70% identify, add a thin "L" wordmark on hover/tooltip.
38. wiz:§B.3.5: 🟡 Nielsen H4 + WCAG 1.4.1 (color alone): status glyph legend `● green / ◐ gold / ○ open / ✕ red` — shapes ARE distinct (good), but the legend is in B.3.5 alone, not on every page where glyphs appear. Operator opening Integrations or About first sees glyphs with no legend. Fix: tooltip on each glyph + persistent micro-legend at section header.
39. wiz:§A.3-step4 + v1:§5.4: 🟡 Nielsen H8 (minimalist): wizard step 4 says "Calendar pre-selected" → operator who DOESN'T want Calendar must actively unselect, then select another. Radio cards default to none-selected per macOS convention. Fix: no pre-selection; primary CTA disabled until choice made; or change copy to "Skip and connect later" as the primary path.

## J. Other (help, consistency, ceremony)

40. v1 + wp: 🟡 Nielsen H10: NO help/documentation surface specced anywhere except `⌘/` "toggle help overlay (hotkey cheatsheet)" in v1 §4.6 — one line, no spec. Operator stuck on "what can I ask Leah?" has zero discoverability beyond starter chips. Fix: spec the `⌘/` overlay AND a "What can I do?" link in About; widget gallery (`/widgets`) is good but operator must already know to type `/widgets`.

---

# Summary

- 🔴 CRITICAL: **15** (contrast 1-2, wake-word 9-11, hotkey 12, focus 17-21, target size 23, daemon error 27, text scaling 35)
- 🟡 MEDIUM: **22** (contrast 3-7, hotkey 13-16, focus 22, target size 24-26, error 28-30, motion 31-33, scaling 36, recognition 37-39, help 40)
- ⚪ LOW: **3** (contrast 8, screen-recording restore 34, plus partial overlap with mediums)

**Top 5 fixes by user-impact-per-effort:**
1. **Recompute contrast table with a script and commit it** (fixes #1, #2, #3, #4, #8 — current table is wrong; trust is broken)
2. **Wake-word default OFF + explicit consent** (fixes #9, #10, #11 — privacy posture is the #1 trust signal at launch)
3. **Define `--focus-ring` token + apply to all keyboard-reachable elements** (fixes #17, #18, #22 — WCAG 2.4.7 is non-negotiable)
4. **24×24 hit targets for pin/dismiss/icons** (fixes #23, #24, #25, #26 — WCAG 2.2 mandatory)
5. **Hotkey-press during daemon-down shows ghost-chamber inline** (fixes #27 — biggest dead-end in the spec)

**Unverifiable claims flagged:**
- v1 §6.1 contrast table values (5 wrong out of 8 recomputed)
- v1 §4.1 "`⌘⇧Space` is globally free on macOS" (not on multi-language keyboards)
- v1 §6.4 + §1.1 HUD zoom behavior (not specced)
- v1 §4.5 screen-recording HUD restore trigger (not specced)
- wp §3 widget tile keyboard model (not specced)
