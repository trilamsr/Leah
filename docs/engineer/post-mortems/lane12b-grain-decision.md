# Lane 12.b grain overlay disposition — operator decision 2026-06-23

## Background

`docs/research/2026-06-21-perf-review-render-memory.md` row #2 flags a "3% noise grain
overlay tiled over every obsidian surface" — a 128×128 PNG composited under blur, costing
~3-4ms/frame GPU on every widget, chamber, and dashboard panel. Lane 12.b verification:
that tiled PNG **never shipped**. The only code matching "grain" intent is
`app/Leah/Sources/LeahWidgets/MinimalMode.swift:11-15`, a flat `Color.black.opacity(0.04)
.blendMode(.multiply)` overlay on the default (non-minimal) branch — no tiling, no PNG,
no per-tile blur stack. The row #2 measurement (~3-4ms/frame) is therefore phantom; the
real overlay is a single SwiftUI `Color` view, sub-millisecond on M1 integrated GPU.

Three dispositions follow.

## Options

### (a) NON-ISSUE final — mark row #2 resolved-by-verification

- Scope: zero code. Annotate perf doc row #2 with verification note + Lane 12.b ref.
- LOC: ~3 lines in `docs/research/2026-06-21-perf-review-render-memory.md`.
- Perf target: none — measurement already proves current overlay is sub-ms.
- Visual change: none.
- Risk: future readers re-discover the row and re-investigate. Doc-only fix is durable
  iff the verification note cites the actual code path.
- Recommend rank: **1**.

### (b) Delete MinimalGuard overlay (perf-rec #7)

- Scope: drop the `else` branch in `MinimalGuard.body`; both modes return `content`
  unmodified, the modifier becomes a no-op pending removal.
- LOC: ~6 lines deleted in `MinimalMode.swift`; ~3 lines in perf doc.
- Perf target: idle widget redraw — expect <0.5ms/frame delta (overlay is already cheap).
  Worth measuring as proof, not as recovery.
- Visual change: obsidian surfaces lose 4% multiply darkening; gradient + hairline rules
  carry the depth cue alone. Subtly brighter widget tiles.
- Risk: design regression. Operator may want the 4% as a "settle" effect against the
  obsidian gradient — removing it shifts the whole HUD tonality. Reversible.
- Recommend rank: **2**.

### (c) Noise PNG + cached bitmap (build the originally-spec'd feature)

- Scope: ship a tiled 128×128 grain PNG, rasterize once into a `CGImage`, composite as a
  `.background(GeometryReader { ... Image(...) })` clipped to surface bounds. Asset
  pipeline, Retina @2x/@3x variants, dark/light tint variants.
- LOC: ~40-60 new (asset import, `GrainOverlay` view, MinimalGuard rewrite, snapshot
  test). 1 new PNG asset (~8KB).
- Perf target: ≤1.5ms/frame composite cost on M1 integrated GPU at 720×480 chamber;
  zero allocation per frame (bitmap cached in `@State` or `Image` view's intrinsic cache).
- Visual change: textured "obsidian" finish. Matches v1 §2.2 spec intent.
- Risk: re-introduces exactly the cost row #2 warned about, even if cached. Stacks under
  blur — blur cache invalidation negates the bitmap cache. References Arc's removal of
  similar grain in 1.x after Activity Monitor complaints.
- Recommend rank: **3**.

## ASCII mock

```
current (a):                  (b) delete overlay:           (c) noise PNG:
+----------------------+      +----------------------+      +----------------------+
| obsidian gradient    |      | obsidian gradient    |      | obsidian gradient    |
| + 4% black multiply  |      |   (no overlay)       |      | + 128px tiled grain  |
| hairline rule ──────  |      | hairline rule ──────  |      | + 4% multiply (opt) |
| widget content       |      | widget content       |      | hairline rule ──────  |
+----------------------+      +----------------------+      | widget content       |
slightly settled tone         brighter, flatter             +----------------------+
sub-ms/frame                  sub-ms/frame                  ~1-1.5ms/frame, textured
```

## Recommendation

**(a) NON-ISSUE final.** Per `CLAUDE.md` decision priority — UX > performance > long-term
benefits, default simpler. The verification finding is the smallest possible answer to
the perf claim: row #2 measured a feature that does not exist; the real overlay is
sub-millisecond. (b) trades a deliberate design choice (4% multiply settles widget tiles
against obsidian) for a performance recovery that isn't there to recover. (c) re-builds a
costly feature the verification just disproved we need. Option (a) closes the perf-doc
item without paying any code, design, or measurement debt. If future profiling on dGPU
machines surfaces a real per-frame cost from the flat overlay, revisit (b) then — not
now, not on speculative grounds.

## Operator decision

- [ ] (a) NON-ISSUE final — annotate perf-doc row #2, no code change
- [ ] (b) Delete MinimalGuard overlay — perf-rec #7, ~6 LOC, brighter widgets
- [ ] (c) Noise PNG + cached bitmap — new feature work, ~40-60 LOC + asset
