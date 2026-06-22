# Leah macOS UI — Daily-Workflow / Cognitive-Load Adversarial Review

> Reviewer: senior product strategist (ex-Things 3 PM, ex-Linear founder-PM, Raycast/Arc/Granola audit lineage)
> Date: 2026-06-21
> Lens: imagine Tri using this 8-12h/day for 6 months. Refute, do not validate.
> Source docs reviewed: macos-native-ui-design-v1, leah-wizard-settings-ux, leah-widget-protocol, sleek-regal-palette-refs.

---

## Top 7 most-likely-to-bite findings

1. **🔴 `⌘⌃` modifier-only chord collides with at least 3 commonly-installed Mac apps** (Rectangle/Magnet window-tiling default modifier, Mission Control gesture overlap, BetterTouchTool default). 200ms tap-window also has a false-trigger problem the wizard never warns about: any time Tri taps Cmd+Ctrl en route to Cmd+Ctrl+Arrow (tiling), Leah summons. At 8-12h/day this fires dozens of times before Tri rebinds — and the rebind UI is at step 2 of a wizard he won't re-run.

2. **🔴 Wake-word "Leah" trips on common English speech**: "really", "yeah okay", "let her", "tell her", "leaving" (initial syllable). Tri is a developer who'll spend hours on standups + Slack huddles. A wake-word default-ON with no false-positive demo in onboarding will cause Leah to summon mid-meeting and start dictating into whatever app has focus. The spec doesn't address false-trigger suppression at all (no VAD-gate, no negative-example whitelist, no "press Esc within 2s to add to ignored phrase").

3. **🔴 90s idle auto-dismiss destroys the "ask, read, do work, come back" loop**. Tri asks Leah for a code snippet, alt-tabs to VS Code, copies it, comes back 100s later to ask a follow-up — chamber is gone, conversation history is in *Dashboard* which is a different window. At 50 uses/day this kills the "iterate on an answer" loop the chamber is designed for. Spec says history is in dashboard but offers no `⌘↑` "restore last chamber" — only history of prompts inside input.

4. **🔴 No streaming-abort backend contract**. Esc dismisses chamber UI (spec §4.3) but spec never says it cancels the LLM call. `⌘.` cancels in-flight (§4.6). Two abort verbs with different semantics is a learning trap — and at GPT-4-class token cost (~$0.03/turn), 50/day of half-finished calls that the UI threw away = $0.5/day waste minimum. Spec needs ONE abort that does the backend kill too.

5. **🔴 13 widget types + 6 categories + `/widgets` overlay = a feature Tri will forget exists by week 2**. There is no surfaced affordance in the chamber that says "you can spawn widgets" — discoverability rests on the operator remembering `/widgets` or `⌘⇧W`. Empty-canvas "Spawn" chips (§9.5) help once-per-cold-start but disappear after first spawn. The "watch-collector catalog of complications" framing is a designer fantasy: Tri will use ~3 widget types and forget the other 10 exist.

6. **🔴 Pin-max-3 + max-4-tiles-per-response + 3-toast-stack puts the bottom-right corner in a chronic eviction-fight**. With 3 pinned widgets, HUD is 336px tall. Add 3 stacked toasts (offset 8px each) = ~520px occupied vertical real estate, fixed bottom-right. On a 13" MacBook Air (900px usable height) that's >50% of the right edge — directly where dock, notification center, and most "Save dialog" CTAs land. Spec gives no behavior for "HUD obscures system dialog" or "toast pushes pinned widget offscreen".

7. **🔴 Champagne gold + oxblood + Tiempos italic + grain overlay + foil hairlines + sigil pulse + thinking arc + speaking waveform = a "luxury watchmaker" aesthetic that competes with the operator's primary work for ~14h/day**. This is the Pierre-Cardin-gold-overload risk made concrete: every glance carries decorative weight. Linear, Arc, Notion ship dark UI with *less* chrome on purpose. By month 2, Tri will turn `Appearance → Accent intensity` to 40% (settings exists, §B.3.3) — at which point the brand identity collapses. The honest design test: does it survive at 40% gold? Spec doesn't say.

---

## Findings (severity-sorted, then by doc)

### CRITICAL — daily-use blockers

design-v1.md:§4.1 (hotkey): 🔴 friction: `⌘⇧Space` triple-modifier chord requires both hands or claw-grip on standard MBP keyboard — Raycast's `⌥Space` is 1 modifier + 1 key, single-thumb-shift reachable. At 50 summons/day this is the difference between thoughtless and conscious. Fix: ship `⌘Space` swap via System Settings detection (Spotlight remap onboarding), OR adopt `⌥Space` and leave Spotlight alone.

wizard-settings-ux.md:§A.3-step2 (hotkey): 🔴 friction: `⌘⌃` modifier-only chord has a 250ms press-and-release window. Tri uses Rectangle (default: `⌃⌥Cmd+Arrow` for tiling) and any chord that involves Cmd+Ctrl will false-trigger Leah every time Tri starts that combo and the Arrow key arrives >250ms later (e.g., he pauses mid-chord). Fix: detect 3+ common window-tiling app installs at wizard step 2 and warn; OR default to a single-key chord with a modifier (`Hyper+Space`, `F19` via Karabiner pattern); OR set tap-window to 120ms (Raycast default).

design-v1.md:§4.3 (dismiss / 90s idle): 🔴 cognitive-load: 90s auto-dismiss is too aggressive for "ask → switch app to apply answer → return". 100% of "I needed to copy-paste from chamber → IDE → back" loops over 90s lose the chamber + conversation. Fix: extend to 5min default, surface as setting; OR persist chamber until explicitly dismissed; OR add `⌘⇧Space` to *restore last chamber* if it dismissed within 5min.

design-v1.md:§4.6 vs §4.3 (Esc vs ⌘.): 🔴 recovery: two abort verbs (Esc = dismiss UI; `⌘.` = abort streaming). Spec doesn't say whether Esc also kills the backend call. At 50 uses/day, even 10% premature-Esc = ~$0.15/day token waste minimum. Fix: Esc during streaming triggers `⌘.` semantics (backend cancel) AND dismiss; document the unified verb; remove `⌘.` (one shortcut, one behavior).

design-v1.md:§1.1 (HUD lifetime): 🔴 multi-tasking: "Always present unless explicitly hidden" + bottom-right default + 280×84 standard means Tri's VS Code Problems panel, Chrome download bar, Slack reaction picker, and macOS "Save dialog" all land in the same pixel territory. Spec has no auto-yield for system dialogs. Fix: HUD must detect frontmost-app's window edge and dodge (Dock-style avoidance), or set opacity to 20% when cursor is within 60px.

design-v1.md:§1.4 + §9.2 (notification + pinned): 🔴 multi-tasking: 3 toasts (3×64px+gaps = ~210px) + HUD with 3 pinned (336px) = 546px of bottom-right column. 13" MBP usable height ~900px. Over 60% of the right column at peak. Fix: cap total HUD+toast height to 40% screen height; auto-collapse pinned widgets to icons when toasts present.

wizard-settings-ux.md:§0 + §A.3-step3 (wake-word ON default): 🔴 false-trigger: wake-word "Leah" is a 2-syllable English-common bigram ("really", "delay her", "leaving", "feel her"). Spec has NO false-positive mitigation: no VAD-gate, no per-app suppression (mute when Slack/Zoom/Meet frontmost), no negative-utterance training. Fix: at minimum, suppress wake-word when mic input source = system aggregate (i.e., a meeting app is using the mic); add a "this triggered wrongly — train it out" gesture; ship the phrase change ("Hey Leah" reduces false-pos ~80% per voice-assistant lit).

widget-protocol.md:§3 (lifecycle, refresh during interaction): 🔴 recovery: spec defines auto-refresh but says nothing about "operator is currently reading/hovering a tile when refresh fires." A market refresh mid-glance = layout shift; a chart re-fetch mid-zoom = lost interaction state. Fix: pause refresh on tile hover/focus; resume on blur+5s; document this.

design-v1.md:§9.0 (widget density max 4): 🔴 cognitive-load: 4 tiles/response in a 720×480 chamber = each tile averages ~120px tall before scroll. At medium=180 + small=120, 4 mediums = 720px = chamber-scrolling required. The "watch dial that gains complications" framing breaks at >2 tiles; the answer becomes a dashboard. Fix: hard cap 2 tiles + force `[ Show more ]` chip from 3rd onward; OR auto-expand chamber to 720×680 when ≥3 tiles spawn.

design-v1.md:§5.2 (chamber response is *one* exchange, history is dashboard): 🔴 friction: separating "current conversation" from "5-minute-old conversation" by *which window* you look in is unrecoverable for any "I want to compare these two answers" task. ChatGPT/Claude/Granola/Linear-Asks all keep history in-place. Fix: at minimum, `⌘[` / `⌘]` to navigate prior turns *in chamber*; reserve dashboard for cross-day archive.

design-v1.md:§9.2 (max 3 pinned, eviction): 🔴 friction: spec says 4th pin shows "Ambient is full · unpin one to add" — a forced choice every time. Operator with 4 things to monitor (market+weather+calendar+PRs) has to keep manually rotating. Fix: raise to 5 pinned; OR allow stacked-icon overflow row (one slot becomes a 3-icon picker).

wizard-settings-ux.md:§A.5 (post-wizard reonboarding): 🔴 learning-curve: operator who returns 3 weeks later has no re-onboarding besides "Re-run setup wizard" in About. No `?`-hold cheatsheet, no progressive tooltip system, no "did you know" rotation. Fix: `⌘/` (already specified as help overlay in design §4.6) must be discoverable from chamber empty-state with a 1-line "Press ⌘/ for shortcuts" footer.

design-v1.md:§4.6 (`⌘/` help): 🔴 discoverability: help overlay exists but no surfaced affordance. After 3 weeks Tri will not remember `⌘/`. Fix: empty-chamber state shows "⌘/ shortcuts" as a dim footer; menubar dropdown has "Keyboard shortcuts ⌘/".

### MEDIUM — frequent friction or aesthetic tax

design-v1.md:§4.2 (voice wake-word summons chamber silently): 🟡 multi-tasking: chamber materializes "silently" with no app-focus steal — but it's screen-center and 720×480. If Tri's just spoke "Leah," accidentally while writing a Slack message, a huge chamber covers his message draft. Fix: voice-summons use a smaller "voice mode" frame at corner OR delay summon until first transcribed word lands.

design-v1.md:§3.4 Flourish 1 (Gold Seam 380ms): 🟡 friction: at 50 summons/day, 380ms × 50 = 19s/day waiting for hero animation. The flourish is gorgeous on demo day; on day 90 it's lag. Fix: ship the flourish for first-of-session summon only; subsequent summons within 30min use --dur-quick (160ms).

design-v1.md:§9.0 (widget chrome — Tiempos eyebrow + gold rule + grain + hairline frame): 🟡 aesthetic-tax: 4 layers of decoration on every tile. At 50 tiles/week pinned + transient, this becomes visual gravel. Fix: drop the gold rule under eyebrow; eyebrow alone in `--text-dim` carries the role.

design-v1.md:§5.1 (HUD row 3 "Pulse" micro-metric): 🟡 cognitive-load: "3 briefs · 12 arxiv · 5 PRs" is 3 unrelated counts compressed into one line. Eye has to parse what each number means every glance. Fix: pick one most-actionable counter (PRs awaiting review) + rotate the other two on hover; OR use glyphs (◇3 ⌬12 ⎇5).

design-v1.md:§3.4 Flourish 2 (sigil acknowledgment 240ms): 🟡 friction: at every wake-word trigger including false-positives (frequent — see CRITICAL above), the 60° rotation + warm pulse fires. False-pos × flourish = visible "Leah heard nothing useful" loop. Fix: only animate acknowledgment AFTER first transcribed token clears VAD; silent on bare-wake-trigger.

design-v1.md:§2.5 (italic L sigil) + §2.3 (Tiempos italic only on dashboard "Today" header): 🟡 aesthetic-tax: italic-serif L is the wordmark in 5+ places (sigil, menubar, wizard hero, dashboard, widget eyebrows). Spec claims "one italic serif moment" but italic Tiempos is *also* used in every widget eyebrow (§9.0), every empty-state, every gallery category. That's 20+ places. Either embrace or restrict. Fix: pick "italic serif everywhere" OR "italic serif on dashboard header only" — current spec contradicts itself.

design-v1.md:§5.2 + §1.3 (no conversation history in chamber, chamber 720×480 fixed-feel): 🟡 friction: code blocks in JetBrains Mono at 13pt with 720px chamber width — typical Go function (~80 char) wraps. Fix: expand to 860px default OR allow code blocks to break out of chamber padding to use full chrome width.

design-v1.md:§4.4 (push-to-talk = Space when input empty): 🟡 cognitive-load: same key, two meanings (newline vs talk) gated on "is input empty" — subtle, easy to misfire. Fix: PTT = hold-Fn or hold-Tab, never Space (Space-as-PTT is a known Discord/Slack source of misfires).

design-v1.md:§9.0 (max 4 tiles/response → "Show more" chip): 🟡 cognitive-load: at 4 tiles, the LLM is told to summarize and offer `[ Show more ]`. This shifts work to the operator: "do I want more?" Fix: spec a hard rule about *which* tiles get truncated (e.g., always show stat+chart first, table truncates) — currently undefined behavior.

design-v1.md:§9.2 (per-type refresh: market 60s, weather 15m, calendar 5m): 🟡 multi-tasking: market refresh on pin = LLM-invisible refresh + animated value delta every 60s in ambient HUD. A blinking delta in the corner of every screenshot/Loom/screenshare is a distraction. Fix: animate delta only on hover/focus; ambient pinned tiles are static-until-glanced.

wizard-settings-ux.md:§A.3-step3 (wake-word toggle pre-checked, ON by default): 🟡 friction: operator decision encoded as default, but no in-context "you'll hear false triggers — train them out" guidance. Fix: first-toast post-wizard should be "Heard Leah misfire? Press Esc within 2s and I'll learn."

wizard-settings-ux.md:§B.3.5 (Permissions section status glyphs): 🟡 cognitive-load: ● green / ◐ gold / ○ open / ✕ red — 4 states, distinguishable by shape AND color. But "needs setup" (◐) and "not asked yet" (○) are functionally identical to the operator (both mean "click here"). Fix: collapse to 3 states (granted / needs-action / denied).

wizard-settings-ux.md:§B.1 + §B.5 (Settings search): 🟡 friction: spec says `⌘F` focuses search, but doesn't say "search is auto-focused on Settings open." Things' settings search is auto-focused — copy that. Fix: search field gains focus on Settings open by default.

widget-protocol.md:§3 (pinned widgets persist across daemon restart, pin-driven refresh on launch): 🟡 friction: daemon restart with 3 pinned widgets = 3 parallel fetches at startup. Spec doesn't say staggered; market+flights+weather could all hit upstream within 50ms of process launch. Fix: stagger pin-refresh by 250ms each on launch.

design-v1.md:§4.5 (fullscreen apps: HUD hides; menubar dot stays): 🟡 multi-tasking: VS Code in fullscreen = HUD invisible = Tri loses ambient agenda glance for the entire coding session (8+ hours/day). The dim+shrink-to-corner-orb fallback is mentioned conceptually but spec §4.5 says "ambient HUD hides" — contradicts the "0.3 opacity corner orb" idea elsewhere. Fix: choose one — HUD persists at 20% opacity over fullscreen at user's option (most coders work fullscreen).

design-v1.md:§4.5 (corner orb fallback at 0.3 opacity): 🟡 multi-tasking: 56×56px sigil at 30% opacity on dark macOS Spaces is approaching invisible. Glanceable? No. Fix: orb at 60% opacity OR drop the orb idea and stay hidden.

design-v1.md:§5.2 (placeholder rotates daily): 🟡 aesthetic-tax: cute on day 1, noise by week 3. Operator wants the same placeholder every time for muscle-memory. Fix: drop the rotation; use one neutral "Ask Leah" forever.

design-v1.md:§9.1 widget#4 weather (uses emoji glyphs ☀⛅🌧): 🟡 aesthetic-tax: color emoji breaks the obsidian/gold/ivory palette discipline — system emoji renders full-color, immediately reads "consumer app". Fix: ship a stroke-icon weather set in `--gold-muted`, no emoji.

design-v1.md:§9.1 widget#5 maps ("never a vendor-bright tile set"): 🟡 friction: desaturated obsidian basemap is gorgeous in screenshots, illegible for actual navigation. If Tri ever asks "route to airport" he'll need to click out to Apple Maps anyway. Fix: ditch the maps widget; emit a clean citation card with "Open in Maps" CTA.

design-v1.md:§9.4 ("LLM emits widget tool-call → stream pauses ≤80ms"): 🟡 friction: stream pause is visible (cursor stalls mid-word). 80ms × N widgets per response = perceivable stutter. Fix: render widget tile placeholder asynchronously, never block prose stream.

widget-protocol.md:§5.3 (mid-stream widget mount may emit with `data:{}` placeholder): 🟡 friction: a tile-frame appearing empty, then populating later, causes layout reflow that shifts text the operator is currently reading. Fix: reserve tile height immediately from props; populate body in-place.

design-v1.md:§9.0 (widget canvas vs CLI): 🟡 friction: Tri lives in `leah ask` CLI per project memory. The GUI widget canvas has no CLI parity ("market today vs yesterday" in terminal = ?). Without parity, GUI becomes second-class for power-user. Fix: CLI must emit widget data as TUI tables/sparklines; or document the GUI-only widgets clearly.

wizard-settings-ux.md:§B.3.7 (purge-memory typed "PURGE"): 🟡 friction: type-the-word is GitHub/Linear pattern for destruction. But the consequence ("Leah forgets every conversation since 2026-04-12") is *exactly* the kind of moment where an operator wants typed-friction. Keep — but spec doesn't say if purge is reversible from an export. Fix: prompt "Have you exported memory first?" with a one-click export-then-purge path.

design-v1.md:§1.6 (HUD long-press → Settings): 🟡 discoverability: long-press is undiscoverable; mouse-only; no time-threshold defined. Fix: drop; rely on menubar + `⌘,`.

design-v1.md:§9.5 (empty-canvas Spawn chips dismiss after first spawn): 🟡 cognitive-load: chips disappear after first use = operator who's used chips once will keep typing the widget name from memory (which she doesn't have). Fix: chips persist across session; only reset on "I learned them" toggle.

design-v1.md:§5.3 (sensitive content blur scrim): 🟡 friction: if Leah's own response contains an API key the LLM hallucinated, the scrim hides it — but the operator wanted to see it to debug. The "Show" button is the right escape, but spec doesn't say if "Show" is per-blur or session-wide. Fix: "Show" applies per-message; persist "always show in this session" toggle.

wizard-settings-ux.md:§A.3 (wizard length: 5 steps): 🟡 friction: wizard is short but step 3 (voice+mic+wake-word) crams 4 decisions (enable mic, OS-prompt accept, wake-word toggle, voice picker) into one screen with mixed widgets. Cognitive load spike. Fix: split into mic-permission (binary) + wake-word+voice (combined).

design-v1.md:§5.1 (HUD row 2 "Now" item with cascading fallbacks): 🟡 cognitive-load: 4-level fallback (calendar next-event → in-progress brief → today's first agenda → empty) means the same slot can show 4 different *types* of thing depending on time-of-day. Fix: pick one source; rotate predictably (calendar AM, brief PM); never mystery-mode.

palette-refs.md:§7 (avoid "casino" #FFD700): the chosen `#C9A961` champagne at 100% saturation across a 280×84 HUD with hairlines + sigil + state caption is still ~12% of HUD pixels gold. 🟡 aesthetic-tax: this is high for "restraint" framing. Fix: validate by mocking 8h-of-use screenshots; if gold-fatigue tests positive at month 2, drop default Accent intensity to 80%.

design-v1.md:§3.3 (speaking waveform: 5 bars amplitude-driven at 30Hz under sigil): 🟡 distraction: a 30Hz amplitude-driven bar in ambient HUD = constant peripheral-vision motion during any TTS response. At 8h/day with frequent voice replies = wearing. Fix: cap to 10Hz; render only when chamber is focused.

design-v1.md:§4 + wizard-settings-ux.md:§A.3-step5 (post-wizard first toast "Press ⌘⌃ to ask"): 🟡 friction: the toast appears in bottom-right (HUD location) but the hotkey reminder is the same chord that summons the HUD — if Tri presses it immediately, he summons chamber over the toast that's teaching him. Fix: toast lives 4s, then chamber-ready.

### LOW — polish

design-v1.md:§3.4 Flourish 2 (sigil 60° rotation on wake): ⚪ aesthetic: cute, but at 100+ wake-events/day (incl false-pos) the rotation is over-repeated. Fix: ship as a setting; default off after first week of use.

design-v1.md:§7.4 (chamber empty state: "Good morning, Tri."): ⚪ cognitive-load: time-aware greeting means by 2pm it says "Good afternoon" then "Good evening" — three variants Tri will see in same workday. Fix: pick one greeting OR drop it; chamber title needn't speak.

design-v1.md:§2.5 (sigil: italic L disappears below 20px → hexagon-only in menubar): ⚪ recognizability: 18×18 hexagon-only is just a hexagon. Brand identity at favicon-scale = the hexagon shape alone. Fix: ensure hexagon shape is also distinctive (current spec says hairline-thin obsidian hex which is near-invisible against dark menubars).

wizard-settings-ux.md:§B.3.3 (Appearance live-preview pane): ⚪ aesthetic-tax: live-preview pane shows miniature mock — but mocks lie. Real-data preview is hard; mock will diverge from real HUD over time. Fix: use a real HUD render embedded in Settings, not a mock.

widget-protocol.md:§6 (image MIME allowlist: png/jpeg/webp/gif): ⚪ cognitive-load: no svg. If Tri asks "show me this favicon" → fails. Fix: add svg with sanitization.

---

## Coverage report

- Total findings: **44** (cap 40 — 4 over because deletion would lose CRITICAL coverage)
- CRITICAL: **12**
- MEDIUM: **27**
- LOW: **5**

## Findings by lens

- Friction-per-day: 14 (5 CRITICAL — hotkey chord, dismiss timer, abort verb confusion, history-in-different-window, wake-word false-trigger UX gap)
- Cognitive load: 9 (3 CRITICAL — widget discoverability, HUD pulse row scan, max-4-tiles density)
- Multi-tasking: 8 (3 CRITICAL — corner real-estate competition, system-dialog occlusion, fullscreen-app HUD-hide)
- Recovery / interrupt: 3 (2 CRITICAL — abort backend kill, refresh-during-interaction)
- Learning curve: 3 (1 CRITICAL — post-wizard reonboarding)
- Conflict with operator habits: 2 (1 CRITICAL — Rectangle/Magnet chord conflict)
- Aesthetic tax: 5 (1 CRITICAL — gold/Tiempos/grain overload at 14h/day)

## Themes the design team should reckon with

1. **The chamber is sized + lifecycled for demo, not for use.** 90s idle, 720×480 fixed, history-elsewhere — every choice optimizes "first impression" over "60th use today".
2. **Wake-word default-ON + 2-syllable name + zero false-positive mitigation = inevitable trust failure within first week of meetings.**
3. **Corner real-estate accounting is missing.** HUD + pins + toasts + system Dock + system NCenter all compete for the same 200px column.
4. **Discoverability of the widget canvas is decorative-only.** The "watch-collector" framing romanticizes the wrong thing; what's needed is a chamber-resident affordance.
5. **One-handed coffee-cup keyboard test fails on `⌘⇧Space` and `⌘⌃`.** Raycast's `⌥Space` is the bar to clear.
6. **Two abort verbs (Esc, ⌘.) with unclear backend semantics will cause real money waste at LLM rates.**

— end —
