# Leah — macOS Native UI Design (canonical spec)

> Date: 2026-06-21 · Version: **v3.2.2**
> Status: design lock. Supersedes `docs/research/2026-06-21-leah-wizard-settings-ux.md`, `docs/research/2026-06-21-leah-widget-protocol.md`, `docs/research/2026-06-21-sleek-regal-palette-refs.md`.
> v2 folded 4 adversarial reviewer reports (Nielsen+a11y, visual-craft+HIG, daily-workflow, perf+memory) and 3 operator overrides (wake-word default OFF, hotkey `⌥Space`, light+dark mode parity). v3 folded all remaining MEDIUM-severity findings. v3.1 folded 3 adversarial v3-reviewer reports (consistency/traceability, implementability, first-impression/brand). v3.2 folded 3 adversarial v3.1-reviewer reports (regression-hunt 50 findings, ship-readiness, coherence) — atomic rename + 19 blind-spot fills + §16.7 parity-test extraction + §19 value-first build order. **v3.2.1 folds 7 operator-locked ship decisions** from 4 vendor-research reports (LLM provider, embeddings/vector, TTS, key custody) — framework lock (SwiftUI + AppKit), LLM mix (Sonnet 4.6 + Haiku 4.5 via Anthropic Go SDK daemon-side), embeddings (Voyage 3.5-lite + BGE-small-en local), vector store (sqlite-vec), TTS (ElevenLabs Flash v2.5 + Apple Ava Premium), API-key BYOK + Keychain, Sparkle EdDSA custody. No scope cuts.
> Audience: implementation team. **Distribution: Developer ID + notarization + Sparkle auto-update via GitHub Releases; NOT Mac App Store.** Framework: **SwiftUI + AppKit (NSPanel for HUD + focus panel)**, Swift Package Manager for deps, macOS 14.0 minimum.

---

## 0. Executive summary

Leah is a presence on the operator's machine — quiet, regal, instantly responsive — that surfaces a glass panel when summoned. The aesthetic is **operator-overlay** (JARVIS lineage) crossed with **haute horlogerie** (Patek / Vacheron / A. Lange restraint). No cyan, no neon, no holographic gradients. A single gold mark against deep obsidian carries the brand.

Seven surfaces share one design language: ambient HUD, menubar, focus panel, notification widget, first-launch wizard, settings pane, dashboard. The focus panel renders a **dynamic widget canvas** — prose and tiles interleave; tiles can be pinned to ambient for persistent glance-value.

Key locked decisions (v2):
- **Hotkey:** `⌥Space` (Option+Space) — single modifier + letter; fires on keydown, no chord-disambiguation latency. Raycast-class felt-instant (<100 ms).
- **Wake-word:** **OFF by default** (opt-in via Settings → Voice). Wizard step 3 asks for microphone only; wake-word is a one-line opt-in card on the same step.
- **Appearance:** **light + dark mode parity.** Auto-follows `NSApp.effectiveAppearance`; both palettes specified in § 2 and § 2.6. Operator override in Settings → Appearance.
- **Tint policy:** gold is **brand-mark only** (mark, focus-panel border, primary CTA, divider seams). All other tinting (selection, hover, focus rings on text fields, slider tracks, links) uses `NSColor.controlAccentColor` per HIG.
- **Permissions:** lazy-prompt at first-use; wizard asks ONLY for microphone. Accessibility (needed for global hotkey) prompted with a clear breakage warning at wizard step 2.
- **Fullscreen behavior:** ambient HUD + panel dim+shrink to a corner orb @ 0.6 opacity (raised from 0.3 — see § 6.5); optional render to a dedicated secondary monitor if operator opts in.
- **Wizard:** 6 steps (welcome / BYOK Anthropic key / hotkey / mic / one-integration / ready). One integration only (Calendar pre-selected); the rest live in Settings. Step 2 (BYOK paste-key) inserted in v3.2.1; see §13.15.
- **Widget protocol:** 13 widget types over a single `render_widget` tool-call, JSON Schema draft-07, daemon-side adapters, persistent local IPC, closed registry (no plugins in v1).
- **Palette (dark mode):** obsidian (`#08090C`) + champagne gold (`#C9A961`) + oxblood (`#7A1F2B` / `#C8434F`) + ivory (`#F2EDE0`). Contrast table recomputed in § 11.1 with corrected `--text-dim` (`#8A8478`) and `--red-alert` (`#D75A66`); all pairs pass WCAG AA at intended sizes.
- **Palette (light mode):** bone (`#F2EFE8`) + darkened champagne (`#7A6332`) + oxblood (`#5C1620` / `#A8323D`) + graphite (`#1E1D1A`). Full token table in § 2.6.

Three doctrinal rules govern every detail: earn every pixel, quiet by default, recognizable at 100 px.

**Distribution + cost (v3.2.1):** BYOK Anthropic (operator pastes their own Anthropic API key in wizard step 2; stored in Keychain `kSecAttrAccessibleWhenUnlocked`; no embedded org-key, no subscription tier — Raycast Pro pattern). Sparkle auto-update via GitHub Releases + GitHub Pages appcast (`https://maydow.github.io/leah/appcast.xml`); $0 infra. **Steady-state operating cost ~$31–42/mo at 100 queries/day**: Anthropic LLM ~$20 (Sonnet 4.6 primary + Haiku 4.5 router, ZDR workspace, prompt-cache 85% hit) + ElevenLabs TTS $11–22 (Flash v2.5 Creator plan) + Voyage 3.5-lite embeddings free-tier + GitHub Pages/Releases hosting free.

### 0.1 Brand positioning (v3.1)

Leah looks like **a tool a serious operator chooses, not a fintech dashboard**. The v3 brand audit flagged "private-banking-app" as the wrong category cousin (Mercury, Ledger, Robinhood Gold). v3.1 retunes: balance the gold restraint with **warmer ivory as the primary fg surface**; reserve oxblood for critical-alert iconography ONLY (no oxblood-on-gold pairings); friendlier user-facing names (no "mark," no "panel," no "transition" in copy). The IA stays — the costume gets a tailor. See §15 anti-pattern "Private-banking-app aesthetic" for what we did instead.

### 0.2 The one screenshot that sells this (v3.1 — first-impression #5)

The marketing-hero composition is **the focus panel summoned mid-flow over a real macOS desktop** — Finder + Slack + a VS Code window visible at 0.6 opacity behind the panel blur — with the panel showing **one widget tile + ivory prose + minimal gold (hairline + L mark only)**. The contextual answer is something only Leah could know ("MAY-19 shipped at 4:47pm — PR #321 merged by Tri, reviewed in 8 minutes; B2 is unblocked"). Every §13 wireframe MUST produce that screenshot; the gold-budget invariant (max 3 gold instances per visible surface) enforces it mechanically. See §13.14 hero-composition spec.

---

## 1. Operator-decision override table

Decisions that override source-doc defaults.

| # | Decision | Source-doc default | Locked here | Citation |
|---|---|---|---|---|
| 1 | Global hotkey | v1: `⌘⌃` modifier-only chord (250 ms tap-window) | **`⌥Space` (Option+Space)** — single modifier + letter; fires on keydown (no disambiguation latency); avoids Rectangle/Magnet/Mission-Control chord collisions | **v2 operator override** (supersedes v1 row 1). Felt-instant <100 ms; perf-review #5; workflow-review #1; HIG review on bare-modifier collision |
| 2 | Wake-word default | v1: ON, pre-checked in wizard | **OFF by default** — opt-in via Settings → Voice. Wizard step 3 asks for mic only; one-line wake-word opt-in below (unchecked) | **v2 operator override** (supersedes v1 row 2). Privacy posture + battery (perf #1) + false-trigger (workflow #2) + Nielsen #9 |
| 3 | Appearance | v1: dark-only ("non-goal") | **Light + dark parity** — auto-follow `NSApp.effectiveAppearance`; both palettes specified (§ 2 dark, § 2.6 light); operator override in Settings → Appearance | **v2 operator override**. HIG mandate; review craft-HIG "v1:everywhere no light mode" |
| 4 | Permissions strategy | main: 3 upfront (mic, accessibility, screen-recording) in wizard | **Mic only in wizard; Accessibility flagged as required-for-hotkey with breakage warning + Settings deep-link; all others lazy-prompt + Settings → Permissions** | wizard/settings § 0; HIG-review on `NSAccessibility` System-Settings dance |
| 5 | Wizard integrations | main: 6 services in wizard step 5 | **One radio-card pick (Calendar / Mail / Files); rest in Settings → Integrations** | wizard/settings § 0, § A.3 step 4 |
| 6 | Wizard length | main: 6 steps | **6 steps** (welcome / BYOK Anthropic paste-key / hotkey + Accessibility / mic / one-integration / ready). HUD-position picker stays in Settings → General; BYOK step is new v3.2.1 — see §13.15. v3.2.2 explicitly locks 6 (was contradicted by §8 "5 steps" prose) | wizard/settings § 0 + §13.15 + §17.18 |
| 7 | Fullscreen behavior | main § 4.5: "ambient HUD hides; menubar dot stays" | **Dim+shrink to corner orb @ 0.6 opacity; optional dedicated secondary monitor if operator opts in (Settings → General)** | new in this spec — § 6.5; workflow-review #34 opacity bumped from 0.3 |
| 8 | Palette source-of-truth | palette-doc A used `#08090C`, `#C9A961`, `#8B1E2D` | **Main-doc tokens win** (`#08090C`, `#C9A961`, `#7A1F2B`); palette doc's `#C9A961` rationale retained | conflict resolution: visual-identity spec specificity beats research memo |
| 9 | Typography | palette-doc A: Cormorant + SF Pro + SF Mono | **Inter (body + display) + JetBrains Mono (code) + New York Italic (one editorial moment — dashboard "Today" header ONLY)** — v3.2: Söhne + New York Italic moved to Tiempos-as-optional-licensed-upgrade footer; Inter + New York are v1 primary | main §2.3 + §3.3; craft-HIG #4 (italic confined); v3.2 type-stack lock |
| 10 | Mark container | n/a in palette doc | **Hexagon, not circle** (mechanical, watch-crown lineage) | main § 2.5 |
| 11 | State paths | v1 used `~/Library/Application Support/Leah/` | **`~/Library/Application Support/Leah/`** for state; `~/Library/Caches/com.leah.daemon/` for purgeable widget cache | HIG-review on Apple convention |
| 12 | Capture detection | v1: `CGScreenIsCaptured()` | **`SCShareableContent` observers (ScreenCaptureKit, macOS 12.3+)**; deprecated API removed | HIG-review #34 (Nielsen) + perf #34 |

---

## 2. Design thesis

Leah is not a chatbot in a window. She is **a presence on the operator's machine** — quiet, regal, instantly responsive — that occasionally surfaces a glass panel when summoned.

The aesthetic is **operator-overlay** (JARVIS lineage) crossed with **haute horlogerie** (Patek Philippe, Vacheron Constantin, A. Lange & Söhne site type, restraint, gold-on-obsidian). The result is sleek, regal, not sci-fi cosplay. **No cyan, no neon, no holographic gradients.** A single mark of gold against deep obsidian carries the brand.

Three rules govern every decision:

1. **Earn every pixel.** Ambient surface shows ≤7 data slots and renders at <2% screen area. Focus surface shows one conversation, full attention.
2. **Quiet by default.** Animation is restrained — Patek not Pixar. Sound off. The hero transition is the focus panel materializing; everything else is sub-200 ms cross-fades.
3. **Recognizable at 100 px.** The mark — a gold engraved "L" inside a hairline-thin obsidian hexagon — is the lockup. Anyone seeing a screenshot at thumbnail size knows it's Leah.

### 2.7 Leah voice canon (NEW v3.1 — first-impression #2)

A named, gendered assistant whose entire premise is "a presence on the operator's machine" cannot ship with the operator picking from three Mac voices. **Leah has one voice the way Siri has one voice.**

| Attribute | Canon |
|---|---|
| **Tone** | Confident, dry-warm, low-affect. NOT cheerful Apple-default. NOT robotic Siri. Closer to a senior colleague speaking calmly across a desk than a chirpy product voice. |
| **Vocal profile** | Alto, ~145 wpm, mid-register, slow-warm cadence. |
| **TTS engine (default — cloud)** | **ElevenLabs Flash v2.5** Professional Voice Clone of the spec'd alto profile. TTFB 75–150 ms (Multilingual v2 was 600–1200 ms — rejected, see §15). Pre-warmed on app launch (decision-log #81). Creator plan ($11 promo / $22 list / mo). |
| **TTS engine (offline + privacy-flagged fallback)** | Apple `AVSpeechSynthesizer` voice **"Ava (Premium)"** — used when (a) cloud TTS unreachable OR (b) the sensitive-content classifier flags the text (calendar/email/finance/memory blockwords). Apple TTS is fully on-device — zero exposure. First time it fires after a network drop the fallback announces itself: `Leah's voice needs the network · using a fallback voice for now`. Privacy-flagged routing fires silently (it would be theatre to announce that the text is sensitive). |
| **Privacy routing (NEW v3.2.1)** | Daemon-side `internal/tts/classifier.go` scans the text-to-speak against a blockword corpus (calendar event titles, email subjects/bodies, finance amounts/account names, memory items). Hit → Apple local. Miss → ElevenLabs cloud. Classifier latency budget < 5 ms; runs before any audio I/O. |
| **Provider abstraction (NEW v3.2.1)** | `internal/tts/provider.go` interface — `Speak(ctx, text, voice) (AudioStream, error)`. Vendor swap = 1-day change. **Rejected:** OpenAI/Gemini Realtime APIs (would lock Leah off the Claude backbone — Anthropic ↔ ElevenLabs separation is doctrinal). See §15. |
| **Wizard step 1** | TTS plays "Hi, I'm Leah" in the canonical ElevenLabs voice. Falls back to Apple Ava Premium + the announcement copy above only if cloud unreachable at wizard time. |
| **Settings → Voice** | Exposes "Leah" voice (ElevenLabs Flash v2.5) as the default + 2 fallbacks (Apple Ava Premium + one ElevenLabs alternate for users who dislike the canon — labeled "Alternate · for variety, not for canon"). No "pick your favorite from a list" framing; the canon is named. |

The visual brand says "personal AI with a name and a face." The audio MUST agree.

---

## 3. Visual identity

### 3.0 Tint policy (HIG)

Gold is **brand-mark only**. Every other tint follows the user's system accent.

| Surface | Tint source | Why |
|---|---|---|
| Mark, focus-panel border, primary CTA fill, divider seams, listening-pulse, thinking-arc | `--gold-primary` (brand) | These are the brand mark. Constant across users. |
| Text-field focus ring, selection, hover, links, slider tracks, checkbox/radio fill, settings toggle on-state | `NSColor.controlAccentColor` (system) | HIG: "use the accent color the user has selected to tint controls." Operator's macOS pref wins. |
| Widget eyebrow rule, widget chart accent series, table header underline | `--gold-primary` (brand-mark context) | These are inside Leah's chrome — accent for brand identity. |

The "Accent intensity" slider (§ 9) was removed. Operators who want a different tint change their macOS accent.

### 3.1 Color tokens (dark mode)

Every value is a real hex. Tokens win over research-memo variants; research-memo rationale retained where it strengthens the choice. Light-mode counterparts live in § 2.6.

**Obsidian (background system)**

| Token | Hex | Use | Justification |
|---|---|---|---|
| `--obsidian-0` | `#08090C` | Focus panel bg, deepest layer | Near-black with 4 pts of blue to feel cool/regal vs muddy. Vercel `#000` is too flat; Linear `#101113` is closest cousin; Patek Philippe `#08090C`. We split the difference. |
| `--obsidian-1` | `#0E1014` | Ambient HUD bg, settings sidebar | One step above the floor for layered surfaces. |
| `--obsidian-2` | `#161922` | Card / row hover, input field bg | Hairline-rule needs a surface to land on; this is it. |
| `--obsidian-3` | `#22262F` | Pressed state, divider-on-card | Top of the stack; reads as "interactive". |

**Gold (primary accent, brand)**

| Token | Hex | Use | Justification |
|---|---|---|---|
| `--gold-primary` | `#C9A961` | Brand mark (L mark) fill, primary CTA, listening-active ring | Champagne gold (not Vegas gold). Reads "antique foil — warm, muted, restrained." WCAG: 8.4:1 on `--obsidian-0`. (v3.1: calibration-derivation note moved to §3.1 footer; `#D4AF37` is **specifically rejected** as Vegas-gold-adjacent — see §15 anti-pattern "Bright #FFD700 gold".) |
| `--gold-hover` | `#D9BC7A` | CTA hover, focused link | +1 step luminance. |
| `--gold-muted` | `#8A7340` | Disabled/decorative gold, hairline frame | −40% saturation; reads as engraved metal not lit metal. |
| `--gold-glow` | `#E8CC8C` | Listening pulse outer ring; never as fill | High-luminance outer for the pulse halo; used at 20–40% opacity. |

**Red (alert, brand secondary)**

| Token | Hex | Use | Justification |
|---|---|---|---|
| `--red-brand` | `#7A1F2B` | Critical-state mark, "L" mark on red background variant for installer/marketing. Always paired with `◆` filled-diamond icon prefix in semantic context (WCAG 1.4.1 — meaning not color-alone) | Oxblood, not Coca-Cola. WCAG: **1.95:1** on `--obsidian-0` (UI-only — outlines / brand mark on red bg; never text body). |
| `--red-alert` | `#D75A66` | Daemon-down banner, permission-denied state, destructive confirm | Recomputed v2 — old `#C8434F` was 4.14:1 (failed AA for body); `#D75A66` is **5.26:1** AA-pass. Always paired with `●` filled-circle icon prefix so meaning is not color-alone (WCAG 1.4.1; deuteranopia-safe vs oxblood). |
| `--red-dim` | `#4A1820` | Sensitive-content blur scrim tint | Background-only; never carries information. |

**Ivory (foreground)**

| Token | Hex | Use | Justification |
|---|---|---|---|
| `--text-primary` | `#F2EDE0` | Body text, response stream | Ivory not pure white. Pure `#FFF` on `#08090C` is harsh and "computer-y"; ivory reads like printed page on stained walnut. NYTimes Cooking dark-mode ivory `#F0E9D9`. WCAG: 14.8:1. |
| `--text-muted` | `#B8B0A0` | Secondary captions, source citations | WCAG: 8.3:1. |
| `--text-dim` | `#8A8478` | Placeholder, timestamps, divider labels | WCAG: **5.36:1** on `--obsidian-0` (recomputed v2 — old `#6B6558` was 3.44:1, failed AA). **Hover-state rule (v3):** placeholders + dim captions on hovered rows (`--obsidian-2` `#161922` bg) compute to 5.04:1 — still AA; freeze color on hover (no auto-lift to `--text-muted`) so hovering doesn't lose meaning (UX #3). **HUD captions on `--obsidian-1` `#0E1014` (Row 2 "Standup in 12m", Row 3 "3 briefs · 12 arxiv · 5 PRs") MUST use `--text-muted` not `--text-dim`** — time-sensitive captions are load-bearing; `--text-dim` on HUD-1 = 3.29:1 (fails AA — UX #4). |
| `--divider` | `#FFFFFF33` | Hairline rules — 20% white baseline | 1 px at 20% opacity; bumps to **1.5 px at 28%** when `backingScaleFactor < 2` (non-Retina). The luxury-watch hairline is THE detail; v1's 8% was sub-pixel on non-Retina. **Tiered divider rule (v3 — UX #5):** decorative hairlines (in-card visual rhythm) MAY use 8% white `#FFFFFF14`; **structural hairlines** that carry information (Settings sidebar separators, focus-panel section rules, table row separators, group boundaries) MUST use `--divider` 20% baseline so they hit the 3:1 UI-component floor. |
| `--focus-ring` | `#E8CC8C` | Keyboard focus indicator (any focusable element) | 2 px ring at 80% opacity, 2 px offset from element edge. Visible against `--obsidian-0` (12.75:1) AND `--bone-0` light bg (5.8:1). Never the only meaning signal — always paired with element-state change. |

**Functional tones**

| Token | Hex | Use |
|---|---|---|
| `--success-quiet` | `#7A9B7A` | One-time "saved" confirm tick. No green chrome elsewhere. |
| `--blur-tint` | `#08090CCC` | Material-blur tint (80% `--obsidian-0`). |

### 3.2 Material + depth

- **Glass blur:** NSVisualEffectView `material: .hudWindow`, `blendingMode: .behindWindow`, `state: .active`, with a tint layer at `--blur-tint`. Web equivalent: `backdrop-filter: blur(18px) saturate(140%)` (ambient HUD) or `backdrop-filter: blur(24px) saturate(140%)` (focus panel) + tint overlay. **Single radius source-of-truth (v3.1 — locked literal values, replacing v3 `<radius>` placeholder):** **18 px ambient HUD · 24 px focus panel** (drops v1's stray 28 px reference). Tiles inside the panel DO NOT add their own blur — they inherit the panel's blur surface (perf-review #18).
- **Degraded-blur state** (NEW v2): when panel overlaps continuously-redrawing content (Final Cut timeline, video player, terminal scrollback >10 Hz), profile-guided downgrade replaces NSVisualEffectView with a solid `--blur-tint` fill (no live blur). Detection via NSWindow occlusion sampling at 1 Hz; switch hysteresis 3 s to avoid flicker. See § 12 `degraded-blur` row.
- **Grain overlay:** **static texture loaded ONCE** at app start (procedurally generated 128×128 tile, cached as a single `CGImage`). NOT a per-frame composite (perf-review #2). Opacity locks to **2.5% on dark surfaces, 1.5% on light surfaces** (single source-of-truth; supersedes v1 drift of 1.5/3/2–4 %). Disabled entirely under `prefers-reduced-motion`, `accessibilityDisplayShouldReduceTransparency`, or when GPU profile reports >2 ms/frame composite cost on the current display (M1-Air-integrated baseline).
- **Hairline rule:** 1 px @ 20% opacity baseline (`--divider` = `#FFFFFF33`). On `backingScaleFactor < 2` (non-Retina): bump to **1.5 px @ 28% opacity** so the rule survives sub-pixel rounding. NEVER colored. The hairline IS the chrome.
- **Elevation system** (3 levels — no Material-style 5-step shadows):
  - **Floor** (ambient HUD, menubar popover): `box-shadow: 0 1px 0 #FFFFFF08 inset, 0 12px 32px #00000099`.
  - **Lifted** (focus panel, dashboard, settings): `box-shadow: 0 1px 0 #FFFFFF12 inset, 0 24px 80px #000000B3, 0 0 0 0.5pt #FFFFFF14`. The inset top-edge highlight is the "polished bezel" effect.
  - **Engraved** (mark on dark, settings sidebar items): `box-shadow: inset 0 1px 0 #00000066, inset 0 -1px 0 #FFFFFF0A`.

### 3.3 Typography

| Role | Family | Weight / size | Justification |
|---|---|---|---|
| **Display** | **Inter** (primary v3.1; Söhne as optional post-launch upgrade if license closed) | 500 / 28–44 pt | Inter ships OFL — bundle freely, zero license risk. Söhne (Klim, ~$600/style + per-quarter MAU app license) is aspirational; v1 ship is Inter-only. |
| **Body** | **Inter** (variable) | 400 / 14 pt @ 1.45 line | Variable axis lets us tune optical weight without font-weight steps. |
| **Mono** | **JetBrains Mono** | 400 / 13 pt | OFL; bundle freely. Code blocks; widget numeric readouts. |
| **Editorial accent** | **New York Italic** (primary v3.1; Tiempos Headline italic as optional upgrade if license closed) | 400 italic / 28 pt | New York is Apple-bundled, free, ships with macOS — visually adjacent serif italic. Tiempos (Klim, same license constraint as Söhne) is aspirational. **ONE location only** (v2 enforcement, restated v3.1): Dashboard "Today" header. Stripped from widget eyebrows, gallery categories, empty-state placeholders, code-language labels (all of those → body sans with small-caps tracking +0.04em). The editorial moment must be rare to be felt. **CJK / Cyrillic fallback chain (v3.1 — implementability B-7):** New York Italic → system serif italic for the script in question (e.g., Hiragino Mincho ProN for Japanese). Tiempos has no CJK glyphs; New York covers most via the system fallback. |

**Dynamic Type (NEW v2):** every body/label size is declared via `NSFont.preferredFont(forTextStyle:)` semantic tokens (`.body`, `.callout`, `.footnote`, `.headline`, `.subheadline`, `.title3`, `.largeTitle`). Pt values above are the defaults at 100% system text-size; surfaces reflow up to 150% system text scale without horizontal scroll. **Only telemetry / numeric readouts** (JetBrains Mono in stat / market / chart axes) use fixed pt — those are alignment-load-bearing and would mis-stack if scaled.

Ambient HUD reflows under Dynamic Type: at OS text-size ≥ 130% the Pulse row (Row 3) drops first; ≥ 150% collapses to Mini (mark only).

### 3.4 Iconography

- **Stroke (Lucide-only — v3.1 consistency qualifier):** 1.5 pt at 24×24; 1 pt at 16×16. SF Symbols inherit system stroke (do NOT apply 1.5 pt — breaks system-icon weight). NEVER filled icons in primary chrome.
- **Hit-area rule (v3 — UX #25):** every glyph-control hit-target is **24 × 24 pt minimum** (WCAG 2.5.8), regardless of visual glyph size. 16-px glyphs in primary chrome wrap in 24×24 padded hit-areas.
- **Corner radius:** 2 pt on all icon strokes (round-line-cap, round-join). Hard corners read as Material Design; rounded reads as Apple.
- **Gold accent strategy:** icons are `--text-muted` by default; gain `--gold-primary` only on focus/active. No always-gold icons (over-saturates the brand). Exception: the mark itself.
- **Source priority (v3 — UI craft #6):** **SF Symbols first** for any concept Apple ships (`mic.fill`, `paperplane`, `gearshape`, `square.and.pencil`, `magnifyingglass`, `chevron.down`, `arrow.up.right`, weather glyphs `sun.max.fill` / `cloud.rain.fill` / `cloud.bolt.fill`, etc.). SF Symbols guarantee consistent stroke-weight against macOS chrome and inherit template tinting. **Lucide stroke set** (restyled 1.5 pt at 24×24, MIT) for novel concepts SF Symbols doesn't cover (mark hex, pin `◆`, custom widget glyphs). Restyled-Lucide never sits next to a system control where the weight mismatch would read "indie app trying to look custom" (UI craft #2).

### 3.5 The Mark (formerly "the sigil")

The Leah **Mark**: an engraved capital **L** (New York Italic, gold) inside a hairline obsidian hexagon. Hexagon stroke is `--gold-muted` at 0.75 pt; L is `--gold-primary` with a subtle inner shadow that reads as "stamped". Standard sizes: 18 px (menubar — hexagon only; the L disappears below 20 px), 24 px (default), 56 px (ambient HUD), 96 px (wizard hero).

**Naming (v3.1 — first-impression #2/3):** internal/engineering name = **mark**; the word "sigil" is the heraldic intent we keep in design discussion only, and **never** appears in user-facing copy. Operator-cosplay naming reduction — see §15 anti-pattern "Cosplay names in user-facing copy."

**Why hexagon, not circle?** Circle = Siri/Cortana. Hexagon = mechanical, watch-crown lineage, structural integrity.

**Why italic L?** A capital sans L is a furniture-store logo. Italic serif L is *script-like* — personal, named, like a wax seal on correspondence. JARVIS without sci-fi.

At 100 px in a screenshot crop: deep obsidian field, a single gold-on-dark hexagon-and-L. Unmistakable. (PNG-vs-vector decision: v3.1 ships **SVG/PDF vector** for all mark sizes — perf-#40 fold, drops the @1x/@2x/@3x PNG triplet at ~150 KB binary cost; see §11.5 update.)

### 3.6 Light mode palette + appearance auto-switch (NEW v2)

Light mode is **table stakes** for a Mac app (HIG). Every dark-mode token has a light-mode counterpart. Auto-switch via `NSApp.effectiveAppearance` KVO observer; manual override in Settings → Appearance (System / Light / Dark).

**Bone (background system — light counterpart to obsidian)**

| Token | Hex | Use | WCAG |
|---|---|---|---|
| `--bone-0` | `#F2EFE8` | Focus panel bg, deepest light layer | Warm bone (not pure white). Mirrors obsidian-0's role. |
| `--bone-1` | `#EAE6DD` | Ambient HUD bg, settings sidebar | One step deeper for layered light surfaces. |
| `--bone-2` | `#DCD7CC` | Card / row hover, input field bg | Hairline-rule lands on this. |
| `--bone-3` | `#C8C2B5` | Pressed state, divider-on-card | Top of light stack; reads as interactive. |

**Gold (primary accent — darkened champagne for AA on bone)**

| Token | Hex | Use | WCAG |
|---|---|---|---|
| `--gold-primary-light` | `#7A6332` | Mark fill, primary CTA, listening-active ring on light bg | **5.00:1** on `--bone-0` — AA pass. Same hue family as dark-mode champagne, dropped luminance so it survives on bone. |
| `--gold-hover-light` | `#8A7340` | CTA hover | Lighter step. |
| `--gold-muted-light` | `#A88F58` | Disabled/decorative gold; never on text | Engraved-metal feel on light. |
| `--gold-glow-light` | `#5C4A20` | Focus-ring on light bg | Darker for contrast against bone (6.6:1). |

**Oxblood + alert (light variants)**

| Token | Hex | Use | WCAG |
|---|---|---|---|
| `--red-brand-light` | `#5C1620` | Brand-mark on installer/marketing light variants | 11.5:1 on `--bone-0`. UI-only context retained. |
| `--red-alert-light` | `#A8323D` | Daemon-down banner on light bg; always paired with `●` icon | **5.74:1** on `--bone-0` — AA pass. |
| `--red-dim-light` | `#E6D8DA` | Sensitive-content scrim tint on light | Background-only. |

**Graphite (foreground — light counterpart to ivory)**

| Token | Hex | Use | WCAG |
|---|---|---|---|
| `--text-primary-light` | `#1E1D1A` | Body text on light | **14.68:1** on `--bone-0` — AAA. |
| `--text-muted-light` | `#3D3A33` | Captions, citations | **9.88:1** on `--bone-0` — AAA. |
| `--text-dim-light` | `#6B6558` | Placeholder, timestamps | **5.04:1** on `--bone-0` — AA. |
| `--divider-light` | `#0000001F` | Hairline at 12% black | 1 px @ 12% black on `--bone-0`. Bumps to 1.5 px @ 18% on non-Retina. |
| `--focus-ring-light` | `#5C4A20` | Keyboard focus indicator | 6.6:1 against bone; never color-alone. |

**Material treatment (light mode)**

- **Glass blur:** NSVisualEffectView `material: .hudWindow`, blendingMode `.behindWindow`, tint `--bone-0E6` (90% bone). Light glass = lighter tint, darker hairlines so the panel edge remains crisp.
- **Grain overlay:** 1.5% opacity (vs 2.5% dark) — light surfaces tolerate less grain before it reads as dirt.
- **Hairline rule:** 1 px @ 12% black on Retina; 1.5 px @ 18% black on non-Retina.
- **Mark on light:** hexagon stroke `--gold-muted-light` 0.75 pt; L is `--gold-primary-light` with subtle two-direction emboss (**canonical v3.1 — reconciles §3.6 vs §18 split**): `text-shadow: 0 -1px 0 #FFFFFF08, 0 1px 0 #00000080` (per palette-doc reconciliation; supersedes the older `0 1px 0 #FFFFFF66, 0 -1px 0 #0000001A` two-direction variant).

**Switch behavior**

- App boot reads `NSApp.effectiveAppearance` → loads matching palette.
- `NSApp.effectiveAppearance` KVO observer swaps palette over `--dur-standard` (240 ms) cross-fade per surface; never abruptly. Mark + listening-pulse continue uninterrupted.
- Settings → Appearance → "Match System / Light / Dark" three-way toggle. "Match System" is default; the other two override.
- Sunrise/sunset auto-switch: respect macOS Display Settings → Appearance → "Auto" — Leah inherits transparently (no separate scheduler).

**Anti-pattern killed (§ 15 cross-ref):** "Ignored system appearance" — Leah honors `NSApp.effectiveAppearance` at all times. Dark-only ships are 2018 startup patterns, not 2026 Mac-native.

---

## 4. Form factors

Seven surfaces. Each one earns its existence; nothing duplicates another.

### 4.1 Ambient HUD

| Attribute | Value |
|---|---|
| **Purpose** | Persistent low-chrome status indicator. Operator glances; sees state, weather of the day's agenda, listening state, pinned widgets. |
| **Trigger** | App launch. Always present unless explicitly hidden. |
| **Dismiss** | `⌘⌥H` (hide to menubar) or quit. Auto-hides during screen recording. |
| **Size** | 280 × 84 px (default, 3 base rows). Mini: 56 × 56 px mark-only. Expanded: 280 × 168 px (adds agenda strip). **Grows in 84-px increments per pinned widget — max 2 pins → 252 px tall** (v3.1 fix: was "max 3 pins → 336 px" — contradicted §10.3 decision #40 cap of 2). Math: 3 base rows × 28 px = 84 px base; + 84 px per pin × 2 = 252 px total at max pinned. |
| **Position** | Bottom-right by default, 24 px from edge. Edge-dockable to 4 corners + 4 mid-edge anchors (snap-on-drag, magnetic). Per-monitor sticky. |
| **Lifetime** | Process lifetime. **NSPanel.collectionBehavior = `[.canJoinAllSpaces, .fullScreenAuxiliary, .stationary]`** (v3.1 — implementability fix A-1). Floats above normal windows; under fullscreen apps it dim+shrinks to a 0.6-opacity corner orb per §6.5 (consolidated rule — see §6.5 reconciliation note). Per operator decision #21, HUD `.canJoinAllSpaces` is retained. |
| **Affordances (v3)** | Click mark → summon focus panel. Drag → reposition. **Long-press → Settings is REMOVED (v3 — workflow MEDIUM "HUD long-press undiscoverable")** — operators reach Settings via menubar → Settings or `⌘,`. Right-click mark → menubar dropdown (mirror). **Chrome hotkey display (v3.1 — consistency #6):** ambient HUD's caption-area exposes `⌥Space` glyph pair as a one-line dim footer on first 5 launches then auto-hides — surfaces the hotkey without permanent chrome clutter; full discoverability via `⌘/` help. |

### 4.2 Menubar item

| Attribute | Value |
|---|---|
| **Purpose** | Lightweight access when HUD is hidden; settings entry; quit; silent listening-state indicator. |
| **Trigger** | Menubar icon click. **NSStatusItem hit-area** is system-padded to ≥24 × 24 pt even though the visual glyph is 18 × 18 (WCAG 2.5.8 — UX #26). |
| **Dismiss** | Click outside, Esc. |
| **Size** | 18 × 18 px **template image** (pure-alpha SF Symbol-style hex; the system tints it per appearance + accent override — UI craft #2). Popover 240 × auto. |
| **State signaling (v3 — UI craft + UX #11)** | **Shape, not color** — macOS template images cannot be colored. Idle = outlined hexagon (template); listening = **filled** hexagon (template); error = filled hexagon **with `●` inner dot** (template). System renders all three in the user's chosen menubar tint so the indicator behaves like a native macOS extra. VoiceOver announces state changes ("Leah, listening" / "Leah, idle" / "Leah, error"). The v1 "gold dot when listening" is dropped — colored dots on template menubar images render as system-tint and read as an OS-applied badge, not Leah state. |

### 4.3 Focus panel (formerly "focus chamber")

**Naming (v3.1 — first-impression #3):** internal/engineering name = **focus panel** (cites Spotlight + Raycast precedent; both call their summon-surface a "panel" / "command palette"). The word "panel" was sci-fi-cosplay; never appears in user-facing copy. Operator-cosplay naming reduction — see §15 anti-pattern.

| Attribute | Value |
|---|---|
| **Purpose** | Query input + streaming response + widget tiles + sources + follow-ups. The primary work surface. |
| **Trigger** | Global hotkey **`⌥Space`** (Option+Space; fires on keydown) **or** click ambient HUD mark **or** wake-phrase if opted in. |
| **Dismiss** | **Esc** (UI-only dismiss) **or** click-outside **or** `⌘W`. **No timed auto-dismiss.** On idle ≥ 5 min, panel **shrinks to an ambient pill at its last position** (mark + 1-line current state); pill click-to-restore-conversation. Conversation history preserved 24 h on disk. (Reviewer fix: v1's 90 s auto-destroy killed the "ask → switch app → return" loop.) |
| **Size** | 860 × 480 px default (raised from 720 for code-block legibility — workflow #41). Reflows to 560 × 400 px on small displays; up to 960 × 640 px on ultrawide. Reflows up to 200% system text-size without horizontal scroll. Secondary-monitor mode: 70% of secondary screen (see § 6.5). |
| **Position** | Screen-center on summon (Linear command-palette convention), 24 px upward bias (Spotlight muscle memory). Remembers last position if dragged. |
| **Lifetime** | Focus panel **IS a regular key-window NSPanel** — `styleMask = [.titled, .nonactivatingPanel]`, `level = .modalPanel`, `becomesKeyOnlyIfNeeded = false`, `worksWhenModal = true` (v3.1 — implementability fix A-2). `collectionBehavior = [.fullScreenAuxiliary, .moveToActiveSpace, .stationary]` — drops `.canJoinAllSpaces` from the panel (was contradictory; the panel summons on the active Space, not every Space). Takes key on summon (text field accepts input as expected); returns key to prior app on dismiss via tracked-prior-app capture-and-restore: capture `NSWorkspace.shared.frontmostApplication` on summon, `NSApp.hide(nil)` + restore on dismiss (matching Spotlight, but explicit — Spotlight uses a dedicated `LSUIElement` agent, which we are not). The v1 claim of "doesn't steal focus until you type" is AppKit-impossible (reviewer HIG-fix). **Ambient HUD remains nonactivating** with its own `.canJoinAllSpaces` mask (different panel) — only the focus panel activates on summon. |

### 4.4 Notification widget

| Attribute | Value |
|---|---|
| **Purpose** | Leah pushes a brief one-liner: arxiv alert, GH release, calendar nudge, brief-ready, daemon error. Replaces noisy macOS notifications for in-Leah events. |
| **Trigger** | Push from daemon (server-pushed widget refresh, MAY-19 B1/B5). |
| **Dismiss** | Auto-fade after **8 s** default (configurable 3–12 s). Click to expand into focus panel. Swipe-right to acknowledge. |
| **Size** | 320 × 64 px (single line + mark); 320 × 112 px (with one action chip row). |
| **Position** | Stacks above ambient HUD (offset 8 px up per new toast, **max 2 visible**; overflow collapses into a single "+N more" expandable card — reviewer workflow #6 cap from 3→2 to keep bottom-right column ≤40% screen height). |
| **Lifetime** | 8 s default; persistent if marked priority-red. Single coalesced NSTimer for fade-out across all visible toasts (perf #35). |

### 4.5 First-launch wizard

5 steps, 720 × 520 px fixed modal. Full flow in § 8.

### 4.6 Settings pane

760 × 560 pt default, resizable to 640 × 480 min. Things-style sidebar (200 pt) + detail (560 pt+). Search field top of sidebar, `⌘F` focus. Live-preview pane on Appearance section only. Full IA in § 9.

### 4.7 Dashboard

| Attribute | Value |
|---|---|
| **Purpose** | Full-window view of memory, agenda, briefs, news bundle, knowledge timeline. Where the operator goes *to look*, not *to ask*. |
| **Trigger** | `⌘⇧D`, menubar → Dashboard, HUD → expand-arrow. |
| **Dismiss** | `⌘W`, Esc returns to ambient. |
| **Size** | 1180 × 760 px default; resizable; remembers last frame. |
| **Position** | Centered on first open; per-monitor sticky thereafter. |

**Explicitly NOT designed:** per-conversation tabs (panel is one stream; history lives in dashboard); floating chat-history sidebar; status bar inside the panel (state lives on the mark); a separate "voice mode" surface (voice is a state of the panel, not a window).

---

## 5. Motion + animation

### 5.1 Easing curves (three only — resist invention)

| Curve | cubic-bezier | Use |
|---|---|---|
| `--ease-out-quiet` | `cubic-bezier(0.16, 1, 0.3, 1)` | All appears: HUD pulse, focus summon, toasts in. Patek-grade quiet. |
| `--ease-in-quiet` | `cubic-bezier(0.7, 0, 0.84, 0)` | All dismisses: focus dismiss, toasts out. Inverse of above. |
| `--ease-standard` | `cubic-bezier(0.4, 0, 0.2, 1)` | Color/opacity transitions, hover states. macOS standard. |

### 5.2 Durations (doctrinal)

| Token | ms | Use |
|---|---|---|
| `--dur-instant` | 80 | Hover color change, focus ring. |
| `--dur-quick` | 160 | Toast in/out, menubar popover. |
| `--dur-standard` | 240 | Ambient HUD state change, mark rotate, widget reveal, tile dismiss. |
| `--dur-hero` | 380 | Focus panel summon + dismiss. |
| `--dur-reduced` | 0 | All durations zeroed under `prefers-reduced-motion`. State changes become cross-fades at `--dur-instant`. |

### 5.3 Activity indicators

**Listening pulse (mic active).** Mark's gold hexagon outline pulses radially outward via a 2nd hexagon at +6 px stroke, fading from `--gold-glow` at 40% opacity to 0% over 1400 ms. Loops with 200 ms gap. **Amplitude clamp (v3 — UX #32):** opacity envelope locked to **30–60 %** (never 0–100 %); under-reduced-motion AND on idle-DND screens, slow to **0.5 Hz** (longer cycle) to avoid migraine triggers in vestibular-sensitive users at the 1 Hz seizure-band edge. **Pause when panel not visible AND ambient HUD occluded** (perf #9) — recovers ~0.4 % sustained CPU. Reduced-motion: static `--gold-glow` 1 pt outer stroke at 30% opacity.

**Thinking ring (LLM processing).** 0.75 pt gold arc travels around the hexagon perimeter. Arc length 90°, rotates at 1080 ms/rev. Stroke gradient `--gold-glow` → `--gold-primary` → transparent. **Render as a 20-frame sprite-sheet loop (v3 — perf #10)**, NOT a per-frame gradient-stroke tessellation — gradient strokes on rounded paths have no GPU-cached path; sprite-sheet costs sub-millisecond/frame vs ~2.2 ms (26 % of 8.3 ms ProMotion budget). Reduced-motion: static dashed perimeter at `--gold-muted`.

**Speaking waveform (TTS playing).** 5 vertical gold bars, 2 pt wide, 8 px tall max, under the response text in the panel AND under the mark in HUD. Each bar height animates to audio amplitude envelope. **Render path (v3 — perf #11):** EITHER an SF Symbol `audio.waveform` with `.variableValue` animation (cheap built-in) OR a single Metal shader (one draw call, 5 quads) — NOT 5 SwiftUI `frame(height:)` per-bar layout passes (which forces 5 layout passes per frame). **Cadence 10 Hz, render only when panel is focused** (v3 — workflow MEDIUM "30 Hz waveform"): 30 Hz constant peripheral motion at 8 h/day is wearing; 10 Hz is still envelope-faithful; HUD-mirrored waveform paints only on focus or active TTS playback, not always-on. **Halt when system idle ≥ 10 min** (v3 — perf MEDIUM #33 + animation-halt-when-idle rule). Reduced-motion: single static gold horizontal bar with slow left-to-right shimmer.

### 5.4 Signature transitions

**Transition 1 — The Gold Transition (focus panel summon, 380 ms).** On hotkey, a 1 px gold transition (`--gold-primary`) appears at screen-vertical-center, expands horizontally from 0 to panel-width over 120 ms (`--ease-out-quiet`), then the panel unfolds vertically from that seam — top half up, bottom half down — over 260 ms with `--ease-out-quiet`. The seam fades to `--divider` as the panel settles. Reads as: a vault opening, a watch-case being cracked, light escaping a slot. Reduced-motion: panel cross-fades in over 160 ms; no seam. **Animation runs as a `transform.scale.y` with anchor-point at seam center, NOT a layout-bounds animation** (perf #12 — bounds animations trigger per-frame layout passes; transform is GPU-cheap). Sparkle (if present) uses `CAEmitterLayer`. **First-summon-per-session rule (v3 — UI craft):** full 380 ms ceremony fires on cold-start summon ONLY. Warm summons (within 30 min of last summon) use `--dur-quick` 160 ms cross-fade — no seam (already encoded in § 6.2; reaffirmed here as the doctrinal transition-fatigue cap).

**Transition 2 — Mark Acknowledgment (wake-word heard, 240 ms).** Mark hexagon rotates 60° clockwise (one hex-vertex of rotation — same visual position, but ticked), with a single warm pulse of `--gold-glow` at 40% opacity expanding to 110% scale and fading. Reads as: a watch tick, a nod of acknowledgment. Reduced-motion: mark color shifts to `--gold-glow` for 200 ms then back. **First-ack-per-window rule (v3 — UI craft + workflow):** the full rotate-and-pulse fires on the FIRST wake-ack per 5-minute window only. Subsequent acks within the window reduce to a 1 px `--gold-glow` border flash (80 ms) — eliminates wake-word fatigue (workflow #2 in MEDIUM lens). **Ack gated by VAD pass:** if the wake-word fires but no transcribed token clears VAD within 600 ms, the mark reverts silently — no "Leah heard nothing useful" loop (workflow MEDIUM "F2-after-VAD"). **Animation frequency cap (v3 — UX #31):** repeated state-change animations on the mark are throttled to **≤1 per 500 ms** to prevent photosensitive triggering on rapid back-to-back wake-events.

These two are the personality. Everything else is a quiet cross-fade.

### 5.5 State transitions

```
[hidden] --⌥Space--> [focus-panel summon, --dur-hero] --type--> [streaming]
                                                                 |
                                          <--Esc, --dur-hero--   v
[ambient HUD]  <--idle ≥5min--  [response complete, ambient-pill shrink armed]
     |                                       |
     |                                       +--ambient pill at last position
     |                                          (preserves 24h conversation)
     |
     +--wake-word detected----> [listening state on HUD]
     +--toast pushed----------> [notification widget above HUD]
     +--⌥Space---------------> [focus panel summons OVER HUD]

Streaming-edge states (v3.1 — implementability fix B-7):
  [streaming] --Esc (UI dismiss)--> [panel-dismissed-mid-stream]
                                       |
                                       +--LLM continues server-side
                                       +--frames buffered to history
                                       +--on re-summon via ⌥Space:
                                            [re-summon-during-stream]
                                              shows live continuation
                                              (NOT a restart)

  [streaming] --app backgrounded--> [app-backgrounded]
                                       |
                                       +--HUD invisible
                                       +--daemon + LLM continue
                                       +--prose buffered
                                       +--on foreground:
                                            replay buffered prose
                                            (no jump-to-end)

  [streaming] --network drops--> [stream-network-down]
                                       |
                                       +--partial response shown
                                       +--inline retry chip
                                       +--auto-resume on connectivity (best-effort)
```

The HUD never disappears during state transitions; it dims to 60% opacity behind the focus panel and resumes 100% on dismiss.

---

### 5.6 Timezone + DST

All time-of-day rendering (ambient HUD greetings, calendar widget, agenda strip, market open/close labels) uses `Calendar.current.timeZone` — the operator's system timezone. Subscribes to `NSSystemTimeZoneDidChangeNotification` and re-renders affected surfaces over `--dur-standard`. DST transitions are handled via Foundation date math (`Calendar.dateInterval`), never raw hour offsets. Travel: when the operator's Mac TZ changes (typical on flight landing), Leah's clocks follow without prompt; calendar events render in whatever TZ the event was authored in (EventKit native).

---

## 6. Interaction model

### 6.1 Global hotkey

**`⌥Space` (Option+Space)** — single modifier + letter; fires on **keydown** (no chord-disambiguation timer, no 250 ms latency floor). Felt-instant target: <100 ms from keydown to panel-interactive. Raycast convention exactly; well-tested muscle memory; one-handed; thumb-shift reachable.

**Why ⌥Space and not ⌘⌃ (v1):** modifier-only chords with tap-vs-hold disambiguation impose a 250 ms latency floor (perf-review #5) AND collide with macOS Spaces nav (`⌘⌃←/→`), Rectangle/Magnet default tiling chords (workflow-review #1), Sticky Keys (WCAG 2.1.4), and any future-pressed third key. `⌥Space` has no such floor, no holding modifiers for accessibility, and conflicts only with optional macOS "Quick Look in Finder" (rare).

**Hotkey-press during daemon-down state:** MUST flash an inline obsidian-1 ghost-panel at screen-center with `● Daemon offline — click to restart` affordance (Nielsen-H9 fix: visible feedback where operator is looking; no silent dead-end). See § 7.3 panel states.

Re-bindable in Settings → General → Hotkey. Conflict detection runs against a curated list of Apple-documented system shortcuts (the `HIToolbox` private-API enumeration in v1 was removed per HIG-review — would have failed notarization). Third-party app conflicts are documented honestly in the UI: `Checked against macOS system shortcuts. Third-party apps may still conflict — try the recorder if a chord misbehaves.`

**Accessibility permission requirement.** Global hotkey requires macOS Accessibility permission (no `NSAccessibility` API for silent grant). Wizard step 2 surfaces this with a clear breakage warning + "Open System Settings →" deep link; hotkey will fail silently in third-party apps until granted.

### 6.2 Summon

- Hotkey (`⌥Space`) from any app → panel materializes at screen-center; input field gains focus synchronously with keydown (Transition 1 plays concurrent with input becoming interactive — never gates input on transition completion, perf #6).
- Panel **takes key window status** on summon; Esc returns key to prior app (Spotlight pattern). The v1 "no-steal-focus" claim is dropped (AppKit-impossible per craft-HIG review).
- **First summon per session:** full Transition 1 (380 ms). **Subsequent summons within 30 min:** `--dur-quick` 160 ms cross-fade (no seam). The ceremony is a first-impression, not a per-summon tax.
- **Wake-word path (only if opted in, default OFF):** see § 6.6. Panel materializes silently; listening pulse already active; input shows live transcription in `--text-muted`. **Voice-summon size + position (v3 — workflow MEDIUM):** voice-summoned panels materialize as a small **"voice mode" frame at the HUD anchor corner (400 × 280 px)**, NOT the screen-center 860 × 480 hotkey-summon size — a screen-center 860 × 480 over a Slack message draft after an accidental "Leah,…" is disproportionate. Operator can press `⌥Space` (or click to expand) to grow to full panel.

### 6.3 Dismiss + abort (two distinct verbs, document both)

| Verb | Action | Backend |
|---|---|---|
| **Esc** | Dismiss panel UI (returns key to prior app) | Does NOT cancel in-flight LLM call. Response continues; next summon shows it complete. |
| **`⌘.`** | **Cancel in-flight LLM call** (saves tokens; ~$0.03/turn at GPT-4-class) | Stops generation server-side; panel stays open showing partial. |
| **Click-outside** | Dismiss panel UI | If input has unsent text → input border pulses gold-glow 1 s, draft preserved on next summon. |
| **`⌘W`** | Dismiss panel UI | Same as Esc. |
| **Idle ≥ 5 min** | Panel shrinks to **ambient pill** at last position (mark + 1-line current state); does NOT destroy conversation | Conversation history preserved 24 h on disk; pill click restores panel with full prior turns. (Reviewer fix: v1's 90 s destroy killed work-in-progress reads.) |
| Menubar Quit | Full app quit | Cancels all in-flight. |

The dual-verb split is intentional: Esc is for "I'm done looking" (free), `⌘.` is for "stop spending tokens" (saves money). Both are documented in the `⌘/` cheatsheet overlay.

### 6.4 Voice ↔ text handoff

- Input field is ALWAYS available. Voice does not replace it.
- Voice dictation: interim text in `--text-muted` → final in `--text-primary`. Operator can type to interrupt; voice yields immediately.
- **Push-to-talk (v3 — workflow MEDIUM "PTT-not-Space"; v3.1 — implementability D-7 fix):** **hold `Fn` on internal keyboards** (Fn modifier flag exposed via `event.modifierFlags.contains(.function)`); **hold right-`⌘` on external keyboards without an Fn key** (left-`⌘` is reserved for app shortcuts; right-`⌘` distinguishable via `event.keyCode`). `⌥` is NOT a PTT fallback (collides with the global `⌥Space` hotkey). **Space is never PTT** — gating PTT on "input is empty" is a known Discord/Slack misfire source. Documented in `⌘/` cheatsheet. Voice-summon path (wake-word opted in) is unchanged.

### 6.5 Multi-tasking + fullscreen behavior

| Scenario | Behavior |
|---|---|
| **Fullscreen app (default)** | Ambient HUD + focus panel **dim+shrink to a corner orb @ 0.6 opacity** (raised from 0.3 — at 30 % on dark Spaces the orb is near-invisible per workflow #34) in the operator's chosen HUD anchor. Mark only, 56 × 56 px. Listening pulse muted; **all animations halted** (DND-class rule, not just dimmed — perf #33). Hover or hotkey restores full panel as overlay above the fullscreen Space. Toasts suppressed unless priority-red. **(v3.1 reconciles §4.1's "below fullscreen unless always-on-top toggled" — that rule is superseded: the orb IS the always-visible state, not hidden-below; the toggle was conceptual ambiguity.)** |
| **Fullscreen app + secondary monitor opt-in** | When Settings → General → "Dedicated display when active app fullscreens" is on AND ≥1 secondary monitor present: ambient HUD + panel **relocate to the chosen secondary monitor**. Ambient widens to 560 px (richer agenda + 3 pin slots side-by-side); panel occupies 70% of secondary screen, centered. Leaving fullscreen → both return to primary at remembered anchor. Fallback if the named monitor disconnects mid-session: dim+shrink to corner orb (default). |
| **Mission Control** | Ambient HUD joins all Spaces (NSPanel `.canJoinAllSpaces`). Visible in MC for orientation; dismissable. |
| **Second monitor (non-fullscreen)** | Each monitor remembers its own HUD anchor. Panel summons on the monitor containing the cursor. |
| **Screen recording / sharing** | Detected via **`NSWorkspace.shared.notificationCenter` `NSWorkspaceScreenIsBeingCapturedNotification`** (push-based, available since macOS 12.1, augmented in 13) as the primary signal; `CGDisplayStream` + `CGDisplayRegisterReconfigurationCallback` as the secondary stream observer for which display is captured (v3.1 — implementability fix A-3, correcting v3's `SCShareableContent` which only *enumerates* shareable windows, doesn't tell you whether the screen is *currently being captured*). HUD auto-hides; menubar dot dims to `--obsidian-3`; toasts suppressed until recording ends. Restore trigger: capture-end notification fires → HUD fades back over `--dur-standard`. While hidden, menubar exposes "Leah hidden (recording) — [Show anyway]". Privacy default ON. (v1's `CGScreenIsCaptured()` deprecated since macOS 12 — HIG-review #34.) |
| **Do Not Disturb** | Leah respects Focus filters; non-priority toasts suppressed; ambient HUD goes 60% opacity. |

### 6.6 Cursor + keyboard navigation

Full no-mouse usability across every interactive surface. Focus indicator: **`--focus-ring`** 2 px @ 80% opacity, 2 px offset, visible against both dark and light backgrounds (never relies on color alone — paired with element-state change).

**Panel**
- **Tab** cycles: input → response widget tiles (each tile = one tab stop, then Tab into tile-controls) → sources → follow-up chips → close. Reverse via Shift-Tab.
- **Arrow keys** navigate response stream (scroll), follow-up chips (←→), source list (↑↓).
- **Enter** → send. **Shift-Enter** → newline.
- **`⌘↑` / `⌘↓`** in input → previous/next prompt from history.
- **`⌘[` / `⌘]`** → navigate to prior / next turn in current panel session (workflow #10: keep history in-place).
- **`⌘/`** → toggle help overlay (hotkey cheatsheet).
- **`⌘.`** → cancel in-flight LLM call (token-saving).
- **Esc** → dismiss panel UI (does not cancel backend call).
- **`⌘⇧W`** → open widget gallery (§ 10.4). Also: `+` button next to input field (panel-resident affordance — discoverable every session, not just empty-state).

**Widget tile keyboard model (NEW v2 — fixes WCAG 2.1.1)**
- Tile focused → arrow keys navigate within (e.g., chart crosshair via ←→; table row via ↑↓).
- **P** → pin / unpin focused tile.
- **X** or **Delete** → dismiss focused tile.
- **Space** → primary action (Quick Look on image; expand on stat; refresh on market).
- **⌘C** → copy tile data (replaces all click/long-press/right-click affordances; those remain for mouse users).
- Every tile control has explicit `accessibilityLabel` (§ 11.3).

**Widget gallery overlay focus trap**
- First focus = search field. Tab cycles search → category list → preview grid. Esc returns focus to panel input.

**Settings**
- `⌘F` focuses search (auto-focused on Settings open per workflow #34 — Things pattern).
- Tab cycles sidebar → detail rows. Sidebar arrow keys navigate sections.

**Wizard**
- Tab cycles heading → primary CTA → secondary skip → back. Esc on any step → abort sheet.

**Dashboard**
- `⌘1..⌘9` jumps to sections (Memory / Agenda / Briefs / News / Knowledge / …).

### 6.7 Wake-word reliability (only if opted in)

Default OFF. When operator opts in via Settings → Voice → "Listen for 'Leah'", this section applies. Wake-word is local-only (no cloud audio, decision-log #6 retained).

| Mitigation | Behavior |
|---|---|
| **VAD-gate** | Voice-activity detector runs before wake-word model; rejects single-syllable false fragments ("really", "let her", "yeah", "leaving") that don't carry the trailing pause expected after "Leah,…". |
| **Per-app suppression list** | When frontmost app is in the operator's "meeting apps" list (Zoom, Meet, Slack-huddle, Teams, FaceTime — pre-populated; operator can edit), wake-word is suppressed entirely. Detected via NSWorkspace frontmost-app observer. |
| **"Ignore for 30 s" voice command** | Saying `"Leah, ignore"` after a false trigger disarms the wake-word for 30 s without summoning. Toast confirms: `Wake-word paused 30 s`. |
| **False-positive learning loop** | If operator presses Esc within 2 s of a wake-trigger, the triggering audio clip (local, never transmitted) is added to a per-user negative-example set; on-device fine-tune runs nightly during charging hours. Toast on first occurrence: `Heard a misfire? Press Esc within 2 s and I'll learn it.` |
| **Battery awareness** | Wake-word model unloads when `ProcessInfo.processInfo.isLowPowerModeEnabled == true` OR battery < 20 % AND not plugged in. Menubar shows `WAKE-WORD PAUSED · LOW POWER`. Resumes on charge. |
| **System-status visibility** | When wake-word is armed, ambient HUD MUST show explicit `LISTENING` text in Row 1 + the listening pulse. Menubar icon: filled hexagon (template) when listening, outlined hexagon when idle. **Shape-based** state — no color-only signal (deuteranopia-safe, WCAG 1.3.3). |
| **Phrase choice** | Default phrase is `"Hey Leah"` (not just `"Leah"`) when operator opts in — 2-word phrase reduces false-pos ~80 % per voice-assistant literature. Settings → Voice → Phrase: `"Leah" / "Hey Leah" / "OK Leah"`. |
| **Onboarding copy** | Wake-word opt-in card in Settings → Voice includes measured battery cost: `Adds approximately 2–4 % to daily battery drain. Honest trade for hands-free.` |
| **Model location (v3.1 — implementability fix B-10)** | Wake-word ML model bundled as `Leah.app/Contents/Resources/Models/wake-leah.mlmodel` (Core ML). Auto-loaded by `internal/wake/` adapter on opt-in. On-device fine-tune via Core ML `MLUpdateTask` writes deltas to `~/Library/Application Support/Leah/Models/wake-leah-user.mlmodel`. No TF Lite runtime; no out-of-bundle download. |
| **CLI parity (v3.1 — silent-drop wf:cli-parity)** | Wake-word toggle, phrase, suppression list, and false-positive learning all expose CLI commands via `leah voice {on,off,phrase,suppress,learn}` — both surfaces same commands; GUI is overlay convenience. See §6.8. |

If all mitigations fail and operator still gets false triggers, the resort is "turn it off" — which is the default. The opt-in design carries the cost; the default-OFF design carries the trust.

### 6.8 CLI ↔ GUI parity (NEW v3.1 — consistency silent-drop wf:cli-parity)

**Both surfaces ship the same commands. The GUI is overlay convenience — never the only path.**

| Domain | CLI verb | GUI surface |
|---|---|---|
| Summon | `leah ask "…"` (one-shot) / `leah chat` (interactive) | `⌥Space` → focus panel |
| Voice | `leah voice {on,off,phrase,suppress,learn}` | Settings → Voice |
| Widgets | `leah widget {list,enable,disable,pin,unpin,spawn}` | Settings → Widgets + panel gallery `+` |
| Memory | `leah memory {browse,export,import,purge}` | Settings → Memory |
| Integrations | `leah connect <integration>` / `leah disconnect <integration>` | Settings → Integrations |
| Permissions | `leah perm {status,grant <name>}` | Settings → Permissions |
| Hotkey | `leah hotkey {get,set,reset}` | Settings → General → Hotkey |
| HUD | `leah hud {show,hide,position,scale}` | Drag/Settings → General |
| Daemon | `leah daemon {status,restart,logs}` | Settings → About → Daemon status |

**Widget rendering parity:** CLI surfaces render widget tiles as TUI tables / sparklines (re-uses Charm/lipgloss patterns). The CLI is not a GUI-only fallback — it's first-class. Operators who live in iTerm get the same affordances; the GUI is for moments when the visual canvas matters (mid-flow summon over a desktop).

This parity is enforced by `make check`: every command in `internal/cli/` MUST have a GUI counterpart and vice versa (cross-doc parity rule extends §16.7).

---

### 6.9 Force-quit recovery

Operator force-quits Leah from Activity Monitor → daemon and HUD die without graceful shutdown. On next launch: `pinned-widgets.json` is read; widgets re-mount and fetch fresh. Conversation history (last 24 h, on disk under `~/Library/Application Support/Leah/history/`) rehydrates into the panel on first summon — any in-flight stream at crash time is replayed up to its last persisted frame and marked `[interrupted — re-ask]`. Pinned-widgets writes use `Data.write(.atomic)`; history uses WAL journal — daemon crash mid-write leaves last-good state on next launch.

### 6.10 Sleep + wake (`NSWorkspaceDidWake`)

On `NSWorkspaceDidWake`, all queued pinned-widget refreshes are coalesced and jittered ±5 s over a 15 s window (randomized per-widget seed). Prevents a thundering herd against rate-limited APIs (GitHub, weather, market). HUD's listening pulse + thinking arc resume at next animation frame; mark rotation is suppressed for the first 1 s after wake to avoid the "app screams at me when I open the lid" pattern.

### 6.11 Low Power Mode

`ProcessInfo.processInfo.isLowPowerModeEnabled == true`: wake-word model unloads (§6.7); pinned-widget auto-refresh paused; decorative animations halt (instant-state-change swaps); grain texture suppressed. Menubar dropdown shows `LOW POWER · refresh paused`. Resumes automatically on AC + non-low-power; no operator action required.

### 6.12 Low Data Mode

`URLSessionConfiguration.allowsConstrainedNetworkAccess == false` (Low Data Mode active on cellular tether or operator opt-in): widgets render from cached-last-good with relative-age caption; fetches defer until network is unconstrained. Quick-spawn chips disabled. A discreet `ⓘ Low Data mode active` chip lives in the panel footer (left of `Press ⌘/ for shortcuts`) so the operator knows why widgets feel stale.

### 6.13 AirPods + audio route change

`AVAudioSession.routeChangeNotification` handler: on disconnect (AirPods removed mid-TTS) → TTS pauses at the current word boundary; mark shows `audio-paused` state (1 px gold rule under hex). On reconnect (AirPods back) → resume from the paused word, no re-spoken syllables. Wake-word re-binds to the new system-default input device. AirPods battery <10 % → toast `AirPods battery low · falling back to built-in mic in 30 s`; auto-fall-back on disconnect.

### 6.14 External keyboard variance

`⌥Space` registers via `RegisterEventHotKey` which filters BEFORE Sticky Keys — chord fires correctly on Option-release-then-Space-press path (Sticky Keys ON) AND on Option-held-Space-tap (Sticky Keys OFF). Bluetooth Magic Keyboard latency budget tolerates +20 ms before the felt-instant target slips; in measured testing, BT keyboard hotkey p95 lands ~108 ms (vs ~92 ms internal keyboard) — acceptable. Function-key layer (`Fn` row) is respected; PTT-via-Fn works on internal keyboards only (external keyboards without Fn fall back to right-`⌘` per §6.4). Sticky-Keys cleanup: modifier-up event triggers cleanup so a stuck Option doesn't leave the panel half-summoned.

### 6.15 VPN + system proxy

All HTTPS requests (LLM API, widget adapters, citation dereferencing, image fetch) inherit `URLSessionConfiguration.default` — which automatically respects system proxy (`kCFNetworkProxiesHTTPEnable`) and active VPN routes. No bypass. On connection error (proxy auth fail, VPN drop mid-request) the affected widget shows the stale-frame oxblood indicator with retry chip; the panel shows a single toast `Network unreachable · check VPN / proxy` (lives 8 s, suppressible per Settings → Privacy → Network alerts).

---

## 7. Information architecture per surface

### 7.1 Ambient HUD — the 7 earned pixels

Default 280 × 84 px shows three rows. Order top → bottom:

1. **Row 1 — Mark + state:** 24 px mark (left), current state caption (right). State = "Idle" / "Listening…" / "Thinking…" / "Ready" / red "Daemon offline". Caption uses `--text-muted` on `--obsidian-1` (8.16:1 — AAA; never `--text-dim` which fails AA on HUD bg — UX #4). **Time-of-day greeting (v3.1 — consistency wf-7.4):** when the panel/HUD shows a greeting, the string is **fixed**: "Good morning, Tri." (06:00–11:00), "Good afternoon, Tri." (11:00–17:00), "Good evening, Tri." (17:00–06:00). No rotating prose ("Hello again, Tri!"). Predictable; honors time-of-day; no novelty noise.
2. **Row 2 — Now:** the single most relevant *now* item. **Predictable source rotation (v3 — workflow MEDIUM #5.1):** time-of-day-gated, not cascading-fallback. **AM (08:00–12:00):** calendar next-event. **PM (12:00–18:00):** in-progress brief title. **Evening (18:00–08:00):** today's first uncompleted agenda item. Else: empty (hidden). One source per time-window, never mystery-mode. Caption `--text-muted` (load-bearing).
3. **Row 3 — Pulse:** ONE micro-metric, primary. **Default (v3 — workflow MEDIUM):** PR review queue (`◇ 5 PRs`). Hover/focus → rotates a second metric (unread briefs `⌬ 3`, then arxiv `⎇ 12`, then back). Glyph prefixes carry the meaning so eye scans by shape not by parsing 3 unrelated counts compressed into one line. Expanded mode (Settings → General → HUD scale = Expanded) shows all 3 at once, glyph-prefixed. Weather is **always glyph-prefixed via SF Symbol** (`sun.max.fill` / `cloud.rain.fill`), never emoji (UI craft #5.1).

Below the 3 base rows, a 1 px gold double-rule pin-divider, then up to **2 pinned widget slots** (84 px each — v3.1 fix from "3"; cap is 2 per decision #40). See § 10.3.

**Hidden by design:** clock (macOS menubar has one), date, weather chrome (just a glyph if shown), CPU/network (not operator-relevant).

### 7.2 Focus panel — anatomy

```
┌─────────────────────────────────────────────────────┐
│                                                     │  ← 24px top breathing room
│   [mark-pulse]   Ask Leah anything…           ⌘.   │  ← input row
│   ────────────────────────────────────────────────  │  ← hairline
│                                                     │
│   [streaming response renders here as markdown,    │
│    interleaved with widget tiles, code blocks,     │
│    inline citations as gold-underline numerals¹²]  │
│                                                     │
│   ────────────────────────────────────────────────  │
│   Sources                                           │
│   1. arxiv.org/abs/2401… — Title                   │
│   2. github.com/foo/bar — README                   │
│   ────────────────────────────────────────────────  │
│   ⌃ Follow up                                       │
│   [ chip ] [ chip ] [ chip ]                       │
│                                                     │
└─────────────────────────────────────────────────────┘
```

- **Input row:** mark shows state (idle/listening/thinking). **Placeholder is the single fixed string `"Ask Leah anything…"`** (v3 — workflow MEDIUM "placeholder rotation"). Rotating placeholders are cute on day 1, noise by week 3 and break muscle memory for power users who scan by silhouette. One string, forever.
- **Response area:** interleaved prose + widget tiles (see § 10). Citations inline as gold-superscript numerals; hover/focus shows source preview. Code blocks have copy button (top-right, appears on hover).
- **Sources:** collapsible list. Default expanded if ≤3; collapsed with "Show 5 sources" if more.
- **Follow-up chips:** 3 max, AI-generated continuations. Tab-navigable. Enter inserts into input.
- **No conversation history in this surface.** History is the dashboard (use `⌘[` / `⌘]` to navigate prior turns within the current panel session).
- **Empty-state footer (v3 — UX #40 + workflow MEDIUM):** panel empty-state shows a single dim `--text-dim` footer line: `Press ⌘/ for shortcuts`. The menubar dropdown also exposes `Keyboard shortcuts ⌘/`. Surfaces the help overlay without forcing operators to remember the chord after a 3-week absence.

### 7.3 States (panel + ambient)

| State | Treatment |
|---|---|
| **Empty** (no query yet) | Placeholder + 3 "starter" chips + up to 3 quick-spawn widget chips (§ 10.5). |
| **Streaming** | Input dims to `--text-muted`; thinking ring on mark; `⌘.` chip appears top-right. |
| **Error — model failed** | Red hairline-bottom on response area; `--red-alert` caption: "Model couldn't respond. [Retry]". |
| **Offline — no network** | Soft red banner at top of panel: "Offline. Local answers only." HUD mark shows obsidian-3 hexagon (no gold). |
| **Daemon down** | **Hotkey press during daemon-down flashes an inline obsidian-1 ghost-panel at screen-center** with `● Daemon offline — click to restart` toast at cursor location (Nielsen-H9 fix: visible feedback where operator is looking; no menubar dead-end). Menubar dot ALSO pulses `--red-alert` for redundancy. Single click on the inline toast → restart sequence + panel summons on success. |
| **Permission denied** (mic, screen) | First time: gentle Settings deep-link sheet. Subsequent: small inline `--red-dim` chip "Mic blocked → System Settings". |
| **Sensitive content detected** (e.g., password, key) | Blur scrim with `--red-dim` tint over the message; "Sensitive content hidden. **Matched pattern: `{pattern-name}`. [Show this message] · [Mark safe] · [Always allow for this app]**" (v3 — UX #28). False-positive recovery: `[Mark safe]` whitelists the exact match for the session; `[Always allow for this app]` (visible when frontmost app at trigger time is in operator's app list) adds the app to a per-app blocklist-exclusion. **`[Show]` is per-message; session-wide "always show" lives in a separate toggle** in the same affordance row (v3 — workflow MEDIUM "blur Show scope"). Pattern label tells operator WHY (regex name, never raw regex). |

---

## 8. First-launch wizard

**6 steps** (v3.2.1+; v3.2.2 reconciles §0/§13.15/§19 to a single canonical count), 720 × 520 px fixed modal, hidden titlebar, traffic lights top-left at 20 pt inset. Linear-style step-dots progress bar top. Esc on any step → "Quit setup? You can finish anytime from the menubar." [Quit] [Keep setting up].

The 6-step canonical order is: **Welcome → BYOK Anthropic API key → Hotkey + Accessibility → Voice (mic) → One thing to connect (Calendar pre-selected) → You're ready**. Step 2 (BYOK paste-key) is fully specified in §13.15. Steps below labeled `1`–`6` map 1:1 to that order.

### 8.1 Flow

```
┌─────────────────┐  no skip; CTA "Begin"
│ 1. Welcome      │
│   mark hero    │
└────────┬────────┘
         ▼
┌─────────────────┐  paste-key + verify-1-token-ping (§13.15 + §17.18)
│ 2. BYOK         │  SecureField; ZDR nudge checkbox; daemon writes Keychain on
│   Anthropic key │  success; no honest skip (Leah non-functional without key)
└────────┬────────┘
         ▼
┌─────────────────┐  default ⌥Space; click-record to re-bind; conflict check
│ 3. Hotkey       │  + Accessibility-perm rationale + Open System Settings deep-link
│  + Accessibility│  + clear "hotkey won't work outside Leah until granted" warning
└────────┬────────┘
         ▼
┌─────────────────┐  TTS demo plays "Hi, I'm Leah" (no perm)
│ 4. Voice (mic)  │  static waveform glyph until grant (no fake animation)
│                 │  → [Enable voice] triggers OS prompt → on grant, real waveform
│                 │  Wake-word toggle UNCHECKED by default (v2 operator override)
│                 │  [ Skip — text only ] equal weight
└────────┬────────┘
         ▼
┌─────────────────┐  3 radio cards: Calendar / Mail / Files
│ 5. One thing    │  Calendar pre-selected (operator decision #14); primary CTA
│    to connect   │  active by default; "Skip — set up later" equal weight
└────────┬────────┘
         ▼
┌─────────────────┐  Big hotkey reminder: ⌥ Space (32 pt glyphs)
│ 6. You're ready │  [ Open Settings ] text-link; CTA "Done" → Transition 1 +
│                 │  ambient HUD materializes + first toast "Press ⌥Space to ask"
└─────────────────┘
```

**Re-entry:** Settings → "Re-run setup wizard" returns to step 1. Existing grants reflected (e.g., mic shows "Already granted ✓"). Wizard is idempotent.

### 8.2 Step 1 — Welcome

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ○  ○  ○  ○  ○                                       step 1/6│
│  ──────────────────────────────────────────────────────────────  │
│                                                                  │
│                              ⬡                                   │
│                          (96px mark)                            │
│                                                                  │
│                          Hi, I'm Leah.                           │
│                  Your personal assistant.                        │
│                                                                  │
│                        [   Begin   ]                            │
│                                                                  │
│                   Takes about a minute.                          │
└──────────────────────────────────────────────────────────────────┘
```

96 px mark; full Transition-2 acknowledgment on entry. TTS plays "Hi, I'm Leah." at low volume via system default voice (no permission needed). Mute glyph `🔇` top-right mutes wizard only. CTA `--gold-primary` fill, ivory text, 160 × 40 pt. "Takes about a minute." in `--text-dim`. No skip; Esc → abort sheet.

### 8.3 Step 3 — Hotkey + Accessibility

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ●  ●  ○  ○  ○                                       step 3/6│
│  ──────────────────────────────────────────────────────────────  │
│   How will you call Leah?                                        │
│   Press a hotkey from any app to open Leah.                     │
│   ──────────────────────────────────────────────────────────────  │
│              ┌──────────────────────────────┐                    │
│              │           ⌥  Space           │                    │
│              │     [ Try it now ]           │  ← when pressed,   │
│              └──────────────────────────────┘    a mock panel  │
│                                                  flashes (160ms) │
│              [ Use a different shortcut ]                        │
│   ──────────────────────────────────────────────────────────────  │
│   ┌──────────────────────────────────────────────────────────┐  │
│   │ ⚠ Accessibility access                                    │  │
│   │ Needed so the hotkey works from ANY app (not just Leah). │  │
│   │ Without it, ⌥Space only works while Leah is focused.     │  │
│   │           [ Open System Settings → Privacy → Accessibility ] │
│   └──────────────────────────────────────────────────────────┘  │
│   ──────────────────────────────────────────────────────────────  │
│                                       [ Back ]    [ Continue ]   │
└──────────────────────────────────────────────────────────────────┘
```

Default `⌥Space` shown as glyph pair, 32 pt, ivory on `--obsidian-2` chip with gold-muted hairline. "Try it now" — pressing the chord live anywhere on macOS triggers a 160 ms mock-panel preview ("I'm here."). Re-record: recorder field listens for next combo (Esc cancels) with **real-time conflict feedback as keys are pressed** (Nielsen #13 fix — not just on save). Conflict check against the curated macOS system-shortcut list (no `HIToolbox` private API — see § 6.1).

**Accessibility permission rationale card** is co-located with the hotkey because they are coupled — without Accessibility, the global hotkey silently fails in third-party apps. v1 wizard lied-by-omission (HIG-review). Continue → saves to `~/Library/Application Support/Leah/settings.json` (no `~/Library/Application Support/Leah/` path).

### 8.4 Step 4 — Voice (mic) — value-first, wake-word deferred

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ●  ●  ●  ○  ○                                       step 4/6│
│  ──────────────────────────────────────────────────────────────  │
│   Talk to Leah.                                                  │
│   Dictate questions with the microphone.                         │
│   ──────────────────────────────────────────────────────────────  │
│         ┌─────────────────────────────────────────────┐          │
│         │     ▏ (waveform glyph, static)              │          │
│         │       Enable mic to see the waveform        │          │
│         └─────────────────────────────────────────────┘          │
│                                                                  │
│   ┌──────────────────────────────────────────────────────────┐   │
│   │ Microphone access                                        │   │
│   │ Needed to dictate questions hands-free.                  │   │
│   │ Example: hold Space in the panel and speak.            │   │
│   │                                       [ Enable voice ]   │   │
│   └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│   ☐  Listen for "Hey Leah" wake-word  (opt-in)                  │
│      Always-listening adds ~2–4 % daily battery drain.           │
│      You can also enable this later in Settings → Voice.         │
│   ──────────────────────────────────────────────────────────────  │
│                          [ Skip — text only ]                    │
│                                       [ Back ]    [ Continue ]   │
└──────────────────────────────────────────────────────────────────┘
```

**Pre-grant:** static waveform glyph with caption "Enable mic to see the waveform." No fake animated sine loop (HIG anti-pattern per craft-HIG review — do not fake input).

**On grant:** static glyph swaps to real mic input (live waveform). On deny: card shows "Microphone blocked in System Settings. [Open System Settings →] or [Continue without voice]" — both buttons work, no dead-end.

**Wake-word toggle: UNCHECKED by default** (v2 operator override). Copy is honest about the battery cost (perf-review #1) and explicit about reversibility. Disabled if mic not granted ("Enable voice above to use wake-word"). Operator who flips it ON here gets the § 6.7 reliability behavior on first launch.

**Visual split (v3 — workflow MEDIUM):** the step layout is **two stacked sub-cards** (not 4 mixed widgets on one screen). Top card = mic-permission (binary `[Enable voice]` / `[Continue without]`). Bottom card = wake-word toggle + phrase picker + voice picker — only enabled after mic grant. Cognitive-load is one decision at a time.

No mention of any other permission here. Accessibility was handled in step 2; screen-recording, automation, etc. deferred to Settings → Permissions, lazy-prompted on first use.

### 8.5 Step 5 — One thing to connect

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ●  ●  ●  ●  ○                                       step 5/6│
│  ──────────────────────────────────────────────────────────────  │
│   Pick one thing for Leah to know about.                         │
│   You can connect more from Settings later.                      │
│   ──────────────────────────────────────────────────────────────  │
│   ◯ ┌──────────────────────────────────────────────────────┐    │
│     │ [📅] Calendar                                         │    │
│     │ "What's on today?" "Move my 3pm to tomorrow."         │    │
│     │ ≈ 20s · uses macOS Calendar (no OAuth)                │    │
│     └──────────────────────────────────────────────────────┘    │
│   ◯ ┌──────────────────────────────────────────────────────┐    │
│     │ [✉️ ] Mail                                             │    │
│     │ "Summarize unread." "Draft a reply to Sam."           │    │
│     │ ≈ 30s · OAuth via your default mail provider          │    │
│     └──────────────────────────────────────────────────────┘    │
│   ◯ ┌──────────────────────────────────────────────────────┐    │
│     │ [📁] Files                                            │    │
│     │ "Find my MAY-19 spec." "Open last week's notes."      │    │
│     │ ≈ 15s · pick one folder to index                      │    │
│     └──────────────────────────────────────────────────────┘    │
│   ──────────────────────────────────────────────────────────────  │
│           [ Skip — set up later ]    [ Back ]    [ Connect ]    │
└──────────────────────────────────────────────────────────────────┘
```

**Calendar pre-selected** per operator decision #14 (load-bearing for ambient HUD's "Now" slot — needs an agenda source within 60 s of finishing wizard). Copy reads `Recommended: Calendar` so the pre-selection is honest, not silent. Primary CTA `Connect` is active by default; operator can re-pick to Mail/Files or Skip. Skip is left-justified, equal weight. Connect button label changes per selection:
- Calendar → `EKEventStore.requestFullAccessToEvents()` OS prompt → on grant, card collapses to "✓ Calendar connected — 12 events this week".
- Mail → opens default browser to OAuth; wizard shows "Waiting for browser…" with [Cancel].
- Files → NSOpenPanel for folder pick; background indexing starts (thin gold underline 0–100%); operator can advance immediately.

Skip left-justified, equal weight to Connect.

### 8.6 Step 6 — You're ready

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ●  ●  ●  ●  ●                                       step 6/6│
│  ──────────────────────────────────────────────────────────────  │
│                              ⬡                                   │
│                       You're all set.                            │
│                                                                  │
│                Press   ⌥  Space  to call me.                     │
│                       (any time, from any app)                   │
│   ──────────────────────────────────────────────────────────────  │
│        Tip: I'll live in the bottom-right of your screen.        │
│              Drag me anywhere you like.                          │
│                                                                  │
│        Need to add more — Mail, Slack, GitHub, Files?            │
│              [ Open Settings ]                                   │
│   ──────────────────────────────────────────────────────────────  │
│                                       [ Back ]      [ Done ]    │
└──────────────────────────────────────────────────────────────────┘
```

Hotkey reminder big (32 pt glyphs). "Open Settings" is text-link (not primary CTA). "Done" closes wizard and:
1. Plays a single Transition-1 Gold Transition at the bottom-right where the HUD will land (380 ms — first-of-session full ceremony).
2. Ambient HUD materializes at default position.
3. First toast above HUD: "Press ⌥Space to ask anything." (lives **4 s**, dismissable). **Hotkey is hot only after the toast dismisses** (v3 — workflow MEDIUM): if the operator presses `⌥Space` during the 4 s toast window, the panel summons AFTER the toast clears (no overlap-over-the-teaching-toast). Toast and HUD do not overlap.
4. If wake-word was enabled in step 3, a follow-up toast 8 s later: `Heard Leah misfire? Press Esc within 2 s and I'll learn it.` (v3 — workflow MEDIUM "wake-word post-toast learn gesture" — teaches the false-positive recovery affordance from § 6.7).

### 8.7 Error / edge-case matrix

| Scenario | Behavior |
|---|---|
| Mic permission denied (step 3) | Red-dim banner with `●` prefix: "Microphone is blocked in System Settings." [Open System Settings] + [Continue without voice]. Wake-word opt-in toggle disables silently. |
| Hotkey conflicts with macOS reserved (step 2) | Red-alert caption with `●` prefix: "Conflicts with macOS Quick Look. Try another." Recorder stays open with **real-time** feedback as keys are pressed. |
| Accessibility permission denied (step 2) | Yellow warning card persists: "Hotkey will only work inside Leah windows until you grant Accessibility." Wizard continues; warning re-surfaces in Settings → Permissions. |
| Calendar / Mail OAuth fails (step 4) | Card returns to un-selected; toast: "Couldn't connect to Mail. [Try again] or pick another." |
| Network down during integration step | Wifi glyph + "You're offline. Set up integrations later from Settings." Connect buttons disable; Skip remains active. |
| Operator quits mid-wizard | Relaunch resumes at step 1 — UNLESS step 2 Anthropic key was saved to Keychain, then resumes at step 3 (forward progress preserved); UNLESS step 4 mic was granted, then resumes at step 5. |
| Re-run wizard from Settings | Each step reflects current state. Step 2 shows `● Already connected — workspace "…"` with [Replace key]. Step 4 shows "✓ Microphone already enabled" with wake-word toggle. Step 5 shows current integrations with "Connect another" CTA. Idempotent. |
| Reduced motion ON | Transitions degrade per § 5; progress dots update instantly. |
| VoiceOver active | Each step announces "Setup, step N of 6: [name]." Focus lands on heading; Tab cycles heading → primary → skip → back. |

---

## 9. Settings preferences pane

### 9.1 Window shape

- **Size:** 760 × 560 pt default; resizable to 640 × 480 pt min, no max.
- **Style:** Things-style — left sidebar (200 pt), right detail pane (560 pt+). Both columns scroll independently.
- **Chrome:** NSWindow `.titled` + `.fullSizeContentView`, traffic lights top-left at 20 pt inset.
- **Search:** top of sidebar, full-width, **auto-focused on Settings open** (Things pattern; `⌘F` re-focuses if cursor wandered). Filters BOTH sidebar items AND detail-row keywords in real-time. Matches highlight `--gold-primary`; non-matches dim to `--text-dim`. Typing a shortcut glyph (e.g., `⌥Space`) jumps to the Hotkey row.
- **Live preview pane:** Appearance section only — right-most 280 pt of detail pane shows a miniature HUD + panel rendered at **50 % scale, 10 fps frame-cap** (not full panel path — perf #7). Settings is a rarely-opened surface; the preview is theater, the cap is the cost-cut.
- **Persistence:** window position + last-section sticky across launches.

### 9.2 IA tree

```
Settings
├── General                ⌘1
│   ├── Start at login
│   ├── Hotkey (recorder + reset; default ⌥Space; real-time conflict check)
│   ├── HUD position (8-anchor grid + drag-anywhere note)
│   ├── HUD scale (Mini / Standard / Expanded)
│   ├── HUD visibility (always / focused-app only / Spaces filter)
│   ├── HUD always-on-top
│   └── Dedicated display when active app fullscreens   ← § 6.5
│       └── Choose display [primary / display 2 / …]
├── Voice                  ⌘2
│   ├── Wake-word toggle (default OFF; opt-in; honest battery copy)
│   ├── Wake-word phrase (default "Hey Leah" / "Leah" / "OK Leah")
│   ├── Per-app suppression list (meeting apps pre-populated)
│   ├── Voice model (3 TTS voices with hover-preview)
│   ├── Push-to-talk fallback (hold Space)
│   ├── Mute schedule (per-day quiet hours)
│   ├── Input language (auto / en / es / fr / …)
│   └── Output volume slider
├── Appearance             ⌘3
│   ├── Theme (Match System / Light / Dark) ← v2: light+dark parity
│   ├── Reduced motion (system / always)
│   ├── Reduced ornament (off / on) ← strips grain, italic, gold accents
│   │                                  to bare functional palette
│   ├── Density (compact / standard / spacious)
│   ├── Dock edge (snap-to-edge / float-free)
│   ├── Grain overlay (system / on / off)
│   └── [Live preview pane on right, 50% scale, 10fps cap]
│   (Removed v2: Accent intensity slider — accent comes from
│    NSColor.controlAccentColor per HIG; hairline opacity slider —
│    auto-adapts to backingScaleFactor per § 3.2)
├── Privacy                ⌘4
│   ├── Sensitive-content blocklist (regex + presets)
│   ├── Screen-recording auto-hide HUD (on by default)
│   ├── DND respect (on by default)
│   ├── Telemetry opt-out (1 toggle)
│   ├── Conversation logging (on / session-only / off)
│   └── Sensitive-app blocklist (1Password, banking, etc.)
├── Permissions            ⌘5    ← all lazy-prompted; this is the dashboard
│   ├── Microphone               [●] Granted
│   ├── Accessibility            [◐] Needed for [feature]
│   ├── Screen recording         [○] Not asked
│   ├── Automation: Calendar     [✕] Denied — [Open System Settings]
│   ├── Automation: Mail         [○] Not asked
│   ├── Contacts                 [○] Not asked
│   ├── Reminders                [○] Not asked
│   ├── Notifications            [○] Not asked
│   ├── Focus filter integration [○] Not asked
│   └── Full Disk Access         [○] Not asked
├── Integrations           ⌘6
│   ├── Connected (Calendar / Files / GitHub / Linear / arxiv / …)
│   ├── Available (Mail / Messages / Reminders / Slack / Notion / News)
│   ├── Errors (token expired etc.)
│   └── (v3) Tiered disconnect copy per integration:
│       — Low-data (Calendar/Mail/Reminders): single-button confirm.
│       — Data-bearing (Files/Memory-adjacent): "Disconnect (keep index)"
│         and "Disconnect + delete index (2,341 entries)" disambiguated.
├── Widgets                ⌘7    ← cross-ref § 10.6
│   ├── Per-type enable toggle
│   ├── Default refresh cadence
│   ├── Pinned widgets (max 3; reorder / unpin)
│   └── Widget registry version
├── Memory                 ⌘8
│   ├── Storage location (path + Reveal in Finder)
│   ├── Total entries (e.g., "1,247 entries · 14.2 MB")
│   ├── Browse memory (opens dashboard → Memory tab)
│   ├── Export memory (JSON / NDJSON / Markdown)
│   ├── Import memory (JSON / NDJSON)
│   └── [Purge memory] — destructive, typed-PURGE confirm
├── Advanced               ⌘9    ← v3.2.2 F4: model-escalation toggles
│   ├── "Use Opus 4.8 for hard queries" — single-shot (next request only)
│   │   • flips daemon model id from claude-sonnet-4-6 → claude-opus-4-8
│   │     for the next IPC request; auto-resets after; default OFF
│   ├── "Use Opus 4.8 for this session" — session-wide override
│   │   • persists until daemon restart OR operator toggles off; default OFF
│   ├── Default model (read-only label: claude-sonnet-4-6)
│   └── Router model (read-only label: claude-haiku-4-5)
└── About                  ⌘0
    ├── Version (with "Check for updates")
    ├── Log location (path + Reveal in Finder)
    ├── Daemon status (green/gold/red dot + restart button)
    ├── Open-source licenses
    ├── Privacy policy
    ├── Support
    └── Re-run setup wizard
```

### 9.3 Status glyph legend (consistent everywhere)

- `●` `--success-quiet` — granted
- `◐` `--gold-primary` — needs setup
- `○` `--text-muted` — not asked yet
- `✕` `--red-alert` — denied / error

**Tooltip + section-header micro-legend (v3 — UX #38):** every glyph carries an `accessibilityLabel` matching the legend; every section that uses glyphs (Permissions, Integrations, About → Daemon status) shows a persistent 1-line micro-legend (`● granted · ◐ needs setup · ○ not asked · ✕ denied`) at the section header so operators landing on a deep-linked page never face glyphs without a legend.

**Per-row toggle pattern (v3 — UI craft):** every Permissions row IS a toggle (NSSwitch) when the OS allows programmatic toggle (e.g., Notifications, Focus filter); glyphs sit as supplemental indicators to the right of the toggle. For permissions that can only be granted via System Settings (Accessibility, Screen Recording, Full Disk Access), the row shows `[ Grant in System Settings → ]` deep-link instead of a toggle, with the glyph still indicating state.

**State count (v3 — workflow MEDIUM):** legend is doctrinally 4 states; UI MAY visually collapse `◐ needs setup` and `○ not asked` into a single `[ Grant ]` CTA where the operator-meaningful action is identical, so the row reads as one decision instead of two. The 4-state legend remains the source of truth for tooltip text + telemetry.

### 9.4 Toggle copy rule

Every toggle has WHY in ≤1 line. No "Enable feature X" tautology.
- GOOD: "Say her name to summon her hands-free."
- GOOD: "Hides HUD + suppresses toasts when another app is recording your screen."
- BAD: "Enable wake-word detection."

If you can't describe a toggle in a sentence, the toggle shouldn't exist.

### 9.5 Destructive-action confirmation (memory purge)

GitHub repo-delete pattern. Two friction points:
1. Count displayed: "1,247 entries will be permanently deleted. This cannot be undone."
2. Type the word `PURGE` to enable the [Purge] button.

**Export-then-purge one-click flow (v3 — folded from v2-deferred + workflow MEDIUM #28):** the dialog asks `Have you exported memory first?` with three buttons: `[ Export, then purge ]` (primary, gold) writes a timestamped JSON+NDJSON+Markdown bundle to `~/Downloads/Leah-memory-export-YYYY-MM-DD.zip` then proceeds to the typed-PURGE step; `[ Purge anyway ]` skips export; `[ Cancel ]` aborts. **10-second grace period:** after operator types `PURGE` and clicks Purge, a non-modal toast appears `Purging in 10s · [ Undo ]` — clicking Undo within the window aborts; on expiry, the purge runs. Recovers from the "I clicked too fast" failure mode without weakening the typed-friction defense.

```
┌──────────────────────────────────────────────────┐
│   Purge all memory?                              │
│                                                  │
│   1,247 entries will be permanently deleted.     │
│   This cannot be undone.                         │
│                                                  │
│   Leah will forget every conversation, every     │
│   note, every observation captured since         │
│   2026-04-12.                                    │
│                                                  │
│   Type PURGE to confirm:                         │
│   ┌──────────────────────────┐                   │
│   │                          │                   │
│   └──────────────────────────┘                   │
│                                                  │
│                       [ Cancel ]    [ Purge ]    │
│                                  ↑ disabled until│
│                                    text=="PURGE" │
└──────────────────────────────────────────────────┘
```

---

## 10. Dynamic widget canvas

The operator does not open a "stocks app" or a "flights tab." She says *"show me today's market vs yesterday"* and a market tile materializes in the response stream. She says *"flights around September dates"* and a date×price matrix lands beside the prose. Widgets are first-class output, not chrome. Each one is dismissible; each one can be pinned to ambient.

### 10.0 Canvas model

The panel's response area is an **interleaved stream** — prose blocks and widget tiles share the same vertical rhythm. Tiles are not modals; they are paragraphs that happen to be pixels.

| Aspect | Behavior |
|---|---|
| **Default layout** | Full-width tiles stacked vertically. |
| **Wide panel (≥860 px)** | Small/medium tiles auto-flow into a 2-column grid; large/hero always span full-width. |
| **Tile chrome** | 1 px hairline @ 20% ivory; 12 px corner radius; **grain is the same single static texture loaded once globally** (NOT a per-tile composite — perf #2 / perf #18); eyebrow title `MARKET · TODAY` in body sans small-caps tracking +80 (New York Italic restricted to Dashboard "Today" header only — decision-log #28); **no gold rule under eyebrow** (v3 — workflow MEDIUM "drop gold rule under eyebrow") — eyebrow in `--text-dim` alone carries the role; gold rule was 4-layers-of-decoration tax visible-as-gravel at 50 tiles/week. Tile body inherits panel blur surface — never adds its own (perf #18). |
| **Top-right cluster** | `◆` pin glyph (12 px visual) + `×` dismiss (12 px visual), **each wrapped in a 24 × 24 padded hit-target** (WCAG 2.5.8); 8 px gap, 12 px inset. Pin: hairline-ivory when unpinned; **filled champagne gold** when pinned. Dismiss: hairline-ivory; hover → `--red-alert` `#D75A66` with `●` icon prefix in tooltip. Both Tab-targets with `accessibilityLabel`; keyboard P / X / Delete shortcuts when tile focused (§ 6.6). |
| **Tile sizes** | `small` 280×120 (1×1, ambient-eligible) · `medium` 720×180 (full-width) · `large` 720×320 (full-width tall) · `hero` 720×440 (panel-takeover; suppresses follow-up chips until dismissed). |
| **Density** | **Max 2 widget tiles per response turn** (reduced from v1's 4 — workflow #8). 3rd+ tile = LLM must summarize and offer `[ Show more ]` chip with explicit truncation order: `stat → chart → table → other`. Enforced at adapter-registration time; widget #3 in a turn = `tool_error` back to LLM (cross-doc fix v1 §9.0 vs proto §5.3). |
| **Gold budget** | **Max 3 visible `--gold-primary` instances per visible surface at once** (v3.2 — per decision #112; supersedes the per-panel-render cap of #39). 4th+ instance auto-demoted to `--gold-muted` or ivory. Enforced as a render-time invariant in the tile compositor + a wireframe-time audit, not a guideline. |
| **Pause-on-interaction** | Auto-refresh PAUSED while tile is hovered, focused, or scrolled (workflow #7). Resumes on blur + 5 s idle. |

### 10.1 Widget catalog

Every widget honors the same chrome contract. Naming, `id` patterns, and `size` enums are the source of truth shared by the protocol (§ 10.6–10.7). The 13 types:

---

**1. `market`** — price · delta · sparkline · multi-symbol grid · intraday candlestick

- **Default size:** `small` (single symbol) / `medium` (≤4 symbols grid; sparkline only) / **`hero` only for candlestick** (v3 — UI craft MEDIUM "candlestick on 720 px medium tile"). Candlestick density floor: **≥ 8 px per candle** for OHLC tick visibility; medium-size 720 px = ~90 candles maximum = less than a single trading day at 5-min resolution → meaningless. Medium tile = sparkline + delta + symbol; `hero` 720 × 440 = adequate candle width. Adapter MUST reject `kind="candlestick"` for size below `hero`.
- **Adapter:** `markets` (NEW — `internal/markets/`; price quotes + history; suggested provider Alpha Vantage or polite Yahoo scrape).
- **Gold lands on:** ticker symbol, current price (JetBrains Mono, 28 px small / 36 px medium). Sparkline stroke `#C9A961` at 60% opacity.
- **Oxblood lands on:** negative delta + `▼`. Positive delta = ivory + `▲`. **No green, ever.**
- **States:** loading = hairline frame + center 1 Hz gold dot pulse, ticker `————`. Empty (pre-open) = `MARKET CLOSED · opens 9:30am ET` (**body sans small-caps tracking +0.04em**, dim ivory — v3.1 sweep, per decision #28). Error = oxblood hairline + `Quote unavailable · retry` chip.
- **Schema:**

```json
{
  "$id": "leah://widget/market/1",
  "required": ["symbols","range"],
  "properties": {
    "symbols":    { "type":"array", "items":{"type":"string","pattern":"^[A-Z0-9.\\-:]{1,12}$"}, "minItems":1, "maxItems":10 },
    "range":      { "enum":["1D","5D","1M","3M","6M","1Y","5Y","MAX"] },
    "compare_to": { "type":["string","null"], "description":"ISO 8601 baseline" },
    "show":       { "type":"array", "items":{"enum":["price","pct","volume","sparkline"]}, "default":["price","pct","sparkline"] }
  }
}
```

```
┌──────────────────────────────────────────────────────────────────┐
│ MARKET · TODAY                                          ◆   ×   │
│ ──────────                                                       │
│  AAPL   228.43    ▲ 1.82 (+0.80%)    ╱╲    ╱╲___╱‾‾‾╲___       │
│  NVDA   142.07    ▼ 3.11 (−2.14%)    ‾‾╲__╱  ╲___      ╲__     │
│  TSLA   251.88    ▲ 4.20 (+1.70%)    __╱‾‾╲__╱‾‾╲___╱‾‾‾       │
│  MSFT   421.55    ▼ 0.45 (−0.11%)    ‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾        │
│  vs yesterday close · refreshed 60s ago                          │
└──────────────────────────────────────────────────────────────────┘
```

---

**2. `flights`** — date×price matrix · single-flight detail card

- **Default size:** `large` (matrix) / `medium` (single-flight card).
- **Adapter:** `flights` (`internal/flights/`).
- **Gold lands on:** **global minimum ONLY = filled `#C9A961` background with obsidian text** (v3 — UI craft MEDIUM "gold lowest-fare cell"). Row-minimums get a **1.5 pt gold left-edge hairline** (subtle, scan-able, not loud) — NOT a filled cell. Origin → destination route label uses gold once. Multiple filled-gold cells in one 28-cell matrix destroyed the "lowest fare" signal via gold clutter; the new rule keeps gold-budget ≤3 per render (canvas invariant § 10.0).
- **Oxblood lands on:** fares ≥30% above row median (subtle oxblood text).
- **Refresh:** manual only (fare polling is expensive + noisy).
- **Schema:**

```json
{
  "$id": "leah://widget/flights/1",
  "required": ["origin","destination"],
  "properties": {
    "origin":      { "pattern":"^[A-Z]{3}$" },
    "destination": { "pattern":"^[A-Z]{3}$" },
    "depart":      { "format":"date" },
    "return":      { "type":["string","null"], "format":"date" },
    "pax":         { "minimum":1, "maximum":9, "default":1 },
    "cabin":       { "enum":["economy","premium","business","first"], "default":"economy" },
    "max_stops":   { "minimum":0, "maximum":2 }
  }
}
```

```
┌──────────────────────────────────────────────────────────────────┐
│ FLIGHTS · SFO → LIS                                     ◆   ×   │
│ ──────────                                                       │
│         Sep 8   Sep 9   Sep 10  Sep 11  Sep 12  Sep 13  Sep 14 │
│  ─→     $812    $798    $760    $742    [$689]  $701    $748   │
│  ─→     $834    $812    $780    $760    $710    $695    $765   │
│  ←─     $755    $740    $720    [$688]  $702    $715    $740   │
│  ←─     $812    $790    $762    $745    $720    $705    $730   │
│  cheapest round-trip: Sep 12 ↔ Sep 19 · $1,377                  │
└──────────────────────────────────────────────────────────────────┘
```

---

**3. `calendar`** — day timeline strip · week 7-col grid · agenda list · next-event card

- **Default size:** `small` (next-event) / `medium` (day strip or agenda) / `large` (week grid).
- **Adapter:** `calendar` (`internal/macos/calendar/`).
- **Gold lands on:** now-line (1 px gold rule across timeline) + next event title. Today's column header in week view = gold underline.
- **Oxblood lands on:** conflicts (overlapping blocks render with oxblood hairline outline on the shorter block) + `LATE` labels.
- **Refresh:** 5 min when pinned.
- **Schema:**

```json
{
  "$id": "leah://widget/calendar/1",
  "required": ["range"],
  "properties": {
    "range":         { "enum":["today","tomorrow","this_week","next_7d","custom"] },
    "from":          { "format":"date-time" },
    "to":            { "format":"date-time" },
    "calendars":     { "type":"array", "items":{"type":"string"} },
    "show_declined": { "type":"boolean", "default":false }
  }
}
```

```
┌──────────────────────────────────────────────────────────────────┐
│ CALENDAR · THIS WEEK                                    ◆   ×   │
│ ──────────                                                       │
│        Mon 22  Tue 23  Wed 24  Thu 25  Fri 26  Sat 27  Sun 28  │
│                                ▔▔▔▔▔                            │
│ 09 ▏                ▕standup▏                                    │
│ 10 ▏ ▕review▏                  ▕1:1 jen▏                        │
│ 11 ▏          ▕spec ▏                                            │
│ 12 ▏──────────────── now ──────────────────────────────────     │
│ 13 ▏                                    ▕lunch▏                  │
│ 14 ▏  ▕deep work, no booking ▏                                   │
│ 15 ▏                                                              │
│ 16 ▏          ▕arch sync ▏                                       │
└──────────────────────────────────────────────────────────────────┘
```

---

**4. `weather`** — current condition · 7-day forecast row · hourly bar

- **Default size:** `small` (current) / `medium` (7-day strip).
- **Adapter:** `weather` (`internal/weather/`).
- **Gold lands on:** current temperature numeral + day-name of any watch-condition day (severe, precip > 70%).
- **Oxblood lands on:** severe-weather chip (`HEAT ADVISORY`, `STORM WARNING`) — chip background oxblood, text ivory.
- **Glyphs (v3 — UI craft #5.1 + workflow MEDIUM):** SF Symbol stroke set (`sun.max.fill`, `cloud.sun.fill`, `cloud.rain.fill`, `cloud.bolt.fill`, `snow`, `wind`, `cloud.fog.fill`) tinted `--gold-muted`. **NEVER emoji** — system emoji renders full-color (breaks obsidian/gold/ivory discipline) AND varies by macOS version / user font.
- **Refresh:** 15 min when pinned.
- **Schema:**

```json
{
  "$id": "leah://widget/weather/1",
  "required": ["location"],
  "properties": {
    "location": { "type":"string" },
    "horizon":  { "enum":["now","hourly_24h","daily_7d","daily_14d"], "default":"daily_7d" },
    "units":    { "enum":["imperial","metric"], "default":"imperial" }
  }
}
```

```
┌──────────────────────────────────────────────────────────────────┐
│ WEATHER · SAN FRANCISCO                                 ◆   ×   │
│ ──────────                                                       │
│   64°   partly cloudy · feels 62° · wind 8 mph W                │
│                                                                  │
│   Sun   Mon   Tue   Wed   Thu   Fri   Sat                       │
│  [sun] [cloud] [cloud] [rain] [rain] [sun] [sun]                │  ← SF Symbol stroke set
│   68°  66°   63°   58°   59°   65°   70°                       │    (sun.max / cloud.sun /
│   52°  50°   49°   48°   47°   51°   54°                       │     cloud.rain), --gold-muted
└──────────────────────────────────────────────────────────────────┘
```

---

**5. `maps`** — place card with mini-map · route card with stops

- **Default size:** `medium` (place) / `large` (route).
- **Adapter:** `maps` (`internal/maps/`).
- **Gold lands on:** pin glyph + place name; route polyline (1.5 px champagne, 80% opacity over desaturated obsidian basemap).
- **Oxblood lands on:** closure notices, "permanently closed".
- **Basemap rule:** never a vendor-bright tile set. Desaturate to obsidian + hairline-ivory roads.
- **Citation-card fallback (v3 — workflow MEDIUM "maps illegible for navigation"):** when operator's intent is *route to somewhere*, the tile MAY render as a clean citation card with origin/destination, ETA, and a primary `[ Open in Apple Maps ]` CTA — no mini-map render. Desaturated mini-map is gorgeous in screenshots but illegible for actual navigation at `medium` tile size; the citation card hands off to the real navigator. The full mini-map render is reserved for `mode:"view"` (place card with no routing intent) and `large` size only.
- **Schema:**

```json
{
  "$id": "leah://widget/maps/1",
  "oneOf": [
    { "required":["center"], "properties":{"mode":{"const":"view"}} },
    { "required":["route"],  "properties":{"mode":{"const":"route"}} }
  ],
  "properties": {
    "mode":   { "enum":["view","route"], "default":"view" },
    "center": { "required":["lat","lon"], "properties":{"lat":{"type":"number"},"lon":{"type":"number"}} },
    "zoom":   { "minimum":1, "maximum":20 },
    "pins":   { "type":"array", "items":{"required":["lat","lon"]} },
    "route":  { "required":["from","to"], "properties":{"from":{"type":"string"},"to":{"type":"string"},"mode":{"enum":["drive","walk","bike","transit"]}} }
  }
}
```

---

**6. `table`** — sortable rows · header accent · negative-cell tint

- **Default size:** `medium` / `large`.
- **Adapter:** pure-LLM (no fetch; LLM emits rows).
- **Gold lands on:** header row (gold 1 px under-rule + **body sans small-caps tracking +0.04em** column names — v3.1 sweep, per decision #28), hovered row (gold hairline left-edge, 2 px). Active sort = gold caret `▾`.
- **Oxblood lands on:** negative numeric cells; LLM-flagged anomalies.
- **Caps:** ≤8 columns, ≤200 rows.
- **Schema:**

```json
{
  "$id": "leah://widget/table/1",
  "required": ["columns","rows"],
  "properties": {
    "columns": { "type":"array", "minItems":1, "maxItems":8, "items":{"required":["key","label"], "properties":{"key":{"type":"string"},"label":{"type":"string"},"align":{"enum":["left","right","center"]},"format":{"enum":["text","number","pct","currency","date","relative_time"]}}} },
    "rows":    { "type":"array", "maxItems":200, "items":{"type":"object"} },
    "sort":    { "properties":{"column":{"type":"string"},"dir":{"enum":["asc","desc"]}} }
  }
}
```

```
┌──────────────────────────────────────────────────────────────────┐
│ TABLE · OPEN PRS                                        ◆   ×   │
│ ──────────                                                       │
│ ▾ #     Title                       Author     +     −          │
│ ─────────────────────────────────────────────────────────────── │
│   321   server-pushed widget hud    @tri      +412  −38         │
│   320   check-amend-after-approve   @tri      +118  −12         │
│   319   handoff-continuity guard    @tri      +204  −9          │
│   313   arxiv+releases adapters     @tri      +287  −214        │
└──────────────────────────────────────────────────────────────────┘
```

---

**7. `chart`** — line · bar · area · scatter · sparkline

- **Default size:** `medium` / `large`.
- **Adapter:** pure-LLM OR fetched (discriminator on `props.source.adapter` — values: `market`, `weather`).
- **Gold lands on:** the ONE accent series (operator's primary subject). Other series in muted ivory @ 40% — no rainbow.
- **Oxblood lands on:** annotation markers for adverse events (outage spike, drawdown). Glyph: small oxblood diamond + **body sans small-caps tracking +0.04em** label (v3.1 sweep, per decision #28).
- **Axes:** hairline ivory @ 20%, JetBrains Mono tick labels, gold tick mark on the "today" / "now" axis position.
- **Schema:**

```json
{
  "$id": "leah://widget/chart/1",
  "required": ["kind","series"],
  "properties": {
    "kind":   { "enum":["line","bar","area","scatter","sparkline"] },
    "x_axis": { "properties":{"label":{"type":"string"},"type":{"enum":["time","numeric","category"]}} },
    "y_axis": { "properties":{"label":{"type":"string"},"min":{"type":"number"},"max":{"type":"number"}} },
    "series": { "type":"array", "items":{"required":["name","points"], "properties":{"name":{"type":"string"},"points":{"type":"array","items":{"required":["x","y"]}}}} },
    "source": { "properties":{"adapter":{"enum":["market","weather"]},"ref":{"type":"string"}} }
  }
}
```

---

**8. `image`** — thumbnail with metadata caption · QuickLook on click

- **Default size:** `small` (thumbnail row) / `medium` (single image with caption).
- **Adapter:** `web` (daemon dereferences; LLM never opens URL directly).
- **Gold lands on:** caption eyebrow (filename in **body sans small-caps tracking +0.04em** — v3.1 sweep, per decision #28) + 1 px gold hairline frame on the thumbnail (the only widget where gold frames the body — the image is the artifact).
- **Oxblood lands on:** "missing" or "permission denied" overlay.
- **Micro-interactions:** click → macOS QuickLook. Space-bar with widget focused = same. Right-click → reveal in Finder.
- **Schema:**

```json
{
  "$id": "leah://widget/image/1",
  "required": ["url"],
  "properties": {
    "url":     { "format":"uri", "pattern":"^https://" },
    "alt":     { "type":"string" },
    "caption": { "type":"string" }
  }
}
```

---

**9. `code`** — syntax-highlit block · "Obsidian Brass" theme · copy button

- **Default size:** `medium` / `large`; never `small`.
- **Adapter:** pure-LLM.
- **Theme "Obsidian Brass":** bg `#0E1014`; default text `#F2EDE0`; keywords `#C9A961`; strings ivory @ 80%; comments ivory @ 35% italic; numbers JetBrains Mono `#C9A961` @ 70%; **errors / linter squiggles oxblood**. No purple, no blue, no green.
- **Chrome:** language eyebrow (`GO`, `PYTHON`, `SQL`) in body sans small-caps, `[ Copy ]` chip top-right (left of pin/dismiss), line numbers in JetBrains Mono @ 30%.
- **Line-number interaction (v3 — UX #24):** click-line-number-to-copy is REMOVED — line numbers at 30% opacity are far below 24×24 hit-target and the affordance is invisible. Replacement: hover-row shows a faint `[ Copy line ]` chip in the right gutter; focused row + `⌘C` copies the line. Block-level `[ Copy ]` chip top-right copies the whole source.
- **Caps:** source ≤16 KB.
- **Schema:**

```json
{
  "$id": "leah://widget/code/1",
  "required": ["language","source"],
  "properties": {
    "language":  { "pattern":"^[a-z0-9_+\\-]{1,24}$" },
    "source":    { "type":"string", "maxLength":16384 },
    "filename":  { "type":"string" },
    "highlight": { "type":"array", "items":{"type":"integer","minimum":1} },
    "runnable":  { "type":"boolean", "default":false }
  }
}
```

```
┌──────────────────────────────────────────────────────────────────┐
│ GO  ·  internal/hud/refresh.go                 [ Copy ]  ◆   ×  │
│ ──────────                                                       │
│   12  func (h *HUD) Push(ev Event) error {                      │
│   13      if h.closed.Load() { return ErrClosed }               │
│   14      select {                                              │
│   15      case h.q <- ev: return nil                            │
│   16      default: return ErrBackpressure                       │
│   17      }                                                     │
│   18  }                                                         │
└──────────────────────────────────────────────────────────────────┘
```

---

**10. `citation`** — paper / article / docs card

- **Default size:** `small` / `medium`.
- **Adapter:** `web` for URL meta; `internal/papers/` + `internal/feeds/` (arxiv, releases) for typed `kind` enrichment.
- **Gold lands on:** source domain name. **Oxblood lands on:** "stale" / "404" flags.
- **Schema:**

```json
{
  "$id": "leah://widget/citation/1",
  "required": ["url"],
  "properties": {
    "url":       { "format":"uri" },
    "title":     { "type":"string" },
    "author":    { "type":"string" },
    "published": { "format":"date" },
    "snippet":   { "type":"string", "maxLength":400 },
    "kind":      { "enum":["paper","article","docs","gh_release","gh_issue","tweet","video","other"] }
  }
}
```

---

**11. `stat`** — single number · label · delta (KPI)

- **Default size:** `small`.
- **Adapter:** pure-LLM.
- **Gold lands on:** headline numeral (JetBrains Mono, 48 px). Label in **body sans small-caps tracking +0.04em** eyebrow above (v3.1 sweep, per decision #28); delta below with `▲ +12%` (ivory) or `▼ −4%` (oxblood).
- **Micro-interactions:** click → spawns `medium` chart tile of the underlying series.
- **Schema:**

```json
{
  "$id": "leah://widget/stat/1",
  "required": ["label","value"],
  "properties": {
    "label":      { "type":"string", "maxLength":60 },
    "value":      { "type":["string","number"] },
    "unit":       { "type":"string" },
    "delta":      { "type":"number" },
    "delta_unit": { "enum":["abs","pct"] },
    "trend":      { "enum":["up","down","flat"] }
  }
}
```

```
┌──────────────────────────┐
│ PRS MERGED · 7d     ◆ × │
│ ──────────               │
│         52               │
│   ▲ +14 vs prior week    │
└──────────────────────────┘
```

---

**12. `list`** — bullet · numbered · scrollable · hover-highlight

- **Default size:** `medium`; scrolls internally past 8 items.
- **Adapter:** pure-LLM.
- **Gold lands on:** bullet glyph (small gold diamond `◆`) or numeral. Hover row = 1 px gold hairline left edge + 80 ms ivory tint.
- **Oxblood lands on:** items flagged with status (`FAILING`, `ERROR`).
- **Caps:** ≤50 items.
- **Schema:**

```json
{
  "$id": "leah://widget/list/1",
  "required": ["items"],
  "properties": {
    "items":   { "type":"array", "maxItems":50, "items":{"required":["text"], "properties":{"text":{"type":"string"},"meta":{"type":"string"},"icon":{"type":"string"},"callback":{"type":"string","pattern":"^leah://action/"}}} },
    "ordered": { "type":"boolean", "default":false }
  }
}
```

---

**13. `diff`** — git-diff render · gold added · oxblood removed

- **Default size:** `medium` / `large`.
- **Adapter:** pure-LLM.
- **Gold lands on:** added lines — gutter `+` glyph in champagne, line background `#C9A961` @ 8% obsidian-tinted.
- **Oxblood lands on:** removed lines — gutter `−` in `#C8434F`, line background `#7A1F2B` @ 12%.
- **Chrome:** file path eyebrow (`internal/hud/refresh.go`) in **body sans small-caps tracking +0.04em** (v3.1 sweep, per decision #28). Hunk separator = 1 px hairline + line-range in JetBrains Mono.
- **Schema:**

```json
{
  "$id": "leah://widget/diff/1",
  "required": ["hunks"],
  "properties": {
    "language": { "type":"string" },
    "filename": { "type":"string" },
    "hunks": { "type":"array", "items":{"required":["old","new"], "properties":{"old":{"type":"string"},"new":{"type":"string"},"context_before":{"type":"string"},"context_after":{"type":"string"},"old_start":{"type":"integer"},"new_start":{"type":"integer"}}} }
  }
}
```

```
┌──────────────────────────────────────────────────────────────────┐
│ DIFF · internal/hud/refresh.go                          ◆   ×   │
│ ──────────                                                       │
│   @@ -12,4 +12,6 @@                                              │
│   12   func (h *HUD) Push(ev Event) error {                     │
│   13       if h.closed.Load() { return ErrClosed }              │
│ +─ 14 ◆   if ev.Priority == PriorityRed {                       │
│ +─ 15 ◆       return h.pushUrgent(ev)                           │
│ +─ 16 ◆   }                                                     │
│   17       select { case h.q <- ev: return nil                  │
│ −─ 18 ×       default: return ErrBackpressure                   │
│   19       }                                                    │
└──────────────────────────────────────────────────────────────────┘
```

### 10.2 Lifecycle + state machine

Per-tile: `spawning → live → refreshing → live | stale | error → dismissed`. Pinned tiles persist across panel dismiss.

| Event | Trigger | Daemon | UI |
|---|---|---|---|
| **Spawn** | LLM emits `render_widget` tool-call mid-stream | `Validate(props)` → `Fetch()` → emit `widget.mount{id,data,height}` frame | **Tile-height reserved from `props` immediately** (v3 — workflow MEDIUM "reserve tile height"); body populates in-place without layout reflow shifting prose the operator is reading. Multiple widgets in one turn: **reveal staggered by 80 ms** (v3 — perf #13) to prevent 4 × 240 ms overlapping gold-seam animations from breaking the 16 ms/60 Hz frame budget on M1-integrated GPUs. Render path is async; never blocks prose stream (workflow MEDIUM). |
| **Refresh (auto)** | `refresh` timer expires | `Refresh(ctx,id,props,prev)` → emit `widget.update{id,data,etag}`; skip if etag unchanged | Animates value delta per § 5 |
| **Refresh (manual)** | Operator clicks refresh chip OR daemon push (MAY-19 B1) | Same as auto; bypasses timer | Same |
| **Pin** | Operator clicks `◆` (or P) | Append `{id, type, props, refresh}` to `~/Library/Application Support/Leah/pinned-widgets.json`; HUD watcher (`internal/hud/`) re-reads via fsnotify with **200 ms debounce** and **single watcher across both pinned-widgets.json + widget-registry.json** (perf #21 — atomic-rename fires 2-3 events, debounce coalesces). | Tile gains filled gold pin badge; ambient slot renders `small` variant |
| **Unpin** | Operator clicks `◆` again OR Settings → Widgets → remove | Remove entry; HUD re-reads; ambient slot frees | Tile loses badge |
| **Dismiss** | Operator clicks `×` OR panel closes on unpinned tile | No state mutation; cancel refresh timer | Tile collapses per § 10.5 dismiss curve |
| **Error** | `Fetch`/`Refresh` returns non-nil err | If `prev` payload exists, emit `widget.stale{id,data:prev,reason}`; else `widget.error{id,reason,retry_in}` | Stale: oxblood underline + cached value + relative-age caption. Error: oxblood frame + retry chip |

**Cached-last-good cache:** `~/Library/Caches/com.leah.daemon/widget-cache/<adapter>/<sha256(props)>.json` (Apple convention — system-purgeable on low disk). TTL = `Payload.StaleAfter` × 4 (cap 24 h). **In-memory LRU + periodic flush** (every 5 min or on dirty-count >10 — perf #22 avoids per-refresh fsync churn on APFS). **Cache directory LRU cap 50 MB**; GC on cache write via cheap dir-size stat (perf #31). Cache survives daemon restart; pin-driven refresh on daemon start re-warms.

**ID stability:** the LLM is instructed to reuse the same `id` when the user iterates (`"add MSFT"` → same `mkt_*` id). Re-emit with same `id` = update-in-place. New `id` = new tile.

### 10.3 Pin-to-ambient flow

Pinning is the load-bearing affordance for the dynamic-canvas idea. The panel is *conversation* — ephemeral by design. The HUD is *presence* — persistent by design. Pin promotes a widget across that boundary.

| Aspect | Behavior |
|---|---|
| **Pin action** | Click `◆` (or **P** when tile focused) in tile top-right. Glyph fills champagne; 240 ms gold-seam-right confirmation animation. |
| **Ambient slot reservation** | Ambient HUD grows downward by one slot row (84 px → 168 px → 252 px) to accommodate up to **2 pinned widgets** (reduced from v1's 3 — workflow #6 corner-real-estate accounting). Each renders as `small` variant — large/medium/hero auto-downsize on pin. |
| **Refresh cadence (when pinned)** | Market: 60 s. Weather: 15 min. Calendar: 5 min. Stat: 5 min. Flights: **manual only**. Code/Diff/Image: never (static artifact). All cadences via `NSBackgroundActivityScheduler` with tolerance (not `NSTimer`); paused on `isLowPowerModeEnabled` or screen-off; paused on tile hover/focus (perf #20, workflow #7). **Ambient pinned tiles are static-until-glanced (v3 — workflow MEDIUM "animate delta on hover only"):** value deltas animate only when the operator hovers or focuses the tile. A blinking delta in the corner of every screenshot / Loom / screenshare is a distraction; the metric updates silently and reveals motion on intent. |
| **Unpin** | Click `◆` again (or **P**) from either surface. Both surfaces decrement in sync. |
| **Max 2 pinned** | 3rd pin attempt → panel shows quiet inline note: `Ambient is full · unpin one to add this one.` with chips for each currently-pinned widget. **Never silently evict.** |
| **Cold-launch refresh** | Pinned widgets paint from cache immediately (perf #36); refresh staggered 250 ms apart in background; stale-frame oxblood indicator only on fetch error. No network-stall on first paint. |
| **Persistence** | Per-operator, restored on app restart. Pinned widgets re-fetch on launch before painting (no stale-fare ghosts). |

**Ambient HUD with 2 pinned widget slots (v3.1 — was "3 pinned"; cap is 2 per decision #40):**

```
                          ┌──────────────────────────────────────────┐
                          │ ⬡  Listening…                            │
                          │ ──────────────────────────────────────── │
                          │ Standup in 12m · 9:30am                  │
                          │ ──────────────────────────────────────── │
                          │ ◇ 5 PRs                                  │
                          │ ════════════════════════════════════════ │  ← pin divider (gold, 1px)
                          │ MARKET                              ◆    │
                          │ AAPL  228.43  ▲0.80%   ╱╲___╱‾‾‾        │
                          │ ──────────────────────────────────────── │
                          │ PRS MERGED · 7d                     ◆    │
                          │ 52   ▲ +14 vs prior week                │
                          └──────────────────────────────────────────┘
                                       bottom-right of screen
```

The pin-divider (double-rule, 1 px gold) is the only place gold lands as a *background-spanning* horizontal — its job is to say "above this line is Leah; below this line is *your* dial."

### 10.4 Widget gallery

**Three discoverable affordances** (workflow #5 fix — v1's `/widgets`-only path failed by week 2):
1. **Panel-resident `+` button** next to the input field — visible every session, every panel open. Tap → gallery overlay.
2. **`⌘⇧W`** while panel focused.
3. **Typing `/widgets`** in panel input (power-user shortcut, surfaced in `⌘/` cheatsheet).

Opens an overlay browser — a catalog of complications, watch-collector style.

| Aspect | Behavior |
|---|---|
| **Layout** | Overlay covers panel response area (input stays visible at top); left rail = category list, right pane = preview grid. |
| **Categories** | `Finance · Travel · Time · Productivity · Web · Code` — 6 fixed at v1. Each rendered as a **body sans small-caps tracking +0.04em** eyebrow (v3.1 sweep, per decision #28). |
| **Preview** | Each catalog cell is a live `small`-variant tile rendered with sample data. Real components, not screenshots — what you see is what spawns. |
| **Spawn** | Hover cell → ivory tint + `[ Spawn ]` chip appears. Click → overlay dissolves (160 ms), tile materializes with standard 240 ms gold-seam-down. |
| **Dismiss** | Esc, click outside, or re-type `/widgets`. |
| **Search** | Top of overlay: slim search field (`Find a widget…`) — fuzzy match across widget name + category + sample-data text. |

```
┌─────────────────────────────────────────────────────────────────┐
│ /widgets                                                   Esc │
│ ───────────────────────────────────────────────────────────────  │
│  CATEGORIES         │   Find a widget…                          │
│                     │   ─────────────────────────────────────   │
│  ▸ Finance          │   ┌─────────────┐  ┌─────────────┐       │
│    Travel           │   │ MARKET      │  │ STAT CARD   │       │
│    Time             │   │ AAPL 228.43 │  │   52        │       │
│    Productivity     │   │ ▲ 0.80%  ╱╲ │  │ ▲ +14 / 7d  │       │
│    Web              │   │ [ Spawn ]   │  │ [ Spawn ]   │       │
│    Code             │   └─────────────┘  └─────────────┘       │
│                     │                                            │
│                     │   ┌─────────────┐  ┌─────────────┐       │
│                     │   │ CHART       │  │ TABLE       │       │
│                     │   │ line · area │  │ sortable    │       │
│                     │   │ [ Spawn ]   │  │ [ Spawn ]   │       │
│                     │   └─────────────┘  └─────────────┘       │
└─────────────────────────────────────────────────────────────────┘
```

### 10.5 Spawn / loading / error states

A widget appearing should feel *deliberate* — not a popup, not a notification, not a flash. The motion language is the same gold transition that opens the panel, scaled down.

| State | Behavior |
|---|---|
| **LLM emits widget tool-call** | Stream pauses ≤80 ms; tile frame paints as 1 px gold horizontal seam at the tile's vertical center. |
| **Reveal (240 ms `ease-out-quart`)** | Seam expands top + bottom to full tile bounds; during the last 80 ms, eyebrow title fades in (`ease-in`). Content paints behind the frame after settle. |
| **Loading** | Hairline frame + eyebrow visible; tile body shows a single centered champagne dot (8 px) pulsing at 1 Hz (`60% → 100% → 60%` opacity, `ease-in-out`). No spinners. No skeleton-shimmer everywhere — restraint. |
| **Error** | Tile frame swaps hairline from ivory @ 20% to **oxblood @ 60%** in 160 ms; body renders error copy: eyebrow `● COULDN'T LOAD` (icon prefix — not color-alone), body in Söhne, **secondary line ≤40 chars naming the failure mode** (e.g., `couldn't reach api.market.io · timeout 5s` — so operator knows network not auth, Nielsen #29) + single `[ Retry ]` chip (gold hairline, ivory text, hover → gold fill). |
| **Stale** | Cached value rendered with oxblood underline + relative-age caption (e.g., "last seen 14m ago"). |
| **Dismiss** | Tile collapses to 1 px gold horizontal seam (240 ms `ease-in-quart`), then fades to nothing (80 ms). Surrounding stream reflows in the same 240 ms. |
| **Pin confirm** | Pin glyph fills gold; 240 ms gold-seam-right sweeps from the pin glyph toward the panel's right edge, signalling "promoted to ambient." Ambient HUD grows simultaneously. |

**Empty-canvas quick-spawn.** A freshly-opened panel with no prior conversation renders the standard empty-state (mark + "Good morning, Tri.") **plus** up to 3 quick-spawn chips below the `⌃ Try` row. Chips show widget eyebrow names (`MARKET · TODAY`, `CALENDAR · THIS WEEK`, `PRS MERGED · 7d`) in body sans small-caps. Source: most-recently-pinned first; if <3 pinned, backfill from operator's most-spawned types over the last 14 days. Click → standard spawn. **Chips persist across the session** (v3 — workflow MEDIUM "spawn chips dismiss after first"); they hide only when the panel transitions out of empty-state (active conversation present) and reappear on the next empty-state. Source rank uses an O(1) rolling top-3 file (`~/Library/Application Support/Leah/widget-recents.json`) updated on spawn — no usage-log scan on every panel-open (perf #32). Operator can hide chips permanently via Settings → Widgets → "Show quick-spawn chips on empty panel" toggle.

### 10.6 Adapter registry + data sources

| Widget    | Adapter      | Status / source                                          |
|-----------|--------------|----------------------------------------------------------|
| market    | `markets`    | **NEW** — add `internal/markets/` (Alpha Vantage or Yahoo polite poller; watchlist stores symbols) |
| flights   | `flights`    | Reuse `internal/flights/`                                |
| calendar  | `calendar`   | Reuse `internal/macos/calendar/`                         |
| weather   | `weather`    | Reuse `internal/weather/`                                |
| maps      | `maps`       | Reuse `internal/maps/`                                   |
| citation  | `web`        | Reuse `internal/web/` for URL meta; `papers/`/`feeds/` for typed enrichment |
| image     | `web`        | Daemon fetches + caches; never exposes URL to LLM round-trip |
| chart     | `markets` \| `weather` \| pure | Discriminator on `props.source.adapter`; absent → pure |
| table / code / stat / list / diff | `PureAdapter` | LLM emits payload; no fetch |

**Adapter contract:**

```go
// internal/widget/adapter.go
package widget

import (
    "context"
    "encoding/json"
    "time"
)

type Payload struct {
    Data        json.RawMessage `json:"data"`
    FetchedAt   time.Time       `json:"fetched_at"`
    StaleAfter  time.Duration   `json:"stale_after"`
    Source      string          `json:"source"`
    Etag        string          `json:"etag,omitempty"`
}

type WidgetAdapter interface {
    Type() string
    Validate(props json.RawMessage) error
    Fetch(ctx context.Context, props json.RawMessage) (Payload, error)
    Refresh(ctx context.Context, id string, props json.RawMessage, prev *Payload) (Payload, error)
}

type PureAdapter struct{ kind string }
func (p PureAdapter) Type() string { return p.kind }
func (p PureAdapter) Validate(props json.RawMessage) error { return validateSchema(p.kind, props) }
func (p PureAdapter) Fetch(_ context.Context, props json.RawMessage) (Payload, error) {
    return Payload{Data: props, FetchedAt: time.Now(), Source: "llm"}, nil
}
func (p PureAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, _ *Payload) (Payload, error) {
    return p.Fetch(ctx, props)
}
```

**Registry file:** `~/Library/Application Support/Leah/widget-registry.json` — single source of truth for "what widget types this daemon understands." Persisted so Settings → Widgets can flip per-widget enable without restart.

```json
{
  "protocol_version": "1",
  "updated_at": "2026-06-21T08:00:00Z",
  "widgets": [
    { "type":"market", "version":"1", "source":"builtin", "enabled":true,
      "schema_uri":"leah://widget/market/1", "adapter":"markets",
      "default_size":"medium", "default_refresh":60,
      "actions":["pin","refresh","dismiss","change_range"] }
  ]
}
```

Daemon rejects tool-calls referencing entries with `enabled=false` or where the binary's compiled-in protocol version differs from the file's `protocol_version` (operator gets a Settings prompt to upgrade — daemon writes `<file>.bak` and rewrites).

### 10.7 Streaming protocol

**Transport:** daemon → UI is one persistent local IPC channel — Unix socket at `~/Library/Application Support/Leah/leah.sock`; framing is length-prefixed JSON per frame. Same channel carries prose deltas and widget events; interleaving is by emission order. **Envelope size cap 256 KB** per frame (perf #25 — protects parser buffer).

**Envelope (shared across all widgets):**

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "leah://widget/envelope/1",
  "required": ["widget", "id", "size"],
  "properties": {
    "widget":  { "enum": ["market","flights","calendar","weather","maps","table","chart","image","code","citation","stat","list","diff"] },
    "id":      { "pattern": "^[a-z0-9_-]{1,64}$", "description": "stable across refresh/pin; LLM may reuse id to update an existing tile" },
    "size":    { "enum": ["small","medium","large","hero"] },
    "refresh": { "type": ["integer","null"], "minimum": 5, "description": "seconds between auto-refresh; null = manual only" },
    "actions": { "type": "array", "maxItems": 4, "items": { "required":["label","callback"], "properties":{"label":{"type":"string","maxLength":32},"callback":{"type":"string","pattern":"^leah://action/[a-z_]+(\\?.*)?$"},"icon":{"type":"string"}} } },
    "props":   { "type": "object" }
  },
  "additionalProperties": false
}
```

**Frame types:**

```ts
type Frame =
  | { kind: "prose.delta",  turn_id: string, seq: number, text: string }
  | { kind: "prose.end",    turn_id: string, seq: number }
  | { kind: "widget.mount", turn_id: string, seq: number, id: string, widget: string, size: string, props: object, data: object, refresh: number|null, actions?: Action[] }
  | { kind: "widget.update",turn_id: string, seq: number, id: string, data: object, etag?: string }
  | { kind: "widget.stale", turn_id: string, seq: number, id: string, data: object, reason: string, fetched_at: string }
  | { kind: "widget.error", turn_id: string, seq: number, id: string, reason: string, retry_in?: number }
  | { kind: "widget.unmount", turn_id: string, seq: number, id: string }
  | { kind: "turn.end",     turn_id: string, seq: number, cost: object }
```

`seq` is monotonic per `turn_id`; UI reorders if the OS coalesces writes. `turn_id` lets a future operator-interrupt cancel an in-flight turn cleanly.

**Interleaving rule.** When the LLM mid-stream emits a complete `render_widget` tool-call (daemon JSON-decoder watches for a balanced `{...}` boundary while accumulating chunks), the daemon:
1. Flushes pending `prose.delta`.
2. Validates + fetches concurrently (does not block subsequent prose deltas).
3. On fetch return, emits `widget.mount` at the current `seq` position.

UI renders chunks in `seq` order: prose appends to current paragraph; widget mounts insert a tile *between* paragraphs (never mid-sentence). If a widget's fetch is still in-flight when the next prose chunk arrives, the daemon may emit `widget.mount` with `data:{}` (placeholder; UI shows "spawning" shimmer) followed by a later `widget.update` once data lands — this preserves visual order. **Tile height MUST be derivable from `props.size` immediately** so the placeholder reserves the final height (no layout reflow when `widget.update` lands — workflow MEDIUM).

**Widget→prose citation (v3 — UX #30):** when schema validation fails AND the LLM falls back to prose, the daemon emits a `prose.delta` with the citation line `tried to show {widget} but data didn't fit — falling back to text` so operators know a widget was attempted (not silently swallowed).

**Hot-path framing (v3 — perf #24 MEDIUM):** prose deltas + widget events share the channel by default; if bursty load is measured (>200 widget.update frames/sec across pinned widgets), upgrade the hot path to msgpack (binary framing) — schema unchanged, only the serializer swaps. Length-prefixed JSON parser caps frame at 256 KB (see envelope cap above).

### 10.8 Security model

| Surface | Control |
|---|---|
| **Schema validation** | Every `render_widget` runs `Validate(props)` before any I/O. Failure → no tile, single `widget.error`; LLM gets `tool_error` it can recover from. |
| **Type allowlist** | Only types present in `widget-registry.json` with `enabled=true` execute. Unknown type → reject. |
| **URL handling** | `image` and `citation` URLs dereferenced by daemon (`internal/web/`), never by LLM. Daemon enforces: `https://` only, max body 10 MB, MIME allowlist for `image` (`image/png`, `image/jpeg`, `image/webp`, `image/gif`), 5 s connect timeout, ≤3 redirects, no private IP ranges (RFC1918, link-local, loopback). |
| **Action callbacks** | `actions[].callback` must match `^leah://action/[a-z_]+`. Daemon maintains allowlist of verbs (`pin`, `refresh`, `dismiss`, `book`, `change_range`, `open_url`, …); unknown verbs dropped at registry-load. `open_url` requires the URL be in a tile already rendered. |
| **Per-widget disable** | Settings → Widgets → toggle off flips `enabled=false`; daemon hot-reloads via fsnotify. Disabled type → LLM call rejected with explanatory `tool_error` so the LLM can fall back to prose. |
| **Adapter sandboxing** | Each adapter runs inside the daemon process with its own `context.WithTimeout` (default 5 s fetch, 3 s refresh). No adapter shells out. Network adapters share `internal/web/` httpclient (TLS verify on; no proxies unless `LEAH_HTTP_PROXY` set). |
| **Pure-LLM caps** | `code.source` ≤16 KB; `table.rows` ≤200; `list.items` ≤50 to bound render cost. |
| **Telemetry** | Every validate/fetch/error emits to `internal/obs/` (`widget.validate`, `widget.fetch.duration_ms`, `widget.fetch.error`, `widget.cache.hit`). |

### 10.9 Extensibility

Adding a new widget type `foo` is three additive PRs (file-disjoint, parallelizable):

1. **Schema** — `internal/widget/schemas/foo.json` (draft-07); register `leah://widget/foo/1`.
2. **Adapter** — `internal/foo/adapter.go` implementing `WidgetAdapter`; **lazy-register** (perf #4): the adapter type is listed in registry at boot, but `Adapter.init()` runs on first `render_widget` for that type. Boot mounts ONLY panel + ambient adapters — `widget.builtins` is now `widget.Factory` map of `string → func() WidgetAdapter`, not pre-instantiated structs. If pure-LLM, use `PureAdapter{"foo"}` factory closure.
3. **UI component** — `FooTile` conforming to panel tile protocol (§ 10.0 chrome contract); registered in UI-side type map; **shared SwiftUI subview protocol** for chrome (eyebrow, hairline, pin/dismiss cluster, error state) so per-widget RSS is data + 1 struct, not 1 SwiftUI module (perf #19).

**v1 has no plugin system.** Third-party widgets are out of scope — registry is closed-set and shipped in the daemon binary. Operator-toggleable, not operator-extensible. Reasoning: review surface + security per widget; one operator does not need plugin breadth.

To deprecate a widget: set `enabled=false` in default registry, ship a migration that drops it from operator's registry on next launch.

Breaking schema change: bump `protocol_version` to `2`; ship a migrator that translates v1 pinned widgets → v2 (or drops with operator notification). The daemon refuses to start against a mismatched registry version without operator confirmation.

---

## 11. Accessibility + responsiveness

### 11.1 Contrast (WCAG AA = 4.5:1 text, 3:1 UI) — v2 recomputed

All ratios computed via WCAG 2.1 relative-luminance formula, committed alongside spec as `scripts/contrast-check.py` for CI regression. v1's table had **5/8 wrong values**; v2 corrects and tightens tokens.

**Dark mode**

| Pair | Ratio | Pass |
|---|---|---|
| `--text-primary` `#F2EDE0` on `--obsidian-0` `#08090C` | **17.03:1** | AAA |
| `--text-muted` `#B8B0A0` on `--obsidian-0` | **9.25:1** | AAA |
| `--text-dim` `#8A8478` on `--obsidian-0` (v2: was `#6B6558` 3.44:1) | **5.36:1** | AA |
| `--gold-primary` `#C9A961` on `--obsidian-0` | **8.85:1** | AAA |
| `--gold-primary` text on `--obsidian-2` `#161922` | **7.80:1** | AAA |
| `--gold-muted` `#8A7340` on `--obsidian-0` | **4.37:1** | AA large-text only (≥18 pt) — never small body |
| `--red-alert` `#D75A66` on `--obsidian-0` (v2: was `#C8434F` 4.14:1) | **5.26:1** | AA — always paired with `●` icon prefix |
| `--red-brand` `#7A1F2B` on `--obsidian-0` | **1.95:1** | UI-only (brand-mark outlines on red bg; never text) |
| `--focus-ring` `#E8CC8C` on `--obsidian-0` | **12.75:1** | AAA |

**Light mode**

| Pair | Ratio | Pass |
|---|---|---|
| `--text-primary-light` `#1E1D1A` on `--bone-0` `#F2EFE8` | **14.68:1** | AAA |
| `--text-muted-light` `#3D3A33` on `--bone-0` | **9.88:1** | AAA |
| `--text-dim-light` `#6B6558` on `--bone-0` | **5.04:1** | AA |
| `--gold-primary-light` `#7A6332` on `--bone-0` | **5.00:1** | AA |
| `--red-alert-light` `#A8323D` on `--bone-0` (always paired with `●`) | **5.74:1** | AA |
| `--red-brand-light` `#5C1620` on `--bone-0` | **11.50:1** | AAA |
| `--focus-ring-light` `#5C4A20` on `--bone-0` | **6.60:1** | AA |

**Color-not-alone (WCAG 1.4.1):** every semantic-color use is paired with an icon prefix:
- `●` filled circle → alert / error (red-alert + red-alert-light)
- `◆` filled diamond → brand-mark critical state (red-brand)
- `▾` filled triangle → active sort (gold)
- `▲ / ▼` triangles → positive / negative delta

Deuteranopia / protanomaly safety: gold + oxblood collapse under protanomaly per craft-HIG #2; pairing with the icon prefixes above keeps meaning recoverable. Verified via Sim Daltonism (committed test fixture in `tests/contrast/colorblind.png`).

### 11.2 Reduced motion

`prefers-reduced-motion: reduce` triggers:
- All durations → 0 by default (true synchronous swap per HIG); only state-changes that would pop confusingly fall back to `--dur-instant` 80 ms crossfade (perf MEDIUM #38).
- Listening pulse → static ring at 30% opacity (amplitude already clamped 30–60%, frequency drops to 0.5 Hz under reduced-motion — UX #32).
- Thinking arc → static dashed perimeter at `--gold-muted`.
- Speaking waveform → single shimmer bar (10 Hz cadence already; cadence-cap rule in § 5.3 still applies).
- Transition 1 (Gold Transition) → cross-fade 160 ms; no seam expand, no panel unfold.
- Transition 2 (Mark Acknowledgment) → opacity fade only (NOT color-shift — color-flash on rapid wake-events can trigger photosensitive responses; UX #31). Rate-cap ≤1 state-change per 500 ms still enforced.
- **Per-widget loading sweeps suppressed (v3 — UX #33):** the flights "gold transition sweeping left-to-right at 240 ms across rows" and similar row-by-row reveal patterns become **static "Loading…" text only**. Discrete row-by-row motion is more attention-grabbing than continuous and is NOT addressed by zero-duration alone.
- **Frame-count parity check (v3 — perf #37):** all sub-200 ms durations are specced in both ms AND target frames per common displays (60 Hz / 120 Hz ProMotion) in test-plan snapshot tables — `--dur-instant` 80 ms = 4.8 frames @ 60 Hz / 9.6 frames @ 120 Hz; verified on both at CI.

### 11.3 VoiceOver labels

Every interactive element has an explicit accessibility label. Labels reflect v3 hotkey (`⌥Space`) and current state.

- Mark (ambient): `"Leah, status: {state}. Press Option Space to ask."`
- Menubar item: `"Leah — {state}. Activate to open Leah menu."` (state announced on shape change: idle/listening/error — UX #11)
- Input field: `"Ask Leah. Type your question or hold Function to dictate."` (Fn = PTT key v3)
- Send: `"Send query."`
- Source link: `"Source 1 of 3, {domain}. Open in browser."`
- Follow-up chip: `"Follow up: {chip text}. Press Return to insert."`
- Pin / dismiss glyphs: `"Pin tile to ambient."` / `"Dismiss tile."` (24 × 24 hit-target wraps 12 px glyph; labels surface even if glyph invisible)
- Widget tile (per type): `"{Type} widget. {Eyebrow}. {summary}. Press P to pin, X to dismiss, Space to {primary action}, Cmd+C to copy."`
- Toast: `"Notification: {text}. Press Return to expand, Escape to dismiss."`
- Widget gallery `+` button: `"Open widget gallery. {N} widgets available."`
- Quick-spawn chip: `"Spawn {widget eyebrow}."`
- Daemon-down inline ghost-panel: `"Daemon offline. Click to restart Leah."` (Nielsen #27 fix — visible+audible feedback where operator is looking)

**Streaming-response announce strategy (v3 — UX):** during LLM streaming, VoiceOver receives `aria-live="polite"` updates batched at sentence boundaries (NOT per token — would saturate the screen-reader queue). On `prose.end`, the full response is re-announced once at `polite` priority for catch-up.

VoiceOver rotor groups: "Mark", "Input", "Response", "Widgets", "Sources", "Follow-ups", "Actions", "Pinned widgets" (ambient HUD).

### 11.4 Window scaling

| Mode | HUD | Focus panel |
|---|---|---|
| Mini | 56 × 56 px mark only | 480 × 360 px |
| Standard | 280 × 84 px | 860 × 480 px (v3 default; was 720) |
| Expanded | 280 × 168 px (+agenda strip) | 960 × 640 px |

Operator-set in Settings. Auto-mini if screen < 1440 px wide.

**Dynamic Type reflow (v3 — UX #36):** at OS text-size ≥ 130 %, HUD drops the Pulse row first; ≥ 150 % collapses HUD to Mini (mark only). Panel grows vertically with text-size; **no horizontal scroll at 200 % zoom** — at 200 %, panel expands to 960 × 720 px or to display bounds, whichever is smaller. WCAG 1.4.10 (reflow) verified in `tests/a11y/zoom_test.swift`. **Panel resize hysteresis (v3 — perf #23):** drag-resize debounces 2-column-grid layout switch to a **850–870 px hysteresis band** (not every-pixel) to avoid 6-12 ms/frame layout-pass stutter on M1 Air.

### 11.5 Multi-monitor / Retina

- Each monitor remembers its own HUD anchor + scale mode.
- Retina: SVG mark (vector). PNG hero only for wizard, served @1x/@2x/@3x.
- Non-retina: grain overlay disabled.

---

## 12. States — empty, error, offline, degraded

| State | Treatment |
|---|---|
| **Empty (panel)** | Mark + "Good morning, Tri." + `⌃ Try` (3 starter chips) + `⌃ Spawn` (up to 3 quick-spawn widget chips). |
| **Empty (ambient row 2)** | Hidden when no "Now" item — no placeholder text. |
| **Empty (widget tile)** | Per-widget **body sans small-caps tracking +0.04em** in dim ivory (e.g., `MARKET CLOSED · opens 9:30am ET`) — v3.1 sweep, per decision #28. |
| **Streaming** | Input dims to `--text-muted`; thinking ring on mark; `⌘.` chip appears top-right. |
| **Error — model failed** | Red hairline-bottom on response area; `--red-alert` caption: "Model couldn't respond. [Retry]". |
| **Error — widget fetch** | Tile frame oxblood @ 60%; `COULDN'T LOAD` eyebrow + [Retry] chip. |
| **Offline — no network** | Soft red banner at top of panel: "Offline. Local answers only." HUD mark shows obsidian-3 hexagon (no gold). |
| **Daemon down** | Panel refuses to summon; menubar dot pulses `--red-alert`; clicking shows: "Daemon offline. [Restart Leah]". |
| **Permission denied** | First time: gentle Settings deep-link sheet. Subsequent: small inline `--red-dim` chip "Mic blocked → System Settings". |
| **Sensitive content detected** | Blur scrim with `--red-dim` tint; "Sensitive content hidden. [Show]". |
| **Degraded — stale widget** | Cached value + oxblood underline + relative-age caption. |
| **Degraded — blur (NEW v2)** | Triggered when NSWindow occlusion sampling detects panel overlaps continuously-redrawing content (Final Cut timeline, video player, terminal scrollback >10 Hz). NSVisualEffectView swaps to solid `--blur-tint` fill (no live blur); hairline opacity bumps from 20% to 28% to compensate; switch hysteresis 3 s. No operator-visible chrome announces the swap — silent. Restore: occlusion sampling reports stable content for 3 s → blur fades back over `--dur-standard`. (Perf #15 — recovers ~6-12 ms/frame on integrated GPU when overlapping busy content.) |
| **Degraded — Reduce Transparency** | When `accessibilityDisplayShouldReduceTransparency == true`: tint becomes fully opaque (`--obsidian-0` solid on dark, `--bone-0` solid on light); grain disabled; hairline bumps to 28% opacity. **Grain is governed by its own user-pref toggle AND by Reduce Transparency** (v3 — UI craft #3 grain-vs-Reduce-Transparency clarification: grain is NOT vibrancy and Apple's "Reduce Transparency" doesn't directly affect noise overlays, but under the solid-fill fallback the grain rule still suppresses to avoid amplifying flat-field banding). Mark + listening pulse unchanged. |
| **Degraded — Increase Contrast** (v3 — UI craft "Increase Contrast system setting") | When `accessibilityShouldIncreaseContrast == true`: hairline bumps from 20% to 40% white; `--text-muted` and `--text-dim` collapse to `--text-primary`; gold accents gain a 1 px ivory inner stroke. Mark retains gold but gains a 1 px ivory ring. |
| **System idle ≥ 10 min** (v3 — perf MEDIUM "animation halt when idle") | All decorative animations (listening pulse, thinking arc, speaking waveform, mark pulses, pinned-widget value-delta animations) halt. State changes become instant swaps. Resumes on wake / user-input event. Recovers idle CPU. |
| **Low Power Mode** (v3 — perf MEDIUM "Power Mode awareness") | `ProcessInfo.processInfo.isLowPowerModeEnabled == true`: wake-word model unloads (already in § 6.7); pinned-widget refresh paused (already in § 10.3); animations downgrade to instant-state-change; grain auto-drops. Menubar exposes `LOW POWER · refresh paused` in the dropdown. |
| **Memory pressure** (v3 — perf MEDIUM "drop cache aggressive") | `os_memory_pressure` reports warning/critical: widget-cache LRU evicts to 25 % of cap (12.5 MB); pinned-widget refresh quiesces; panel response-history truncates to last 24 h. Toast: `Memory pressure — caches trimmed.` |
| **Screen-recording end → HUD restore** (v3 — UX #34 LOW elevated) | `SCShareableContent` stream-end observer fires → menubar pulses idle once → HUD fades back over `--dur-standard` from `accessibilityDisplayShouldReduceTransparency`-aware starting opacity. While hidden, menubar dropdown shows `Leah hidden — recording detected · [Show anyway]` so operator never thinks Leah crashed. |

---

## 13. ASCII wireframes

### 13.1 Ambient HUD (standard mode)

```
                          ┌──────────────────────────────────────────┐
                          │ ⬡  Listening…                            │
                          │ ──────────────────────────────────────── │
                          │ Standup in 12m · 9:30am                  │
                          │ ──────────────────────────────────────── │
                          │ ◇ 5 PRs                                  │   ← v3.1: ONE primary metric
                          └──────────────────────────────────────────┘    (hover rotates secondary;
                                       bottom-right of screen              decision #66 wireframe lag fix)
```

### 13.2 Ambient HUD (mini mode)

```
                                              ┌──────┐
                                              │  ⬡   │
                                              └──────┘
```

### 13.3 Focus panel (streaming response with widget interleaved)

```
┌───────────────────────────────────────────────────────────────────┐
│  ⌥Space                                                           │  ← v3.1: hotkey chrome
│   ⬡  What's the status of MAY-19?                     [+]  ⌘.    │
│      ↑ mark/state                widget gallery button ↑          │
│   ──────────────────────────────────────────────────────────────  │
│                                                                   │
│   MAY-19 B1 and B5 shipped 2026-06-21 — server-pushed widget     │
│   refresh is live in PR #321. Remaining work: B2 (toast queue    │
│   coalescing) is unblocked; B3-B4 await design review¹.          │
│                                                                   │
│   ▎ The dispatch cap is 6 file-disjoint PRs².                    │
│                                                                   │
│   ──────────────────────────────────────────────────────────────  │
│   Sources                                                         │
│   1. linear.app/MAY-19 — Server-pushed widget refresh            │
│   2. CLAUDE.md — Dispatch parallelism rules                      │
│   ──────────────────────────────────────────────────────────────  │
│   ⌃ Follow up                                                     │
│   [ Open PR #321 ]  [ Show B2 spec ]  [ Brief me on MAY-20 ]    │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
                            screen-center
```

### 13.4 Focus panel (empty / first open) — v3.1 adds ⌥Space chrome + ⌘/ footer

```
┌───────────────────────────────────────────────────────────────────┐
│  ⌥Space                                                           │  ← v3.1: hotkey chrome
│   ⬡  Ask Leah anything…                                  [+]     │
│   ──────────────────────────────────────────────────────────────  │
│                            ⬡                                      │
│                  Good morning, Tri.                               │   ← fixed greeting per
│                                                                   │     time-of-day (§7.1)
│   ⌃ Try                                                           │
│   [ What's new today? ]  [ Open my brief ]  [ Status of MAY-19 ] │
│                                                                   │
│   ⌃ Spawn                                                         │
│   [ MARKET · TODAY ]  [ CALENDAR · THIS WEEK ]  [ PRS · 7d ]    │
│   ──────────────────────────────────────────────────────────────  │
│   Press ⌘/ for shortcuts                                          │  ← v3.1: ⌘/ help footer
└───────────────────────────────────────────────────────────────────┘
```

### 13.4-light Focus panel (light mode mirror — same anatomy)

```
┌───────────────────────────────────────────────────────────────────┐
│  ⌥Space                                                           │  ← v3.1: hotkey chrome
│  ╲ ╲ ╲ ╲ ╲ ╲ bone bg (#F2EFE8) — graphite text  ╲ ╲ ╲ ╲ ╲ ╲ ╲ ╲ │
│   ⬡  Ask Leah anything…                                  [+]     │
│   ────────────────────────  (1px @ 12% black hairline)  ──────   │
│                            ⬡                                      │
│                  Good morning, Tri.                               │
│                            ↑ graphite #1E1D1A                     │
│                                                                   │
│   ⌃ Try                                                           │
│   [ What's new today? ]  [ Open my brief ]  [ Status of MAY-19 ] │
│   ↑ darkened-champagne #7A6332 hairline · graphite text          │
│                                                                   │
│   ⌃ Spawn                                                         │
│   [ MARKET · TODAY ]  [ CALENDAR · THIS WEEK ]  [ PRS · 7d ]    │
└───────────────────────────────────────────────────────────────────┘
```

Same anatomy as § 13.4; only the palette swaps. Mark L is `--gold-primary-light` `#7A6332`; panel tint `--bone-0E6`; hairlines `--divider-light` 12% black. Focus ring `--focus-ring-light` `#5C4A20`. Auto-loaded when `NSApp.effectiveAppearance == .aqua` (or operator override in Settings → Appearance → Light).

### 13.5 Hotkey-summon transition (Transition 1 — Gold Transition)

```
Frame 1 (t=0ms)              Frame 2 (t=120ms)            Frame 3 (t=380ms)
                                                          ┌──────────────┐
                                                          │              │
       (nothing)         ────────────────────────         │   panel    │
                                                          │   settles    │
                                                          └──────────────┘
                          gold transition expands              vault opens
```

### 13.6 Menubar dropdown

```
                    [⬡] menubar icon ← filled-hex (template) when listening; outlined when idle
                              ▼
                    ┌──────────────────────┐
                    │ ⬡  Leah — Listening  │
                    │ ──────────────────── │
                    │ Open                 │
                    │ Dashboard         ⌘⇧D│
                    │ ──────────────────── │
                    │ Settings           ⌘,│
                    │ Re-run setup         │
                    │ ──────────────────── │
                    │ Hide HUD          ⌘⌥H│
                    │ Quit Leah          ⌘Q│
                    └──────────────────────┘
```

### 13.7 Notification widget (stacked above HUD)

```
                          ┌──────────────────────────────────────────┐
                          │ ⬡  Brief ready: MAY-19 ship report      │
                          └──────────────────────────────────────────┘
                          ┌──────────────────────────────────────────┐
                          │ ⬡  arxiv: new paper on memory routing   │
                          └──────────────────────────────────────────┘
                          ┌──────────────────────────────────────────┐
                          │ ⬡  Listening…                            │  ← HUD
                          │ Standup in 12m · 9:30am                 │
                          │ ◇ 5 PRs                                  │
                          └──────────────────────────────────────────┘
```

### 13.8 Ambient HUD with 2 pinned widgets (v3.1 — was "3 pinned"; cap is 2 per decision #40)

```
                          ┌──────────────────────────────────────────┐
                          │ ⬡  Listening…                            │
                          │ Standup in 12m · 9:30am                  │
                          │ ◇ 5 PRs                                  │
                          │ ════════════════════════════════════════ │
                          │ MARKET                              ◆    │
                          │ AAPL  228.43  ▲0.80%   ╱╲___╱‾‾‾        │
                          │ ──────────────────────────────────────── │
                          │ PRS MERGED · 7d                     ◆    │
                          │ 52   ▲ +14 vs prior week                │
                          └──────────────────────────────────────────┘
```

### 13.9 Settings — General (with HUD anchor grid)

```
┌─────────────────────────────────────────────────────────────────┐
│ [🔍 Search settings…]   │  General                              │
│ ────────────────────────│  ──────────────────────────────────── │
│ ● General               │   Hotkey                              │
│ ○ Voice                 │   ┌──────────────┐                    │
│ ○ Appearance            │   │   ⌥  Space   │  [ Re-record ]    │
│ ○ Privacy               │   └──────────────┘                    │
│ ○ Permissions           │                                       │
│ ○ Integrations          │   Ambient HUD position                │
│ ○ Widgets               │   ┌────┐ ┌────┐ ┌────┐               │
│ ○ Memory                │   │ TL │ │ T  │ │ TR │               │
│ ○ About                 │   └────┘ └────┘ └────┘               │
│                         │   ┌────┐         ┌────┐               │
│                         │   │ L  │   ⬡    │ R  │  ← preview     │
│                         │   └────┘         └────┘               │
│                         │   ┌────┐ ┌────┐ ┌────┐               │
│                         │   │ BL │ │ B  │ │●BR │  ← current     │
│                         │   └────┘ └────┘ └────┘               │
│                         │   Scale  ◯ Mini  ●Standard  ◯Expanded │
│                         │   ☑ Hide during recording             │
│                         │   ☐ Always on top                     │
│                         │                                       │
│                         │   Dedicated display when fullscreens  │
│                         │   ☐  Use secondary monitor            │
│                         │       ▾ [auto-detect / Display 2]     │
└─────────────────────────────────────────────────────────────────┘
```

### 13.10 Widget gallery overlay

(See § 10.4 wireframe.)

### 13.11 First-launch wizard steps 1–5

(See § 8.2–8.6 wireframes.)

### 13.12 Settings — Permissions (status glyph legend in action)

```
┌────────────────────────────────────────────────────────────────────┐
│ [🔍 Search settings…]   │  Permissions                             │
│ ────────────────────────│  ──────────────────────────────────────  │
│ ● Permissions           │  ● Microphone — Granted                  │
│                         │  ◐ Accessibility — [Grant in Settings →] │
│                         │  ○ Screen recording — Not asked yet      │
│                         │  ✕ Automation: Calendar — Denied         │
│                         │       [ Open System Settings → ]         │
│                         │  ○ Automation: Mail            [ Ask ]   │
│                         │  ○ Contacts                    [ Ask ]   │
│                         │  ○ Reminders                   [ Ask ]   │
│                         │  ○ Notifications               [ Ask ]   │
│                         │  ○ Focus filter integration    [ Ask ]   │
│                         │  ○ Full Disk Access            [ Ask ]   │
└────────────────────────────────────────────────────────────────────┘
```

### 13.13 Memory purge confirmation

(See § 9.5 wireframe.)

### 13.14 Marketing hero composition (NEW v3.1 — first-impression #5 "killer screenshot")

**The single screenshot that sells this product.**

```
┌──────────────────────────────────────────────────────────────────────┐
│  Finder window at 0.6 opacity                                        │
│  ────────────────────────────────────────────────────────────────────│
│  Slack window at 0.6 opacity                                         │
│  ────────────────────────────────────────────────────────────────────│
│  VS Code window at 0.6 opacity (visible behind blur)                 │
│                                                                      │
│                  ┌──────────────────────────────────────┐            │
│                  │  ⬡  MAY-19 status?            [+]    │            │
│                  │  ──────────────────────────────────  │            │
│                  │                                      │            │
│                  │  MAY-19 shipped at 4:47pm —          │            │
│                  │  PR #321 merged by Tri, reviewed     │            │
│                  │  in 8 minutes; B2 is unblocked.      │            │
│                  │                                      │            │
│                  │  ┌────────────────────────────────┐  │            │
│                  │  │ PRS MERGED · 7d                │  │            │
│                  │  │   52   ▲ +14 vs prior week     │  │            │
│                  │  └────────────────────────────────┘  │            │
│                  │                                      │            │
│                  │  Press ⌘/ for shortcuts              │            │
│                  └──────────────────────────────────────┘            │
│                              focus panel mid-flow                    │
└──────────────────────────────────────────────────────────────────────┘
```

**Composition rules (binding for any marketing render):**

| Element | Rule |
|---|---|
| **Backdrop** | Real macOS desktop with **three** apps visible at 0.6 opacity behind the panel's blur (Finder + Slack + VS Code in the canonical hero). NOT a black background — the blur sampling the desktop is the brand. |
| **Panel content** | **One** widget tile + ivory prose. Not three. Not zero. |
| **Gold instances visible** | **Maximum 3** total across the entire frame (panel hairline + L mark + one widget accent). The gold-budget invariant from §10.0 enforces this mechanically. |
| **Contextual answer** | Something only Leah could know — references a real artifact (PR number, ticket, code path), recent timestamp, the operator's name. Generic answers ("Here's how to write a for-loop") kill the hero. |
| **Mark + L** | Visible in the panel input row; the brand signature in a single 24 px gold-on-obsidian. |
| **Hotkey chrome** | ⌥Space glyph visible (subtle chrome dim) so the screenshot teaches the summon mechanic. |
| **Help footer** | ⌘/ shortcut line at panel footer — surfaces the discoverability story. |
| **NOT in frame** | No oxblood (alert state); no settings; no toast; no second widget; no scrollbar; no error state. Every accidental element competes with the answer. |

The killer screenshot sells: ambient ubiquity + contextual omniscience + restrained beauty in one frame. Every §13 wireframe must be composable into this hero — if a section can't, that section needs a redraw, not the hero.

### 13.15 Wizard — Paste your Anthropic API key (NEW v3.2.1 — BYOK step)

**Inserts between §8.2 Welcome and §8.3 Hotkey + Accessibility** (wizard becomes 6 steps; step-dot count updates accordingly). Implementation lives under `internal/wizard/api_key/`. Backed by §17.18.

```
┌──────────────────────────────────────────────────────────────────┐
│  ●  ●  ○  ○  ○  ○                                       step 2/6│
│  ──────────────────────────────────────────────────────────────  │
│   Connect Leah to Anthropic.                                     │
│   Leah uses your own Anthropic API key — bring-your-own.         │
│   ──────────────────────────────────────────────────────────────  │
│   ┌──────────────────────────────────────────────────────────┐   │
│   │  sk-ant-•••••••••••••••••••••••••••••     [👁 show]      │   │
│   └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│   We send 1 token to verify, then store it in macOS Keychain.    │
│   Your key never leaves your machine except to call Anthropic.   │
│                                                                  │
│   [ How do I get a key? → console.anthropic.com/settings/keys ]  │
│   ──────────────────────────────────────────────────────────────  │
│   ☐  My workspace has Zero Data Retention enabled                │
│      (recommended · console.anthropic.com → Privacy)             │
│   ──────────────────────────────────────────────────────────────  │
│                          [ Paste from clipboard ]                │
│                                       [ Back ]    [ Verify ]     │
└──────────────────────────────────────────────────────────────────┘
```

**Verify state (after Continue press):**

```
   ┌──────────────────────────────────────────────────────────┐
   │  ◐  Pinging Anthropic…   (≤ 2 s)                          │
   └──────────────────────────────────────────────────────────┘
```

**Success state:**

```
   ┌──────────────────────────────────────────────────────────┐
   │  ●  Connected — workspace "tri-personal"                  │
   │     Stored in Keychain · Settings → API Keys to rotate    │
   └──────────────────────────────────────────────────────────┘
   [ Continue ] is now enabled (gold-primary).
```

**Failure state (401 / 403 / network):**

```
   ┌──────────────────────────────────────────────────────────┐
   │  ✕  Key rejected: 401 invalid_api_key                     │
   │     Check console.anthropic.com/settings/keys             │
   │                                       [ Try again ]       │
   └──────────────────────────────────────────────────────────┘
```

**Rules:**
- Paste field uses `SecureField` (SwiftUI) — obscured by default; toggle reveals.
- Verify-ping is a 1-token `messages.create` request via daemon (the HUD process never sees the key); daemon writes to Keychain on success ONLY.
- ZDR checkbox is informational + nudge — does not block continue (operator may toggle it later in the Anthropic console). The label links out.
- Esc on this step → standard "Quit setup?" sheet per §8.
- Skip is NOT offered (the rest of Leah is non-functional without a key — there's no honest skip path).
- Existing key on re-run: shows `●  Already connected — workspace "tri-personal"  [ Replace key ]` and skip-forward is enabled.

---

## 14. Open decisions log

| # | Decision | Rationale |
|---|---|---|
| 1 | Killed multi-conversation tabs. Chamber is one stream; history lives in dashboard. | Tab chrome competes with the response; operator-overlay aesthetic forbids browser-like UI. |
| 2 | **Wake-word OFF by default** (v2 operator override — supersedes v1's ON). | v1 prior text: "ON, pre-checked." v2 reverses: privacy posture is the #1 trust signal at launch (Nielsen #9); perf #1 documents ~1-5% sustained CPU + 80MB RAM + 1Wh/day battery; workflow #2 documents false-trigger on common English bigrams. Opt-in in Settings → Voice with honest battery copy. Wake-word implementation remains local-only (no cloud audio). |
| 3 | Italic serif L in the sigil (not a sans-serif glyph). | Distinctiveness at 100 px. Sans L is generic; italic serif is a wax-seal moment. |
| 4 | Hexagon, not circle, for sigil container. | Circle = Siri/Cortana. Hexagon = mechanical, watch-crown lineage, operator-tool. |
| 5 | **`⌥Space` (Option+Space)** (v2 operator override — supersedes v1's `⌘⌃` modifier-only chord, which itself superseded the original `⌘⇧Space`). | Operator decision. Single modifier + letter; fires on keydown — no 250 ms disambiguation latency floor (perf #5). Avoids Rectangle/Magnet/Mission-Control chord collisions (workflow #1) and Sticky Keys issues. v1 chord-display copy ("Press and release ⌘⌃") obsoleted; keydown-trigger semantics are now standard. |
| 6 | Three easing curves only; doctrinal durations. | Motion proliferation is the #1 luxury-violation. Three curves enforce restraint. |
| 8 | Auto-hide HUD during screen recording, ON by default. | Privacy default — shipping OFF would create demo-moment leaks. |
| 9 | No green chrome anywhere; success is a one-time quiet tick. | Palette is obsidian / gold / red / ivory. Green pollutes. Confirmation feels like a printed receipt mark. |
| 10 | **Chamber takes key window on summon, returns key on dismiss** (Spotlight pattern). v2 reversal of v1's "no-steal-until-type" claim. | The original claim was AppKit-impossible: `.nonactivatingPanel` cannot host a first-responder text field AND defer key events. Ambient HUD remains nonactivating; only the chamber activates. Reviewer fix: craft-HIG #3. |
| 11 | **5 wizard steps, not 6.** | Folded HUD-position into Settings → General. Saves a step with no wrong answer. |
| 12 | **Wake-word opt-in lives below the mic prompt (NOT pre-checked).** | Co-located because wake-word requires mic; default OFF because the cost is real (perf #1) and the consent must be active (Nielsen #9). |
| 13 | **ONE integration in wizard.** | Wizard-length anxiety is the #1 abandonment cause. 3 radio cards → 1 click. Others self-discover via usage. |
| 14 | **Calendar pre-selected.** | Highest-frequency value: ambient HUD's "Now" slot needs an agenda source within 60 s of finishing wizard. |
| 15 | **Permissions accumulate (lazy-prompt).** | Operator decision. Avoids "give me 9 permissions upfront" Mac-app-store smell. |
| 16 | Settings search via Things-pattern (sidebar AND row labels). | Highest-rated macOS-app settings UX; multiple App Store reviews single it out. |
| 17 | Live preview pane ONLY on Appearance. | Voice / Privacy / Permissions toggles change behavior, not visuals. A preview would be theater. |
| 18 | Destructive memory-purge requires typed "PURGE". | Two friction points (count + typed word) = effectively unaccidentable. |
| 19 | Status glyphs unified: ● green, ◐ gold, ○ open, ✕ red. | One legend across Permissions + Integrations + Daemon status. Color-blind safe (shape-distinguishable). |
| 20 | "Skip" is equal-weight, not a dim text-link. | Notion's onboarding inflates abandonment with dim Skip links; Linear-new uses equal weight. |
| 21 | **Fullscreen → corner-orb default; secondary monitor opt-in.** | Operator override. Default preserves single-screen muscle memory; opt-in serves multi-monitor power users without making them the path of least resistance. |
| 22 | Closed widget registry, no plugins, v1. | Review surface bounded; one operator does not need third-party extensibility; protocol can evolve without semver-locking plugins. |
| 23 | Pure-LLM widgets (table/code/stat/list/diff/most chart) skip adapters. | Cuts fetch latency to zero; avoids adapter-per-shape sprawl. Adapters exist only where an external data source is the truth. |
| 24 | One IPC channel with `seq` numbers; cached-last-good with oxblood frame. | Interleaving is the product requirement; ordering across channels is a known foot-gun. Degradation is visible, not silent. |
| 25 | **Light + dark mode parity (v2 operator override).** | HIG mandate ("Support both appearances. People expect most apps to honor the system appearance setting"). v1's dark-only justification was designer preference, not user need. § 2.6 specifies full light palette with all tokens recomputed for AA pass; § 13.4-light wireframes the mirror. |
| 26 | **Gold is brand-mark only; all other tint = NSColor.controlAccentColor (v2).** | HIG fix (craft-HIG #1): macOS users set system accent in System Settings; hardcoding gold ignored that preference. Gold remains on sigil, focus-chamber border, primary CTA, divider seams, brand chrome inside widget tiles. Selection / hover / focus rings on text fields / slider tracks / links honor system accent. Settings → Appearance "Accent intensity" slider removed (the right place for accent is System Settings). |
| 27 | **Hairline 1 px @ 20% baseline; 1.5 px @ 28% on non-Retina (v2).** | Reviewer craft-HIG: 8% white @ 0.5 pt was sub-pixel-invisible on 1× displays. New rule auto-adapts via `backingScaleFactor`. Drops the user-facing "Hairline opacity slider" — wrong knob; operators won't know default is invisible on their second monitor. |
| 28 | **Tiempos italic ONE location only — Dashboard "Today" header (v2 enforcement).** | v1 declared "one place" then sprinkled italic eyebrows across every widget, gallery category, empty-state, code-language label. Reviewer craft-HIG #4 + workflow #6 + perf #3 all flagged the drift. v2: strip from all other locations; replace with body sans small-caps tracking +80. The editorial moment must be rare to be felt. |
| 29 | **Dynamic Type via NSFont.preferredFont semantic tokens (v2).** | HIG mandate (craft-HIG): hardcoded pt fails for ~7% of users who bump system text. Only fixed-pt exception: telemetry / numeric readouts (JetBrains Mono) — alignment-load-bearing. Ambient HUD drops Pulse row at 130%, collapses to Mini at 150%. |
| 30 | **State paths under `~/Library/Application Support/Leah/`; cache under `~/Library/Caches/com.leah.daemon/` (v2).** | HIG-review on Apple convention. v1's `~/.leah-state/` was Unix-y dotfile; breaks Time Machine, Migration Assistant, "Clean Cache" tools. Cache under Caches makes it system-purgeable on low disk. |
| 31 | **`SCShareableContent` (ScreenCaptureKit) replaces deprecated `CGScreenIsCaptured()` (v2).** | Deprecated since macOS 12. Push-based stream observer (not poll) — also removes the 2 s poll cost (perf #34). |
| 32 | **Recomputed contrast table; CI script enforces (v2).** | Nielsen #1 — v1 had 5/8 wrong values. New tokens: `--text-dim #8A8478` (5.36:1, was 3.44:1), `--red-alert #D75A66` (5.26:1, was 4.14:1). `scripts/contrast-check.py` committed; regressions fail build (§ 16.1). |
| 33 | **`--focus-ring` token defined (v2).** | Nielsen WCAG 2.4.7 — v1 had no focus-ring spec. New: 2 px gold-glow @ 80% opacity, 2 px offset. Light counterpart `--focus-ring-light` `#5C4A20`. Never the only meaning signal. |
| 34 | **24×24 hit-targets for pin/dismiss + all glyph controls (v2).** | WCAG 2.2 2.5.8 mandatory. Visual glyph stays 12 px; hit-area wraps to 24×24. |
| 35 | **`Esc` and `⌘.` documented as distinct verbs (v2).** | Workflow #4 — v1's ambiguity caused token waste. Esc = dismiss UI only (free); `⌘.` = cancel backend call (saves money). Both in `⌘/` cheatsheet. |
| 36 | **Chamber idle = shrink to ambient pill, NOT destroy (v2).** | Workflow #3 — v1's 90 s auto-dismiss killed work-in-progress reads. New: ≥ 5 min idle shrinks to pill at last position; conversation preserved 24 h. |
| 37 | **Chamber-resident `+` button for widget gallery (v2).** | Workflow #5 — `/widgets` and `⌘⇧W` are forgotten by week 2. Discoverable affordance every session. |
| 38 | **Toast stack cap 2 visible + "+N more" expandable (v2).** | Workflow #6 — corner-real-estate accounting. 3 stacked toasts + 3 pinned widgets blew bottom-right column past 60% of 13" MBP height. |
| 39 | **[SUPERSEDED by #112 — gold-budget visible-surface cap]** Original v2 text retained for history: max widget tiles per turn 2 (down from 4); gold-budget 3 visible/chamber-render. Per-surface cap (#112) is now the binding rule. | Workflow #8 + craft-HIG #5 — 4 tiles forced chamber scroll; 30+ simultaneous gold instances broke 'earn every pixel' thesis. v3.2: per-surface cap (#112) supersedes per-chamber-render cap. |
| 40 | **Pinned widgets max 2 (down from 3) (v2).** | Workflow #6 corner-accounting. Two pins is enough; third is dashboard's job. |
| 41 | **Static grain texture loaded once, not per-frame; auto-drop on GPU >2 ms/frame (v2).** | Perf #2 — v1 spec had inconsistent grain opacity (1.5/3/2–4 %) and implied per-frame composite. Single source-of-truth: 2.5% dark, 1.5% light. Profile-guided fallback. |
| 42 | **`degraded-blur` state — solid fill when chamber over busy content (v2).** | Perf #15 — blur over continuously-redrawing content invalidates cache every frame; 14-22 ms/frame on M1 Air integrated. Profile-guided downgrade silently. § 12. |
| 43 | **Pinned-widget refresh via `NSBackgroundActivityScheduler`; pause on low-power (v2).** | Perf #20 — raw `NSTimer` cadences prevented App Nap, drained battery. Scheduler coalesces wakes; pauses on `isLowPowerModeEnabled` + battery + screen-off. |
| 44 | **fsnotify 200 ms debounce + single watcher (v2).** | Perf #21 — atomic-rename fires 2-3 events; settings-toggle session was 60+ wakes. |
| 45 | **Lazy widget adapter registration (v2).** | Perf #4 — 13 eagerly-registered adapters were ~15-25 MB launch RSS. Boot mounts only chamber + ambient adapter factories. |
| 46 | **Wizard waveform: static glyph pre-grant, real waveform post-grant (v2).** | HIG anti-pattern (craft-HIG): faking input is dishonest UX. Removes pre-grant canned sine loop. |
| 47 | **Settings Appearance preview: 50% scale, 10 fps (v2).** | Perf #7 — full-chamber render path in a rarely-opened settings pane is wasteful. Preview is theater; cap is the cost-cut. |
| 48 | **Wake-word reliability mitigations (§ 6.7) only fire when opted in (v2).** | VAD-gate, per-app suppression, "Leah, ignore" voice command, false-positive learning loop, low-power unload, shape-based menubar state. Default-OFF means none of this is on the critical path. |
| 49 | **Reduced ornament toggle in Settings → Appearance (v2).** | Workflow #7 (aesthetic-tax-at-14h/day). Strips grain, italic, gold accents to bare functional palette. Default OFF; operator finds when they want quiet. |
| 50 | **Color-not-alone — every semantic color paired with icon prefix (v2).** | WCAG 1.4.1 + craft-HIG #2. `●` alert, `◆` brand-critical, `▾` sort, `▲/▼` delta. Deuteranopia-safe. |
| 51 | **Menubar icon is a pure-alpha template image; state via shape, not color (v3).** | UI craft + UX #11. Colored dots on template menubar images render as system-tint (look like an OS badge). Idle = outlined hex; listening = filled hex; error = filled hex + `●` inner dot. VoiceOver announces shape transitions. |
| 52 | **Dividers tiered — decorative 8% vs structural ≥20% (v3).** | UX #5. v2 collapsed everything to 20%; v3 lets in-card visual rhythm use 8% while group/section boundaries that carry information get the 3:1 UI-component floor. |
| 53 | **HUD captions use `--text-muted`, not `--text-dim` (v3).** | UX #4. `--text-dim` on HUD `--obsidian-1` bg = 3.29:1 (fails AA). Time-sensitive captions are load-bearing. |
| 54 | **SF Symbols first; restyled Lucide only for novel concepts (v3).** | UI craft #6 + #5.1. Weather glyphs, mic, paperplane, gearshape — all SF. Lucide retained for sigil + custom widget glyphs. |
| 55 | **Push-to-talk = Fn (or ⌥ where Fn absent), never Space (v3).** | Workflow MEDIUM "PTT-not-Space". Space-as-PTT gated on empty-input is a known Discord/Slack misfire path. |
| 56 | **Voice-summoned chamber = 400 × 280 px corner frame, not screen-center (v3).** | Workflow MEDIUM "voice-summon silently materializes". Hotkey-summon at 860 × 480 over a Slack draft after an accidental "Leah,…" is disproportionate. |
| 57 | **First-summon-per-session full flourish; warm summons cross-fade (v3).** | UI craft. 380 ms ceremony on cold-start only; 30-min-warm-window summons use 160 ms cross-fade. Eliminates 19 s/day "ceremony-as-latency" at 50 summons/day. |
| 58 | **First-ack-per-5min sigil rotation; subsequent acks reduce to border flash (v3).** | UI craft + workflow MEDIUM. Full F2 rotation at 80 wake-events/day is decorative noise after day 3. |
| 59 | **F2 ack gated on VAD pass + 600 ms transcribed-token window (v3).** | Workflow MEDIUM. False-pos × flourish = visible "Leah heard nothing useful" loop. Silent sigil revert on no-VAD. |
| 60 | **Listening pulse opacity envelope clamped 30-60%; 0.5 Hz under reduced-motion (v3).** | UX #32. Avoids 1 Hz seizure-band edge + migraine triggers in vestibular-sensitive users. |
| 61 | **Speaking waveform: SF Symbol .variableValue or single Metal shader, NOT 5 layout passes (v3).** | Perf #11. 30 Hz × 5 per-bar layout passes = 5 layouts/frame; new render = sub-millisecond. Also 10 Hz cadence + focus-only render. |
| 62 | **Thinking ring = 20-frame sprite-sheet loop, not gradient-stroke tessellation (v3).** | Perf #10. Gradient on rounded paths has no GPU-cached path; sprite-sheet ~26 % faster on ProMotion 8.3 ms budget. |
| 63 | **Flourish 1 = `transform.scale.y` with anchor-at-seam-center, not layout-bounds animation (v3).** | Perf #12. Bounds animations trigger per-frame layout passes; transform is GPU-cheap. |
| 64 | **Sigil animation frequency cap ≤1 per 500 ms (v3).** | UX #31. Prevents photosensitive triggering on rapid back-to-back wake-events. |
| 65 | **HUD Row 2 ("Now") is time-of-day-gated, not cascading-fallback (v3).** | Workflow MEDIUM #5.1. AM=calendar, PM=brief, evening=agenda. One source per window; never mystery-mode. |
| 66 | **HUD Row 3 ("Pulse") = ONE glyph-prefixed primary metric (v3).** | Workflow MEDIUM #5.1 + #66. Default PR queue; hover rotates secondary metrics. Glyphs (`◇ ⌬ ⎇`) carry meaning so eye doesn't parse 3 unrelated counts. |
| 67 | **Chamber placeholder fixed to "Ask Leah anything…" (v3).** | Workflow MEDIUM. Daily-rotating placeholders break muscle memory; one string forever. |
| 68 | **Chamber default 860 × 480 (raised from 720 px) (v3).** | Workflow MEDIUM. JetBrains Mono 13 pt × 80-char Go function wrapped at 720; 860 accommodates without breakout. |
| 69 | **Sensitive-content blur reveals pattern + per-message + Mark-safe + Always-allow-for-this-app (v3).** | UX #28 + workflow MEDIUM "blur Show scope". Replaces single `[Show]` with diagnose + recovery + scope-toggle. |
| 70 | **Widget tile chrome drops gold rule under eyebrow; Tiempos out of eyebrow (v3).** | Workflow MEDIUM "widget chrome 4-layer decoration". Eyebrow in `--text-dim` body sans alone. Consistent with decision-log #28 Tiempos-one-location rule. |
| 71 | **Candlestick = `hero` size only; medium = sparkline (v3).** | UI craft MEDIUM. ≥8 px/candle density floor. Adapter rejects below `hero`. |
| 72 | **Flights matrix: global min only filled-gold; row mins use 1.5 pt gold left-edge hairline (v3).** | UI craft MEDIUM. Multiple filled-gold cells destroyed the lowest-fare signal. |
| 73 | **Maps widget renders as citation card with "Open in Apple Maps" for routing intents (v3).** | Workflow MEDIUM. Desaturated mini-map at medium size is illegible for actual navigation. |
| 74 | **Weather widget glyphs are SF Symbols, never emoji (v3).** | UI craft + workflow MEDIUM. Emoji breaks palette discipline AND varies by macOS version. |
| 75 | **Widget reveal staggered 80 ms; tile height reserved from props immediately (v3).** | Perf #13 + workflow MEDIUM. Eliminates overlapping animation budget breach AND prose-reflow shift during async populate. |
| 76 | **Ambient pinned tiles are static-until-glanced (animate delta on hover only) (v3).** | Workflow MEDIUM. Blinking deltas in screenshots/Looms = distraction. |
| 77 | **Pinned widget cold-launch: paint from cache first; refresh staggered 250 ms in background (v3).** | Perf #36 + workflow MEDIUM. No network-stall on first paint. |
| 78 | **Widget cache: in-memory LRU + 50 MB dir cap + 5-min flush (v3).** | Perf #22 + #31. Avoids APFS small-file churn AND unbounded cache growth. |
| 79 | **Streaming envelope size cap 256 KB per frame (v3).** | Perf #25. Protects length-prefixed JSON parser buffer. Hot-path may upgrade to msgpack on measured burst. |
| 80 | **Quick-spawn chips persist across session via O(1) rolling top-3 file (v3).** | Workflow MEDIUM + perf #32. Chips reappear in every empty-state; no usage-log scan on chamber-open. |
| 81 | **TTS pre-warm on app launch (background thread) (v3).** | Perf #2 MEDIUM. AVSpeechSynthesizer first-use cold-loads voice model 150-300 ms; pre-warm before sigil-settle. |
| 82 | **Single coalesced NSTimer for all visible toasts (v3).** | Perf #35 MEDIUM. Per-toast timers each prevent App Nap; one timer computes next-fade on tick. |
| 83 | **Reduced-motion = true 0ms swap (Apple HIG); 80 ms cross-fade only when state would pop confusingly (v3).** | Perf #38 MEDIUM. v2 had ambiguity between 0 and `--dur-instant`. |
| 84 | **Per-row Permissions toggle where OS allows; deep-link CTA otherwise; tooltip + section-header micro-legend (v3).** | UI craft + UX #38. Operators trained on toggles see toggles; glyphs supplement. |
| 85 | **Tiered Integrations disconnect: low-data single-confirm; data-bearing "keep index / delete index" disambig (v3).** | UI craft MEDIUM. Files-disconnect with 2,341-entry corpus = destructive; needs the same care as memory-purge. |
| 86 | **VoiceOver streaming-response = sentence-boundary `aria-live="polite"`, full re-announce on prose.end (v3).** | UX. Per-token would saturate the screen-reader queue. |
| 87 | **Animations halt under system-idle ≥10 min, Low Power Mode, memory pressure (v3).** | Perf MEDIUM. v2 only specified pause-when-occluded; v3 adds idle + power-budget triggers. |
| 88 | **Increase Contrast system setting honored: hairlines 20→40%, muted/dim collapse to primary, gold gains 1 px ivory inner stroke (v3).** | UI craft HIG. v2 missed `accessibilityShouldIncreaseContrast`. |
| 89 | **Chamber resize hysteresis band 850-870 px for 2-column-grid switch (v3).** | Perf #23 MEDIUM. Per-pixel layout switch = 6-12 ms stutter on M1 Air. |
| 90 | **Conversation history paged from disk via NSFetchedResultsController pattern (v3).** | Perf #30 MEDIUM. 6 months × 10k convos × 5 KB = 50 MB eager-load. |
| 91 | **Wizard step 3 split: mic-permission row above, wake-word+voice combined below (v3).** | Workflow MEDIUM. Original step 3 crammed 4 decisions; split reduces cognitive-load spike. |
| 92 | **Wizard step 5 toast lives 4 s before hotkey is hot — toast doesn't get summoned-over (v3).** | Workflow MEDIUM. Toast teaches hotkey; pressing hotkey would summon chamber over the teaching toast. |
| 93 | **Wizard hotkey recorder = real-time conflict feedback as keys pressed, not on save (v3).** | UX #13. Operator should never save a chord then learn at next conflict. |
| 94 | **Settings search auto-focused on Settings open (Things pattern) (v3).** | Workflow MEDIUM. Reduces friction; `⌘F` re-focuses if cursor wandered. |
| 95 | **`⌘/` help-overlay surfaced from chamber empty-state footer (v3).** | UX #40 + workflow MEDIUM "post-wizard reonboarding". After 3 weeks operator does not remember `⌘/`; chamber empty-state shows "Press ⌘/ for shortcuts" as dim footer. |
| 96 | **HUD long-press → Settings removed; rely on menubar + `⌘,` (v3).** | Workflow MEDIUM. Long-press is undiscoverable + mouse-only + no threshold defined. |
| 97 | **Fullscreen corner orb opacity raised to 0.6 (from 0.3) (v3).** | Workflow MEDIUM #34. 30 % on dark Spaces = near-invisible. |
| 98 | **Code-widget line-number click affordance demoted to row-level copy (v3).** | UX #24. JetBrains Mono 30% opacity line numbers are below 24×24 hit-target; replace with `⌘C` on focused row + hover-row copy chip. |
| 99 | **Widget gallery overlay focus-trap rules: first focus = search; Tab cycles search→category→preview-grid; Esc returns to chamber input (v3).** | UX #22. Without explicit trap, Tab leaks focus to chamber input behind overlay. |
| 100 | **Pinned widgets cap revisited to 2 (already v2 — restated for cross-ref with new perf decisions).** | No change; restated for v3 cross-ref clarity. |
| 101 | **Distribution: Developer ID + notarization + Sparkle auto-update; NOT Mac App Store (v3.1).** | Global `⌥Space` hotkey + AF_UNIX socket + always-listening wake-word are incompatible with App Sandbox. Hardened Runtime required for notarization. See §17.1. |
| 102 | **NSPanel mask resolution: focus panel `[.titled, .nonactivatingPanel]` + `[.fullScreenAuxiliary, .moveToActiveSpace, .stationary]`; ambient HUD different panel with `.canJoinAllSpaces` (v3.1).** | Implementability A-2 fix. The "single panel with all behaviors" was AppKit-impossible. Ambient HUD's cross-Space behavior lives on its own panel; focus panel summons on the active Space. |
| 103 | **Screen-capture detection via `NSWorkspaceScreenIsBeingCapturedNotification` (push) + `CGDisplayStream` observer; NOT `SCShareableContent` (v3.1).** | Implementability A-3 fix. v3's `SCShareableContent` only *enumerates* shareable content; it doesn't signal "screen currently being captured." Correct API is `NSWorkspace` notification (macOS 12.1+). |
| 104 | **macOS minimum bumped to 14.0 (Sonoma) (v3.1).** | Implementability D-9. Unlocks `NSView.displayLink(target:selector:)` (CADisplayLink equivalent), SF Symbols variable-value reliable, `EKEventStore.requestFullAccess`, AccessibilityFoundation reliable. Drops v3's 13.0 floor. |
| 105 | **Unix socket moved to `~/Library/Caches/Leah/leah.sock` (v3.1).** | Implementability C-1 fix. Caches is appropriate for sockets in non-sandboxed apps; Application Support is for state, not transient IPC endpoints. Survives reboot via daemon recreate-on-launch. |
| 106 | **JSON Schema validator locked to `github.com/qri-io/jsonschema` (Go) — draft-07 + 2020-12 (v3.1).** | Implementability Top-7 #6. xeipuuv allocates aggressively on hot path; qri-io is the leaner option for draft-07 + 2020-12 dual support. Locked in `internal/widget/` go.mod. |
| 107 | **Font stack flipped: Inter + New York Italic as primary (free, OFL/system-bundled); Söhne + Tiempos as optional post-launch upgrades (v3.1).** | Implementability D-2/D-3. Klim licensing is per-quarter MAU-tiered and blocks shipping. New York Italic ships with macOS; Inter is OFL. v1 ships without licensing risk. |
| 108 | **Streaming-state-machine edge cases enumerated: chamber-dismissed-mid-stream, app-backgrounded, re-summon-during-stream, stream-network-down (v3.1).** | Implementability Top-7 #7 + §5.5 expansion. Buffer behavior + replay rules spec'd; LLM continues server-side on Esc; frames buffered to history. |
| 109 | **Leah voice canon: ElevenLabs custom voice (alto, ~145 wpm, dry-warm) as default; Samantha fallback offline-only (v3.1).** | First-impression #2. A named, gendered assistant cannot ship with operator-picks-from-3-Mac-voices. One canonical voice; Settings exposes 2 fallbacks labeled "alternate · not canon." |
| 110 | **Renames: sigil → mark; focus chamber → focus panel; flourish → transition; aesthetic-reduced → minimal mode (v3.1).** | First-impression #2/3/4/5. Cosplay names killed in user-facing copy. Engineering may retain internal slang. |
| 111 | **Brand category retarget: "tool a serious operator chooses, not a fintech dashboard" (v3.1).** | First-impression #1. v3 brand audit flagged "private-banking-app" cousin (Mercury/Ledger/Robinhood Gold). v3.1 keeps obsidian + gold restraint, drops oxblood-on-gold pairings, makes ivory-on-obsidian the primary fg surface, reserves oxblood for critical-alert iconography ONLY. §0.1 + §15 anti-pattern. |
| 112 | **Gold-budget visible-surface cap: max 3 gold instances per surface at once (v3.1 — strengthens decision #39).** | First-impression #4. v3 capped gold per chamber-render; v3.1 caps gold per *visible surface* (wireframe-time + render-time). Mechanically enforced by audit pass on every §13 wireframe. |
| 113 | **Killer-screenshot spec added as §13.14 (v3.1).** | First-impression #5. Marketing-hero composition is now binding spec: focus panel mid-flow over real macOS desktop, one widget, ivory prose, ≤3 gold instances, contextual answer only Leah could know. |
| 114 | **Esc/⌘. unification REJECTED — distinct verbs preserved (v3.1 — promoted from §18 changelog footer to decisions log).** | Workflow CRITICAL "abort two verbs." Esc = dismiss UI (free); ⌘. = cancel backend (saves money). Reviewer asked to unify; rejected because the cost asymmetry is real. Surfacing here so future reviewers don't re-raise. |
| 115 | **"Drop the maps widget" REJECTED — kept with citation-card fallback for routing intent (v3.1 — promoted from §18 footer).** | Workflow CRITICAL. Operator may genuinely ask "show me this place." Routing intent renders as citation card per decision #73; place-viewing renders as desaturated mini-map only at `large` size. Widget stays. |
| 116 | **CLI ↔ GUI parity enforced (v3.1 — silent-drop wf:cli-parity).** | New §6.8. Both surfaces ship same commands; GUI is overlay convenience. `make check` rule extends §16.7 to verify cli ↔ settings parity. |
| 117 | **Wizard chrome explicit: hidden title bar, traffic lights visible at 20 pt inset (v3.1 — silent-drop ui:wizard-titlebar).** | Reviewer asked for thin titlebar so users can grab + identify. v3.1 keeps hidden titlebar BUT documents the rationale (regal cold-open; modal already grabbable by Esc/anywhere-drag; window-identity carried by hero mark in step 1). Decision is explicit, not silent. |
| 118 | **`⌘/` help footer in chamber empty-state (v3.1 — silent-drop wf:cmd-slash-discoverability).** | Decision #95 added the footer rule; wireframes §13.4/13.4-light now render it. Visible chrome on every empty-state. |
| 119 | **Mark asset format: SVG/PDF vector for all sizes (v3.1 — perf #40 fold).** | Drops the v3 PNG @1x/@2x/@3x triplet (~150 KB binary). Vector = zero raster cost, single asset, infinite scale. |
| 120 | **Emboss canonical: `text-shadow: 0 -1px 0 #FFFFFF08, 0 1px 0 #00000080` (v3.1 — reconciles §3.6 vs §18 split).** | Two emboss specs existed (§3.6 inverted/heavier opacity vs §18 per-palette-doc). v3.1 picks the §18 variant as canonical; §3.6 updated to match. |
| 121 | **Framework lock: SwiftUI + AppKit native; Swift Package Manager (v3.2.1).** | Per `docs/research/2026-06-21-llm-provider-research.md` + adversarial confirmation: webview backdrop-filter cannot sample windows behind the browser — kills the glass-blur thesis. SwiftUI hosts UI; AppKit `NSPanel` hosts ambient HUD + focus panel for `canJoinAllSpaces` + nonactivating masks. Wails and webview_go paths killed. |
| 122 | **LLM SDK: Anthropic Go SDK (`github.com/anthropics/anthropic-sdk-go`) daemon-side; HUD never sees the key (v3.2.1).** | Per `docs/research/2026-06-21-llm-provider-research.md`. Daemon owns ALL Anthropic API calls; proxies streaming SSE → IPC frames to HUD. Hard architectural rule — kills the entire class of HUD-process-key-leak vulns. See §17.14. |
| 123 | **Model mix: Sonnet 4.6 primary + Haiku 4.5 router + Opus 4.8 escalation (opt-in) (v3.2.1).** | Per `docs/research/2026-06-21-llm-provider-research.md`. Sonnet for chat/agent; Haiku for widget-class + summarization + intent + rerank; Opus behind explicit user toggle (Settings → Advanced). Prompt caching ON; ZDR workspace required. ~$20/mo @ 100 q/day with 85% cache hit. See §17.14. |
| 124 | **Embeddings: Voyage 3.5-lite (cloud, 1024d) + BGE-small-en-v1.5 (local ONNX, 384d) fallback (v3.2.1).** | Per `docs/research/2026-06-21-embedding-vector-research.md`. Voyage free-tier covers personal-scale; BGE for offline + `LEAH_EMBED_LOCAL=1` + Settings → Privacy → "Embed locally." Schema namespaced by `(model, dim)` so cloud↔local toggle does NOT force re-embed. See §17.15. **Rejected:** OpenAI `text-embedding-3-small` (no privacy story). See §15. |
| 125 | **Vector store: `sqlite-vec` extension loaded into existing Leah SQLite via `mattn/go-sqlite3` CGo (v3.2.1).** | Per `docs/research/2026-06-21-embedding-vector-research.md`. Vector tables JOIN against memory/conversation/integration tables in the same DB — single-file backup story preserved. Brute-force cosine at < 200K vectors = 10–15 ms p95 (within 50 ms widget-mount budget); IVF index deferred until corpus > 200K. See §17.10 + §17.16. |
| 126 | **`(model, dim)` embedding-namespace schema invariant (v3.2.1).** | Vectors stored in tables keyed by `(model_id, dim)` — search picks the active model's table; cloud↔local toggle is instant, no re-embed forced. Backfill is a background opt-in only on "permanent default" switch. Carved out as its own decision (not bundled with #124) because the schema rule binds future provider swaps too. |
| 127 | **TTS: ElevenLabs Flash v2.5 (cloud) + Apple Ava Premium (offline + privacy-flagged) (v3.2.1).** | Per `docs/research/2026-06-21-tts-research.md`. **Flash v2.5 not Multilingual v2** — TTFB 75–150 ms vs 600–1200 ms. ElevenLabs Creator plan ($11 promo / $22 list). Apple `AVSpeechSynthesizer` voice "Ava (Premium)" for offline + sensitive-content (calendar/email/finance/memory blockword classifier). Provider abstraction `internal/tts/provider.go` makes vendor swap a 1-day change. See §2.7 + §17.17. **Rejected:** OpenAI/Gemini Realtime APIs (locks off Claude backbone); Cartesia Sonic (removed pro voice clone tier mid-eval). See §15. |
| 128 | **Key custody: BYOK Anthropic via Keychain (`com.maydow.leah.anthropic`, `default`, `kSecAttrAccessibleWhenUnlocked`); `ANTHROPIC_API_KEY` env honored first; rotation with 60 s undo (v3.2.1).** | Per `docs/research/2026-06-21-key-custody-research.md`. No embedded org-key; no subscription tier (Raycast Pro pattern). Wizard step 2 = paste-key → verify-1-token-ping → confirm (§13.15 wireframe). Rotation in Settings → API Keys → verify-then-swap + 60 s undo. See §17.18. |
| 129 | **Sparkle EdDSA custody: login Keychain primary + 3-place backup (1Password vault + age-encrypted Time Machine file + paper BIP39 in safe); multi-machine signing via `op read` injection, never disk replication (v3.2.1).** | Per `docs/research/2026-06-21-key-custody-research.md`. Apple Developer ID notarization is required (second key, rotation safety-net if EdDSA leaks). Deferred for v1: GitHub Actions signing, Sparkle delta-updates, beta channels, signed-appcasts. See §17.19. |
| 130 | **Distribution: GitHub Pages appcast (`https://maydow.github.io/leah/appcast.xml`) + GitHub Releases binaries; $0 infra (v3.2.1).** | Per `docs/research/2026-06-21-key-custody-research.md`. HTTPS via GitHub TLS; CDN-cached at edge. Sparkle daily check; install on next launch. See §17.20. |

---

## 15. Anti-patterns explicitly killed

| Anti-pattern | Why we don't ship it |
|---|---|
| Bright `#FFD700` gold | Reads casino / costume jewelry. We use `#C9A961`. |
| Pure cyan / electric blue | Old direction, killed in this pivot. |
| Gradient gold | Reads CSS trick, not metal. Flat gold + foil hairline instead. |
| Animated shimmer on gold | Gimmick. |
| Red used as positive/neutral fill | Breaks "red = brand or alert" discipline. |
| Glassmorphism everywhere | Blur is for summon overlays only. |
| Green chrome | Palette pollution. Success = one-time quiet tick. |
| Filled icons in primary chrome | Reads as "noisy" Material Design; we use 1.5 pt strokes. |
| Hard corner radii on icons | Reads as Material; we use 2 pt rounded. |
| Per-conversation tabs in the chamber | Browser chrome competes with response. |
| Sci-fi cyan/neon HUD | JARVIS-cosplay tax. |
| Decorative red panels / red buttons | Red on screen = brand mark or alert. Two contexts only. |
| Material-style 5-step shadow elevation | We have 3: floor / lifted / engraved. |
| Fake loading bars ("Personalizing your experience…") | No artificial delays. |
| Confirmshaming ("No thanks, I don't want a better experience") | Skip is neutral text. |
| Forced sequential permission gates | Only mic is asked; rest lazy-prompted. |
| Pre-checked marketing opt-ins | v2: NOTHING is pre-checked. Wake-word default OFF (was ON in v1 — reversed). |
| Bait-and-switch wake-word ("off" then enabling later) | Toggle is visible, on-screen, UNCHECKED by default (v2); flipping it on shows § 6.7 reliability behavior immediately. |
| Dark-pattern "Are you sure?" sheets on skip | Skip is one click. |
| Lengthy welcome video | One sigil + one line + 380 ms total. |
| Pre-filled email/name fields harvested for marketing | Wizard collects ZERO personal info. |
| Multi-page T&Cs / "I agree" wall | None. Privacy policy linked from Settings → About only. |
| Pop-up "Rate us" / "Send analytics" early | Analytics opt-out lives in Settings → Privacy. |
| Notification permission upfront | Deferred to first daemon-push event with inline rationale. |
| Vendor-bright basemap tiles | Maps widget desaturates to obsidian + ivory roads. |
| Rainbow chart series | Single accent series in gold; others muted ivory @ 40%. |
| Spinner overlays / skeleton-shimmer everywhere | Single champagne dot at 1 Hz for loading. Restraint. |
| Silent cache-eviction on widget pin | "Ambient is full · unpin one" — never silently evict. |
| Plugin/extension surface in v1 | Closed registry; security review per widget. |
| **Ignored system appearance** (v2) | Dark-only ships are 2018 startup patterns. HIG: support both. § 2.6 light palette. |
| **Custom tint overrides accent-color** (v2) | HIG: use `NSColor.controlAccentColor`. Gold is brand-mark only. § 3.0 tint policy. |
| **Fake AppKit panel state** (v2) | `.nonactivatingPanel` with first-responder text field + key-deferral to background app is AppKit-impossible. Chamber takes key on summon; returns on dismiss. |
| **Color-only meaning** (v2) | Every semantic color paired with icon prefix (`●` alert, `◆` critical, `▾` sort, `▲/▼` delta). WCAG 1.4.1 + deuteranopia-safe. |
| **Faked input UI** (v2) | Wizard step 3 cannot show animated waveform before mic granted. Static glyph + caption. |
| **Modifier-only chord hotkeys with disambiguation timer** (v2) | 250 ms tap-window imposed a latency floor + collided with Mission Control / window-tiling chords / Sticky Keys. Single modifier + letter (⌥Space). |
| **Pre-checked privacy-cost defaults** (v2) | Wake-word OFF by default. Active opt-in only. |
| **Unix dotfile state paths on Mac** (v2) | `~/.leah-state/` violates convention. Use `~/Library/Application Support/Leah/`. |
| **Deprecated capture detection** (v2) | `CGScreenIsCaptured()` deprecated since macOS 12. Use `SCShareableContent` push observer. |
| **Per-frame grain composite** (v2) | Grain is a one-time-loaded static texture. Not a per-frame blend. Auto-drops on slow GPUs. |
| **`HIToolbox` private-API enumeration** (v2) | Would fail App Store / notarization audit. Use curated macOS shortcut list + honest "third-party may conflict" UI copy. |
| **Always-on animations during DND / fullscreen** (v2) | Animations halt, not just dim. Recovers idle CPU. |
| **90 s chamber auto-destroy** (v2) | Killed work-in-progress reads. Idle = shrink to ambient pill at last position; preserve 24 h. |
| **Colored dots on menubar template images** (v3) | Template images are pure-alpha; system tints them. A "gold dot when listening" renders as accent-color, reads as OS-applied badge. State via shape (filled vs outlined hex). |
| **Space-as-PTT inside the chamber** (v3) | Discord/Slack-grade misfire path; gating on "input empty" is a brittle modal. PTT = Fn (or ⌥). |
| **Emoji in functional UI** (v3) | Renders 5 different ways across macOS versions/fonts; breaks obsidian/gold/ivory palette discipline. SF Symbols only for functional glyphs. |
| **Per-tile grain composite** (v3) | Grain is the single global static texture, inherited by every surface — not a per-tile blend. Stacked grain + blur per tile = compositor death. |
| **Per-frame gradient stroke for animated arcs** (v3) | No GPU-cached path; pre-bake to sprite-sheet. |
| **Per-bar SwiftUI `frame(height:)` for waveform** (v3) | 5 layout passes per frame at 30 Hz. Use SF Symbol `.variableValue` or single Metal draw call. |
| **Daily-rotating chamber placeholder** (v3) | Cute on day 1; breaks muscle memory by week 3. One string forever. |
| **HUD long-press → Settings** (v3) | Undiscoverable, mouse-only, no time threshold defined. Menubar + `⌘,` are sufficient. |
| **Single `[Show]` button on sensitive-content blur** (v3) | No diagnose (why was it flagged), no recovery (false-positive escape), no scope (per-message vs session). Replace with pattern reveal + Mark safe + Always-allow-for-this-app + per-message default. |
| **Faked left-to-right row-by-row loading sweeps under reduced-motion** (v3) | Discrete row-by-row motion is more attention-grabbing than continuous AND isn't fixed by zero-duration. Replace with static "Loading…" text. |
| **Always-filled candlestick widgets at medium tile size** (v3) | Density floor 8 px/candle — below `hero` is meaningless. Adapter rejects. |
| **Multiple filled-gold cells in a single matrix** (v3) | Destroys the "minimum" signal. Global min only filled; row mins use 1.5 pt left-edge hairline. |
| **Mini-map render at medium tile for routing intents** (v3) | Desaturated map is gorgeous in screenshots, illegible for navigation. Fallback to citation card + "Open in Apple Maps" CTA. |
| **Per-toast NSTimer fan-out** (v3) | Each timer prevents App Nap. Single coalesced timer computes next-fade. |
| **Unbounded widget-cache directory** (v3) | LRU cap 50 MB; GC on cache write. |
| **Eager-load conversation history in Dashboard** (v3) | 6 months × 10k convos × 5 KB = 50 MB; paged from disk only. |
| **Per-token VoiceOver streaming announce** (v3) | Saturates screen-reader queue. Batch at sentence boundaries; full re-announce on `prose.end`. |
| **Reduced-motion = 80 ms cross-fade everywhere** (v3) | True reduced-motion is synchronous swap (0 ms); cross-fade only when state-change would otherwise pop confusingly. |
| **Per-pixel layout-switch on chamber resize** (v3) | 6-12 ms/frame layout pass. Hysteresis band 850-870 px. |
| **Private-banking-app aesthetic** (v3.1) | v3 brand audit named this as Leah's wrong-category cousin (Mercury, Ledger, Robinhood Gold). The "haute horlogerie" doctrine over-rotated into "wealth-management dashboard." Fix: ivory-on-obsidian primary fg surface (warmth); oxblood reserved for critical-alert iconography only (no oxblood-on-gold pairings); balanced gold restraint; killer-screenshot composition that shows context + answer, not chrome + numbers. See §0.1 brand positioning. |
| **Cosplay names in user-facing copy** (v3.1) | "Sigil," "chamber," "flourish," "aesthetic-reduced toggle" read as Tolkien-glossary in shipped strings. Internal/engineering names may retain heraldic intent; UI copy uses "mark," "focus panel," "transition," "minimal mode." Brand-confidence reads like everyday words; brand-insecurity reads like a brand deck. |
| **Operator picks Leah's voice from a list** (v3.1) | A named, gendered assistant has ONE canonical voice (the way Siri does). Settings exposes 2 fallbacks but labels them "alternate · not canon" so the brand owns the moment. See §2.7 voice canon + decision #109. |
| **Mac App Store distribution** (v3.1) | App Sandbox blocks global `⌥Space` hotkey, AF_UNIX socket placement, and always-listening wake-word. Distribution via Developer ID + notarization + Sparkle. See §17.1. |
| **`SCShareableContent` as screen-capture detector** (v3.1) | API enumerates *shareable* windows/displays; doesn't tell you whether the screen is *being captured.* Correct API is `NSWorkspaceScreenIsBeingCapturedNotification` (push-based, macOS 12.1+). See decision #103. |
| **NSPanel single-mask all-behaviors** (v3.1) | `canJoinAllSpaces` + `nonactivatingPanel` + key-on-summon on one panel is AppKit-contradictory. Split: ambient HUD has `.canJoinAllSpaces`; focus panel uses `[.fullScreenAuxiliary, .moveToActiveSpace, .stationary]` + tracked-prior-app capture-and-restore. |
| **Söhne/Tiempos as v1 default fonts without closed license** (v3.1) | Klim per-quarter MAU app-license blocks ship. v1 uses Inter (OFL) + New York Italic (Apple-bundled, free). Söhne + Tiempos = post-launch upgrade if licensing closes. |
| **PNG @1x/@2x/@3x triplet for the mark** (v3.1) | ~150 KB binary cost for a single-asset use case. SVG/PDF vector is the macOS-2026 answer. |
| **Greeting-time-ambiguous strings** (v3.1) | "Good morning, Tri." rendered all day = lazy. Fixed time-of-day rotation per §7.1 (morning / afternoon / evening). |
| **Bracketed-emoji ASCII in spec wireframes** (v3.1) | Wireframes that say "SF Symbol" in body but show `[☀] [⛅] [🌧]` in ASCII lie about the spec. Wireframes show `[sun]` `[cloud.sun]` `[rain]` bracketed-name tokens. |
| **Oxblood-on-gold pairings** (v3.1 — first-impression brand retarget) | Triggers private-bank-portal cousin. Oxblood + gold across the same surface reads as wealth-warning dashboard. Oxblood lands only on critical-alert iconography; gold lands on mark + primary chrome. Never co-located unless the alert IS about the brand identity itself (vanishingly rare). |
| **Embedded org-key for Anthropic API** (v3.2.1) | We'd front the inference cost + carry liability for token spend + need a billing relationship with every operator. **What we ship instead:** BYOK — operator pastes their own Anthropic API key in wizard step 2, stored in Keychain `kSecAttrAccessibleWhenUnlocked`. Raycast Pro pattern. See §17.18 + decision #128. |
| **Subscription tier in v1** (v3.2.1) | Charging a subscription requires Stripe/Paddle integration, billing portal, dunning, refunds, tax — none of which exists, and none of which is on-thesis for a personal-use tool. **What we ship instead:** BYOK — operator pays Anthropic directly at cost. See §17.18 + decision #128. |
| **OpenAI/Gemini Realtime API for voice loop** (v3.2.1) | Locks the entire stack off the Claude backbone (reasoning + TTS bundled in one vendor call). **What we ship instead:** Anthropic for reasoning (§17.14) + ElevenLabs Flash v2.5 for TTS (§17.17), separated by `internal/tts/provider.go` so we can swap voice vendors in a day without touching the reasoner. See decision #127. |
| **OpenAI `tts-1` voices for Leah** (v3.2.1) | Generic OpenAI alloy/echo/onyx voices are not brand-distinct — ChatGPT's voice features ship the same six voices, so Leah would sound like a ChatGPT skin. **What we ship instead:** ElevenLabs Flash v2.5 Professional Voice Clone of the spec'd alto profile (§2.7). See decision #127. |
| **Cartesia Sonic v1 as TTS primary** (v3.2.1) | Cartesia removed the pro-tier voice clone mid-evaluation, killing the brand-voice path. **What we ship instead:** ElevenLabs Flash v2.5 (75–150 ms TTFB, mature voice-clone API). See decision #127. |
| **ElevenLabs Multilingual v2 (the older model) as primary** (v3.2.1) | TTFB 600–1200 ms — the operator hears latency before they hear Leah; the voice canon's "instantly responsive" thesis dies. **What we ship instead:** Flash v2.5 (75–150 ms TTFB, identical voice clone). See decision #127. |
| **OpenAI `text-embedding-3-small` for personal knowledge embeddings** (v3.2.1) | No clear privacy posture for indexed personal data (calendar/email/files/memory). **What we ship instead:** Voyage 3.5-lite (cloud, with clear no-train terms) + BGE-small-en local fallback for `LEAH_EMBED_LOCAL=1` operators. See §17.15 + decision #124. |
| **Replicating Sparkle EdDSA private key to every signing machine's disk** (v3.2.1) | Increases leak surface linearly with machine count; one stolen laptop = full update-channel compromise. **What we ship instead:** `op read "op://Engineering/leah-eddsa/notesPlain"` from 1Password CLI at sign time → pipe to `sign_update` → key bytes leave memory at process exit. See §17.19 + decision #129. |

---

## 16. Test plan

### 16.1 Visual contract tests

- Snapshot-test each surface (HUD, panel, wizard step, settings section, widget tile size variants) at standard scale + reduced-motion + dim+shrink corner-orb mode + secondary-monitor mode.
- Per-token contrast assertions in CI (every fg/bg pair in § 11.1 passes its claimed WCAG floor; regressions fail the build).
- Animation budget: assert no curve outside the three named easings; assert no duration outside the five named tokens (`--dur-instant`, `--dur-quick`, `--dur-standard`, `--dur-hero`, `--dur-reduced`).

### 16.2 Schema validation tests (`internal/widget/schema_test.go`)

| Case | Expected |
|---|---|
| Each widget type — minimal valid payload | accepted |
| Each widget type — fully-populated valid payload | accepted |
| Each widget type — missing required field | rejected with `missing_required` |
| Each widget type — extra field | rejected (`additionalProperties:false`) |
| `id` over 64 chars / illegal chars | rejected |
| Unknown `widget` discriminator | rejected by registry, not schema |
| `code.source` > 16 KB | rejected |
| `table.rows` > 200 | rejected |
| `list.items` > 50 | rejected |
| `actions[].callback` not matching `leah://action/…` | rejected |
| `image.url` non-https | rejected |
| `image.url` pointing at 192.168.x.x | rejected by adapter (not schema) |

### 16.3 Adapter contract tests

For each non-pure adapter (`internal/{markets,flights,calendar,weather,maps,web}/adapter_test.go`):

- **Happy path** — mock upstream returns canned payload → `Fetch` returns Payload with non-zero `FetchedAt`, expected `Data` shape, expected `StaleAfter`.
- **Upstream 5xx** → returns error; cache untouched.
- **Upstream timeout** (`httptest` server delays past `context.WithTimeout`) → context-cancelled error.
- **Etag short-circuit** — `Refresh` with matching prev Etag → returns prev `Payload` with `FetchedAt` unchanged, no upstream call (verified via mock counter).
- **Validate** — golden good/bad props table.

Pure adapters: round-trip identity test — `Fetch` echoes props as data; `Validate` rejects malformed props per schema.

### 16.4 Lifecycle integration test (`internal/widget/lifecycle_test.go`)

Single fake adapter + in-memory state + in-memory IPC channel:

```
1. mount(id=m1, refresh=60s)        → assert: live, widget.mount frame
2. tick(61s)                         → assert: widget.update frame, value delta
3. pin(m1)                           → assert: pinned-widgets.json contains m1
4. simulate panel dismiss          → assert: tile still tracked (pinned), refresh timer alive
5. simulate fetch error              → assert: widget.stale frame (cache hit) — NOT widget.error
6. drop cache, fetch error           → assert: widget.error frame
7. unpin(m1)                         → assert: pinned-widgets.json empty, ambient HUD watcher fires
8. dismiss(m1)                       → assert: widget.unmount, timer cancelled, no state mutation
```

### 16.5 Streaming test (`internal/daemonloop/stream_test.go`)

- Driver injects synthetic LLM stream: `"hello "` → `"world."` → tool-call → `" continuing."` → end.
- Assert frame order on socket: `prose.delta("hello ")`, `prose.delta("world.")`, `widget.mount(...)`, `prose.delta(" continuing.")`, `turn.end`.
- Assert: `widget.mount` `seq` is between the two prose deltas; never split mid-prose-delta.
- Concurrency: simulate slow fetch (200 ms) — assert `widget.mount` with `data:{}` placeholder emitted promptly, `widget.update` lands later; subsequent prose deltas are NOT blocked.

### 16.6 Security tests

- `image` URL = `http://...` → reject.
- `image` URL = `https://10.0.0.1/x.png` → adapter rejects (private IP).
- `render_widget` for a `widget` whose registry entry has `enabled=false` → reject with `tool_error`.
- Action callback `leah://action/rm_rf` → not registered, dropped before render.
- Registry hot-reload: write `enabled=false` to disk → next `render_widget` for that type fails within 1 s.

### 16.7 Cross-doc parity check (`make check` rule)

- Every `widget` enum value in §10.7 envelope has (a) a per-type schema file, (b) a registry entry, (c) an adapter (or `PureAdapter`), (d) at least one test case in §16.2.
- Section §10.1 widget catalog ⊇ this protocol's widget list (visuals may stub future widgets, but every protocol widget must have a tile design).
- Settings → Widgets toggle list ⊇ widget registry.
- **CLI ↔ GUI parity (v3.1 — silent-drop wf:cli-parity):** every command in `internal/cli/` has a Settings counterpart and vice versa per §6.8; verified by `make check` enumeration cross-walk via `scripts/check-cli-gui-parity.sh`.

**Spec-body parity sweep (v3.2 — extracted to external script).** The forbidden-phrase rules live in `scripts/check-spec-parity.sh`; the spec body intentionally does NOT enumerate them inline (v3.1's inline table self-cannibalized — the grep matched its own table). The script:

1. Reads this spec file as `$1`.
2. Walks the file tracking the current `## N.` heading.
3. While the current section is in the **allow-list** — `§14` (decisions log), `§15` (anti-patterns), `§18` (changelog) — skips all checks (those sections may legitimately cite the historical names).
4. Outside the allow-list, fails the build on any forbidden-phrase hit. The forbidden-phrase set lives in the script header comment (one entry per renamed term + ancillary cosmetic-debt rules), not in this spec — so the spec body never enumerates its own forbidden tokens.
5. Exits 0 on clean; non-zero with `file:line: forbidden phrase: <phrase>` on first hit.

**Makefile wiring:**

```makefile
check-spec-parity:
	@scripts/check-spec-parity.sh docs/superpowers/designs/2026-06-21-leah-macos-native-ui-design.md

check: check-spec-parity
```

Per CLAUDE.md token-economy rule, `make check` runs `make check-spec-parity` as a sub-target and fails fast on first mismatch.

### 16.8 Accessibility / a11y tests (v3)

- **VoiceOver smoke** — script-driven VO walk of HUD, panel, wizard step 3, Settings → Permissions; assert every interactive element announces a non-empty label matching § 11.3.
- **Zoom reflow** (`tests/a11y/zoom_test.swift`) — render panel at 100/130/150/200% system text-size; assert no horizontal scroll at 200%, Pulse row dropped at 130%, Mini collapse at 150%.
- **Reduced-motion swap** — assert transition 1 fires zero `CAAnimation` instances under `prefers-reduced-motion: reduce`; assert listening pulse renders a static layer (no animation key).
- **Increase Contrast swap** — assert hairline opacity 40% under `accessibilityShouldIncreaseContrast == true`; assert `--text-dim` → `--text-primary` substitution.
- **Color-blind safety** — render every semantic-color usage through Sim Daltonism deuteranopia + protanopia filters; assert each pair distinguishable (the icon prefix is the load-bearing signal, not hue).
- **24×24 hit-target audit** — programmatic assertion that every `accessibilityElement(children: .ignore)`-tagged glyph control has `accessibilityFrame.size >= CGSize(24, 24)`.

### 16.9 Perf budget tests (v3 — `tests/perf/`)

- **Cold-launch budget** — `xcrun xctrace record --template "Time Profiler"` against a fresh launch; assert p50 < 300 ms, p95 < 600 ms to first ambient-HUD paint.
- **Hotkey-latency budget** — script-driven keydown of `⌥Space` from a foreground app; measure to panel-input-becomes-first-responder; assert p95 < 100 ms (felt-instant target).
- **Frame budget** — Core Animation FPS Instruments template against panel-over-Final-Cut-timeline scenario; assert mean frame time ≤ 16 ms (60 Hz) OR degraded-blur state engages within 3 s.
- **Widget mount** — synthetic LLM stream emits widget → measure tile to first paint; assert < 120 ms with warm cache, < 400 ms cold.
- **Idle RAM** — `vmmap` after 5 min ambient idle with 2 pinned widgets; assert resident < 80 MB.
- **Idle CPU** — `powermetrics --samplers cpu_power -i 1000` over 60 s ambient-only; assert mean < 0.5 % (wake-word OFF default).
- **Sub-200 ms animations frame-count parity** — `--dur-instant` (80 ms) rendered on 60 Hz external + 120 Hz ProMotion; assert frame counts 4.8 / 9.6 land correctly + no first-frame stutter.

### 16.10 Battery + power tests (v3 — `tests/power/`)

- **Low Power Mode response** — flip `isLowPowerModeEnabled`; assert wake-word model unloads within 1 s, pinned-refresh quiesces, animations halt to instant-swap.
- **Memory pressure response** — simulate `os_memory_pressure` warning; assert widget-cache LRU evicts to 25 % cap; toast surfaces.
- **App Nap eligibility** — 10 min ambient idle with wake-word OFF and no pinned-widget refresh due; assert process enters App Nap (verifiable via Energy Log).

### 16.11 v3.2.1 vendor + custody integration tests

- **LLM streaming integration** (`internal/reasoner/stream_integration_test.go`) — daemon receives Anthropic SSE from `claude-sonnet-4-6` (recorded fixture); asserts daemon proxies frames to IPC; HUD test client asserts incremental `prose.delta` arrives < 100 ms after first daemon-side byte; widget render mid-stream is interleaved per §16.5 contract.
- **Embedding namespace migration** (`internal/knowledge/embed_namespace_test.go`) — seed `embeddings_voyage_3_5_lite_1024` with 1k vectors; flip Settings → "Embed locally" to BGE; assert (a) search queries route to `embeddings_bge_small_en_v1_5_384` for NEW vectors immediately, (b) existing vectors are NOT touched (no forced re-embed), (c) "permanent default" toggle triggers background backfill job, (d) flip back to Voyage during backfill cancels-and-resumes cleanly.
- **TTS privacy routing** (`internal/tts/classifier_test.go`) — feed sensitive-content corpus (calendar titles with names, email subjects, finance amounts, memory blockwords); assert each routes to Apple `AVSpeechSynthesizer`, neutral content routes to ElevenLabs; classifier p99 latency < 5 ms; no network call made on Apple-routed path (verified via mocked HTTP client counter).
- **API key custody + rotation** (`internal/keychain/rotation_test.go`) — store key A; rotate to key B with verify-ping success → assert Keychain holds B; rotate to invalid key C → assert Keychain still holds B (no swap on verify-fail); after successful swap, undo within 60 s → assert Keychain reverts to A; undo after 60 s → assert undo button disabled. Also: set `ANTHROPIC_API_KEY=env-key`, store keychain-key in Keychain → assert daemon reads env-key first.
- **Sparkle EdDSA verify** (`tests/release/sparkle_verify_test.sh`) — generate test appcast + EdDSA-signed test binary; run Sparkle's `verify_update`; assert exit 0; flip one byte of binary → assert verify exit non-zero; flip one byte of `.sig` → assert verify exit non-zero.

---

## 17. Implementation notes (framework-agnostic)

### 17.1 Distribution + entitlements (NEW v3.1 — implementability D-1 lock)

**Distribution channel: Developer ID + notarization + Sparkle auto-update.** NOT Mac App Store. Rationale: global `⌥Space` hotkey + AF_UNIX socket + always-listening wake-word are App Sandbox–incompatible. See decision #101.

**App Sandbox: OFF.** Hardened Runtime: ON. Notarization: required (`xcrun notarytool`). Code-signing: Developer ID Application certificate.

**Entitlements (entitlements.plist — non-sandboxed, Hardened Runtime):**

| Entitlement | Purpose |
|---|---|
| `com.apple.security.device.audio-input` | Microphone (dictation, wake-word). |
| `com.apple.security.network.client` | LLM API, web-fetch for image/citation widgets. |
| `com.apple.security.automation.apple-events` | Calendar / Reminders / Mail integrations via AppleScript (where EventKit doesn't suffice). |
| `com.apple.security.files.user-selected.read-write` | File widget; folder picker for indexing. |
| `com.apple.security.personal-information.calendars` | Calendar adapter (EventKit). |
| `com.apple.security.cs.disable-library-validation` | Wake-word `.mlmodel` loaded from `Resources/Models/` — Hardened Runtime exclusion required. |
| `com.apple.security.cs.allow-jit` | Reserved (not currently used); included for future Metal shader pipelines. |

**TCC prompts (lazy, at first use):** Microphone, Accessibility, Screen Recording, Calendar (`EKEventStore.requestFullAccess` on macOS 14+), Notifications.

**Auto-update: Sparkle (EdDSA signed appcast).** Update feed: `https://maydow.github.io/leah/appcast.xml` (GitHub Pages); binaries hosted on GitHub Releases. Daily check; deferred prompt; "Install on next launch" default. Operator can disable in Settings → About. See §17.19 for EdDSA key custody + §17.20 for distribution channel.

**Minimum macOS: 14.0 (Sonoma) — bumped from v3's 13.0.** Drivers (decision #104):
- `NSView.displayLink(target:selector:)` — reliable CADisplayLink-equivalent on macOS 14+.
- SF Symbols variable-value (`.variableValue` animation for waveform) — macOS 13+ but reliable from 14+.
- `EKEventStore.requestFullAccess` (replaces `requestAccess`) — macOS 14+.
- AccessibilityFoundation reliable announce APIs — macOS 14+.

### 17.2 Framework primitives

**Framework lock (v3.2.1): SwiftUI + AppKit native.** Swift Package Manager for deps. macOS 14.0 minimum (already specced §17.5). Wails and webview_go paths are killed — webview cannot sample `backdrop-filter` from windows behind the browser, which is a hard blocker for the glass-blur thesis. Decision #121.

- **HUD + menubar** → SwiftUI `MenuBarExtra` + native `NSPanel` (for `canJoinAllSpaces` + nonactivating behavior; SwiftUI `Window` doesn't expose the panel masks we need).
- **Focus panel** → native `NSPanel`: `styleMask = [.titled, .nonactivatingPanel]`, `level = .modalPanel`, `becomesKeyOnlyIfNeeded = false`, `worksWhenModal = true`; `collectionBehavior = [.fullScreenAuxiliary, .moveToActiveSpace, .stationary]`. Takes key on summon; returns key on dismiss via tracked-prior-app capture-and-restore (NOT `NSApp.activate(options:)` which is deprecated macOS 14+). Ambient HUD is a sibling NSPanel with its own `.canJoinAllSpaces` mask. SwiftUI content hosted inside via `NSHostingView`.
- **Daemon comms** → Go daemon over localhost Unix socket; the HUD process is a thin Swift client. The HUD process **never sees the Anthropic API key** — daemon owns all LLM I/O (§17.14). Push events (MAY-19 B1/B5 substrate) drive the notification widget queue.
- **Material blur** → `NSVisualEffectView` wrapped via `NSViewRepresentable` for SwiftUI. Real glass against the desktop.
- **Reduced motion / reduced transparency** → respond to `NSAccessibility.differentiateWithoutColor` and `NSAccessibility.reduceMotion` notifications.
- **Mark (v3.1 — was "Mark")** → SVG/PDF vector; render via Core Animation for the rotation/pulse transitions at 60 fps. No PNG triplet.
- **Animation primitive** → `NSView.displayLink(target:selector:)` (macOS 14+); fallback `CVDisplayLink` only on `< macOS 14` (we don't ship to < 14, so this is dead branch).
- **IPC** → Unix socket `~/Library/Caches/Leah/leah.sock` (v3.1 — moved from Application Support per decision #105); length-prefixed JSON frames; one persistent channel for prose + widget events. Daemon proxies Anthropic SSE → IPC frames so the HUD never opens a network connection of its own.
- **JSON Schema validator** → `github.com/qri-io/jsonschema` (Go; draft-07 + 2020-12). Locked in `internal/widget/go.mod` per decision #106.
- **Registry hot-reload** → fsnotify (FSEvents under the hood — watch parent dir + filter, not the file handle, to survive atomic rename); 200 ms debounce; 1 s reload SLA.
- **Pinned widgets persistence** → `~/Library/Application Support/Leah/pinned-widgets.json`; fsnotify keeps HUD in sync with focus-panel pin/unpin actions.
- **Widget cache** → `~/Library/Caches/com.leah.daemon/widget-cache/<adapter>/<sha256(props)>.json`; TTL = `Payload.StaleAfter` × 4 (cap 24 h); LRU cap 50 MB.
- **Wake-word ML model** → bundled at `Leah.app/Contents/Resources/Models/wake-leah.mlmodel`; on-device fine-tune deltas at `~/Library/Application Support/Leah/Models/wake-leah-user.mlmodel`; loaded via Core ML `MLUpdateTask` for fine-tune.

---

### 17.3 Localization + i18n

v1 ships **English (en-US) only**. All user-facing strings flow through `NSLocalizedString(_:comment:)` from day one — even though no other locales ship — so post-v1 translation requires zero UI surgery. RTL: layout flips for Arabic / Hebrew are out of scope for v1; the spec's anchor logic (HUD bottom-right, mark on left, state on right) is LTR-baked. Non-Latin font fallback: Inter falls through to system fallback for CJK / Cyrillic; New York Italic falls through to Hiragino Mincho ProN / Times New Roman per system. RTL ship requires §7.1 redraw — tracked for v1.x.

### 17.4 Multi-user macOS (Fast User Switching)

Daemon runs per-user via `~/Library/LaunchAgents/com.leah.daemon.plist` (one daemon per logged-in user; no global root daemon). Fast User Switching: on `NSWorkspaceSessionDidResignActiveNotification`, the background user's daemon suspends (pauses refresh, stops listening, halts animations, unloads wake-word model); on `NSWorkspaceSessionDidBecomeActiveNotification` it resumes. Microphone, Accessibility, and other TCC permissions are per-user — each operator grants independently. Global hotkey `⌥Space` belongs to the foreground session only.

### 17.5 macOS compatibility matrix

**Minimum: macOS 14.0 (Sonoma).** Test matrix:
- **macOS 14 (Sonoma)** — `xcrun simctl` smoke + key-window NSPanel verification.
- **macOS 15 (Sequoia)** — primary dev target; full visual-contract suite runs here.
- **macOS 26 (current GA)** — current OS at ship; full suite + Stage Manager + new-window-controller checks.

CI runs all three per push. `LSMinimumSystemVersion = 14.0` in `Info.plist`. Older OS releases (13.x, 12.x) are rejected at install with a friendly Sparkle-stage dialog.

### 17.6 Telemetry opt-in

**Default: OFF.** Wizard step 5 (You're ready) appends a one-line opt-in checkbox: `☐ Help improve Leah by sending anonymous error reports + performance metrics. Never includes message content.` Settings → Privacy → Telemetry mirrors the toggle. When opted-in, the telemetry payload is restricted to: crash backtraces (no PII), `internal/obs/` numeric histograms (frame budget, hotkey latency, fetch durations), and OSLog redaction-policy violations. NEVER: prompt text, response text, widget data, file paths beyond bundle, system identifiers beyond OS major version.

### 17.7 iCloud sync

Out of scope for v1. Pinned-widgets file, conversation history, memory store, settings JSON are all **local-only**. The data files live under `~/Library/Application Support/Leah/` — a path that operators may symlink to iCloud Drive in rare power-user setups. Leah opens its data files with `NSFileProtectionComplete` and does not advertise `NSUbiquitousKeyValueStore` — iCloud sync of any Leah state is unsupported in v1. Placeholder slot for v2: a single Settings → About → `Enable iCloud sync (experimental)` toggle is reserved and ships disabled.

### 17.8 Crash recovery

HUD UI process crash → daemon survives (separate process). launchd re-spawns the HUD via `KeepAlive { Crashed = true }` with backoff (max 2 respawns/min, then exponential). On respawn, the HUD reconnects to the daemon socket and rehydrates the conversation in-flight: the daemon replays the last-frame buffer so an interrupted stream resumes mid-prose without a re-ask. Daemon crash → launchd respawns the daemon; HUD shows the daemon-down ghost-panel state (§7.3) until reconnected. Both crash paths log to OSLog (§17.9) and, if telemetry is opted-in, file a backtrace upstream.

### 17.9 Logging

**Primary:** OSLog via `Logger(subsystem: "app.leah", category: ...)` — Console.app + `log show` discoverable; `sysdiagnose` ingest works out of the box. Categories: `daemon`, `hud`, `wake`, `widget`, `tts`, `ipc`. **Mirror:** file at `~/Library/Logs/Leah/leah.log` — rotated at 10 MB × 5 (50 MB ceiling). All user content (prompts, responses, transcribed audio) is logged at `.privacy(.private)` so it redacts in non-developer log views. Settings → About → `Reveal logs` opens Console.app filtered to subsystem.

### 17.10 Knowledge store backend

**SQLite + `sqlite-vec` extension via `modernc.org/sqlite/vec`** for memory + knowledge stores. Database at `~/Library/Application Support/Leah/leah.db`. v3.2.2 F1 resolution: the pure-Go `modernc.org/sqlite` v1.47.0+ (2026-03-17) ships an **in-tree port of `sqlite-vec`** — no CGo, no extension-load-at-runtime dance, no driver swap. Activation = blank import `_ "modernc.org/sqlite/vec"` from `internal/sqlstore/`. Vector columns are declared via `CREATE VIRTUAL TABLE knowledge_chunk_vec USING vec0(embedding float[1024])` and queried with the `vec0` `MATCH` operator; the virtual table JOINs against `knowledge_chunk(doc_id, ordinal, content)` and the memory/conversation/integration tables in the same DB (no second store, single-file backup story preserved). Brute-force `vec0` scan at < 200K vectors hits ~10–15 ms p95 — within the 50 ms widget-mount budget; the `vec0` index extension to IVF/HNSW is deferred until corpus crosses 200K (no premature index complexity). FTS5 virtual tables on top for keyword search. Daily auto-vacuum runs during the operator's idle window (≥ 5 min idle, AC power) via `PRAGMA incremental_vacuum`. See `docs/research/2026-06-21-vector-store-best-practice.md` for the pure-Go migration rationale; see §17.15 for embedding model + dimension namespacing.

### 17.11 Settings persistence

Settings live in `~/Library/Application Support/Leah/settings.json` — single JSON document; atomic write via temp-file + `rename(2)` (POSIX-atomic on APFS). Scalar prefs (hotkey, HUD position, theme, telemetry) are also mirrored to `UserDefaults` (`~/Library/Preferences/app.leah.plist`) so that future iCloud Keychain-style sync paths can pick them up without a migration. JSON is the source of truth; UserDefaults is a mirror written on every save.

### 17.12 Marketing-hero assets

**DEFER to v3.2.1.** §13.14 specifies the killer-screenshot composition; the actual asset bundle (PNG @2x/@3x renders, ProRes 4K Loom of the summon, Figma source for the panel, hero-mark SVG/PDF in 8 sizes from 18 px to 256 px) is produced in a v3.2.1 sub-task. Placeholder asset spec: 2880 × 1800 PNG @2x screenshot, sRGB color profile, displays the §13.14 composition exactly. Hero mark exported once as SVG + PDF + 4 PNG fallbacks at canonical sizes (18 / 24 / 56 / 96 px).

### 17.13 Touch ID + passkey for sensitive operations

Memory purge (§9.5) and telemetry-toggle (§17.6) require Touch ID confirmation when available via `LAContext.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics)`. Fallback: typed `PURGE` (already specced for memory) or system password prompt (for telemetry on machines without Touch ID). Touch ID is **an additional** friction point, not a replacement — the typed friction stays so screenshots-of-screen can't fingerprint-bypass.

### 17.14 LLM provider (NEW v3.2.1)

**Daemon owns ALL Anthropic API calls** via `github.com/anthropics/anthropic-sdk-go`. The HUD process never sees the API key and never opens a network connection to api.anthropic.com — daemon proxies streaming SSE → IPC frames to HUD (§17.2). This is a hard architectural rule, not a convention.

**Model mix:**

| Tier | Model | Use |
|---|---|---|
| **Primary** | `claude-sonnet-4-6` | User-facing chat + agent reasoning. Streaming via Anthropic event-stream. |
| **Router / utility** | `claude-haiku-4-5` | Widget-classification, conversation summarization, intent detection, search rerank — anything sub-second and structural. |
| **Escalation (opt-in, Phase 1)** | `claude-opus-4-8` | Explicit user toggle only — Settings → Advanced → "Use Opus for hard queries." Default OFF. v3.2.2 F4 resolution: ships in Phase 1 (was implied; now binding). Toggle flips daemon model id from `claude-sonnet-4-6` to `claude-opus-4-8` either single-shot (next request only) or session-wide, per operator pref also lives in Settings → Advanced. Daemon reads the flag from IPC frame OR settings file at request time; default remains Sonnet 4.6. |

**Caching + privacy:**

- **Ephemeral prompt caching ENABLED** on system-prompt + conversation-history prefix (Anthropic `cache_control: { type: "ephemeral" }`). Target 85% cache-hit at steady state → ~$20/mo at 100 queries/day.
- **Zero Data Retention (ZDR)** checkbox enabled at the Anthropic workspace level. Verified during onboarding (verify-ping reads workspace metadata).

**Provider abstraction:** `internal/reasoner/provider.go` interface — `Stream(ctx, messages, model) (SSEStream, error)`. The Anthropic backbone is doctrinal (TTS lives separately on ElevenLabs per §17.17; embedding lives separately on Voyage per §17.15), but the interface keeps a future Anthropic-Bedrock or Anthropic-Vertex swap as a 1-day change.

### 17.15 Embeddings (NEW v3.2.1; v3.2.2 F2 resolution: Phase 1 ships HashGenerator local fallback, BGE ONNX moves to Phase 2)

**Primary (cloud, Phase 1 + Phase 2):** `voyage-3.5-lite` @ 1024d via Voyage API. Free tier covers years of personal-scale usage (~100M tokens/yr); cost-neutral at v1 scale.

**Local fallback — Phase 1:** the existing `internal/embed/` **`HashGenerator`** (hash-bigram, no model weights, zero binary cost). Satisfies the offline-degraded path until Phase 2. Triggered when `VOYAGE_API_KEY` is unset OR Voyage unreachable for > 30 s (circuit breaker). No ONNX runtime is bundled in Phase 1.

**Local fallback — Phase 2:** `BGE-small-en-v1.5` (384d) via ONNX Runtime, loaded from `Leah.app/Contents/Resources/Models/bge-small-en-v1.5.onnx`. **Replaces** the HashGenerator path. Triggered when `LEAH_EMBED_LOCAL=1` OR Voyage unreachable for > 30 s. Settings → Privacy → **"Embed locally (slower, private)"** toggle pins it to local regardless of network.

**Schema invariant — `(model, dim)` namespacing:** vectors are stored in tables keyed by `(model_id, dim)` — `embeddings_voyage_3_5_lite_1024`, `embeddings_hash_bigram_*` (Phase 1), `embeddings_bge_small_en_v1_5_384` (Phase 2) — so cloud↔local toggle **does not force a re-embed**. Search queries pick the table matching the currently-active model; a backfill job re-embeds in the background only when the operator explicitly switches "permanent default." Decision #126.

### 17.16 Vector store (NEW v3.2.1)

See §17.10 for the SQLite + `sqlite-vec` implementation. Key invariants restated for vendor-spec completeness:

- Single SQLite file at `~/Library/Application Support/Leah/leah.db` — vector tables JOIN against memory/conversation/integration tables in the same DB.
- Brute-force `vec0` scan at < 200K vectors hits 10–15 ms p95 (within 50 ms widget-mount budget).
- IVF/HNSW index added when corpus crosses 200K (deferred trigger; no premature complexity).
- Single-file backup story preserved — `cp leah.db leah.db.bak` is the complete backup primitive.
- **`modernc.org/sqlite` v1.47.0+ pure-Go driver** with in-tree `sqlite-vec` port (v3.2.2 F1 resolution). Activation = blank import `_ "modernc.org/sqlite/vec"`; no CGo, no `sqlite3_load_extension` call, no `mattn/go-sqlite3` migration. The earlier CGo path is killed.

### 17.17 TTS (NEW v3.2.1)

See §2.7 voice canon for the user-visible behavior. Implementation contract:

- **Provider abstraction:** `internal/tts/provider.go` interface — `Speak(ctx, text, voice) (AudioStream, error)`. Two implementations: `internal/tts/elevenlabs/` (Flash v2.5 cloud) and `internal/tts/apple/` (`AVSpeechSynthesizer` wrapping voice "Ava (Premium)").
- **Privacy classifier:** `internal/tts/classifier.go` runs first (< 5 ms budget); checks text against a blockword corpus per content domain (calendar event titles, email subjects/bodies, finance amounts/account names, memory items). Hit → Apple local; miss → ElevenLabs cloud.
- **Daemon-side only:** the HUD never sees the ElevenLabs API key. Daemon synthesizes → streams Opus/AAC frames over IPC → HUD plays via `AVAudioEngine`.
- **Pre-warm:** ElevenLabs HTTP/2 connection + first synthesis to /dev/null on app launch (decision #81 retained); Apple voice model warm-loaded at daemon start.
- **Rejected:** OpenAI/Gemini Realtime APIs (lock Leah off the Claude backbone — Anthropic ↔ ElevenLabs separation is doctrinal). See §15.

### 17.18 API key UX — BYOK Anthropic (NEW v3.2.1)

**No embedded org-key. No subscription tier.** Operator pastes their own Anthropic API key; Leah uses it directly. Reference pattern: Raycast Pro's BYOK flow.

**Wizard step 2 (NEW — inserts between Welcome and Hotkey; wireframe in §13.15):** 3-step inner flow — `welcome → paste-key → verify-1-token-ping → confirm`. Paste field obscures the key (toggle "show"); on continue, daemon sends a 1-token `messages.create` ping; on 200 → green check + workspace name displayed; on 4xx → friendly error + retry.

**Storage:**
- Service: `com.maydow.leah.anthropic`
- Account: `default`
- Class: `kSecClassGenericPassword`
- Accessibility: **`kSecAttrAccessibleWhenUnlocked`** (the device must be unlocked; survives reboot)
- Read at daemon start ONLY (never cached in HUD process).

**Env override:** `ANTHROPIC_API_KEY` is honored FIRST when set (developer convenience + CI). Keychain is the fallback path. Env override is logged at daemon start so operators don't get surprised by a stale env var.

**Rotation flow:** Settings → API Keys → "Replace key" → paste new → verify-ping with new key → on success, swap; on failure, keep old. **60 s undo toast** after swap (`Keychain reverted` — same pattern as Gmail's "undo send"). Decision #127.

### 17.19 Sparkle key custody (NEW v3.2.1)

**Generation:** once, via Sparkle's `generate_keys` shipped helper.

**Primary storage:** Sparkle default — login Keychain on the dev machine where the key was generated (single signing machine).

**Backup (3-place mandatory):**

| # | Place | Form |
|---|---|---|
| 1 | 1Password vault item | Encrypted note containing the full private key (base64). |
| 2 | age-encrypted file on Time Machine | `leah-eddsa-${date}.age`, age recipient = operator's age public key. |
| 3 | Paper printout in physical safe | BIP39 mnemonic of the raw EdDSA seed bytes. |

**Multi-machine signing:** the private key file is **never replicated to a second disk**. Each additional signing machine injects via `op read "op://Engineering/leah-eddsa/notesPlain"` from the 1Password CLI at sign time → pipe to Sparkle's `sign_update` → key bytes leave memory at process exit. Decision #129.

**Apple Developer ID notarization:** required (second signing key — Developer ID Application cert). This second key is the rotation safety-net: if the Sparkle EdDSA leaks, we ship a Developer-ID-signed + notarized update that bundles a new EdDSA key, and operators install via the existing trust chain.

**Deferred for v1:**
- GitHub Actions signing (defer until release cadence > 1/week demands automation).
- Sparkle delta-updates (not worth the additional appcast complexity at v1 binary sizes).
- Beta channels (single-channel until installed-base > 100).
- Signed-appcasts (EdDSA-signed update binaries are sufficient — appcast tampering would still be caught at the binary signature check).

### 17.20 Distribution channel (NEW v3.2.1)

- **Appcast:** `https://maydow.github.io/leah/appcast.xml` — served from GitHub Pages (the `gh-pages` branch of the `maydow/leah` repo). HTTPS via GitHub's TLS termination; CDN-cached at edge.
- **Binaries:** `https://github.com/maydow/leah/releases` — uploaded via `gh release create` from the signing machine; each release attaches the notarized `.zip` and the EdDSA `.sig` sidecar.
- **Cost:** $0 — GitHub Pages + GitHub Releases free tier covers personal-scale distribution.
- **Update flow:** Sparkle checks the appcast daily (configurable in Settings → About), downloads from GitHub Releases CDN, verifies Developer ID signature + Apple notarization + Sparkle EdDSA signature; installs on next launch (deferred prompt per §17.1).

---

## 18. Versioning + change log

- **v3.3.0** (2026-06-23, this spec): Phase 3 ship criterion met. TTS subsystem landed (ElevenLabs Flash v2.5 cloud primary + Apple Ava Premium local fallback, `tts.cloud.frame` + `tts.local` IPC fan-out, daemon-side privacy classifier). Wake-word adapter shipped with `wake-leah.mlmodel` bundled under `Resources/Models/`, VAD-gate + per-app suppression list ON by default. Push-to-talk landed on Fn (internal) and right-⌘ (external). Minimal-mode runtime toggle wired to Settings → Appearance — strips grain, italic, gold-accents at runtime. Touch ID gate added for memory purge + telemetry-toggle per §17.13. Push-source IPC fan-out completed (knowledge + memory + integrations push deltas to HUD). KG-backed citations join the answer-engine streaming path. MCP publish ships read-only (queries only, no mutations). Sparkle auto-appcast generator landed with EdDSA verify + rollback channel — appcast hosted on GitHub Pages, EdDSA signing-key custody per §17.19. §4.7 dashboard surface implemented (Memory + agenda + briefs + news + knowledge views over existing widget adapters). §17.12 marketing-hero asset slots finalized (4 hero PNGs + SVG/PDF mark).
- **v3.2.2** (2026-06-21, this spec): folds 4 operator decisions on pre-flight conflict findings against the Phase 1 implementation plan (`docs/superpowers/plans/2026-06-21-leah-macos-native-phase1.md`). Source: `/tmp/leah-phase1-preflight.md` + `docs/research/2026-06-21-vector-store-best-practice.md`. No scope cuts; reconciles spec ↔ plan to one canonical truth.
  - **F1 — sqlite-vec via `modernc.org/sqlite/vec` (pure-Go, no CGo migration).** §17.10 + §17.16 updated: activation = blank import `_ "modernc.org/sqlite/vec"` against the existing `modernc.org/sqlite` v1.47.0+ driver; vector columns declared via `CREATE VIRTUAL TABLE … USING vec0(embedding float[1024])`; `MATCH` queries replace the brute-force Go cosine loop. Earlier "CGo via mattn/go-sqlite3 at daemon start" path killed. ~1 day cost; preserves single-binary, no extension-load dance.
  - **F2 — HashGenerator Phase 1; BGE-small-en-v1.5 ONNX Phase 2.** §17.15 amended: Phase 1 ships Voyage 3.5-lite cloud primary + the existing `internal/embed/` HashGenerator as the offline-degraded fallback (no ONNX runtime in Phase 1). Phase 2 replaces HashGenerator with BGE-small ONNX. §19 Phase 1/Phase 2 bullets updated to match.
  - **F3 — Full 6-step wizard per §13.15.** §0, §8 (intro + flow diagram + all per-step headings + step-dot graphics + edge-case matrix + VoiceOver line), and §19 Phase 1 wizard bullet all reconciled to the canonical order: Welcome → BYOK Anthropic paste-key + verify-ping → Hotkey + Accessibility → Microphone permission (wake-word demo, default OFF toggle) → ONE integration (Calendar pre-selected per operator decision #14) → "You're ready" (hotkey reminder + Settings link). Prior "5 steps" / "no pre-selection" prose contradictions removed.
  - **F4 — Opus 4.8 escalation toggle ships Phase 1 with Settings → Advanced section.** §17.14 model-mix table row updated from "opt-in only" implication to explicit Phase 1 commitment + single-shot vs session-wide semantics. §9.2 IA tree adds an Advanced (`⌘9`) section between Memory and About (About renumbers to `⌘0`). §19 Phase 1 LLM + Settings bullets updated.
- **v3.2.1** (2026-06-21, this spec): folds 7 operator-locked ship decisions from 4 vendor-research reports (`docs/research/2026-06-21-llm-provider-research.md`, `…embedding-vector-research.md`, `…tts-research.md`, `…key-custody-research.md`). No scope cuts; no design-language changes. Substantive v3.2→v3.2.1 changes:
  - **§0 cost projection added** — ~$31–42/mo @ 100 q/day (Anthropic ~$20 + ElevenLabs $11–22 + Voyage free + GH Pages/Releases free); BYOK distribution noted.
  - **§2.7 voice canon updated** — TTS engine pinned to **ElevenLabs Flash v2.5** (was generic "ElevenLabs custom voice"); offline + privacy-flagged fallback switched from Samantha to **Apple Ava Premium** (`AVSpeechSynthesizer`); privacy classifier (calendar/email/finance/memory blockwords → Apple local) added; provider abstraction `internal/tts/provider.go` named.
  - **§13.15 wireframe added** — wizard step 2 BYOK Anthropic API key paste + verify-1-token-ping + ZDR nudge + rotation re-entry. Wizard expands from 5 → 6 steps.
  - **§14 decisions log rows 121–130 added** — framework lock (SwiftUI + AppKit + SPM), Anthropic Go SDK daemon-side, model mix (Sonnet 4.6 + Haiku 4.5 + Opus opt-in), Voyage 3.5-lite + BGE local, `(model, dim)` namespacing, `sqlite-vec`, ElevenLabs Flash v2.5 + Apple Ava Premium, BYOK Keychain, Sparkle 3-place backup, GitHub Pages/Releases distribution.
  - **§15 anti-patterns extended** — embedded org-key, subscription tier v1, OpenAI/Gemini Realtime API, OpenAI `tts-1` voices, Cartesia Sonic (pro clone removed mid-eval), ElevenLabs Multilingual v2 (slow TTFB), OpenAI `text-embedding-3-small` (no privacy story), Sparkle EdDSA disk replication.
  - **§16.11 added** — vendor + custody integration tests (LLM streaming, embedding namespace migration, TTS privacy routing, API-key rotation + 60 s undo, Sparkle EdDSA verify).
  - **§17.2 framework primitives** — locked to SwiftUI + AppKit native; webview/Wails paths killed; HUD-never-sees-API-key invariant stated.
  - **§17.10 knowledge store** — sqlite-vec specifics added (CGo via mattn/go-sqlite3, brute-force < 200K, IVF deferred trigger, single-file backup invariant).
  - **§17.14 LLM provider added** — Anthropic Go SDK, model mix, ephemeral prompt caching, ZDR workspace, provider abstraction.
  - **§17.15 embeddings added** — Voyage 3.5-lite primary, BGE-small-en-v1.5 ONNX local fallback, `(model, dim)` namespacing schema invariant.
  - **§17.16 vector store added** — restates sqlite-vec invariants for vendor-spec completeness.
  - **§17.17 TTS added** — provider interface, privacy classifier, daemon-side synthesis + IPC audio frames, pre-warm.
  - **§17.18 API key UX added** — BYOK wizard flow, Keychain storage (service `com.maydow.leah.anthropic`, account `default`, `kSecAttrAccessibleWhenUnlocked`), `ANTHROPIC_API_KEY` env override honored first, rotation with 60 s undo.
  - **§17.19 Sparkle key custody added** — login Keychain primary + 3-place backup (1Password + age-encrypted Time Machine + paper BIP39), multi-machine `op read` injection (never disk replication), Apple Developer ID notarization required as rotation safety-net, GitHub Actions signing / delta updates / beta channels / signed-appcasts all deferred.
  - **§17.20 distribution channel added** — `https://maydow.github.io/leah/appcast.xml` (GitHub Pages) + `https://github.com/maydow/leah/releases` (GitHub Releases); $0 infra.
  - **§19 Phase 1 LLM bullet** — generic "LLM streaming" specified as Sonnet 4.6 + Haiku 4.5 + Anthropic Go SDK + ZDR + ephemeral prompt cache.
  - **§19 Phase 3 TTS bullet** — F1 nit fixed: "ElevenLabs Flash v2.5 primary; Apple Ava Premium fallback for offline + privacy-flagged content."
- **v3.2** (2026-06-21, this spec): fixes regressions from v3.1's incomplete rename + parity test; reprioritizes build order (no scope cuts). Substantive v3.1→v3.2 changes:
  - **Atomic rename completed.** `chamber` → `panel` (all 131 hits outside §14/§15/§18). `sigil` → `mark` (all 60+ hits including §11.3 VoiceOver rotor label and §3.5 prose; one paragraph in §3.5 retains the word "sigil" to explain heraldic intent). `Flourish 1/2` proper-noun → `Transition 1/2`. `aesthetic-reduced` → `minimal mode`. `gold seam` → `gold transition`. Canonical "focus chamber" / "the sigil" historical citations preserved only in section headers (`### 3.5 The Mark (formerly "the sigil")`, `### 4.3 Focus panel (formerly "focus chamber")`).
  - **Minimal mode** added to Settings → Appearance IA (§9.2) and §11.2 reduced-motion contract — the toggle is now spec'd, not just named.
  - **Gold budget reconciled.** Decision #112 (per-surface) is binding; #39 marked SUPERSEDED. §10.0 prose now cites #112.
  - **Type stack locked.** Inter + New York Italic primary (free + system-shippable); Söhne + Tiempos optional licensed upgrade. §10/§14/§16.7 references swept to New York Italic.
  - **§14 row 7 deleted** (zombie row claimed Tiempos in eyebrows; row 28 is the canonical one-location rule).
  - **Palette `#08090C` locked** — drops `#0A0A0C` drift.
  - **§13.8 wireframe** confirmed at 2 pinned widgets (matches decision #40 cap).
  - **§16.7 parity test rewritten.** Forbidden-phrase table moved out of the spec body into `scripts/check-spec-parity.sh` (self-cannibalization fixed). Allow-list (§14/§15/§18) and Makefile wiring (`make check-spec-parity` as sub-target of `make check`) are spec'd.
  - **19 NEW-GAP sections added** (concise — each 1–2 paragraphs):
    - §5.6 Timezone + DST
    - §6.9 Force-quit recovery · §6.10 Sleep + wake (NSWorkspaceDidWake jitter) · §6.11 Low Power Mode · §6.12 Low Data Mode · §6.13 AirPods + audio route change · §6.14 External keyboard variance · §6.15 VPN + system proxy
    - §17.3 Localization + i18n · §17.4 Multi-user macOS (Fast User Switching) · §17.5 macOS compatibility matrix (14/15/26) · §17.6 Telemetry opt-in (default OFF) · §17.7 iCloud sync (out of scope v1) · §17.8 Crash recovery · §17.9 Logging (OSLog + file mirror) · §17.10 Knowledge-store backend (SQLite + sqlite-vec) · §17.11 Settings persistence · §17.12 Marketing-hero assets (DEFER to v3.2.1) · §17.13 Touch ID + passkey for sensitive ops
  - **§19 Build order added** (NEW): Phase 1 answer engine + minimal shell (4 wk) → Phase 2 ambient + widget breadth + light parity (+3 wk) → Phase 3 voice + polish (+3 wk). Total 10 wk solo; no scope cuts. Old §19 (source-doc cross-reference) renumbered to §20.
- **v1.0** (2026-06-21, superseded by v2): original design lock.
- **v3.1** (2026-06-21, this spec): folded 3 adversarial v3-reviewer reports — consistency/traceability (22 findings, 8 BLOCKER), implementability (49 findings, 8 BLOCKER), first-impression/brand (REVISE verdict). Substantive v3→v3.1 changes:
  - **§0.1 brand positioning** added (first-impression #1 retarget): Leah looks like a tool a serious operator chooses, not a fintech dashboard. Drops oxblood-on-gold pairings; reserves oxblood for critical-alert iconography ONLY; ivory-on-obsidian primary fg surface.
  - **§0.2 killer-screenshot spec** added (first-impression #5): focus panel mid-flow over real macOS desktop, one widget, ivory prose, ≤3 gold instances, contextual answer.
  - **§2.7 Leah voice canon** added (first-impression #2): ElevenLabs custom voice (alto, ~145 wpm, dry-warm) as default; Samantha fallback offline-only; wizard plays canonical voice not system default; Settings exposes 2 fallbacks labeled "alternate · not canon."
  - **§3.1 calibration cleanup**: gold-primary rationale trimmed (`#D4AF37` cited only via §15 rejection cross-ref).
  - **§3.2 blur radius locked** to literal numerals (18 px ambient / 24 px panel) — drops v3 `<radius>` placeholder.
  - **§3.3 typography**: Inter + New York Italic as v1 primary stack (free, OFL/system-bundled); Söhne + Tiempos as optional post-launch upgrades. CJK fallback chain spec'd.
  - **§3.4 stroke rule qualified** to Lucide-only (SF Symbols inherit system stroke).
  - **§3.5 renamed to "the mark"**; SVG/PDF vector format locked (drops PNG triplet — perf #40 fold); rationale for "sigil" word preserved in design discussion only.
  - **§3.6 emboss canonical** = `text-shadow: 0 -1px 0 #FFFFFF08, 0 1px 0 #00000080` (reconciles §3.6 vs §18 split).
  - **§4.1 ambient HUD**: pin cap 3→2 (consistency BLOCKER); 252 px max-pinned height; NSPanel mask resolved (canJoinAllSpaces + fullScreenAuxiliary + stationary); ⌥Space chrome added; §4.1 vs §6.5 fullscreen reconciliation.
  - **§4.3 renamed to "focus panel"**; NSPanel mask `[.titled, .nonactivatingPanel]` + `level = .modalPanel` + tracked-prior-app key-restore; drops `.canJoinAllSpaces` from focus panel (was contradictory).
  - **§5.5 state diagram fixed**: kills `idle 90s` arrow → `idle ≥5min → ambient pill`; adds streaming-edge states (chamber-dismissed-mid-stream, app-backgrounded, re-summon-during-stream, stream-network-down).
  - **§6.4 PTT**: `⌥` fallback removed (collides with `⌥Space`); right-`⌘` substituted for external keyboards.
  - **§6.5 screen-capture detection** corrected: `NSWorkspaceScreenIsBeingCapturedNotification` + `CGDisplayStream` observer (push-based) — supersedes v3's `SCShareableContent` (wrong API).
  - **§6.7 wake-word model location** spec'd (`Resources/Models/wake-leah.mlmodel`); CLI parity referenced.
  - **§6.8 CLI ↔ GUI parity** section added (silent-drop wf:cli-parity fold).
  - **§7.1 greeting time-of-day rule** fixed: morning/afternoon/evening; no rotating prose.
  - **§10.1 Tiempos italic sweep**: 8 widget-description citations replaced with "body sans small-caps tracking +0.04em" per decision #28.
  - **§10.1 weather wireframe**: emoji ASCII `[☀] [⛅] [🌧]` replaced with SF Symbol bracketed-names `[sun] [cloud.sun] [rain]`.
  - **§10.3 wireframe title fixed**: "3 pinned widgets" → "2 pinned" + redrawn with 2 pins per decision #40.
  - **§13.1 Row 3 wireframe fixed**: multi-metric replaced with `◇ 5 PRs` (single primary metric per decision #66).
  - **§13.3 / §13.4 / §13.4-light wireframes**: ⌥Space chrome added to chamber header; `⌘/` help footer added to empty-state.
  - **§13.7 stacked-toast wireframe**: Row 3 updated to single primary metric.
  - **§13.8 wireframe title + body fixed**: "3 pinned" → "2 pinned" + dropped one widget.
  - **§13.14 hero-composition spec** added (NEW — marketing screenshot binding).
  - **§14 row 7 cleaned up** as zombie v1-era contradiction with row 28; v3.1 rows 101–120 added (distribution lock, NSPanel mask, capture API, macOS 14 minimum, socket move, JSON validator, font flip, streaming-state, voice canon, renames, brand retarget, gold-budget visible-surface cap, hero-screenshot, Esc/⌘. rejection surfaced, drop-maps-widget rejection surfaced, CLI parity, wizard titlebar decision, ⌘/ footer, vector mark format, emboss canonical).
  - **§15 anti-patterns extended**: private-banking-app aesthetic, cosplay names in user-facing copy, operator-picks-voice-from-list, MAS distribution, SCShareableContent-as-capture-detector, NSPanel-single-mask-all-behaviors, Klim-fonts-without-license, PNG-triplet-mark, ambiguous-greeting, bracketed-emoji-ASCII, oxblood-on-gold pairings.
  - **§16.7 `make check` parity rule** extended: spec-body grep-fail for `90 s`, `max 3 pin`, `Tiempos italic`, `⌘⌃`, `wake-word ON`, `sigil`, `focus chamber`, `flourish`, `aesthetic-reduced`, emoji-in-wireframes, `SCShareableContent` outside its enumerate-only use; CLI↔GUI cross-walk.
  - **§17.1 distribution + entitlements** section added (NEW): Developer ID + notarization + Sparkle; entitlements.plist enumerated; macOS 14 minimum locked; auto-update path spec'd.
  - **§17.2 framework primitives** updated: NSPanel masks explicit; socket moved to `~/Library/Caches/Leah/leah.sock`; JSON validator locked to qri-io; webview backdrop-filter limitation called out.
- **v3** (2026-06-21, superseded by v3.1): folded every remaining MEDIUM-severity reviewer finding (88 across 4 reports) + any LOW touching correctness/HIG. Each fold logged below; rejections enumerated at end with rationale. Substantive v2→v3 changes:
  - **§ 3.1 dividers tiered** (8 % decorative / 20 % structural — UX #5); **HUD captions pinned to `--text-muted`** not `--text-dim` (UX #4); **hover-rows freeze placeholder color** (UX #3).
  - **§ 3.4 SF Symbols first**; Lucide restricted to novel concepts (UI craft #6); **24×24 hit-area rule globalized** (UX #25).
  - **§ 4.2 menubar = pure-alpha template image; state via shape** (idle = outlined, listening = filled, error = filled + inner `●`) — colored dots removed (UI craft + UX #11 + UX #26).
  - **§ 5.3 listening pulse**: opacity-clamped 30-60 %; 0.5 Hz under reduced-motion; pause-when-occluded (UX #32 + perf #9). **Thinking ring** = 20-frame sprite-sheet (perf #10). **Speaking waveform** = SF Symbol `.variableValue` or single Metal shader; 10 Hz; focus-only render; halts at 10 min idle (perf #11 + workflow MEDIUM + perf MEDIUM idle).
  - **§ 5.4 Flourish 1** uses `transform.scale.y` (not layout-bounds) anchored at seam center (perf #12); **first-summon-per-session only** for full ceremony; warm summons = 160 ms cross-fade (UI craft). **Flourish 2** = first-ack-per-5 min; VAD-gated; ≤1 state-change/500 ms cap (UX #31 + UI craft + workflow MEDIUM).
  - **§ 6.4 PTT = Fn (or ⌥), never Space** (workflow MEDIUM).
  - **§ 6.5 voice-summon = 400 × 280 corner frame**, not screen-center (workflow MEDIUM).
  - **§ 7.1 HUD Row 2 time-of-day-gated** (AM/PM/evening — workflow MEDIUM #5.1); **Row 3 = one glyph-prefixed primary metric**, hover rotates secondary (workflow MEDIUM).
  - **§ 7.2 placeholder fixed to "Ask Leah anything…"** (workflow MEDIUM); **§ 7.3 sensitive-content blur** reveals pattern + Mark-safe + Always-allow-for-this-app + per-message scope (UX #28).
  - **§ 9.3 status-glyph tooltip + section-header micro-legend everywhere** (UX #38); **per-row Permissions toggle** where OS allows + deep-link CTA otherwise (UI craft); **collapse-to-3-states visual treatment** allowed while 4-state legend remains source of truth (workflow MEDIUM).
  - **§ 9 Integrations tiered disconnect** (low-data vs data-bearing; "keep index / delete index" disambig — UI craft MEDIUM).
  - **§ 10.0 tile chrome drops gold rule under eyebrow**; Tiempos out of eyebrow (workflow MEDIUM + decision-log #28 carryover).
  - **§ 10.1 candlestick = `hero` only** (UI craft MEDIUM); **flights global-min only filled-gold; row mins use 1.5 pt left-edge hairline** (UI craft MEDIUM); **weather glyphs = SF Symbols**, never emoji (UI craft + workflow MEDIUM); **maps citation-card fallback** for routing intents (workflow MEDIUM).
  - **§ 10.2 stagger widget reveals 80 ms; reserve tile height from props** (perf #13 + workflow MEDIUM). **In-memory LRU + 50 MB cache cap + 5-min flush** (perf #22 + #31).
  - **§ 10.3 cold-launch paint-from-cache; refresh staggered 250 ms in background** (perf #36 + workflow MEDIUM); **ambient pinned tiles static-until-glanced** (workflow MEDIUM).
  - **§ 10.4 widget gallery focus-trap rules** (UX #22); **§ 10.5 quick-spawn chips persist across session** via O(1) rolling top-3 file (workflow MEDIUM + perf #32).
  - **§ 10.7 envelope size cap 256 KB**; widget→prose citation on schema-fail (UX #30); msgpack hot-path fallback documented (perf #24).
  - **§ 11.2 reduced-motion** = true 0 ms swap; row-by-row loading sweeps become static text (UX #33 + perf #38); **frame-count parity check** spec'd for sub-200 ms animations (perf #37).
  - **§ 11.3 VoiceOver** labels rewritten for ⌥Space + Fn-PTT + pin/dismiss glyphs + daemon-down ghost-chamber + widget gallery `+`; **streaming response uses sentence-boundary aria-live=polite** + full re-announce on `prose.end`.
  - **§ 11.4 chamber default 860 × 480** (workflow MEDIUM); **200% zoom reflow without horizontal scroll** spec'd (UX #36); **chamber resize hysteresis 850-870 px** (perf #23).
  - **§ 12 new states**: Increase Contrast (UI craft HIG); system-idle ≥10 min (animation halt — perf MEDIUM); Low Power Mode (perf MEDIUM); memory pressure (perf MEDIUM "drop cache aggressive"); screen-recording restore (UX #34 LOW elevated to MEDIUM).
  - **§ 12 grain Reduce Transparency clarification** (UI craft #3).
  - **§ 14 decisions log** rows 51–100 add v3 design calls.
  - **§ 15 anti-patterns** extended with v3 patterns explicitly killed.
  - **§ 16 test plan** adds 16.8 a11y, 16.9 perf budgets, 16.10 battery/power.
- **v2** (2026-06-21, superseded by v3): folded 4 adversarial reviewer reports + 3 operator overrides. Substantive changes:
  - **Hotkey:** `⌘⌃` modifier-only chord → **`⌥Space`** (keydown trigger, no 250 ms floor)
  - **Wake-word:** ON pre-checked → **OFF default, opt-in in Settings → Voice**
  - **Appearance:** dark-only → **light + dark parity** (§ 2.6 light palette; `NSApp.effectiveAppearance` auto-switch)
  - **Tint policy:** custom gold everywhere → **gold = brand-mark only; system accent for all other tint** (§ 3.0)
  - **Contrast:** recomputed table; `--text-dim` `#6B6558 → #8A8478` (3.44 → 5.36); `--red-alert` `#C8434F → #D75A66` (4.14 → 5.26). All pairs pass AA at intended sizes.
  - **Focus ring:** new `--focus-ring` token defined for both modes.
  - **Hit targets:** pin/dismiss 12 px → 24 × 24 hit-area (WCAG 2.5.8).
  - **Chamber lifetime:** 90 s auto-destroy → idle ≥ 5 min shrinks to ambient pill (preserve 24 h).
  - **Chamber:** key-window on summon (Spotlight pattern); `.nonactivatingPanel` no-steal claim dropped (AppKit-impossible).
  - **Widget tiles:** max 4/turn → 2/turn; pinned max 3 → 2; gold-budget 3 visible/render.
  - **Widget gallery:** add chamber-resident `+` button (was `/widgets`-only — undiscoverable).
  - **Daemon-down:** menubar-only → inline ghost-chamber on hotkey-press at cursor location.
  - **Tiempos italic:** enforced one-location-only (Dashboard "Today" header); stripped from widget eyebrows + categories + empty-states.
  - **Material:** blur radius locked 18 px ambient / 24 px chamber; grain locked 2.5% dark / 1.5% light static texture; hairline 20% baseline + 28% non-Retina.
  - **State paths:** `~/.leah-state/` → `~/Library/Application Support/Leah/`; cache → `~/Library/Caches/com.leah.daemon/`.
  - **Capture detection:** `CGScreenIsCaptured()` → `SCShareableContent` observers.
  - **Performance:** lazy adapter registration; fsnotify 200 ms debounce; `NSBackgroundActivityScheduler` for pinned refresh; pause on `isLowPowerModeEnabled`; Settings preview 50% scale + 10 fps; wizard waveform static-pre-grant.
  - **New `degraded-blur` state** in § 12 for chamber-over-busy-content.
  - **New § 6.7** wake-word reliability (VAD-gate, per-app suppression, learning loop, low-power unload) — only relevant when opted in.
  - **New "Reduced ornament" toggle** in Settings → Appearance.
  - **Color-not-alone:** every semantic color paired with icon prefix (`●` alert, `◆` critical, `▾` sort, `▲/▼` delta).
- **Folded in v3 (was v2-deferred):**
  - Iconography stroke harmonization with SF Symbols (craft-HIG #6) → § 3.4 SF Symbols first.
  - Memory-purge "export-then-purge" one-click flow (workflow #28) → § 9.5 prompt "Have you exported memory first?" + one-click export-then-purge path added inline.
  - Letterpress emboss spec scoping (craft-HIG #14) → confined to § 3.6 light-mode sigil + Dashboard "Today" header only; two-direction `text-shadow: 0 -1px 0 #FFFFFF08, 0 1px 0 #00000080` per palette-doc fix.
  - Custom hotword model XPC-process RSS isolation (perf #27) → decision-log #48 (§ 6.7) re-states this as XPC-budget separation.
- **Deferred to v3.1 / v2-horizon** (genuinely out of v1 ship scope OR contradicts a higher-priority operator decision):
  - **Dashboard default-size adaptation for 13" MBP** (HIG #5): cosmetic — `min(1180, screenW × 0.85)` is the right fix but Dashboard is a non-MVP-5 surface; lands when Dashboard implementation begins.
  - **Per-Space pinning `.moveToActiveSpace` default** (HIG #19): contradicts operator decision #21 (HUD canJoinAllSpaces — operator wants HUD visible from every Space). Re-evaluate post-launch with usage data.
  - **HUD edge-dodge for system dialogs** (workflow #5): requires NSWindow occlusion sampling against frontmost-app dialogs — non-trivial heuristic; ship after Stage Manager collectionBehavior lands so both occlusion paths share code.
  - **HUD/toast Stage Manager `.stationary + .fullScreenAuxiliary`** (perf #39): coupled to HUD edge-dodge; same timeline.
  - **Curated macOS shortcut list maintenance pipeline** (craft-HIG #11 + UX #14): the list exists; the "kept fresh per macOS release" CI job is a v3.1 ops task.
  - **Sigil cold-eye user-test with 10 unprimed users** (UX #37): research task, not a spec change.
  - **Per-app suppression list ships pre-populated**: covered in § 6.7 — implementation is v3 (decision-log #48); the **operator-editable UI** is v3.1.
- **Rejected (with rationale):**
  - **UX #39 wizard step 4 "no pre-selection"**: rejected — operator decision #14 (Calendar pre-selected) is load-bearing for ambient HUD "Now" slot value within 60 s of wizard end. v3 keeps decision #14; copy clarifies "Recommended: Calendar" so the pre-selection is honest, not silent.
  - **Workflow CRITICAL "drop the maps widget entirely"**: rejected — operator may genuinely ask "show me this place"; the widget stays but routing-intent renders as citation card (decision-log #73) instead.
  - **Pal §6 Cormorant Garamond + SF Pro + SF Mono swap**: rejected — Söhne/Inter/JetBrains-Mono are already deployed across the spec; SF Pro substitution would require a full type-stack re-audit and contradicts decision-log #28 (Tiempos one-location). v1 vs pal contradiction resolved in favor of v1.
  - **Conversation-history search inside chamber**: noted as workflow CRITICAL #3 partly addressed (⌘[ / ⌘] navigates prior turns + ambient pill preserves chamber 24 h); full cross-conversation search remains a dashboard concern (decision-log #1), not chamber.
  - **Drop blur entirely under heavy backgrounds** (craft-HIG MEDIUM "widget chrome over text bleed"): rejected as a binary toggle — v3 keeps the `degraded-blur` state (decision-log #42) which already handles continuously-redrawing content; static heavy backgrounds are not a blur-cache miss.
- **v1.1 candidates** (still NOT in scope):
  - Per-conversation theming.
  - Alternative sigils.
  - Multi-profile / multi-workspace setup.
  - Cloud sync / account creation (local-first).
  - iCloud Keychain for integration tokens (uses macOS Keychain locally).
  - Localized wizard (en-US only).
  - Third-party widget plugin system.
- **v2 horizon → v3:**
  - iOS companion (sigil scales; layout reflows).
  - Widget protocol v2 (breaking schema migrations supported by daemon + registry version bump).

**Protocol version:** `widget-protocol/1`. Semver; bump major on breaking schema change. Migrator translates v1 pinned widgets → v2 (or drops with operator notification). Daemon refuses to start against mismatched registry version without operator confirmation.

---

## 19. Build order (NEW v3.2 — value-first sequencing, no scope cuts)

**Doctrine.** The coherence reviewer named the real wedge — *contextual omniscience* ("MAY-19 shipped, PR #321, 8 minutes, B2 unblocked"). Raycast structurally cannot do this. v3.1 spent 431 lines on widget schemas and 4 lines on what makes the answer real. v3.2 keeps all 13 widgets, both light + dark palettes, the voice canon, the wizard, settings, ambient HUD, focus panel, dashboard — and **resequences** the build so the answer engine ships in Phase 1, ornament + breadth in Phase 2, voice + polish in Phase 3. Each phase is a usable product.

Build order is binding for dispatch sequencing. Phase boundaries are merge gates — Phase 2 PRs do not start until Phase 1 ships internally on operator's machine.

### Phase 1 — Answer engine + minimal shell (target: 4 weeks)

The smallest thing that proves the wedge: ⌥Space summons a panel, you ask, the daemon answers with contextual knowledge of your repo + Linear + calendar, streaming text + 3 widget primitives renders the answer, ship signed + notarized + updatable. Dark-mode only.

- **Daemon ↔ LLM streaming** — **Anthropic Go SDK (`github.com/anthropics/anthropic-sdk-go`)** daemon-side; HUD never sees API key. Primary model `claude-sonnet-4-6`; router `claude-haiku-4-5` for widget-class + summarization + intent + rerank; **Opus 4.8 escalation toggle ships Phase 1** (v3.2.2 F4 — Settings → Advanced single-shot OR session-wide). **Ephemeral prompt caching ON** on system-prompt + conversation-history prefix; **ZDR workspace required** at the Anthropic console. Streaming via Anthropic event-stream → IPC frames to HUD. Daemon reads opus-escalation flag from IPC frame OR settings file. Lock model ids in `internal/reasoner/`. See §17.14.
- **Knowledge store** — SQLite + `sqlite-vec` via **`modernc.org/sqlite/vec` pure-Go blank import** per §17.10 (v3.2.2 F1 — no CGo, no driver swap). `vec0` virtual table over the operator's repo + Linear backlog + recent commits; Voyage 3.5-lite cloud embeddings with HashGenerator (existing `internal/embed/`) as the offline-degraded fallback (v3.2.2 F2 — BGE-small ONNX is Phase 2).
- **Memory pipeline** — auto-capture conversation + state to `~/Library/Application Support/Leah/leah.db`. Replays on summon for context.
- **Focus panel + ⌥Space hotkey** — NSPanel per §17.2; streaming text rendering; tracked-prior-app key-restore; key-window summon.
- **Menubar item** — template image, idle/listening shape (no widget interleave yet).
- **6-step wizard** (v3.2.2 F3) — Welcome → BYOK Anthropic paste-key + verify-ping → Hotkey + Accessibility → Microphone permission (wake-word demo, default OFF toggle) → ONE integration (Calendar pre-selected per operator decision #14) → "You're ready" (hotkey reminder + Settings link). Full wireframes in §13.15 (BYOK) and §8 (others).
- **Settings (4 sections)** — General + Privacy + Permissions + **Advanced** (v3.2.2 F4 — Opus 4.8 escalation toggle, single-shot + session-wide). Voice, Appearance, Integrations, Memory, About deferred to Phase 2.
- **Developer ID + notarization + Sparkle** — GitHub Releases as appcast host per §17.1; EdDSA keys generated + offline-backed.
- **Dark mode only** — light palette deferred.
- **3 widget primitives** — `stat`, `table`, `list`. Hardcoded into panel; no adapter registry yet.

**Ship criterion:** operator types "what's the status of MAY-19?" and gets the real answer cited from Linear + GitHub in <3 s, with the answer visibly rendered as a stat + list. The hero screenshot is reproducible from this build.

### Phase 2 — Ambient + widget breadth (target: +3 weeks → week 7)

Once the answer is real, surround it with ambient presence and the full widget catalog. Light-mode parity.

- **Ambient HUD** with §7.1 time-of-day rows (greeting, agenda, pulse metric).
- **Notification widget** with toast queue, 2-cap, coalesce timer.
- **Remaining 10 widget types** — `market`, `flights`, `weather`, `maps`, `calendar`, `image`, `chart`, `code`, `citation`, `diff`. Each gets an adapter; registry hot-reload via fsnotify.
- **Widget gallery overlay** + spawn affordance (`+` button, `⌘⇧W`, `/widgets`).
- **Pin-to-ambient flow** with 2-pin cap.
- **Light mode parity** — palette swap, KVO observer, cross-fade.
- **Settings — remaining 5 sections** (Voice, Appearance, Integrations, Memory, About).
- **Wizard step 5** — extra integrations (mail, files) on top of Calendar.
- **BGE-small-en-v1.5 ONNX local embedding** (v3.2.2 F2) — replaces the Phase 1 HashGenerator fallback. ONNX Runtime bundled under `Leah.app/Contents/Resources/Models/bge-small-en-v1.5.onnx` per §17.15.

**Ship criterion:** the §13.14 hero screenshot ships verbatim. Operator can pin a widget, see it ambient, switch palettes.

### Phase 3 — Voice + polish (target: +3 weeks → week 10)

Voice canon, wake-word, and the trim that makes it luxury.

- **TTS subsystem** — **ElevenLabs Flash v2.5 primary; Apple Ava Premium fallback for offline + privacy-flagged content** per §2.7 + §17.17. Provider abstraction `internal/tts/provider.go`. Privacy classifier (calendar/email/finance/memory blockwords) routes sensitive text to Apple local. Both pre-warmed on app launch.
- **Wake-word adapter** — `wake-leah.mlmodel` bundled per §6.7; opt-in only.
- **VAD-gate + per-app suppression** (Zoom, Meet, etc.).
- **Push-to-talk modifier** — Fn (internal) / right-⌘ (external).
- **Minimal mode toggle** — Settings → Appearance → Minimal mode strips grain, italic, gold-accents to bare functional palette.
- **Dashboard** surface — Memory + agenda + briefs + news + knowledge views; reuses widget adapters.
- **Touch ID** for sensitive ops (memory purge, telemetry-toggle) per §17.13.
- **Marketing-hero assets** — finalized SVG/PDF marks, 4 hero PNGs per §17.12.

**Ship criterion:** v1.0 public launch. Operator on a fresh Mac runs through the wizard, opts into wake-word, says "Hey Leah, what shipped today?", and the panel streams a cited answer from the knowledge store in the canonical Leah voice over AirPods.

### Total: 10 weeks solo. Each phase = usable product.

Phase 1 ends with an internal-use tool the operator can rely on. Phase 2 ends with a screenshot that sells the product. Phase 3 ends with a public-launchable build.

---

## 20. Source-doc cross-reference

| Section here | Sourced from |
|---|---|
| § 0 Executive summary | new — synthesis |
| § 1 Operator overrides | new — operator decisions |
| § 2 Design thesis | main-doc § 0 |
| § 3 Visual identity | main-doc § 2 + palette-doc § 2 (rationale), § 3 (gold variants), § 4 (red variants) |
| § 4 Form factors | main-doc § 1 + wizard-doc § A.1 / B.1 |
| § 5 Motion + animation | main-doc § 3 |
| § 6 Interaction model | main-doc § 4 + wizard-doc § 0 (overrides) + new § 6.5 (fullscreen + secondary monitor) |
| § 7 IA per surface | main-doc § 5 |
| § 8 First-launch wizard | wizard-doc § A (full lift) |
| § 9 Settings preferences | wizard-doc § B (full lift) + Widgets section added (cross-ref to § 10.6) |
| § 10.0 Canvas model | main-doc § 9.0 |
| § 10.1 Widget catalog | main-doc § 9.1 (visual) + widget-protocol § 1 (schemas) interleaved per widget |
| § 10.2 Lifecycle | widget-protocol § 3 |
| § 10.3 Pin-to-ambient | main-doc § 9.2 |
| § 10.4 Widget gallery | main-doc § 9.3 |
| § 10.5 Spawn / loading / error | main-doc § 9.4 + § 9.5 |
| § 10.6 Adapter registry | widget-protocol § 2 + § 4 |
| § 10.7 Streaming protocol | widget-protocol § 1.0 envelope + § 5 |
| § 10.8 Security model | widget-protocol § 6 |
| § 10.9 Extensibility | widget-protocol § 7 |
| § 11 Accessibility | main-doc § 6 |
| § 12 States | main-doc § 5.3 + widget-protocol error paths |
| § 13 Wireframes | main-doc § 7 + wizard-doc § A.3 / B.3 + new § 13.8 / § 13.9; **v2 adds § 13.4-light** (light-mode panel mirror) |
| § 14 Open decisions | main-doc § 8 + wizard-doc § C (merged) |
| § 15 Anti-patterns | palette-doc § 7 + wizard-doc § A.5 + new additions |
| § 16 Test plan | widget-protocol § 8 + new visual contract tests |
| § 17 Implementation notes | main-doc "Implementation notes" + new IPC / persistence details |
| § 18 Versioning | main-doc "Versioning" + wizard-doc § E + widget-protocol "Protocol version" |

— end —
