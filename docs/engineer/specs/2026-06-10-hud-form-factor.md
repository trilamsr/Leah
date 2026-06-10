# HUD Form-Factor — decision spec (stub)

**Date:** 2026-06-10
**Status:** STUB — placeholder filed to honor pointer in UX audit v4. Options listed, NOT ranked. Engineering writes options; UX designer + owner pick.
**Decision owner:** operator (Tri) + UX designer (TBD). Engineering author defers the pick.
**Source:** flagged by UX audit (`docs/engineer/briefs/2026-06-10-ux-audit-cross-surface.md`) §3. Audit explicitly says "Audit doesn't answer" — that disclaimer carries into this stub.

**Shipping impact:** Form-factor is **NOT a blocker for the 8 UX-audit top blockers** (polling state machine, voice earcons, memory inspector, first-launch wizard, undo, daemon-offline, accessibility, environmental robustness). Path (a) full ship per project memory `Leah ships path (a) full` proceeds in parallel. Form-factor decision can land before or after those blockers; current detached-window form is the interim default.

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

## Competitive baseline (2026, claims UNSOURCED — verify before deciding)

Claims below are the stub author's understanding as of 2026-06-10 and NOT linked to primary sources. Treat as starting point for owner's research, not as evidence.

| Product | Form-factor (claimed) | Trade-off (claimed) |
|---------|----------------------|---------------------|
| Apple Intelligence | Rendered into active app via system extensions (Mail, Notes, Notification Center) | Tight system integration; OS-locked |
| Granola | Inline floating panel following active meeting context | Context-aware; meeting-only scope |
| Limitless | Pendant + always-on inline overlay | Hardware tie-in; overlay needs OS hook |
| ChatGPT Voice | Full-screen takeover, no ambient | Modal; assumes singular attention |
| Rabbit R1 | Dedicated device | Separates input surface entirely |
| Leah today | Detached browser window | Portable; un-integrated |

**Before owner picks:** verify each row against primary source (WWDC video, product blog, hands-on review). Drop or correct any row that doesn't survive verification.

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

### E. Web app + Progressive Web App (PWA)

Leverage existing browser-shaped HUD; ship as installable PWA across macOS / Linux / iOS / Android.

Pros: zero new tech stack; cross-platform free; existing HTML/JS/CSS in `internal/hud/static/` reused.
Cons: same un-integrated detached-window feel as A; PWA install adoption uncertain; iOS Safari PWA support patchy.

### F. Mobile companion (iOS/Android native)

Native phone app as primary ambient surface; macOS HUD optional secondary.

Pros: ambient where user actually looks (phone); push notifications native; LiveActivities/widgets on iOS.
Cons: out of scope for v1 per audit; adds entire second codebase + app-store overhead; not what user asked for today.

### G. IDE extension (VS Code / JetBrains side panel)

Inline Leah inside the IDE the user is already in.

Pros: zero glance-tax for code-focused users; matches user's actual primary task; rich context (open file, selection).
Cons: only useful for code workflow — Leah's broader scope (calendar/mail/trip-planning) doesn't fit IDE surface; needs separate ambient surface for non-coding moments.

### H. Voice-only (no visual)

No HUD window. Audio-only ambient. Leah speaks status, summaries, recommendations.

Pros: zero attention tax; works for eyes-busy/blind users; environmental robustness (no public-space screen-glance issue).
Cons: discoverability cliff; no glanceable status; high accessibility cost for deaf users; no visual confirmation of what Leah heard.

---

## Option matrix (axis × option)

| Axis | A: detached | B: menubar+Spotlight | C: system-ext | D: hybrid | E: PWA | F: mobile | G: IDE | H: voice-only |
|------|:--:|:--:|:--:|:--:|:--:|:--:|:--:|:--:|
| macOS native feel | ✗ | ✓✓ | ✓✓✓ | ✓✓✓ | ✗ | n/a | ✓ | n/a |
| Linux support | ✓ | ✗ | ✗ | ✗ | ✓✓ | n/a | ✓ | ✓ |
| Cross-app context | ✗ | ✗ | ✓✓✓ | ✓ | ✗ | ✗ | partial | ✗ |
| Glance-tax | high | low | none | low | high | none | low | none |
| Impl effort (weeks) | 0 | 4–6 | 12+ | 8–10 | 2 | 16+ | 4 | 3 |
| Eyes-busy support | ✗ | ✗ | ✗ | ✗ | ✗ | partial | ✗ | ✓✓✓ |
| Public-space friendly | ✗ | ✓ | ✓ | ✓ | ✗ | ✓✓ | ✓ | ✗ (audio leaks) |
| Reuses existing code | ✓✓ | partial | ✗ | partial | ✓✓✓ | ✗ | partial | partial |

(Effort estimates by stub author; verify before commitment.)

---

## Recommendation

**No recommendation from engineering.** All eight options remain open. UX designer + operator pick based on:
- which axes matter most for personal-use bar (probably: native feel + cross-app context + low glance-tax),
- platform commitment (macOS-only vs Linux-included),
- effort budget per project memory `Leah ships path (a) full`.

---

## Decision & planning

- [ ] **Owner confirms:** form-factor decision IS / IS NOT a blocker for v1 ship. (Default per `Leah ships path (a) full`: NOT a blocker — current detached-window form is interim default.)
- [ ] **If blocker:** Owner + UX designer pick option (A–H) by [DATE]; engineering expands stub to full spec.
- [ ] **If not blocker:** Status quo (A) ships as interim. Form-factor research scheduled for [WEEK X] post-blockers.
- [ ] **Platform decision:** macOS-only acceptable for v1, or Linux parity required? (Eliminates B/C/D if Linux required.)
- [ ] **UX designer assigned** (NOT the stub author — engineering).
- [ ] **Competitive table verified** against primary sources before any pick.

---

## Out of scope for this stub

- Detailed wireframes (depends on chosen option).
- Implementation plan (depends on chosen option).
- Migration path from current browser-window HUD.
- Wails W38 native-wrap intersection — defer until option picked.

---

## Next step

This stub merges as-is to close v4 UX-audit's dangling pointer. Path (a) shipping continues in parallel on the 8 UX-audit blockers; form-factor research is non-blocking. When owner + UX designer are ready, this stub expands to a full spec with chosen path's wireframes, IPC contract, packaging plan, migration sequencing.
