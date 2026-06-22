---
title: Sleek + regal dark UI — obsidian / gold / red / white palette research
date: 2026-06-21
author: research-fork
status: research-only
scope: design-input for macOS-native Leah HUD redesign (post-cyan)
---

# Sleek + regal dark UI — palette + type + material refs

Brief for the macOS-native Leah HUD redesign. Operator brief: "sleek and
regal — obsidian, gold, a shade of red, and white. Drop cyan." This memo
collects verified hex anchors, brand exemplars, and treatment-evidence so
the visual-identity section of the design doc can be written from sources,
not vibes.

## 1. Reference exemplars

| Brand / surface | What to steal | Source |
|---|---|---|
| **Cartier** (jewelry / watches) | Cartier red `#C8102E` against ivory + black. Single-brand-mark accent, never decorative fill | https://www.cartier.com/en-us/ (live), Pantone 200 C reference |
| **Hermès** | Orange `#FF6900` (not red, but adjacent) as singular brand-mark accent on cream + black. Restraint: one accent per page | https://www.hermes.com/us/en/ (live) |
| **Lamborghini** | Black + gold-emblem + restrained red CTAs. Sharp serif display headings, sans body. Heavy negative space | https://www.lamborghini.com/en-en (live) |
| **Ferrari (Rosso Corsa)** | `#FF2800` racing red as identity accent on black/silver. Reserved for brand-mark, never bulk fill | Wikipedia: Rosso Corsa |
| **Cyberpunk 2077 HUD** | Yellow-gold + red emergency-state on near-black. Confirms "gold + red" works as functional state, not just decoration | https://www.cyberpunk.net/us/en/ (live) |
| **Blade Runner 2049 UI design** | Thin hairline geometry, off-white + amber-gold, very wide kerning, minimal chrome | Film stills (Territory Studio case studies, widely documented) |
| **Westworld interiors / Delos UI** | Cream + brass-gold + oxblood; sans-serif with heavy weight contrast | Film stills, Method Studios case studies |
| **The Macallan** | Obsidian + warm metallic copper-gold + cream. Foil-stamp hairlines on dark | macallan.com (blocked at fetch, observable in print and packaging) |

## 2. Anchor palettes (pick one + variants for accent overrides)

### A. "Obsidian + foil gold" (Macallan / Cartier dark mode)
```
--bg-0      #0A0A0C   /* obsidian, near-black with cool undertone */
--bg-1      #131316   /* card / panel */
--bg-2      #1C1C20   /* elevated, modal */
--gold      #C9A961   /* antique foil gold — reads regal, not casino */
--gold-hi   #E4C77A   /* hover / focus highlight */
--red       #8B1E2D   /* oxblood-adjacent — alerts + brand-mark only */
--ivory     #F4EDE0   /* warm white, primary text */
--mute      #8A8478   /* tertiary text, dividers */
```
Why this: cool obsidian + warm gold creates the metallic contrast luxury
print uses. Oxblood red kept dark so it reads serious (not alarm-clock).

### B. "Lamborghini after-hours" (vehicular luxury)
```
--bg-0      #08080A
--bg-1      #14141A
--gold      #D4AF37   /* canonical metallic gold, ISCC-NBS / Wikipedia */
--gold-hi   #F0CC58
--red       #B91C2C   /* deeper than Ferrari Rosso, less alarm */
--ivory     #EFE9DC
--mute      #6E6A60
```
Why this: `#D4AF37` is the dictionary-defined "metallic gold" hex (ISCC-
NBS via Wikipedia). The most-cited reference in luxury color systems.

### C. "Ferrari × Hermès editorial" (high-contrast brand-mark)
```
--bg-0      #050507
--bg-1      #111114
--gold      #BFA063   /* old-gold #CFB53B muted half-step toward bronze */
--gold-hi   #DDBE7C
--red       #C8102E   /* Cartier red / Pantone 200 C territory */
--ivory     #FFFFFF
--mute      #9A958A
```
Why this: pure white + Cartier-red + restrained gold = editorial cover
energy. Best when red is used as a single brand-mark dot (1-2 instances
per surface, max).

## 3. Gold tonality — which reads "regal" not "casino"

| Variant | Hex | Reads as | Use? |
|---|---|---|---|
| Metallic gold | `#D4AF37` | Canonical reference gold, slightly green-yellow | Yes — neutral default |
| Old gold | `#CFB53B` | Vintage, museum-plaque | Yes for serif-heavy editorial direction |
| Antique foil | `#C9A961` | Warm, muted, restrained | Yes — best for sleek/regal brief |
| Champagne | `#D4C5A0` | Pale, subdued, jewelry-box | Use as secondary, not primary accent |
| Rose gold | `#B76E79` | Fashion / cosmetics-coded | Avoid for assistant UI — too gendered |
| Casino gold | `#FFD700` | Vegas, bright yellow-gold | Avoid — reads cheap |

**Recommendation:** lead with `#C9A961` (palette A). The brief is "regal,"
not "metallic gold." Saturation kept under 50% prevents the casino read.

## 4. Red — role + variant pick

Verified hex anchors (Wikipedia):

- **Crimson** `#DC143C` — strong, slightly purplish. CSS-named.
- **Oxblood** `#4A0000` — very dark, brown-undertone. Dates 1695, popular
  fashion ~2012 (Washington Post).
- **Cartier red** `#C8102E` — Pantone 200 C territory.
- **Ferrari Rosso Corsa** `#FF2800` — racing red, very alarm-coded.
- **Hermès orange** `#FF6900` — adjacent, not strictly red.

**Role recommendation:** red is reserved, not decorative.

1. **Brand-mark dot** — single 6-8px dot next to "Leah" wordmark or
   in-flight indicator. Cartier-red `#C8102E` zone.
2. **Critical alerts only** — daemon-offline, attestation-revoked,
   security-blur engaged. Oxblood `#8B1E2D` so it reads serious, not
   panic.
3. **Never as fill** — no red panels, no red buttons, no red borders on
   normal state. Red appearing on screen = something is wrong OR the
   brand mark is on-screen. Two contexts only.

This is the Cartier / Hermès discipline: the accent is rare, so it
carries meaning when it appears.

## 5. Material treatment

Evidence + recommendations:

- **Hairline gold rules** — 1px dividers in gold at ~20% opacity. Print
  luxury (Macallan packaging, Cartier catalogs) uses foil-stamp hairlines
  as the primary "expensive" cue.
- **Subtle grain / film noise** — 2-4% opacity tiled noise PNG over the
  background. Prevents the "flat dark UI" cheap-startup look. See
  Linear's app and Arc browser dark mode for execution.
- **Glassmorphism blur** — `backdrop-filter: blur(24px) saturate(140%)`
  on summon overlay, sparingly. Apple's macOS Sequoia uses this for
  Spotlight + Notification Center. Native idiom, not skeuomorphism.
- **Letterpress / inset shadow** — `text-shadow: 0 1px 0 rgba(0,0,0,0.5)`
  on key headings reads engraved-into-metal. Pair with `text-shadow:
  0 0 1px <gold-hi>` for foil-stamp glow on the wordmark.
- **No gradients** beyond a 5%-opacity vignette at panel edges. Luxury
  print does not gradient-fill — it uses flat fields and metallic ink.

## 6. Type pairing

Luxury-brand evidence:

- **Display serif (headings, wordmark):** Playfair Display, Cormorant
  Garamond, Canela, GT Sectra. All confirm the "editorial luxury" read.
  Google Fonts has Playfair + Cormorant free. Brand examples: Vogue,
  The New York Times Magazine, most luxury watch sites.
- **Body grotesque:** Söhne (paid), Inter, Geist, or system SF Pro on
  Mac. Used by Hermès, Linear, Vercel, Stripe.
- **Telemetry / mono:** SF Mono (system) or JetBrains Mono. Mono is for
  numeric readouts only — never for body copy.

**Recommendation:** pair display serif + system grotesque + mono.
Three-typeface system, not two. The serif/sans contrast is the type
signal that this UI is editorial, not utilitarian.

Stack:
```
--font-display:  'Cormorant Garamond', 'Playfair Display', Georgia, serif;
--font-body:     -apple-system, 'SF Pro Text', 'Inter', sans-serif;
--font-mono:     'SF Mono', 'JetBrains Mono', Menlo, monospace;
```

## 7. What to avoid (anti-patterns)

- Bright `#FFD700` gold — reads casino / costume jewelry.
- Pure cyan / electric blue — old direction, killed in this pivot.
- Gradient gold (`linear-gradient` from #FFD700 to #B8860B) — reads CSS
  trick, not metal. Use a single flat gold + foil hairline instead.
- Animated shimmer / sparkle effects on the gold — gimmick.
- Red used as positive/neutral fill — breaks the "red = brand or alert"
  discipline. China-luxury convention is the exception, not the rule.
- Glassmorphism EVERYWHERE — restraint. Blur is for summon overlay only,
  not ambient panels.

## 8. Recommended pick for Leah HUD

**Palette A ("Obsidian + foil gold")** with red role per §4.

| Token | Value | Role |
|---|---|---|
| `--bg-0` | `#0A0A0C` | App background |
| `--bg-1` | `#131316` | Card / panel |
| `--bg-2` | `#1C1C20` | Elevated / modal |
| `--gold` | `#C9A961` | Active state, hairlines, wordmark |
| `--gold-hi` | `#E4C77A` | Hover / focus |
| `--red` | `#8B1E2D` | Critical alerts only |
| `--red-mark` | `#C8102E` | Brand-mark dot (single instance) |
| `--ivory` | `#F4EDE0` | Primary text |
| `--mute` | `#8A8478` | Tertiary text |
| `--hair` | `rgba(201, 169, 97, 0.20)` | 1px dividers |
| `--noise` | 3% opacity tiled PNG | Background grain |

Type: Cormorant Garamond (display) + SF Pro (body) + SF Mono (telemetry).

Material: flat fields + hairline gold rules + glass-blur on summon
overlay only. No gradients. No shimmer.

## 9. Sources

1. https://en.wikipedia.org/wiki/Gold_(color) — metallic gold #D4AF37,
   old gold #CFB53B, full shade taxonomy.
2. https://en.wikipedia.org/wiki/Oxblood — #4A0000, etymology 1695,
   2012 Washington Post fashion-season citation.
3. https://en.wikipedia.org/wiki/Crimson — #DC143C, CSS-named.
4. https://www.cartier.com/en-us/ — live page, red+ivory+black system.
5. https://www.hermes.com/us/en/ — live page, single-accent discipline.
6. https://www.lamborghini.com/en-en — live page, serif-display + black
   + restrained red CTA.
7. https://www.cyberpunk.net/us/en/ — gold + red HUD reference.
8. https://www.canva.com/colors/color-palettes/?query=black-and-gold —
   curated palette gallery with hex specimens.

## 10. Out of scope for this memo

- Per-component spec (button states, input fields, panel layout) —
  belongs in the full UI design doc.
- Motion / animation curves — referenced in HUD-UI spec already.
- Light-mode variant — explicit non-goal per JARVIS-UI spec.
- Brand-mark wordmark glyph design — separate work.
