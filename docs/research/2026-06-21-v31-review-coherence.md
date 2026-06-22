# Leah v3.1 macOS Native UI — Coherence Review

> Reviewer: senior design director (Apple/Linear/Vercel lineage), adversarial coherence lens.
> Spec under review: `docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md` (2,564 lines, v3.1, 8 prior reviewers, 80+ folds).
> Date: 2026-06-21.

---

## 1. Soul verdict

Leah no longer hangs together as one thing. It hangs together as the **scar tissue of eight reviewers**. The v3.1 spec is a compromise document wearing a thesis as a hat: §0 still promises "a presence on the operator's machine — JARVIS crossed with haute horlogerie," but every later section is an amendment apologizing for that sentence. §3.6 grafts a light palette on because HIG said so. §2.7 grafts a voice canon on because reviewer #2 said so. §3.3 swaps Söhne→Inter and Tiempos→New York because someone read the Klim license. §3.5 renames "sigil"→"mark" but the word sigil still appears 139 times in the spec — the rename swept the noun, not the **shape**, and certainly not the soul. The killer screenshot (§13.14) is the tell: it now shows ivory prose, three gold instances, a KPI tile, and a desktop blur — that is **Linear's command palette with a hexagon**, not "a presence." The original thesis was a JARVIS-Patek fever dream; the current spec is a sensible Mac app trying not to embarrass itself. Neither is wrong, but **the document doesn't know which one it is**, and that incoherence will produce a UI that reads as "high-effort Raycast clone that mysteriously has a serif L."

## 2. Five sharpest coherence failures

1. **Rename without reshape.** "Chamber" → "focus panel" and "sigil" → "mark" were swept in user-facing copy, but the spec body still says "chamber" 315 times and "sigil" 139 times. More damning: §13.3's wireframe still draws the same gold-bordered glass surface over obsidian — a panel only by relabeling, a chamber by shape. If you rename because the word is sci-fi-cosplay, the shape was the problem. Renaming is admission, not fix.

2. **Light mode is a graft, not a counterpart.** §3.6's "bone + darkened champagne" palette passes WCAG but has zero soul justification. The dark-mode obsidian gets two paragraphs about Patek Philippe, walnut-stained paper, NYTimes Cooking. The light-mode bone gets a contrast number. There is no "what does Leah feel like in light mode" — because nobody designed light mode, they computed it. §13.4-light says "same anatomy" — exactly. Same anatomy, no animating principle.

3. **§2.7 voice canon vs. §3 visual identity speak different products.** Voice is "alto, ~145 wpm, slow-warm, named, gendered, ElevenLabs-cloned." Visual is "earn every pixel, hairline rules, no ornament." A named, gendered, warm voice that talks to you wants a warmer, more humanist visual frame (think Granola, Hume, Bee). The obsidian-and-gold visual wants a colder, anonymous voice (think Linear, Arc, Raycast). The spec ships both — meaning the first time you hear Leah you'll feel a mismatch between what she looks like and what she sounds like.

4. **The widget catalog is a feature pile, not a system.** 13 widget types (market, flights, calendar, weather, maps, table, chart, image, code, citation, stat, list, diff) over 431 lines (§10.1). Nowhere does the spec say **when** the LLM decides to spawn a `chart` vs. a `stat` vs. a `table` for the same numeric answer — the spec assumes the LLM judges. That is not a design system; that is hoping. A real system would force-rank: prose first, single-stat second, chart only if delta-over-time is the answer. v3.1 instead optimizes the schema for all 13 in parallel — implementation cost, surface area, QA cost all 13×.

5. **"Reduced ornament toggle" is an admission, not a fix.** The presence of a "reduce ornament" preference is the spec confessing the default IS ornament-heavy. A coherent design with restraint as a thesis does not need a toggle to reduce restraint further — restraint is already the floor. The toggle exists because §3.6 grain, §5.4 signature flourishes, §13.5 gold seam transition, and the §3.5 italic-L emboss are all decorative work the operator can opt out of. Each one earned its way into the spec; together they prove the design lost the "earn every pixel" rule it claims to govern itself by.

## 3. What to delete (to make it coherent again)

1. **§3.6 light mode palette as currently written.** Either redesign light mode from a soul ("Leah in a sun-warmed Eames lounge"), or ship dark-only v1 and add light when it has a thesis. Computed palettes never feel native.
2. **§2.7 voice canon.** Defer to v2. Shipping a named ElevenLabs-cloned alto on launch raises liability + brand-confusion + cost; system voice + neutral copy is enough for v1.
3. **§5.4 signature flourishes + §13.5 gold seam transition.** Both are decoration the toggle disables anyway. Cut, and the toggle disappears with them.
4. **The 13-widget catalog → 5.** Keep `stat`, `table`, `chart`, `code`, `list`. Defer `market`, `flights`, `weather`, `maps`, `calendar` to v2 — those are vertical integrations, not display primitives. Drop `image`, `citation`, `diff` (image is QuickLook, citation is a `list` row, diff is a `code` block).
5. **§3.5 italic-L emboss + grain + per-mode mark shadow variants.** The mark should be one SVG, period. Inner shadow + emboss + grain on a 24-px hexagon at 100% display scale is invisible computation cost.
6. **§13.14 killer screenshot as currently composed.** See replacement below.
7. **"Operator" as the user noun (210 occurrences).** This is cosplay. The user is the user, or Tri, or "you." "Operator" pretends Leah ships to spies. Sweep to "you."
8. **§4.5 first-launch wizard from 5 steps to 2.** Welcome+hotkey (1), mic+ready (2). One integration is already overstated; calendar is an opt-in card in step 2, not a step.
9. **§14 open-decisions log (~120 entries).** Decisions logs are working memory, not specs. Move to a separate `decisions.md`. Spec drops ~400 lines.
10. **§18 versioning + change log section.** Specs are not changelogs. Git is the changelog. Spec drops ~50 lines.

Net: spec goes from 2,564 → ~1,400 lines and becomes a design document instead of an archaeological record.

## 4. New killer screenshot (§13.14 rewrite, 100 words)

> A real macOS desktop, mid-morning. You see Finder, an email, a code editor — sharp, not blurred, at full opacity. Floating above them, near screen-center, a small panel: roughly 600 × 360. Warm-ivory background (not obsidian). One line of dark-graphite serif at the top: *"MAY-19 shipped. PR #321 in 8 minutes."* Below it, one inline number — `52 PRs · ▲ 14 vs last week` — set in mono, no tile chrome. The only gold in the frame is the L mark in the top-left corner, 20 px. No blur. No hairlines. No widget border. The desktop behind is visible because Leah has nothing to hide.

Why this works and the current §13.14 doesn't: the current one **hides the desktop behind 0.6 opacity + blur**, which is exactly what private-banking apps do to seem important. The new composition shows Leah as **confident enough to share the screen** — the answer is the design, not the chrome around it.

## 5. Differentiation verdict (vs. Raycast Pro)

**No, Tri would not switch.** Raycast launches in ~80 ms, has 800+ commands, AI chat with multiple models, pro-grade extension ecosystem, and costs $10/mo. Leah's differentiation per the spec is: ambient HUD, widgets-on-demand, voice, "regal sleek" aesthetic.

- **Ambient HUD**: Tri already glances at the menubar. The HUD is 7 earned pixels showing data he can get from a glance at his calendar app. Not a wedge.
- **Widgets-on-demand**: 13 widgets the LLM decides between is a worse UX than 800 commands the user picks deterministically. Less control, not more.
- **Voice**: Siri exists. Whisper-flow exists. Voice as differentiator in 2026 is table stakes, not edge.
- **Regal sleek**: aesthetic is not a switching cost. Tri has used Raycast for years; "prettier" doesn't beat muscle memory.

The actual potential wedge — and the spec barely names it — is **contextual omniscience** ("MAY-19 shipped, PR #321, 8 minutes, B2 unblocked"). That answer requires Leah to be **wired into Tri's work**: Linear, GitHub, his repo, his calendar, his email. The spec spends 431 lines on widget schemas and 4 lines on what makes the answer real. If Leah ships its current spec, it loses to Raycast Pro. If Leah ships **just the answer-engine + minimal panel** and skips the ornament, it has a chance — because Raycast Pro can't ever know Tri's repo, Linear, and calendar the way a personal local daemon can.

## 6. Final verdict

**Cut-then-ship.** The thesis is salvageable; the spec is not. Concretely:

- Delete §2.7 voice canon, §3.6 light mode, §5.4 flourishes, §13.5 gold seam, 8 of 13 widgets, §14 decisions log, §18 changelog, "operator" noun, 3 wizard steps, killer-screenshot composition.
- Rewrite §0 thesis to lead with **contextual omniscience**, not "presence + JARVIS + haute horlogerie." The aesthetic is the wrapper; the answer is the product.
- Rewrite §13.14 to the warm-ivory-no-blur composition above.
- Then ship: ~1,400 lines, dark-only v1, 5 widgets, system voice, panel + menubar + wizard. Add light mode + voice canon + more widgets in v2 once the answer-engine is proven.

The spec as-is is the design equivalent of a dispatch with 80 commits and no commit message — every fold is defensible in isolation; the whole is incoherent. Ship the cut version.

---

### Appendix — measured noise

| Metric | Count | Reading |
|---|---|---|
| Total lines | 2,564 | 1-person product, 8 reviewers |
| "Gold" mentions | 128 lines | brand-mark only? |
| Motion/animation mentions | 149 lines | "quiet by default"? |
| "Chamber" (renamed to "panel") | 315 occurrences | sweep incomplete |
| "Sigil" (renamed to "mark") | 139 occurrences | sweep incomplete |
| "Operator" (cosplay noun) | 210 occurrences | not swept |
| "Summon" / "summoned" | 109+6 occurrences | not swept |
| Hairline mentions | 45 lines | the chrome IS the chrome |
| Widget catalog | 431 lines, 13 types | feature pile |
| Anti-patterns explicitly killed | ~45 rows | reactive, not generative |
| Reviewers consulted | 8 | each fold defensible; sum incoherent |
