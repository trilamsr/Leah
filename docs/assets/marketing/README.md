# Marketing asset bundle

Canonical home for the Leah hero PNGs and Mark in all required sizes. Bundle slots are reserved by placeholder bytes that pass `scripts/check-marketing-assets.sh`; operator hand-replaces with Figma + Loom exports pre-launch.

## Inventory

| File | Surface | Source | Status |
|---|---|---|---|
| `hero-01-summon.png` | §17.12 hero — hotkey summon | Loom capture → Figma frame | placeholder |
| `hero-02-ambient.png` | §17.12 hero — ambient mode | Loom capture → Figma frame | placeholder |
| `hero-03-focus.png` | §17.12 hero — focus panel | Loom capture → Figma frame | placeholder |
| `hero-04-dashboard.png` | §17.12 hero — dashboard | Loom capture → Figma frame | placeholder |
| `mark.svg` | §13.14 Mark — vector master | Figma export | placeholder |
| `mark.pdf` | §13.14 Mark — vector print | Figma export | placeholder |
| `mark-18.png` | menubar template icon | Figma export @1x | placeholder |
| `mark-24.png` | settings + about | Figma export @1x | placeholder |
| `mark-56.png` | docs + favicon @2x | Figma export @2x | placeholder |
| `mark-96.png` | App Store + GitHub avatar | Figma export @3x | placeholder |

## Replacement protocol

Drop the real exports in place — same filename, same path. `scripts/check-marketing-assets.sh` runs in CI (`.github/workflows/check.yml`) and gates only file presence + non-empty bytes; real Figma exports will be orders-of-magnitude larger than the 68-byte / 423-byte placeholders, so the gate passes trivially once swapped.

## Spec refs

- `docs/superpowers/designs/2026-06-21-leah-macos-native-ui-design.md` §17.12 (hero captures), §13.14 (Mark sizes).
