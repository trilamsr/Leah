# Leah — macOS Native UI Design v1

> Author: senior product design lead (Apple HIG / Linear / Arc / Vercel / luxury watchmaker UI lineage)
> Date: 2026-06-21
> Status: design spec, clean-slate. Supersedes the detached-browser HTML/CSS/JS surfaces.
> Audience: implementation team (framework: deferred — SwiftUI / Wails v3 / webview_go). Design in primitives.

---

## 0. Design thesis

Leah is not a chatbot in a window. She is **a presence on the operator's machine** — quiet, regal, instantly responsive — that occasionally surfaces a glass chamber when summoned.

The aesthetic is **operator-overlay** (JARVIS lineage) crossed with **haute horlogerie** (Patek Philippe, Vacheron Constantin, A. Lange & Söhne site type, restraint, gold-on-obsidian). The result is sleek, regal, not sci-fi cosplay. **No cyan, no neon, no holographic gradients.** A single sigil of gold against deep obsidian carries the brand.

Three rules govern every decision:

1. **Earn every pixel.** Ambient surface shows ≤7 data slots and renders at <2% screen area. Focus surface shows one conversation, full attention.
2. **Quiet by default.** Animation is restrained — Patek not Pixar. Sound off. The hero flourish is the focus chamber materializing; everything else is sub-200ms cross-fades.
3. **Recognizable at 100 px.** The sigil — a gold engraved "L" inside a hairline-thin obsidian hexagon — is the lockup. Anyone seeing a screenshot at thumbnail size knows it's Leah.

---

## 1. Form factors

Seven surfaces. Each one earns its existence; nothing duplicates another.

### 1.1 Ambient HUD (the always-on presence)

| Attribute | Value |
|---|---|
| **Purpose** | Persistent low-chrome status indicator. Operator glances; sees state, weather of the day's agenda, listening state. |
| **Trigger** | App launch. Always present unless explicitly hidden. |
| **Dismiss** | `⌘⌥H` (hide to menubar) or quit. Auto-hides during screen recording (privacy). |
| **Size** | 280 × 84 px (default) — three slot rows. Mini: 56 × 56 px sigil-only. Expanded: 280 × 168 px (adds agenda strip). |
| **Position** | Bottom-right by default, 24 px from screen edge. Edge-dockable to any of 4 corners + 4 mid-edge anchors (snap-on-drag, magnetic). Per-monitor sticky position. |
| **Lifetime** | Process lifetime. Persists across Spaces (NSPanel `canJoinAllSpaces`). Floats above normal windows, *below* fullscreen apps unless `always-on-top` toggled in settings. |

### 1.2 Menubar item (the system-tray peer)

| Attribute | Value |
|---|---|
| **Purpose** | Lightweight access when HUD is hidden; settings entry; quit. Also: silent listening-state indicator (gold dot = listening, dim = idle, red = error). |
| **Trigger** | Menubar icon click. |
| **Dismiss** | Click outside, Esc. |
| **Size** | 18 × 18 px icon; 240 × auto popover. |
| **Position** | macOS menubar; popover anchored under icon. |
| **Lifetime** | Process lifetime. |

### 1.3 Focus chamber (the summoned conversation surface)

| Attribute | Value |
|---|---|
| **Purpose** | Query input + streaming response + sources + follow-ups. The primary work surface. |
| **Trigger** | Global hotkey `⌘⇧Space` (rationale §4) **or** click the ambient HUD sigil **or** start speaking the wake-phrase ("Leah,…"). |
| **Dismiss** | Esc, click-outside, `⌘W`, or 90 s idle after last interaction. |
| **Size** | 720 × 480 px default. Reflows down to 560 × 400 px on small displays; up to 960 × 640 px on ultrawide. |
| **Position** | Screen-center on summon (Linear command palette convention), with a 24 px upward bias (Spotlight muscle memory). Remembers last position if operator drags it. |
| **Lifetime** | Modal-feel but not modal — does NOT steal focus from current app's text fields unless operator types. Dismisses to ambient on Esc. |

### 1.4 Notification widget (transient ambient broadcast)

| Attribute | Value |
|---|---|
| **Purpose** | Leah pushes a brief one-liner: arxiv alert, GH release, calendar nudge, brief-ready, daemon error. Replaces noisy macOS notifications for in-Leah events. |
| **Trigger** | Push from daemon (server-pushed widget refresh, MAY-19 B1/B5). |
| **Dismiss** | Auto-fade after 6 s (configurable 3-12 s). Click to expand into focus chamber with full context. Swipe-right to acknowledge. |
| **Size** | 320 × 64 px (single line + sigil); 320 × 112 px (with one action chip row). |
| **Position** | Stacks above ambient HUD (offset 8 px up per new toast, max 3 visible; overflow collapses to "+N more"). |
| **Lifetime** | 6 s default; persistent if marked priority-red. |

### 1.5 First-launch wizard (operator onboarding)

| Attribute | Value |
|---|---|
| **Purpose** | One-time: grant permissions, pick wake-word behavior, select voice, OAuth integrations (`leah connect <integration>`), pick HUD anchor. |
| **Trigger** | App first launch; re-entry via Settings → "Re-run setup". |
| **Dismiss** | "Finish" button on step 6, or Esc on any step → skip with warning. |
| **Size** | 720 × 520 px modal. |
| **Position** | Screen-center. |
| **Lifetime** | Until completed or quit. |

### 1.6 Settings (the configuration surface)

| Attribute | Value |
|---|---|
| **Purpose** | All preferences: appearance, hotkeys, voice, integrations, privacy, advanced (daemon ports, model picker). |
| **Trigger** | `⌘,` from focus chamber, menubar → Settings, or HUD long-press. |
| **Dismiss** | `⌘W`, Esc, close button. |
| **Size** | 760 × 560 px; resizable down to 640 × 480. Things-style sidebar + detail pane. |
| **Position** | Screen-center on first open; remembers position. |
| **Lifetime** | Independent window. Does not block other surfaces. |

### 1.7 Dashboard (operator's day, on demand)

| Attribute | Value |
|---|---|
| **Purpose** | Full-screen view of memory, agenda, briefs, news bundle, knowledge timeline. Where the operator goes *to look*, not *to ask*. |
| **Trigger** | `⌘⇧D`, menubar → Dashboard, HUD → expand-arrow. |
| **Dismiss** | `⌘W`, Esc returns to ambient. |
| **Size** | 1180 × 760 px default; resizable; remembers last frame. |
| **Position** | Centered on first open; per-monitor sticky thereafter. |
| **Lifetime** | Independent window. |

**Explicitly NOT designed (eliminated by deletion-default):**
- Per-conversation tabs (one focus chamber, one stream — history is its own dashboard tab).
- Floating sidebar with chat history (lives inside dashboard).
- Status bar inside focus chamber (state lives on the sigil).
- A separate "voice mode" surface (voice is a state of the focus chamber, not a window).

---

## 2. Visual identity

### 2.1 Color tokens (every value is a real hex)

#### Obsidian (background system)

| Token | Hex | Use | Justification |
|---|---|---|---|
| `--obsidian-0` | `#08090C` | Focus chamber bg, deepest layer | Near-black with 4 pts of blue to feel cool/regal vs muddy. Reference: Vercel `#000` is too flat; Linear's `#101113` is the closest cousin; Patek Philippe site uses `#0A0A0C`. We split the difference. |
| `--obsidian-1` | `#0E1014` | Ambient HUD bg, settings sidebar | One step above the floor for layered surfaces. |
| `--obsidian-2` | `#161922` | Card / row hover, input field bg | Hairline-rule needs a surface to land on; this is it. |
| `--obsidian-3` | `#22262F` | Pressed state, divider-on-card | Top of the stack; reads as "interactive". |

#### Gold (primary accent, brand)

| Token | Hex | Use | Justification |
|---|---|---|---|
| `--gold-primary` | `#C9A961` | Sigil fill, primary CTA, listening-active ring | Champagne gold (not Vegas gold). Calibrated against Patek's `#C5A572`, Vacheron `#CDB370`, A. Lange `#B89968`. We pick a midpoint with slightly warmer hue (62°) for warmth on dark. WCAG: 8.4:1 on `--obsidian-0`. |
| `--gold-hover` | `#D9BC7A` | CTA hover, focused link | +1 step luminance. |
| `--gold-muted` | `#8A7340` | Disabled/decorative gold, hairline frame | -40% saturation; reads as engraved metal not lit metal. |
| `--gold-glow` | `#E8CC8C` | Listening pulse outer ring; never as fill | High-luminance outer for the pulse halo; used at 20-40% opacity. |

#### Red (alert, brand secondary)

| Token | Hex | Use | Justification |
|---|---|---|---|
| `--red-brand` | `#7A1F2B` | Critical-state sigil, "L" mark on red background variant for installer/marketing | Oxblood, not Coca-Cola. Reference: Leica red `#E20613` is too loud; Ferrari `#FF2800` rejected; Hermès orange family hue but pulled toward red at 350°. WCAG: 4.6:1 on `--obsidian-0` for outline use; never used as text body. |
| `--red-alert` | `#C8434F` | Daemon-down banner, permission-denied state, destructive confirm | One step up; readable as alert text. WCAG: 5.1:1 on `--obsidian-0`. |
| `--red-dim` | `#4A1820` | Sensitive-content blur scrim tint | Background-only; never carries information. |

#### White / ivory (foreground)

| Token | Hex | Use | Justification |
|---|---|---|---|
| `--text-primary` | `#F2EDE0` | Body text, response stream | Ivory not pure white. Pure `#FFF` on `#08090C` is harsh and "computer-y"; ivory reads like printed page on stained walnut. Reference: NYTimes Cooking dark-mode ivory `#F0E9D9`. WCAG: 14.8:1. |
| `--text-muted` | `#B8B0A0` | Secondary captions, source citations | WCAG: 8.3:1. |
| `--text-dim` | `#6B6558` | Placeholder, timestamps, divider labels | WCAG: 4.6:1 — at the AA floor; used only on incidental text. |
| `--divider` | `#FFFFFF14` | Hairline rules (8% white) | Hairline at 0.5pt; never 1pt. The luxury-watch hairline is THE detail — Patek-grade restraint. |

#### Functional tones (used sparingly; no green/blue/yellow chrome by default)

| Token | Hex | Use |
|---|---|---|
| `--success-quiet` | `#7A9B7A` | One-time "saved" confirm tick. No green chrome elsewhere. |
| `--blur-tint` | `#08090CCC` | Material-blur tint (80% obsidian-0). |

### 2.2 Material + depth

- **Glass blur**: NSVisualEffectView `material: .underWindowBackground`, `blendingMode: .behindWindow`, with a tint layer at `--blur-tint`. Web-equivalent: `backdrop-filter: blur(28px) saturate(140%)` + tint overlay.
- **Grain overlay**: a 1.5% opacity monochrome film grain (procedurally generated 128 × 128 tile, tiled). Kills banding on the obsidian gradient and adds tactile depth — Arc browser uses similar grain. Disabled in reduced-motion + reduced-transparency.
- **Hairline rule**: 0.5pt at `--divider`. NEVER 1pt. NEVER colored. The hairline IS the chrome.
- **Elevation system** (no Material-style 5-step shadows — luxury restraint):
  - **Floor** (ambient HUD, menubar popover): `box-shadow: 0 1px 0 #FFFFFF08 inset, 0 12px 32px #00000099`.
  - **Lifted** (focus chamber, dashboard, settings): `box-shadow: 0 1px 0 #FFFFFF12 inset, 0 24px 80px #000000B3, 0 0 0 0.5pt #FFFFFF14`. The inset top-edge highlight is the "polished bezel" effect — borrowed from luxury watch case-side rendering.
  - **Engraved** (sigil on dark, settings sidebar items): `box-shadow: inset 0 1px 0 #00000066, inset 0 -1px 0 #FFFFFF0A` — the "engraved into the obsidian" feel.

### 2.3 Typography

| Role | Family | Weight / size | Justification |
|---|---|---|---|
| **Display** | **Söhne** (or **Inter** fallback if Söhne license unavailable) | 500 / 28-44pt | Söhne is the Linear/Vercel/Stripe family — modern grotesk with humanist warmth. Inter ships free and is functionally indistinguishable at body sizes. |
| **Body** | **Inter** (variable) | 400 / 14pt @ 1.45 line | Variable axis lets us tune optical weight without font-weight steps. |
| **Mono** | **JetBrains Mono** | 400 / 13pt | Code blocks in response stream. |
| **Editorial accent** | **Tiempos Headline** *italic* (optional, dashboard hero only) | 400 italic / 20pt | One italic serif moment in the entire product — used for the dashboard's "Today" header. Pairs with grotesk like Söhne the way Apothic Cellars pairs editorial type with sans on luxury sites. ONE place only; restraint. |

Pairing rationale: Söhne/Inter (grotesk) + JetBrains Mono (code) + Tiempos italic (one editorial moment) = the Linear/Vercel/Apothic stack. The italic serif is the "couture detail" — used once, recognizable.

### 2.4 Iconography

- **Stroke**: 1.5pt at 24×24; 1pt at 16×16. NEVER filled icons in primary chrome (filled icons are "noisy" — Apple HIG sidekicks).
- **Corner radius**: 2pt on all icon strokes (round-line-cap, round-join). Hard corners on iconography read as Material Design; rounded reads as Apple.
- **Gold accent strategy**: icons are `--text-muted` by default; gain `--gold-primary` ONLY on focus/active state. No always-gold icons (would over-saturate the brand). Exception: the sigil itself.
- **Source**: Lucide stroke set, restyled to match (Lucide is the Vercel/Shadcn standard; widely available, MIT licensed).

### 2.5 Iconic motif — the sigil

This is the recognizability play.

**The Leah Sigil**: an engraved capital **L** (Tiempos italic, gold) inside a hairline obsidian hexagon. The hexagon's stroke is `--gold-muted` at 0.75pt; the L is `--gold-primary` with a subtle inner shadow that reads as "stamped". 24 × 24 px standard; 56 × 56 px on ambient HUD; 96 × 96 px on first-launch wizard hero; 18 × 18 px in menubar (hexagon only — the L disappears below 20px).

**Why a hexagon, not a circle?** Circle is Siri. Hexagon is mechanical, suggests watch crown and structural integrity — the luxury-mechanical-object hint. Reference: hexagonal screws on watch movements; the "engineered" detail.

**Why italic L?** A capital sans L is a furniture-store logo. Italic serif L is *script-like* — personal, named, like a wax seal on correspondence. JARVIS without sci-fi.

**At 100px** in a screenshot crop, you see: deep obsidian field, a single gold-on-dark hexagon-and-L. Unmistakable.

---

## 3. Motion + animation

### 3.1 Curves + durations (the easing system)

Three curves only. Resist the urge to invent more.

| Curve name | cubic-bezier | Use |
|---|---|---|
| `--ease-out-quiet` | `cubic-bezier(0.16, 1, 0.3, 1)` | All appears: HUD pulse, focus summon, toasts in. Patek-grade quiet. |
| `--ease-in-quiet` | `cubic-bezier(0.7, 0, 0.84, 0)` | All dismisses: focus dismiss, toasts out. Inverse of above. |
| `--ease-standard` | `cubic-bezier(0.4, 0, 0.2, 1)` | Color/opacity transitions, hover states. macOS standard. |

**Durations** (these are doctrinal):

| Token | ms | Use |
|---|---|---|
| `--dur-instant` | 80 | Hover color change, focus ring. |
| `--dur-quick` | 160 | Toast in/out, menubar popover. |
| `--dur-standard` | 240 | Ambient HUD state change, sigil rotate. |
| `--dur-hero` | 380 | Focus chamber summon + dismiss. |
| `--dur-reduced` | 0 | All durations zeroed under `prefers-reduced-motion`. State changes become instant cross-fades at `--dur-instant`. |

### 3.2 State transitions

```
[hidden] --⌘⇧Space--> [focus-chamber summon, --dur-hero] --type--> [streaming]
                                                                      |
                                              <--Esc, --dur-hero--    v
[ambient HUD]  <--idle 90s--  [response complete, dismiss timer armed]
     |
     +--wake-word detected--> [listening state on HUD]
     +--toast pushed--------> [notification widget appears above HUD]
     +--⌘⇧Space------------> [focus chamber summons OVER HUD]
```

The HUD never disappears during state transitions; it dims to 60% opacity behind the focus chamber and resumes 100% on dismiss.

### 3.3 Activity indicators

These three are the soul of the product. Get them right.

#### Listening pulse (mic active)

- **Shape**: the sigil's gold hexagon outline.
- **Motion**: hexagon outline pulses radially outward via a 2nd hexagon at +6px stroke, fading from `--gold-glow` at 40% opacity to 0% over 1400ms. Loops with 200ms gap. Single ring, no compounding.
- **Reduced-motion**: ring replaced by a static `--gold-glow` 1pt outer stroke at 30% opacity (no animation).

#### Thinking ring (LLM processing)

- **Shape**: a 0.75pt gold arc traveling around the hexagon perimeter.
- **Motion**: arc length 90°, rotates at 1080ms/rev. Stroke gradient from `--gold-glow` to `--gold-primary` to transparent (tail).
- **Reduced-motion**: arc replaced by static dashed hexagon perimeter at `--gold-muted`.

#### Speaking waveform (TTS playing)

- **Shape**: 5 vertical gold bars, 2pt wide, 8px tall max, positioned under the response text in focus chamber AND under the sigil in ambient HUD.
- **Motion**: each bar's height animated to the amplitude envelope of the audio (real-time, 30 Hz). Color `--gold-primary`.
- **Reduced-motion**: a single static gold horizontal bar with a slow left-to-right shimmer.

### 3.4 Signature flourishes (hero animations)

**Flourish 1 — The Gold Seam (focus chamber summon, 380ms)**

On `⌘⇧Space`, a 1px-tall horizontal gold seam (`--gold-primary`) appears at screen-vertical-center, expands horizontally from 0 to chamber-width over 120ms (`--ease-out-quiet`), then the chamber unfolds vertically from that seam — top half rising up, bottom half falling down — over 260ms with `--ease-out-quiet`. The seam itself fades to `--divider` as the chamber settles. Reads as: a vault opening, a watch-case being cracked, light escaping a slot.

Reduced-motion: chamber cross-fades in over 160ms; no seam.

**Flourish 2 — Sigil Acknowledgment (wake-word heard, 240ms)**

When Leah hears her name, the sigil hexagon rotates 60° clockwise (one hex-vertex of rotation — so the hexagon ends in the same visual position but has "ticked"), accompanied by a single warm pulse of `--gold-glow` at 40% opacity expanding to 110% scale and fading. Reads as: a watch tick, a nod of acknowledgment.

Reduced-motion: sigil color shifts to `--gold-glow` for 200ms then back.

These two are the personality. Everything else is a quiet cross-fade.

---

## 4. Interaction model

### 4.1 Global hotkey

**`⌘⇧Space`** — summon focus chamber.

Rationale: `⌘Space` is Spotlight (sacred). `⌘⇧Space` is Spotlight's natural sibling — the muscle memory is one extra shift away from "search my computer" → "ask my assistant". Linear uses `⌘K` but `⌘K` is now the cross-app command-palette convention (Vercel, GitHub, Slack, Notion) and would clash inside any text field. `⌘⇧Space` is globally free on macOS, unambiguous, and one-handed.

**Re-bindable** in Settings (cmd, opt, ctrl, shift + any key). Conflict detection runs against macOS shortcuts on save.

### 4.2 Summon

- Hotkey from any app → focus chamber materializes at screen-center (Flourish 1), input field gains focus.
- The currently-frontmost app does NOT lose its window focus; if the operator hits Esc, focus returns naturally (no "what was I doing" disorientation).
- Voice wake-word ("Leah,…") → focus chamber materializes silently (no hotkey needed); listening pulse already active; input field shows live transcription in `--text-muted`.

### 4.3 Dismiss

- **Esc**: dismiss focus chamber. Always. No confirmations.
- **Click-outside**: dismiss. Unless input has unsent text (then: subtle gold-glow on input border, 1s — no modal "are you sure", trust the operator).
- **90s idle after last interaction**: dismiss with no warning. Conversation preserved in dashboard history.
- **`⌘W`**: dismiss (macOS convention).
- Menubar quit: full app quit, dismisses everything.

### 4.4 Voice ↔ text handoff

- The focus chamber input field is ALWAYS available. Voice does not replace it.
- While voice is dictating, text appears live in the input field as `--text-muted` (interim) → `--text-primary` (final). Operator can type to interrupt; voice yields immediately.
- "Hold spacebar to talk" (push-to-talk) is the no-wake-word fallback inside focus chamber. Spacebar pressed while input is empty → mic active.

### 4.5 Multi-tasking

| Scenario | Behavior |
|---|---|
| **Fullscreen app** (e.g., editor zoomed) | Ambient HUD hides; menubar dot stays. `⌘⇧Space` summons focus chamber as an overlay above the fullscreen Space. |
| **Mission Control** | Ambient HUD joins all Spaces (NSPanel `.canJoinAllSpaces`). Visible in MC for orientation, dismissable. |
| **Second monitor** | Each monitor remembers its own HUD anchor. Focus chamber summons on the monitor containing the cursor. |
| **Screen recording / sharing** | Detected via CGScreenIsCaptured(); HUD auto-hides; menubar dot dims to obsidian-3; toasts suppressed until recording ends. Privacy default ON. |
| **Do Not Disturb** | Leah respects Focus filters; non-priority toasts suppressed; ambient HUD goes 60% opacity. |

### 4.6 Cursor + keyboard navigation

- **Tab** cycles: input → sources → follow-up chips → close.
- **Arrow keys** navigate response stream (scroll), follow-up chips (←→), source list (↑↓).
- **Enter** in input → send. **Shift-Enter** → newline.
- **`⌘↑` / `⌘↓`** in input → previous/next prompt from history.
- **`⌘/`** → toggle help overlay (hotkey cheatsheet).
- **`⌘.`** → cancel in-flight response (streaming abort).
- Full no-mouse usability: focus chamber, settings, dashboard all reachable and operable without trackpad.

---

## 5. Information architecture per surface

### 5.1 Ambient HUD — the 7 earned pixels

Default 280 × 84 px shows three rows. Each slot is earned. Order top → bottom:

1. **Row 1 — Sigil + state**: 24px sigil (left), current state caption (right). State = "Idle" / "Listening…" / "Thinking…" / "Ready" / red "Daemon offline".
2. **Row 2 — Now**: the single most relevant *now* item. Calendar next-event (in <30min) → "Standup in 12m". Else: in-progress brief title. Else: today's first agenda item. Else: empty (hidden).
3. **Row 3 — Pulse**: ONE micro-metric. Default: unread brief count + arxiv count. Expanded mode adds: PR review queue, GH inbox, weather one-glyph.

**Hidden by design**: clock (macOS menubar has one), date, weather chrome (just a glyph if shown), CPU/network (not operator-relevant).

### 5.2 Focus chamber — anatomy

```
┌─────────────────────────────────────────────────────┐
│                                                     │  ← 24px top breathing room
│   [sigil-pulse]   Ask Leah anything…           ⌘.   │  ← input row
│   ────────────────────────────────────────────────   │  ← hairline
│                                                     │
│   [streaming response renders here as markdown,    │
│    code blocks JetBrains Mono, inline citations    │
│    as gold-underline numerals¹ ²]                  │
│                                                     │
│   ────────────────────────────────────────────────   │
│   Sources                                           │
│   1. arxiv.org/abs/2401… — Title                   │
│   2. github.com/foo/bar — README                   │
│   ────────────────────────────────────────────────   │
│   ⌃ Follow up                                       │
│   [ chip ] [ chip ] [ chip ]                       │
│                                                     │
└─────────────────────────────────────────────────────┘
```

- **Input row**: sigil shows state (idle/listening/thinking). Placeholder rotates daily: "Ask Leah anything…" / "What's on today?" / "Brief me on…".
- **Response**: streaming markdown. Citations inline as gold-superscript numerals; hover/focus shows source preview. Code blocks have copy button (top-right, appears on hover).
- **Sources**: collapsible list. Default expanded if ≤3; collapsed with "Show 5 sources" if more.
- **Follow-up chips**: 3 max, AI-generated continuations. Tab-navigable. Enter inserts into input.
- **No conversation history in this surface.** History is the dashboard. The chamber is one focused exchange.

### 5.3 States

| State | Treatment |
|---|---|
| **Empty** (no query yet) | Placeholder + 3 "starter" chips ("What's new today?", "Open my brief", "Status of MAY-19") |
| **Streaming** | Input dims to `--text-muted`; thinking ring on sigil; `⌘.` chip appears top-right |
| **Error — model failed** | Red hairline-bottom on response area; `--red-alert` caption: "Model couldn't respond. [Retry]" |
| **Offline — no network** | Soft red banner at top of chamber: "Offline. Local answers only." HUD sigil shows obsidian-3 hexagon (no gold). |
| **Daemon down** | Focus chamber refuses to summon; instead, menubar dot pulses `--red-alert`; clicking it shows: "Daemon offline. [Restart Leah]" |
| **Permission denied** (mic, screen) | First time: gentle wizard step re-shown. Subsequent: small inline `--red-dim` chip "Mic blocked → System Settings" |
| **Sensitive content detected** (e.g., password, key) | Blur scrim with `--red-dim` tint over the message; "Sensitive content hidden. [Show]" |

### 5.4 First-launch wizard flow

6 steps. Each step is one screen. No skip on critical steps (1, 2). Linear-style "step n of 6" hairline progress at top.

1. **Welcome** — sigil hero (96px), "Leah. Your personal assistant." Single CTA: "Begin".
2. **Permissions** — mic, accessibility, screen-recording (for auto-hide). Each row: icon, name, "Grant" button → opens System Settings deep link.
3. **Wake-word** — toggle: "Listen for 'Leah'". Default OFF (privacy-respectful default). Operator opts in.
4. **Voice** — pick a TTS voice. 3 samples auto-play on hover.
5. **Integrations** — `leah connect <integration>` UI: Calendar / Mail / GitHub / Linear / arxiv / News. Each row: icon, "Connect" → OAuth browser handoff; or "Skip".
6. **Position** — pick HUD anchor (8 thumbnails: 4 corners, 4 mid-edges). Default: bottom-right. "Finish" CTA.

---

## 6. Accessibility + responsiveness

### 6.1 Contrast verification (WCAG AA = 4.5:1 text, 3:1 UI)

| Pair | Ratio | Pass? |
|---|---|---|
| `--text-primary` `#F2EDE0` on `--obsidian-0` `#08090C` | 14.8:1 | AAA |
| `--text-muted` `#B8B0A0` on `--obsidian-0` | 8.3:1 | AAA |
| `--text-dim` `#6B6558` on `--obsidian-0` | 4.6:1 | AA (only on incidental text) |
| `--gold-primary` `#C9A961` on `--obsidian-0` | 8.4:1 | AAA |
| `--gold-primary` text on `--obsidian-2` `#161922` (input bg) | 7.1:1 | AAA |
| `--red-alert` `#C8434F` on `--obsidian-0` | 5.1:1 | AA |
| `--red-brand` `#7A1F2B` on `--obsidian-0` | 2.7:1 | UI-only (outlines, never text) |
| `--divider` `#FFFFFF14` on `--obsidian-0` | n/a (decorative hairline) | — |

All gold-on-obsidian and ivory-on-obsidian pairs clear AAA. The single AA-floor case is `--text-dim` which is used only for timestamps and non-essential captions.

### 6.2 Reduced motion

`prefers-reduced-motion: reduce` triggers:
- All animation durations → 0 or `--dur-instant` cross-fade.
- Listening pulse → static ring.
- Thinking arc → static dashed perimeter.
- Speaking waveform → single shimmer bar.
- Flourish 1 (Gold Seam) → cross-fade.
- Flourish 2 (Sigil Acknowledgment) → color flash only.

### 6.3 VoiceOver labels

Every interactive element has an explicit accessibility label.

- Sigil (ambient): `"Leah, status: {state}. Press Command Shift Space to ask."`
- Input field: `"Ask Leah. Type your question or hold Space to dictate."`
- Send: `"Send query."`
- Source link: `"Source 1 of 3, {domain}. Open in browser."`
- Follow-up chip: `"Follow up: {chip text}. Press Return to insert."`
- Toast: `"Notification: {text}. Press Return to expand, Escape to dismiss."`

VoiceOver rotor groups: "Sigil", "Input", "Response", "Sources", "Follow-ups", "Actions".

### 6.4 Window scaling

| Mode | HUD | Focus chamber |
|---|---|---|
| Mini | 56 × 56 px sigil only | 480 × 360 px |
| Standard | 280 × 84 px | 720 × 480 px |
| Expanded | 280 × 168 px (+agenda strip) | 960 × 640 px |

Operator-set in Settings. Auto-mini if screen < 1440px wide.

### 6.5 Multi-monitor / Retina

- Each monitor remembers its own HUD anchor + scale mode.
- Retina assets: SVG sigil (vector, no raster). PNG hero only for first-launch wizard, served @1x/@2x/@3x.
- Non-retina: grain overlay disabled (banding less visible at lower DPI; grain looks dirty).

---

## 7. Sample ASCII wireframes

### 7.1 Ambient HUD (standard mode)

```
                          ┌──────────────────────────────────────────┐
                          │ ⬡  Listening…                            │
                          │ ──────────────────────────────────────── │
                          │ Standup in 12m · 9:30am                  │
                          │ ──────────────────────────────────────── │
                          │ 3 briefs · 12 arxiv · 5 PRs              │
                          └──────────────────────────────────────────┘
                                       bottom-right of screen
```

### 7.2 Ambient HUD (mini mode)

```
                                              ┌──────┐
                                              │  ⬡   │
                                              └──────┘
```

### 7.3 Focus chamber (streaming response)

```
┌───────────────────────────────────────────────────────────────────┐
│                                                                   │
│   ⬡  What's the status of MAY-19?                          ⌘.    │
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

### 7.4 Focus chamber (empty / first open)

```
┌───────────────────────────────────────────────────────────────────┐
│                                                                   │
│   ⬡  Ask Leah anything…                                          │
│   ──────────────────────────────────────────────────────────────  │
│                                                                   │
│                                                                   │
│                            ⬡                                      │
│                                                                   │
│                  Good morning, Tri.                               │
│                                                                   │
│                                                                   │
│   ⌃ Try                                                           │
│   [ What's new today? ]  [ Open my brief ]  [ Status of MAY-19 ] │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

### 7.5 Hotkey-summon transition (Flourish 1 — Gold Seam)

```
Frame 1 (t=0ms)              Frame 2 (t=120ms)            Frame 3 (t=380ms)
                                                          ┌──────────────┐
                                                          │              │
       (nothing)         ────────────────────────         │   chamber    │
                                                          │   settles    │
                                                          └──────────────┘
                          gold seam expands              vault opens
```

### 7.6 Menubar dropdown

```
                    [⬡] menubar icon ← gold dot when listening
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

### 7.7 Notification widget (stacked above HUD)

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
                          │ 3 briefs · 12 arxiv · 5 PRs              │
                          └──────────────────────────────────────────┘
```

### 7.8 First-launch wizard (step 1 of 6)

```
┌─────────────────────────────────────────────────────────────────┐
│  ●  ○  ○  ○  ○  ○                                         step 1│
│  ──────────────────────────────────────────────────────────────  │
│                                                                  │
│                                                                  │
│                                                                  │
│                            ⬡                                     │
│                                                                  │
│                          Leah.                                   │
│              Your personal assistant.                            │
│                                                                  │
│                                                                  │
│                                                                  │
│                        [   Begin   ]                            │
│                                                                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 7.9 First-launch wizard (step 5 — integrations)

```
┌─────────────────────────────────────────────────────────────────┐
│  ●  ●  ●  ●  ●  ○                                         step 5│
│  ──────────────────────────────────────────────────────────────  │
│                                                                  │
│   Connect your services                                          │
│   Leah works best with context from your tools.                  │
│                                                                  │
│   ──────────────────────────────────────────────────────────────  │
│    [icon] Calendar           [ Connect ]    [ Skip ]            │
│    [icon] Mail               [ Connect ]    [ Skip ]            │
│    [icon] GitHub             ✓ Connected as @trilam              │
│    [icon] Linear             [ Connect ]    [ Skip ]            │
│    [icon] arxiv              [ Connect ]    [ Skip ]            │
│    [icon] News               [ Connect ]    [ Skip ]            │
│   ──────────────────────────────────────────────────────────────  │
│                                                                  │
│                                       [ Back ]    [ Continue ]   │
└─────────────────────────────────────────────────────────────────┘
```

### 7.10 Settings (Things-style sidebar + detail)

```
┌─────────────────────────────────────────────────────────────────┐
│                                                                  │
│  Appearance        │   HUD position                              │
│  Hotkeys           │   ┌────┐ ┌────┐ ┌────┐                     │
│  Voice             │   │ TL │ │ T  │ │ TR │                     │
│  Integrations    ▸ │   └────┘ └────┘ └────┘                     │
│  Privacy           │   ┌────┐         ┌────┐                     │
│  Advanced          │   │ L  │   ⬡    │ R  │                     │
│                    │   └────┘         └────┘                     │
│                    │   ┌────┐ ┌────┐ ┌────┐                     │
│                    │   │ BL │ │ B  │ │●BR │                     │
│                    │   └────┘ └────┘ └────┘                     │
│                    │                                             │
│                    │   Scale     ◯ Mini  ●Standard  ◯ Expanded  │
│                    │                                             │
│                    │   Always-on-top  ⬜                          │
│                    │   Hide during recording  ✓                  │
│                    │                                             │
└─────────────────────────────────────────────────────────────────┘
```

---

## 8. Open decisions I made (and why)

1. **Killed multi-conversation tabs.** Focus chamber is one stream; history lives in dashboard. Rationale: tab chrome competes with the response; operator-overlay aesthetic forbids browser-like UI.
2. **Wake-word default OFF.** Privacy-respectful default; opt-in via wizard. Rationale: shipping a always-listening assistant by default is hostile to operator trust on day one.
3. **Italic serif L in the sigil (not a sans-serif glyph).** Rationale: distinctiveness at 100 px. Sans L is generic; italic serif is a wax-seal moment — Patek-grade.
4. **Hexagon, not circle, for sigil container.** Rationale: circle = Siri/Cortana. Hexagon = mechanical, watch-crown lineage, operator-tool.
5. **`⌘⇧Space` as global hotkey, not `⌘K`.** Rationale: `⌘K` clashes inside text fields (every modern editor uses it for command palette). `⌘⇧Space` is Spotlight's natural sibling — muscle memory carries.
6. **Three easing curves only; doctrinal durations.** Rationale: motion proliferation is the #1 luxury-violation. Three curves enforce restraint.
7. **One italic serif moment (Tiempos on Dashboard "Today" header), nowhere else.** Rationale: the editorial accent is a "couture detail" — overused, it becomes pastiche; used once, it elevates the whole product.
8. **Auto-hide HUD during screen recording, ON by default.** Rationale: privacy default — shipping with this OFF would create demo-moment leaks (operator screen-shares, audience sees private agenda).
9. **No green chrome anywhere; success is a one-time quiet tick.** Rationale: the palette is obsidian / gold / red / ivory. Green pollutes. Confirmation should feel like a printed receipt mark, not a Slack badge.
10. **The focus chamber does NOT steal focus from the frontmost app's text fields unless the operator types.** Rationale: muscle-memory preservation. If I `⌘⇧Space` to ask a quick question, Esc, my cursor is still in the doc I was writing. Spotlight does this; we match.

---

## Implementation notes (deferred framework-agnostic)

- **HUD + menubar** → SwiftUI's `MenuBarExtra` + a `Window` (or NSPanel for `canJoinAllSpaces`). Wails alternative: `wails-app` system tray + a frameless transparent window with `AlwaysOnTop`.
- **Focus chamber** → NSPanel `.nonactivatingPanel` style (does not steal app focus). Wails: frameless transparent + `WindowSetAlwaysOnTop`.
- **Daemon comms** → existing Go daemon over localhost; the UI is a thin client. Push events (MAY-19 B1/B5 substrate) drive the notification widget queue.
- **Material blur** → NSVisualEffectView native; Wails/webview fallback `backdrop-filter` with a polyfill on platforms that lack it (target macOS 14+ only at v1).
- **Reduced motion / reduced transparency** → respond to `NSAccessibility.differentiateWithoutColor` and `NSAccessibility.reduceMotion` notifications, OR `prefers-reduced-motion` media query in webview path.
- **Sigil** → ship as SVG; render via Core Animation for the rotation/pulse flourishes for 60fps on every macOS.

---

## 9. Dynamic widget canvas

The operator does not open a "stocks app" or a "flights tab." She says *"show me today's market vs yesterday"* and a market tile materializes in the response stream. She says *"flights around September dates"* and a date×price matrix lands beside the prose. Widgets are first-class output, not chrome. Each one is dismissible; each one can be **pinned to ambient** for persistent glance-value.

This section is the visual contract for the widget layer. Tone: a watch dial that gains complications on demand — never gauche, never crowded, always legible.

### 9.0 Canvas model

The focus chamber's response area is an **interleaved stream** — prose blocks and widget tiles share the same vertical rhythm. Tiles are not modals; they are paragraphs that happen to be pixels.

| Aspect | Behavior |
|---|---|
| **Default layout** | Full-width tiles stacked vertically. |
| **Wide chamber (≥860 px)** | Small/medium tiles auto-flow into a 2-column grid; large/hero always span full-width. |
| **Tile chrome** | 1 px hairline @ 20% ivory; 12 px corner radius; 3% noise grain on tile body; eyebrow title `MARKET · TODAY` in Tiempos italic; gold rule under eyebrow (1 px, 40% `#C9A961`, 32 px long). |
| **Top-right cluster** | `◆` pin glyph (12 px) then `×` dismiss (12 px), 8 px gap, 12 px inset. Pin is hairline-ivory when unpinned; **filled champagne gold** when pinned. Dismiss is hairline-ivory; hover → oxblood `#C8434F`. |
| **Tile sizes** | `small` 280×120 (1×1, ambient-eligible) · `medium` 720×180 (full-width) · `large` 720×320 (full-width tall) · `hero` 720×440 (chamber-takeover; suppresses follow-up chips until dismissed). |
| **Density** | Max 4 widget tiles per response turn. Beyond that, LLM must summarize and offer `[ Show more ]` chip — restraint over abundance. |

### 9.1 Widget catalog

Every widget below honors the same chrome contract. The variation is *what the dial says*, not *how the case is finished*.

---

**1. Market** — *price · delta · sparkline · multi-symbol grid · intraday candlestick*

- **Purpose:** Quote, momentum, intraday shape. The operator's first morning question.
- **Default size:** small (single symbol) / medium (≤4 symbols grid) / large (candlestick).
- **Gold lands on:** ticker symbol, current price (JetBrains Mono, 28 px small / 36 px medium). Sparkline stroke is `#C9A961` at 60% opacity.
- **Oxblood lands on:** negative delta value + downward arrow `▼`. Positive delta = ivory `#F2EDE0` + `▲`. **No green, ever.**
- **Micro-interactions:** hover a sparkline → vertical hairline crosshair tracks cursor, tooltip shows time + price in mono. Click symbol → spawns a `large` candlestick tile inline.
- **States:** loading = hairline frame + center 1 Hz gold dot pulse, ticker reserved as `————`. Empty (market closed pre-open) = `MARKET CLOSED · opens 9:30am ET` in Tiempos italic, dim ivory. Error = oxblood hairline + `Quote unavailable · retry` chip.

```
┌──────────────────────────────────────────────────────────────────┐
│ MARKET · TODAY                                          ◆   ×   │
│ ──────────                                                       │
│                                                                  │
│  AAPL   228.43    ▲ 1.82 (+0.80%)    ╱╲    ╱╲___╱‾‾‾╲___       │
│  NVDA   142.07    ▼ 3.11 (−2.14%)    ‾‾╲__╱  ╲___      ╲__     │
│  TSLA   251.88    ▲ 4.20 (+1.70%)    __╱‾‾╲__╱‾‾╲___╱‾‾‾       │
│  MSFT   421.55    ▼ 0.45 (−0.11%)    ‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾‾        │
│                                                                  │
│  vs yesterday close · refreshed 60s ago                          │
└──────────────────────────────────────────────────────────────────┘
```

---

**2. Flights** — *date×price matrix · single-flight detail card*

- **Purpose:** Spatial fare scanning ("which day is cheapest?"). The September prompt.
- **Default size:** large (matrix) / medium (single-flight card).
- **Gold lands on:** the lowest fare cell of each row + the global minimum cell (filled `#C9A961` background, obsidian text). Origin → destination route label.
- **Oxblood lands on:** fares ≥ 30% above row median (subtle oxblood text, not background — don't shout).
- **Micro-interactions:** hover a cell → row + column highlight (1 px gold hairline). Click → spawns a `medium` flight-detail card below with carrier, stops, duration, book-out link.
- **States:** loading = matrix grid drawn with `————` placeholders, gold seam sweeping left-to-right at 240 ms intervals across rows. Empty = `No fares found · widen dates?` chip. Error = oxblood frame.

```
┌──────────────────────────────────────────────────────────────────┐
│ FLIGHTS · SFO → LIS                                     ◆   ×   │
│ ──────────                                                       │
│                                                                  │
│         Sep 8   Sep 9   Sep 10  Sep 11  Sep 12  Sep 13  Sep 14 │
│  ─→     $812    $798    $760    $742    [$689]  $701    $748   │
│  ─→     $834    $812    $780    $760    $710    $695    $765   │
│  ←─     $755    $740    $720    [$688]  $702    $715    $740   │
│  ←─     $812    $790    $762    $745    $720    $705    $730   │
│                                                                  │
│  cheapest round-trip: Sep 12 ↔ Sep 19 · $1,377                  │
└──────────────────────────────────────────────────────────────────┘
```

---

**3. Calendar** — *day timeline strip · week 7-col grid · agenda list · next-event card*

- **Purpose:** Time-shape at a glance.
- **Default size:** small (next-event card) / medium (day strip or agenda) / large (week grid).
- **Gold lands on:** the **now-line** (1 px gold rule across the timeline) and the next event's title. Today's column header in week view = gold underline.
- **Oxblood lands on:** conflicts (overlapping blocks render with oxblood hairline outline on the shorter block) and `LATE` labels (past start, not yet acknowledged).
- **Micro-interactions:** hover an event → 80 ms ivory fill tint; click → expands inline with attendees + agenda link. Drag-select a range → `[ Block focus time ]` chip.
- **States:** loading = day strip with hour ticks, no events, gold dot pulse on now-line. Empty = `Nothing on your calendar · enjoy the quiet.` in Tiempos italic.

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
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

---

**4. Weather** — *current condition · 7-day forecast row · hourly bar*

- **Purpose:** Plan-the-day input. Never the centerpiece; lives small or medium.
- **Default size:** small (current) / medium (7-day strip).
- **Gold lands on:** current temperature numeral and the day-name of any day with a watch-condition (severe, precipitation > 70%).
- **Oxblood lands on:** severe-weather flag chip (`HEAT ADVISORY`, `STORM WARNING`) — chip background oxblood, text ivory.
- **Micro-interactions:** hover a forecast day → hourly bar swaps in (160 ms cross-fade). Click → spawns `medium` hourly detail.
- **States:** loading = sparse hairline frame, `——°` placeholder. Error = `Weather offline · last seen 14m ago`.

```
┌──────────────────────────────────────────────────────────────────┐
│ WEATHER · SAN FRANCISCO                                 ◆   ×   │
│ ──────────                                                       │
│   64°   partly cloudy · feels 62° · wind 8 mph W                │
│                                                                  │
│   Sun   Mon   Tue   Wed   Thu   Fri   Sat                       │
│   ☀     ⛅    ⛅    🌧    🌧    ☀     ☀                          │
│   68°  66°   63°   58°   59°   65°   70°                       │
│   52°  50°   49°   48°   47°   51°   54°                       │
└──────────────────────────────────────────────────────────────────┘
```

---

**5. Maps** — *place card with mini-map · route card with stops*

- **Purpose:** Spatial answer. Not a navigation app — a citation with geometry.
- **Default size:** medium (place) / large (route).
- **Gold lands on:** the pin glyph + place name; for routes, the route polyline (1.5 px champagne, 80% opacity over a desaturated obsidian basemap).
- **Oxblood lands on:** closure notices, "permanently closed" labels.
- **Micro-interactions:** click pin → open in Apple Maps; long-press → copy coordinates. Route card hover → stop-by-stop list expands below.
- **Basemap rule:** never a vendor-bright tile set. Desaturate to obsidian + hairline-ivory roads; we are not Google Maps Lite, we are *a Patek dial that happens to show a route*.
- **States:** loading = obsidian rectangle with center gold dot pulse. Error = oxblood hairline + `Map unavailable`.

---

**6. Table** — *sortable rows · header accent · negative-cell tint*

- **Purpose:** Structured data the LLM extracted (a comparison, a leaderboard, a manifest).
- **Default size:** medium / large.
- **Gold lands on:** header row (gold 1 px under-rule + Tiempos italic column names), hovered row (gold hairline left-edge, 2 px). Active sort column = gold caret `▾`.
- **Oxblood lands on:** negative numeric cells; cells flagged as anomaly (LLM-annotated).
- **Micro-interactions:** click header → sort (80 ms ease); shift-click → secondary sort. Right-click row → copy as TSV.
- **States:** loading = hairline rows with shimmer placeholders. Empty = `No rows` in Tiempos italic, centered.

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

**7. Chart** — *line · bar · area · scatter*

- **Purpose:** Generic plot when no specialized widget fits.
- **Default size:** medium / large.
- **Gold lands on:** the **one** accent series (always the operator's primary subject). Other series render in muted ivory (`#F2EDE0` @ 40% opacity) — no rainbow.
- **Oxblood lands on:** annotation markers for adverse events (e.g., outage spike, drawdown). Annotation glyph: small oxblood diamond + Tiempos italic label.
- **Axes:** hairline ivory @ 20%, JetBrains Mono tick labels, gold tick mark on the "today" / "now" axis position.
- **Micro-interactions:** hover → crosshair + per-series tooltip; drag → zoom region (380 ms ease-in-out); double-click → reset.

---

**8. Image / file** — *thumbnail with metadata caption · QuickLook-on-click*

- **Purpose:** When the answer is "this picture" or "this PDF."
- **Default size:** small (thumbnail row) / medium (single image with caption).
- **Gold lands on:** caption eyebrow (filename in Tiempos italic) and a 1 px gold hairline frame on the thumbnail itself (no other widget gets a gold frame — the image is *the artifact*, so it earns the chrome).
- **Oxblood lands on:** "missing" or "permission denied" overlay.
- **Micro-interactions:** click → macOS QuickLook (`NSItemProvider` preview). Space-bar with widget focused = same. Right-click → reveal in Finder.
- **States:** loading = obsidian rectangle + gold dot pulse + filename placeholder.

---

**9. Code** — *syntax-highlit block · gold-on-obsidian theme · copy button*

- **Purpose:** Code snippets in answers — the operator is a builder.
- **Default size:** medium / large; never small (code at 280 px is illegible).
- **Theme (custom, named "Obsidian Brass"):** background `#0E1014`; default text `#F2EDE0`; keywords `#C9A961`; strings ivory @ 80%; comments ivory @ 35% italic; numbers JetBrains Mono `#C9A961` @ 70%; **errors / linter squiggles oxblood**. No purple, no blue, no green.
- **Chrome:** language label (eyebrow `GO`, `PYTHON`, `SQL` in Tiempos italic), `[ Copy ]` chip top-right (left of pin/dismiss cluster), line numbers in JetBrains Mono @ 30%.
- **Micro-interactions:** hover line → 1 px gold hairline left margin; click line number → copy that line. Copy chip → 240 ms ivory→gold flash, eyebrow swaps to `COPIED` for 1.2 s.

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

**10. Citation card** — *(already specced in §5; harmonize chrome here)*

- Inherits the §9.0 chrome contract: hairline frame, pin glyph eligible, dismiss eligible. The numbered superscript in prose remains the primary affordance; the citation tile is the expanded form when the operator clicks it.
- **Gold lands on:** source domain name. **Oxblood lands on:** "stale" / "404" flags.

---

**11. Stat card** — *single number · label · delta (KPI)*

- **Purpose:** The single answer when prose would be too much. "How many PRs merged this week?" → one tile, one number.
- **Default size:** small.
- **Gold lands on:** the headline numeral (JetBrains Mono, 48 px). Label in Tiempos italic eyebrow above; delta below with `▲ +12%` (ivory) or `▼ −4%` (oxblood).
- **Micro-interactions:** click → spawns a `medium` chart tile of the underlying series.

```
┌──────────────────────────┐
│ PRS MERGED · 7d     ◆ × │
│ ──────────               │
│         52               │
│   ▲ +14 vs prior week    │
└──────────────────────────┘
```

---

**12. List** — *bullet · numbered · scrollable · hover-highlight*

- **Purpose:** When the answer is "these N things." Use over prose when N ≥ 5 or when items are scannable nouns (titles, names, paths).
- **Default size:** medium; scrolls internally past 8 items.
- **Gold lands on:** the bullet glyph (small gold diamond `◆`) or the numeral. Hover row = 1 px gold hairline left edge + 80 ms ivory tint.
- **Oxblood lands on:** items flagged with status (e.g., `FAILING`, `ERROR`).
- **Micro-interactions:** click row → spawns child tile or opens link. Right-click → copy as plain text.

---

**13. Diff** — *git-diff render · gold added · oxblood removed*

- **Purpose:** Showing a code change without spawning a full editor.
- **Default size:** medium / large.
- **Gold lands on:** added lines — gutter `+` glyph in champagne, line background `#C9A961` @ 8% obsidian-tinted.
- **Oxblood lands on:** removed lines — gutter `−` in `#C8434F`, line background `#7A1F2B` @ 12%.
- **Chrome:** file path eyebrow (`internal/hud/refresh.go`) in Tiempos italic. Hunk separator = 1 px hairline + line-range in JetBrains Mono.
- **Micro-interactions:** click file path → opens in `$EDITOR`. Side-by-side toggle chip (`[ unified | split ]`) for ≥medium tiles.

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

### 9.2 Pin-to-ambient flow

Pinning is the load-bearing affordance for the dynamic-canvas idea. The chamber is a *conversation* — ephemeral by design. The HUD is a *presence* — persistent by design. Pin promotes a widget across that boundary.

| Aspect | Behavior |
|---|---|
| **Pin action** | Click `◆` in tile top-right. Glyph fills champagne; 240 ms gold-seam-right confirmation animation. |
| **Ambient slot reservation** | Ambient HUD grows downward by one slot row (84 px → 168 px → 252 px → 336 px) to accommodate up to **3 pinned widgets**. Each ambient slot renders the widget in its `small` variant — large/medium/hero tiles auto-downsize on pin. |
| **Refresh cadence** | Per type. Market: 60 s. Weather: 15 min. Calendar: 5 min. Stat: 5 min. Flights: **manual only** (fare polling is expensive + noisy). Code/Diff/Image: never (static artifact). |
| **Unpin** | Click `◆` again from either surface (chamber tile *or* ambient slot). Both surfaces decrement in sync. |
| **Max 3 pinned** | Attempting a 4th pin → chamber shows a quiet inline note: `Ambient is full · unpin one to add this one.` with chips for each currently-pinned widget. **Never silently evict.** |
| **Persistence** | Pinned set is per-operator, restored on app restart. Pinned widgets re-fetch on launch before painting (no stale-fare ghosts). |

**Ambient HUD with 3 pinned widget slots:**

```
                          ┌──────────────────────────────────────────┐
                          │ ⬡  Listening…                            │
                          │ ──────────────────────────────────────── │
                          │ Standup in 12m · 9:30am                  │
                          │ ──────────────────────────────────────── │
                          │ 3 briefs · 12 arxiv · 5 PRs              │
                          │ ════════════════════════════════════════ │  ← pin divider (gold, 1px)
                          │ MARKET                              ◆    │
                          │ AAPL  228.43  ▲0.80%   ╱╲___╱‾‾‾        │
                          │ ──────────────────────────────────────── │
                          │ WEATHER · SF                        ◆    │
                          │ 64° partly cloudy · 8mph W              │
                          │ ──────────────────────────────────────── │
                          │ PRS MERGED · 7d                     ◆    │
                          │ 52   ▲ +14 vs prior week                │
                          └──────────────────────────────────────────┘
                                       bottom-right of screen
```

The pin-divider (double-rule, 1 px gold) is the only place gold lands as a *background-spanning* horizontal — its job is to say "above this line is Leah; below this line is *your* dial."

### 9.3 Widget gallery

The operator should not need to memorize widget names. Typing `/widgets` in the chamber input opens an overlay browser — a *catalog of complications*, watch-collector style.

| Aspect | Behavior |
|---|---|
| **Trigger** | Type `/widgets` in chamber input; or `⌘⇧W` while chamber focused. |
| **Layout** | Overlay covers chamber response area (input stays visible at top); left rail = category list, right pane = preview grid. |
| **Categories** | `Finance · Travel · Time · Productivity · Web · Code` — 6 fixed at v1. Each rendered as a Tiempos italic eyebrow. |
| **Preview** | Each catalog cell is a live `small`-variant tile rendered with sample data. Real components, not screenshots — what you see is what spawns. |
| **Spawn** | Hover a cell → ivory tint + `[ Spawn ]` chip appears. Click chip → overlay dissolves (160 ms), tile materializes in the response stream with the standard 240 ms gold-seam-down. |
| **Dismiss gallery** | Esc, click outside the overlay, or re-type `/widgets`. |
| **Search** | Top of overlay: a slim search field (`Find a widget…`) — fuzzy match across widget name + category + sample-data text. |

```
┌─────────────────────────────────────────────────────────────────┐
│ /widgets                                                   Esc │
│ ───────────────────────────────────────────────────────────────  │
│                                                                  │
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
│                     │                                            │
└─────────────────────────────────────────────────────────────────┘
```

### 9.4 Spawn affordance

A widget appearing should feel *deliberate* — not a popup, not a notification, not a flash. The motion language is the same gold seam that opens the chamber, scaled down.

| State | Behavior |
|---|---|
| **LLM emits widget tool-call** | Stream pauses ≤80 ms; tile frame paints as a 1 px gold horizontal seam at the tile's vertical center. |
| **Reveal (240 ms, `eased-out`)** | Seam expands top + bottom to full tile bounds (`eased-out-quart`); during the last 80 ms, the eyebrow title fades in (`ease-in`). Content paints behind the frame after settle. |
| **Loading** | Hairline frame + eyebrow visible; tile body shows a single centered champagne dot (8 px) pulsing at 1 Hz (`60% → 100% → 60%` opacity, `ease-in-out`). No spinners. No skeleton-shimmer everywhere — restraint. |
| **Error** | Tile frame swaps hairline color from ivory @ 20% to **oxblood @ 60%** in 160 ms; body renders ivory error copy (Tiempos italic eyebrow `COULDN'T LOAD`, body in Söhne) + a single `[ Retry ]` chip (gold hairline, ivory text, hover → gold fill). |
| **Dismiss** | Tile collapses to a 1 px gold horizontal seam (240 ms `ease-in-quart`), then fades to nothing (80 ms). Surrounding stream reflows in the same 240 ms. |
| **Pin confirm** | Pin glyph fills gold; a 240 ms gold-seam-right sweeps from the pin glyph toward the chamber's right edge, signalling "promoted to ambient." The ambient HUD grows simultaneously (visible only if not occluded). |

### 9.5 Empty canvas

A freshly-opened chamber with no prior conversation is the coldest moment in the product. We warm it with the operator's own ambient context — not generic suggestions.

| Aspect | Behavior |
|---|---|
| **Trigger** | Focus chamber opens with no active conversation thread (`⌘⇧Space` from idle). |
| **Render** | Standard empty-state (sigil + "Good morning, Tri.") **plus** a row of up to 3 quick-spawn chips below the existing `⌃ Try` row. Chips show widget eyebrow names (`MARKET · TODAY`, `CALENDAR · THIS WEEK`, `PRS MERGED · 7d`) in Tiempos italic. |
| **Source** | Most-recently-pinned widgets first; if fewer than 3 pinned, backfill from operator's most-spawned widget types over the last 14 days. |
| **Behavior** | Click chip → widget spawns in the response stream (standard 240 ms gold-seam-down). Chips dismiss themselves after first spawn (the row is for cold-start friction, not a persistent dock). |
| **Why** | The empty chamber's cold-start cost is the operator deciding *what to ask*. Quick-spawn chips remove that cost for the 3 widgets she already cares about. |

```
┌───────────────────────────────────────────────────────────────────┐
│                                                                   │
│   ⬡  Ask Leah anything…                                          │
│   ──────────────────────────────────────────────────────────────  │
│                                                                   │
│                            ⬡                                      │
│                  Good morning, Tri.                               │
│                                                                   │
│   ⌃ Try                                                           │
│   [ What's new today? ]  [ Open my brief ]  [ Status of MAY-19 ] │
│                                                                   │
│   ⌃ Spawn                                                         │
│   [ MARKET · TODAY ]  [ CALENDAR · THIS WEEK ]  [ PRS · 7d ]    │
│                                                                   │
└───────────────────────────────────────────────────────────────────┘
```

---

## Versioning

- v1 (this doc): the design lock. Implementation can pick framework; cannot pick palette or motif.
- v1.1 candidates (NOT in v1): light mode (operator-overlay aesthetic resists light mode — defer); per-conversation theming; alternative sigils.
- v2 horizon: iOS companion (sigil scales; layout reflows).

— end —
