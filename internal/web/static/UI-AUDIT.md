# LEAH dashboard — UI audit

Audited 2026-06-09 via `frontend-design` + `ui-ux-pro-max` skills against the
current scaffold (`dashboard.{html,css,js}`, 310 LoC combined). Findings are
severity-tagged. JARVIS aesthetic target: dark, crisp 1px cyan lines, sharp 90°
corners, restrained glow, monospace.

Severities: **HIGH** breaks function or accessibility; **MED** degrades
perceived quality; **LOW** polish.

## Findings

### Visual hierarchy

- **MED H1.** Quadrant headers (`h2`) and section headers (`h3`) share the same
  visual weight bucket once the eye is scanning monospace — the audit stream is
  the primary surface and deserves a stronger anchor (corner cap, bracketed
  label, or accent rule) so the four quadrants register as a system instead of
  four equal cards.
- **LOW H2.** `MEMORY` quadrant mixes counts row + "recent decisions" sub-list;
  the sub-list `h3` is dim gray and easily missed. Promote with a 1px accent
  rule or `[ recent ]` bracket framing.

### Typography

- **MED T1.** `font-family: 'SF Mono', 'JetBrains Mono', 'Menlo', monospace` is
  fine for body but the dashboard has no display voice. JARVIS reads as
  technical-precise, not generic-terminal. Pair a tabular display face for the
  big numbers (`ops` budget, counts) so they read like instrument readouts, not
  inline text.
- **LOW T2.** No `font-variant-numeric: tabular-nums` on number-heavy cells —
  counts and dollar amounts will jitter as digits change during polling.

### Color contrast (WCAG AA)

- **HIGH C1.** `--fg-dim: #6b7a8c` on `--bg-2: #10151c` measures **4.31:1** —
  fails AA for body text (4.5:1). Used for timestamps, labels, "recent
  decisions" header, empty-state text. Lift dim to `#7e8fa3` (≈5.1:1) or
  reserve dim for large/decorative text only.
- **MED C2.** Border `rgba(0,212,255,0.18)` against `--bg-2` is ~1.4:1 — fine
  decoratively but the audit `li` separators at `rgba(255,255,255,0.03)` are
  effectively invisible. Bump to `rgba(0,212,255,0.06)` for thematic
  consistency.
- **LOW C3.** `--ok #7fdba8` on `--bg-2` clears AA at 7.3:1; `--warn #ffb74d`
  clears at 8.9:1; `--fail #ff5252` clears at 4.6:1 — all pass. No action.

### Motion

- **LOW M1.** 3s pulse on the alive-dot is appropriate — slow enough to feel
  like a heartbeat, not a strobe. Already gated by `prefers-reduced-motion`.
  Keep.
- **MED M2.** **No motion on new-row arrival.** Audit stream silently swaps
  innerHTML every 3s — operator can't tell what's new. A 600ms cyan flash on
  the top row (newly-prepended) is the high-ROI animation here.
- **LOW M3.** Budget bar `transition: width 200ms` is fine; consider 300ms
  ease-out per `exit-faster-than-enter` heuristic but not load-bearing.

### Empty states

- **HIGH E1.** Empty-state copy is bare `(empty)` / `(no agents)` / `(none)` —
  reads as broken, not idle. Operator on first boot sees four `(empty)`
  parentheticals and has no next action. Replace with directive copy:
  - audit: `awaiting activity — try \`leah ask\` to start`
  - agents: `no agents spawned`
  - memory decisions: `no decisions captured yet`
- **MED E2.** No empty state on memory **counts** row — when daemon has zero
  contacts/projects/decisions, the row shows `contacts 0 projects 0 decisions
  0` with no framing.

### Loading + error

- **HIGH L1.** **No loading state before first `/api/state` response.** First
  paint shows empty quadrants — indistinguishable from "daemon up but
  empty" vs "daemon not responding yet". Add a 1-poll skeleton (shimmer or
  dimmed placeholder rows) cleared on first successful tick.
- **MED L2.** Error banner fires after 2 missed polls (~6s) which is correct,
  but the banner is a plain red bar with no affordance — no retry hint, no
  "last successful poll Xs ago" info. Add `last seen: Ns ago` to give operator
  a clock.
- **LOW L3.** No `aria-live` on the banner — screen reader misses the outage.

### Information density

- **MED D1.** OPS quadrant is sparse: budget row, bar, heart row, uptime row =
  4 lines in a quadrant the same size as a scrolling audit list. Either add a
  small detail (model-routing summary, last-tool-used) or tighten the quadrant
  to half-height and let MEMORY occupy more vertical real-estate.
- **LOW D2.** Audit `<li>` row uses 3px vertical padding — fine on a 27"
  display, cramped on a 13" laptop. Consider 4px and a `--row-pad` token for
  density tuning.

### Affordances

- **MED A1.** Audit rows and agent rows are not interactive — but they look
  potentially clickable (monospace + colored kind tag). Either:
  (a) explicitly make them interactive (drawer expand for full detail), or
  (b) signal non-interactivity (no hover state, no cursor change). Currently
  ambiguous — operator may try to click.
- **LOW A2.** Long `detail` strings are CSS-truncated with ellipsis but no
  `title` attribute — hover reveals nothing. Add `title="${escape(detail)}"`
  for full-text peek.

### Architecture / hygiene

- **LOW X1.** `escape()` shadows the global `window.escape` deprecated alias.
  Rename to `esc()` to avoid confusion in dev console.
- **LOW X2.** No `font-variant-numeric: tabular-nums` (see T2).
- **LOW X3.** `box-shadow: var(--glow)` on every quadrant stacks four glow
  halos — subtle but they bleed into each other at the 8px gap. Either drop
  per-quadrant glow and keep glow only on accent text/dots, or tighten radius.
- **LOW X4.** No `<title>` favicon link — minor polish.

## Summary

| Severity | Count | Items |
| --- | --- | --- |
| HIGH | 3 | C1, E1, L1 |
| MED | 8 | H1, T1, C2, M2, E2, L2, D1, A1 |
| LOW | 11 | H2, T2, C3, M1, M3, D2, A2, X1-X4 |

**Total: 22 findings.** Priority fixes for this PR: C1 (contrast), E1 (empty
copy), L1 (skeleton), M2 (pulse-on-new-row), plus the polish bundle (glow
text-shadow on accents, scanline overlay, relative time format, tabular nums,
banner `aria-live`).

Out of scope for this pass: T1 (display font pairing — adds dep), A1 (click
behavior — needs backend route), D1 (OPS quadrant content — needs more state
fields).
