---
title: Leah macOS UI — adversarial review (visual craft + Apple HIG conformance)
date: 2026-06-21
reviewer: senior visual designer (Apple HIG alum / Linear brand / ex-Stripe brand)
stance: refute, not validate
scope: visual craft + Apple HIG; not IA, not protocol correctness
docs reviewed:
  - 2026-06-21-leah-macos-native-ui-design-v1.md          (v1)
  - 2026-06-21-leah-wizard-settings-ux.md                 (wiz)
  - 2026-06-21-leah-widget-protocol.md                    (proto)
  - 2026-06-21-sleek-regal-palette-refs.md                (pal)
severity: CRITICAL=must-fix-pre-impl · MEDIUM=fix-before-v1.0 · LOW=tracking
---

# Findings (38)

## CRITICAL — visual craft

v1:§2.1 oxblood vs alert-red: 🔴 CRITICAL craft: `--red-brand #7A1F2B` and `--red-alert #C8434F` share hue (~352°) and chroma; at 12pt they read as the same red to non-experts and to anyone with mild protanomaly (8% of men). Brand-mark vs "things are on fire" must differ on more than luminance. Fix: pull `--red-alert` to ~10° (true alert-red `#D14530`) OR keep oxblood + signal alert via shape (filled chip + icon), not hue.

v1:§2.1 gold vs oxblood collision: 🔴 CRITICAL craft: `--gold-primary #C9A961` (Y=68 L*) and `--red-alert #C8434F` (Y=44 L*) have identical R channel (`C8`/`C9`) and similar perceived luminance under deuteranopia simulation — the two "meaning" colors of the whole product collapse to one for ~5% of male operators. Fix: simulate via `colorbrewer` or Sim Daltonism — verify gold and alert are distinguishable in deuteranopia AND protanopia; if not, shift gold warmer (`#D4B570`) or add a non-color signal (always-paired glyph) to alert.

v1:§2.2 hairline @ 8% on non-Retina: 🔴 CRITICAL craft: `--divider #FFFFFF14` (8% white) at 0.5pt is sub-pixel on 1× displays (MacBook Air 13" external 1080p monitor, Studio Display in mirror mode) — renders as a flicker of ~3% effective opacity or disappears entirely depending on row alignment. Spec doesn't address non-Retina at all. Fix: branch via `window.backingScaleFactor`: 0.5pt @ 14% on Retina, 1pt @ 18% on non-Retina. Settings → Appearance "Hairline opacity 4/8/12/16%" (wiz:B.3.3) is exactly the wrong knob — operators won't know the system-default is invisible on their second monitor.

v1:§2.3 Tiempos italic @ 11pt: 🔴 CRITICAL craft: editorial italic serif at small sizes is the #1 "trying too hard" tell. Tiempos Headline is *display* cut (designed for ≥20pt); using it for table eyebrows (proto:§9.6 `▾ #`), code-block language labels (v1:§9.9 `GO ·`), and widget eyebrows (`MARKET · TODAY`) at 11pt produces muddy serifs + ink-traps swallowed by sub-pixel rendering. Fix: confine Tiempos to the single ≥20pt "Today" moment per v1:§2.3 decision; use Söhne small-caps tracking +80 for all eyebrows. The doc contradicts itself — §2.3 says "one place only" then §9 sprinkles italic eyebrows on every widget.

v1:§9 widget-canvas gold fatigue: 🔴 CRITICAL craft: each tile carries a 32px gold rule under the eyebrow + gold sparkline + gold accent series + pinned-tile gold-filled glyph + gold hover hairline + gold-filled lowest-fare cell + gold now-line + gold tick mark + gold language-label flash. Two-column grid of 4 tiles in a wide chamber = 30+ gold instances on one screen. The whole "earn every pixel" thesis dies the moment widgets render. Fix: gold-budget per chamber — max 3 instances of `--gold-primary` visible simultaneously; everything else falls to `--gold-muted` or ivory. Make this a render-time invariant, not a guideline.

v1:§2.5 hexagon-L recognizability claim: 🟡 MEDIUM craft, but refute: Notion's logo is a stacked-character glyph in a rounded square; Vercel is a hex-adjacent triangle pair; GitHub Octocat is contained; Linear is a hexagon-adjacent. Hexagon containers are NOT distinctive in dev-tool space — they're the default. Doc claims "circle = Siri, hexagon = mechanical" — that's the designer's frame, not the market's. At 16×16 (menubar), the hexagon collapses to "rounded blob" anyway. Fix: pick a non-hex container (custom-asymmetric shield, or italic-L *without* container) and stress-test against 5 dev-tool menubars side-by-side before committing.

## CRITICAL — Apple HIG conformance

v1:§2.1 custom gold breaks system accent: 🔴 CRITICAL HIG: macOS users set a system accent color in System Settings → Appearance (Apple's tint preference, surfaced in every native app since 10.14). Leah hardcodes `--gold-primary` as the CTA + focus-ring color, ignoring the user's chosen tint. HIG explicitly says "Use the accent color the user has selected to tint controls" (Apple HIG: Color — accent colors). Fix: gold is *brand-mark only* (sigil, divider seams). All interactive tint — buttons, focus rings, selection — uses `NSColor.controlAccentColor`. The "Accent intensity slider" in wiz:B.3.3 is a wrong abstraction; operators expect this to live in System Settings.

v1:§2.2 glass-blur material misuse: 🔴 CRITICAL HIG: spec uses `material: .underWindowBackground` for the focus chamber. That material is intended for *window backgrounds behind sidebars* — it's mostly opaque + slightly desaturating. The Spotlight/Notification Center equivalent is `.hudWindow` or `.popover` on a `NSPanel` with `.fullScreen` blending mode. Using `.underWindowBackground` will produce a flat dim panel without the vault-opening glass effect the doc describes. Fix: `material: .hudWindow`, `blendingMode: .behindWindow`, `state: .active`; verify in Sonoma + Sequoia (materials shifted in 14.2).

v1:§4.1 ⌘⌃ chord conflict (Auto-redirected from wizard): 🔴 CRITICAL HIG: wiz:§A.2 sets default hotkey to bare `⌘⌃` (modifier-only). macOS reserves `⌘⌃` as the prefix for several system shortcuts: `⌘⌃Q` (lock screen), `⌘⌃F` (fullscreen toggle), `⌘⌃Space` (Character Viewer), `⌘⌃D` (Dictionary lookup), `⌘⌃↑/↓/←/→` (Spaces nav). A "tap and release" listener on `⌘⌃` will fire when the user is mid-chord toward any of these — race condition with 250ms debounce. Fix: bare-modifier hotkey is a known foot-gun; require at least one non-modifier key OR document the race-window suppression and ship a kill-switch on first conflict detected.

v1:§1.3 NSPanel `.nonactivatingPanel` text-field claim: 🔴 CRITICAL HIG: spec says chamber "does NOT steal focus from current app's text fields unless operator types." A `.nonactivatingPanel` cannot host a first-responder text field AND defer key events to a background app — those are mutually exclusive. Either the panel has key-window status (focus stolen) or the text field doesn't accept input. Spotlight handles this by being a regular panel that takes key on summon, returns key on dismiss — there is no "no-steal until type" middle state in AppKit. Fix: drop the claim; accept that focus shifts on summon and returns on Esc (Spotlight does exactly this). Update v1:§8 decision #10.

wiz:§A.1 frameless wizard violates HIG: 🟡 MEDIUM HIG: wizard uses NSWindow `.titled + .fullSizeContentView` with traffic lights at 20pt inset, no titlebar. Setup windows per HIG should have a standard titlebar so users can drag and identify the window. Spotlight/Notification Center are panels (one-time, transient); a 5-step wizard is a window — operators expect to grab the titlebar. Fix: keep `.fullSizeContentView` but show a thin titlebar; or commit to the panel idiom and lose the resizable claim.

v1:§6 Dynamic Type silent: 🔴 CRITICAL HIG: spec hardcodes 14pt body / 13pt mono / 18pt secondary across HUD, chamber, settings, wizard. macOS Sonoma+ ships system-wide text-size preferences (System Settings → Display → Larger Text) AND per-app Dynamic Type via `NSFont.systemFont(ofSize: NSFont.systemFontSize)`. Hardcoded pt fails for ~7% of users who bump system text. Fix: every text token references `NSFont.preferredFont(forTextStyle:)` or a scaled variant; verify at 100/115/130/150% system text scale; ambient HUD shrinks slot count gracefully (drop "Pulse" row before truncating).

v1:everywhere no light mode: 🔴 CRITICAL HIG: spec is dark-only ("explicit non-goal" in pal:§10). HIG (macOS): "Support both appearances. People expect most apps to honor the system appearance setting." Killing light mode kills enterprise/medical/outdoor use, breaks operators who flip appearance with sunset, and disables auto-switching. The "JARVIS aesthetic" justification is the designer's preference, not a user need. Fix: ship light-mode tokens (obsidian → bone `#F2EFE8`, gold-primary darkens to `#8A7340` for AA on bone, oxblood holds, ivory → graphite `#1E1D1A`). Yes, this is work. It is also table stakes for a Mac app.

## MEDIUM — visual craft

v1:§3.4 Flourish-2 sigil rotation fatigue: 🟡 MEDIUM craft: wake-word ack rotates the hex 60° + warm pulse every time Leah hears her name. With wake-word ON by default (wiz operator decision), this fires on every "Leah,..." utterance — easily 30-80 times/day for a heavy user. After day 3 this is decorative noise, not delight. Fix: rotation only on the first ack per N-minute window; subsequent acks reduce to a 1px gold-glow border flash (80ms).

v1:§3.4 Flourish-1 Gold Seam fatigue: 🟡 MEDIUM craft: 380ms vault-opening on every `⌘⇧Space` summon. Power-user summon count = 30-100/day. After week 1, the "ceremony" reads as latency. Spotlight summons in <80ms with no flourish for this reason. Fix: full flourish on cold-start only (first summon per session); warm summon = 160ms cross-fade. Or run flourish concurrent with input-field-becoming-interactive (don't gate input on flourish completion).

v1:§9.0 4 tiles × small ambient slot: 🟡 MEDIUM craft: pinned widgets render in the ambient HUD's "small variant" — 280×120 spec. Stack 3 pinned tiles + 3 HUD rows + 1-3 toast widgets and the bottom-right is a 600px stack of dark cards. At 1440×900 (MacBook Air 13"), that's >60% of screen height. The "<2% screen area" promise (v1:§0) is broken by the pin system itself. Fix: hard cap of 2 pinned widgets in ambient; overflow becomes a "+N pinned" disclosure that opens dashboard.

v1:§9 candlestick on 720px medium tile: 🟡 MEDIUM craft: market widget renders candlestick at `large` (720×320). A useful candlestick needs ~8px/candle minimum for OHLC tick visibility — 720px = 90 candles maximum, less than a single trading day at 5-min resolution. Fix: candlestick is `hero` only (720×440); medium = sparkline + delta + symbol; document the candlestick density floor (px/candle) in proto:§1.1.

v1:§9.6 gold lowest-fare cell on flights matrix: 🟡 MEDIUM craft: filled `#C9A961` background with obsidian text in a single grid cell of a 7-day matrix creates a high-attention focal point — but multiple rows each get their own row-minimum gold fill PLUS the global minimum gets a bracket. That's 5+ gold-filled cells in a 28-cell matrix. The "lowest fare" signal is lost in gold clutter. Fix: only the global min is filled-gold; row mins get a 1.5pt gold left-edge hairline (subtle, scan-able, not loud).

pal:§5 letterpress text-shadow on dark: 🟡 MEDIUM craft: spec recommends `text-shadow: 0 1px 0 rgba(0,0,0,0.5)` for "engraved" effect on headings. On `--obsidian-0 #08090C`, a black shadow is invisible (black on near-black). The engraved effect requires a *highlight* (lighter shadow above + darker below). Fix: `text-shadow: 0 -1px 0 #FFFFFF08, 0 1px 0 #00000080` — two-direction emboss, not single-direction drop. Spec also doesn't say where this applies; without scope it'll get sprinkled everywhere.

v1:§2.4 1.5pt Lucide icons restyled: 🟡 MEDIUM craft: Lucide icons are designed at 2pt stroke for 24×24. Restyling to 1.5pt @ 24×24 and 1pt @ 16×16 will produce visibly thinner-than-system iconography that fights NSImage `.symbolConfiguration` SF Symbols (everywhere else in macOS). The visual mismatch reads "indie app trying to look custom". Fix: use SF Symbols where they exist (`mic.fill`, `paperplane`, `gearshape`), fall back to Lucide only for novel concepts (the sigil, pin glyph); match SF Symbols' default stroke weight.

v1:§9 widget chrome over text bleed: 🟡 MEDIUM craft: focus chamber uses glass-blur (NSVisualEffectView behind-window) — when summoned over prose-heavy windows (a long markdown doc, Discord, Slack), the blur shows the underlying text *colors* even after 28px blur. Text below the chamber will tint the chamber visibly red/blue/etc, breaking the obsidian discipline. The doc's "obsidian + blur" treats blur as a tint additive, not a contamination risk. Fix: tint layer must be opaque enough (`--blur-tint #08090CCC` = 80%) is borderline; bump to `#08090CE6` (90%) when prose density behind exceeds 50% text coverage, OR drop blur entirely under heavy backgrounds (detect via NSWindow occlusion sampling).

## MEDIUM — Apple HIG conformance

v1:§1.1 NSPanel canJoinAllSpaces + Mission Control: 🟡 MEDIUM HIG: HUD is `canJoinAllSpaces`, persistent. In Mission Control / Stage Manager preview, an always-on-top widget appears in every Space's MC preview, breaking the spatial mental model ("which Space is this?"). HIG Stage Manager guidance: respect the user's Space partitioning unless the app is a *system* utility (clock, battery). Fix: HUD respects per-Space pinning (`collectionBehavior: .moveToActiveSpace` instead of `.canJoinAllSpaces`); user opts into all-Spaces via Settings → General → Visibility (already there — change the default).

v1:§1.2 menubar icon not template: 🟡 MEDIUM HIG: spec says "18×18 monochrome" icon with "gold dot when listening". Menubar template images (Apple-required for monochrome menubar in light/dark mode + accent override) are pure-alpha — you cannot color a menubar template image gold. The listening-state must be conveyed via shape (filled vs outlined) or NSStatusBarButton tinted via `appearsDisabled` state, NOT a colored dot. Fix: listening state = filled hex glyph (template); idle = outlined hex (template). Drop the gold dot — it will render as system-tint on macOS and look like an OS-applied badge.

v1:§3 grain overlay + Reduce Transparency: 🟡 MEDIUM HIG: spec disables grain under `prefers-reduced-motion` and `reduced-transparency`. Grain isn't transparency — it's an additive texture. macOS System Settings → Accessibility → Display → Reduce Transparency replaces vibrancy with solid colors; it doesn't affect noise overlays. Fix: grain is governed by `differentiateWithoutColor` (no — irrelevant) or its own user-pref toggle. Also: noise on top of glass-blur amplifies blur banding under "Reduce Transparency" fallback (solid bg) — under that mode, increase grain to 3% to hide the now-flat field.

wiz:§A.2 Permissions accumulate strategy: 🟡 MEDIUM HIG: lazy-prompting permissions on first-use IS Apple's preferred pattern (matches iOS HIG). However, Accessibility permission (needed for global hotkey) cannot be requested via a normal `NSAccessibility` API — it requires a manual System Settings open + restart-the-app dance. Without it the hotkey silently fails. Wizard step 2 demos the hotkey ("Try it now") but cannot demo it until Accessibility is granted. Fix: wizard step 2 must ask for Accessibility OR show "Hotkey won't work outside Leah windows until you grant Accessibility — set up later" with clear breakage warning. Currently the spec lies-by-omission.

v1:§1.7 dashboard size 1180×760 on small displays: 🟡 MEDIUM HIG: default size > MacBook Air 13" usable area (1440×900 minus menubar/dock = ~1440×824). Window opens taller than fits, partially off-screen-bottom. HIG: default windows must fit the smallest supported display. Fix: default = `min(1180, screen.visibleFrame.width × 0.85)` × `min(760, screen.visibleFrame.height × 0.85)`; user-resized state persists.

wiz:§B.3.5 status glyph meaning collision with macOS: 🟡 MEDIUM HIG: `●` green = granted, `◐` gold = needs setup, `○` open = not asked, `✕` red = denied. macOS System Settings privacy pane uses *toggles* (NSSwitch) — operators trained on toggles will look for them. The glyph-as-status pattern is Linear's, not Apple's. Fix: per-permission row IS a toggle when state allows (granted → toggle on, can disable in System Settings via deep link); ungranted → "Grant" CTA. Keep glyphs as supplemental indicators only.

v1:§4.5 CGScreenIsCaptured() deprecated: 🟡 MEDIUM HIG: this API was deprecated in macOS 12; replaced by `CGDisplayStream` capture detection / `SCStreamConfiguration`. Spec leans on a deprecated call for the privacy-default-ON auto-hide-during-recording feature. Fix: use `SCShareableContent` observers (Sonoma+) or fall back to `CGWindowListCopyWindowInfo` filtered for capture-process names; document the macOS-14-min requirement honestly.

proto:§3 cached-last-good Files path: 🟡 MEDIUM HIG: spec uses `~/.leah-state/widget-cache/...` — that's a Unix-y dotfile path, not the Apple-blessed location. macOS apps cache under `~/Library/Caches/<bundle-id>/` (system-purgeable on low disk) and state under `~/Library/Application Support/<bundle-id>/`. Wiz:§B.3.7 correctly uses `~/Library/Application Support/Leah/memory/` — but proto and v1 sprinkle `~/.leah-state/` elsewhere. Fix: enforce Apple paths across all four docs; otherwise Time Machine, Migration Assistant, and "Clean Cache" tools misbehave.

v1:§4.1 Settings → "Conflict detection runs against macOS shortcuts": 🟡 MEDIUM HIG: there is no public API to enumerate all installed macOS keyboard shortcuts — `HIToolbox` (wiz:§A.3 step 2 cites it) is a private API; using it risks App Store / notarization audit failure. Conflict detection can hit Apple-documented system shortcuts via a hardcoded list but cannot detect third-party app shortcuts. Spec overpromises. Fix: detect *known macOS system shortcuts only* (maintain a curated list updated per macOS release); document the limit honestly in UI ("checked against system shortcuts; third-party apps may conflict").

wiz:§B.3.6 disconnect "single-confirm": 🟡 MEDIUM HIG: disconnecting an integration triggers a single-button confirmation. Fine for Calendar; insufficient for Files where disconnect deletes the indexed corpus (2,341 entries). HIG: destructive actions that lose data require typed confirmation OR a clear "your X will be deleted" warning + Undo. Fix: tier by data loss — Calendar/Mail = single confirm; Files/Memory-adjacent = typed-confirm OR "Disconnect (keep index)" / "Disconnect + delete index" disambiguated.

v1:§5.1 ambient HUD weather glyph: 🟡 MEDIUM HIG: spec offers "weather one-glyph" in pulse row. macOS HIG Menu Bar/Notification Center guidance: never use emoji as functional UI (rendering varies by macOS version, by user font; ☀️ in 2026 macOS is 5 different glyphs depending on Apple/Twitter/Mozilla rendering). Fix: SF Symbols weather glyphs (`sun.max.fill`, `cloud.rain.fill`) — guaranteed consistent, support template tinting.

## MEDIUM — multi-doc / contradictions

v1 vs wiz hotkey contradiction: 🟡 MEDIUM cross-doc: v1:§4.1 doctrinally specifies `⌘⇧Space` with multi-paragraph rationale ("⌘K clashes, ⌘⇧Space is Spotlight's sibling"). Wiz:§0 overrides to bare `⌘⌃` with one-line operator decision. Both docs ship as canonical. Reviewer who reads v1 first will design a Spotlight-sibling system; reviewer who reads wiz first will design a Raycast-modifier-only system. Fix: pick one in v1, delete the other; or v1 must reference wiz override at §4.1 inline.

v1 vs pal palette contradiction: 🟡 MEDIUM cross-doc: pal:§8 recommends `--gold #C9A961`, `--red #8B1E2D`, `--ivory #F4EDE0`, `--mute #8A8478`. v1:§2.1 uses `--gold-primary #C9A961` (same), `--red-brand #7A1F2B` (different — pal said #8B1E2D), `--text-primary #F2EDE0` (different — pal said #F4EDE0), `--text-muted #B8B0A0` (different — pal said #8A8478). Four palette tokens diverge silently. Fix: reconcile in v1; remove pal:§8 "recommended pick" or mark it superseded.

v1 vs pal type stack contradiction: 🟡 MEDIUM cross-doc: pal:§6 recommends Cormorant Garamond + SF Pro + SF Mono ("three-typeface system"). v1:§2.3 uses Söhne + Inter + JetBrains Mono + Tiempos (four families, none of which are SF system fonts). Designer who reads pal will ship Cormorant; engineer who reads v1 will license Söhne. Fix: pick — and if the answer is "Söhne not SF Pro", justify why we abandon system-text in v1:§2.3.

proto:§9 vs v1:§9.0 widget tile-count: 🟡 MEDIUM cross-doc: v1:§9.0 says "max 4 widget tiles per response turn." Proto:§5.3 streaming rule has no cap; the LLM can emit unlimited `widget.mount` frames per turn. Fix: validation in `internal/widget/` enforces the 4-tile cap at adapter-registration time; widget #5 in a turn = `tool_error` back to LLM.

## LOW — visual craft + HIG

v1:§3.3 Listening pulse 1400ms loop: ⚪ LOW craft: 1400ms full-cycle, 200ms gap = 1.6s per pulse, ~37 pulses/min. The Pixel 8 Now Playing UI uses 800ms; macOS dictation uses 600ms. 1400ms reads as "slow heartbeat" — could be intentional regal pacing, or could read as "Leah is barely awake". Fix: A/B at 900ms and 1400ms with operator over a week; doc shouldn't assume.

v1:§7 ASCII wireframes vs production: ⚪ LOW craft: every wireframe uses monospace box-drawing characters that imply a hairline-grid layout. Implementation in SwiftUI doesn't produce pixel-grid-aligned hairlines without manual offset. Fix: ship Figma frames at 1:1 px alongside ASCII; the ASCII is a sketch, not a spec.

wiz:§A.3 mic step "live waveform mock animates BEFORE grant": ⚪ LOW craft: spec animates a "canned sine loop" labeled "(preview)" before mic permission granted. Apple HIG anti-pattern: do not fake input. The fake waveform is dishonest UX (looks like Leah hears you when she doesn't). Fix: show a *static* waveform glyph + caption "Enable mic to see the waveform"; reveal real waveform on grant.

v1:§9.4 weather widget emoji icons: ⚪ LOW craft: `☀ ⛅ 🌧` in the ASCII wireframe. If implemented as actual emoji glyphs (vs SF Symbols), see the HIG emoji finding above — same fix applies here.

---

## Summary

**Total: 38 findings** — 11 CRITICAL · 22 MEDIUM · 5 LOW

**Top-5 blockers (must resolve before any SwiftUI work):**
1. Custom gold overrides system accent color (HIG)
2. Dark-only ships, light-mode parity missing (HIG)
3. NSPanel `.nonactivatingPanel` "no-steal-focus" claim is impossible (HIG)
4. Tiempos italic at 11pt across widget eyebrows contradicts §2.3 "one place only" (craft)
5. Gold-fatigue invariant — widget canvas violates v1:§0 "earn every pixel" (craft)

**Two cross-doc tracks that need a reconciliation pass:**
- Hotkey (v1 vs wiz)
- Palette + type stack (v1 vs pal)

**Themes:**
- The luxury-watch metaphor is doing too much load-bearing work; HIG conformance loses every collision
- Dark-only + non-system-tint is a 2018-era startup pattern, not a 2026 Mac-native one
- Editorial italic serif at small sizes is the single most over-applied moment — strip it back to its declared one-place use
- Cross-doc drift is already at v1; will compound as more designers touch the spec — fold pal and wiz overrides into v1 as authoritative
