# HUD Form-Factor — decision spec (stub)

**Date:** 2026-06-10
**Status:** STUB — placeholder filed to honor pointer in UX audit v4.
**Decision owner:** operator (Tri).
**Source:** flagged by UX audit (`docs/engineer/briefs/2026-06-10-ux-audit-cross-surface.md`) §3, moved out of audit findings because form-factor is orthogonal to UX criteria audit. Lives here for a focused decision.

---

## Problem

Today Leah's HUD ships as two detached browser windows:

- **Ambient HUD** — 320px panel, always-on, shows clock/weather/calendar/news/market + recommendation rings (`internal/hud/static/ambient.html`).
- **Focus panel** — 720px panel, on-demand, for query/response and recommendation cards (`internal/hud/static/focus.html`, `recommendations.html`).

Pros:
- Portable across monitors.
- Browser tech stack — fast iteration.
- Wails-shaped (per W34) → future native wrap option.

Cons:
- No system integration. Detached window competes with user's primary task.
- No per-app context. HUD doesn't know which app the user is in.
- Requires explicit user attention to peek at it. Ambient surface still costs glance-tax.
- Visual chrome (border, header, dismiss button) too heavy for ambient surface.

---

## Competitive baseline (2026)

| Product | Form-factor | Trade-off |
|---------|-------------|-----------|
| Apple Intelligence | Rendered into the active app via system extension (Mail, Notes, Notification Center) | Tight system integration; OS-locked |
| Granola | Inline floating panel that follows active meeting context | Context-aware; meeting-only scope |
| Limitless (post-acquisition) | Pendant + always-on inline overlay; minimal chrome | Hardware tie-in; overlay needs OS hook |
| ChatGPT Voice | Full-screen takeover, no ambient | Modal; assumes singular attention |
| Rabbit R1 | Dedicated device | Different tradeoff — separates input surface entirely |
| Leah today | Detached browser window | Portable; un-integrated |

---

## Options

### A. Stay detached browser window (status quo)

Pros: zero work. Wails wrap path open (W38).
Cons: every con above persists.

### B. macOS menubar menulet + Spotlight-style overlay panel

Menubar item with pull-down ambient widgets (a la iStat Menus / Bartender). Global hotkey opens a Spotlight-shaped focus overlay (centered, dim background) for query/response.

Pros: native macOS feel; ambient surface costs no window-management; focus panel respects user attention pattern (Spotlight muscle memory).
Cons: macOS-only (Linux Leah deferred); requires Cocoa/SwiftUI shell or robust Tauri/Wails native menubar plumbing.

### C. Per-app sheet via system extension (Apple Intelligence parity)

Inject Leah into Mail/Notes/Messages/Calendar via system extensions; ambient surface lives in Notification Center widget.

Pros: matches Apple Intelligence pattern; per-app context for free.
Cons: locked to apps Apple lets you extend; loses cross-app entity graph payoff; high impl cost; security review surface area.

### D. Hybrid — menubar + Notification Center widget + Spotlight overlay

Combine B's menubar pull-down for at-a-glance, Notification Center widget for ambient persistence, Spotlight-shaped overlay for focus query.

Pros: native, low-chrome ambient; respects OS conventions; cross-app stays in Leah's own context engine (not Apple's).
Cons: three surfaces to maintain; design coherence overhead.

---

## Recommended (NOT YET DECIDED — owner picks)

Audit's instinct: **D (hybrid).** Best matches "personal-OS that sees everything, never demands attention." But cost is high. Decision deferred to owner.

---

## Decision required

- [ ] Pick A / B / C / D / other.
- [ ] If B or D: macOS-only acceptable for v1? Linux deferred?
- [ ] If C: which apps to extend first?
- [ ] Target version for form-factor change?
- [ ] Owner who designs (this stub author is engineering, not UX designer).

---

## Out of scope for this stub

- Detailed wireframes (depends on chosen option).
- Implementation plan (depends on chosen option).
- Migration path from current browser-window HUD.
- Wails W38 native-wrap intersection — defer until option picked.

---

## Next step

Operator selects option. Author expands stub into full spec with chosen path's wireframes, IPC contract, packaging plan, migration sequencing. Spec ships as separate PR; this stub merges as-is to honor v4 audit pointer.
