# UX Audit — Cross-Surface (CLI / HUD / Voice) — v4

**Date:** 2026-06-10
**Baseline SHA:** `fc01ee2d12d5bacbd762ccd6919b1fd27b0675bd` (origin/main at audit time). All code-line references below are valid against this commit. Re-validate before acting if working tree has drifted.
**Scope:** All three primary user surfaces (`cmd/leah`, `cmd/leah-hud`, `internal/voice` + intent + recommend + brief + notify + memory/persona).
**Goal:** Define the criteria that make Leah's UX best-in-class, then audit current product against those criteria. Flag gaps as Severity (🔴 critical / 🟡 needs-work / 🟢 acceptable / ⚪ unverified).

**Decision owner:** operator (Tri). Ship-gate decision required before any consumer launch — see Part 4.

**Version log:**
- v1: initial pass, 7 pillars, surface-level audit.
- v2: applied reviewer #1 fact-corrections (forget DOES prompt; usage IS narrative-grouped); added pillars H (Attention), I (Network-Trust), J (Accessibility); deepened 9 surfaces.
- v3: applied reviewer #2 findings. New pillar K (Memory & Personalization Trust). New pillar L (Environmental Robustness). Re-ranked top blockers. Added HUD form-factor critique. Added latency calibration vs 2026 baselines.
- v4: applied reviewer #3 findings. Pinned baseline SHA. Reframed 80/20 ranking as funnel-stage-conditional (current-product = bootstrap; ranking shifts after PMF). Bolder ship-gate (NO consumer ship until blockers #1–#3 + accessibility clear). Decision owner named. Form-factor critique moved to follow-up spec stub. Added competitive positioning paragraph.

---

## Part 1 — UX Criteria (10 pillars)

### Pillar A: Responsiveness (latency budgets, calibrated vs 2026 baselines)

Baselines: Siri wake-ack ~100–150ms · Alexa wake-ack ~120ms · Granola voice-query e2e 1.2–1.5s · Apple Intelligence on-device 800ms–1.2s · ChatGPT Voice first audio ~600–900ms.

| # | Criterion | Target | Baseline note |
|---|-----------|--------|---------------|
| A1 | Wake → first acknowledgement (audio earcon mandatory) | ≤150 ms | Matches Siri |
| A2 | Voice utterance end → transcript ready | ≤600 ms p50, ≤1.2 s p95 | **Tightened from v2 — Granola 1.5s end-to-end means STT alone must be <1s** |
| A3 | Transcript → intent classification | ≤50 ms | Regex floor |
| A4 | Intent → first audio of reply (streaming TTS) | ≤700 ms | Match ChatGPT Voice; v2 said 300ms which only achievable if reply is pre-canned |
| A5 | HUD widget update after underlying state change | ≤1 s end-to-end | SSE push, not poll |
| A6 | CLI command first byte stdout | ≤200 ms (excluding LLM calls) | |
| A7 | CLI long-running command shows progress within | ≤500 ms | Spinner or token-stream |
| A8 | Barge-in: TTS cuts off mid-sentence within | ≤200 ms | Voice session confirmed implements this |
| A9 | **Time-to-first-useful-reply from `brew install`** | ≤5 min | New: end-to-end onboarding budget |

**All targets currently UNVALIDATED — no measurement data exists.** Add Prometheus histogram per criterion before sign-off.

### Pillar B: Event-driven over Polling

| # | Criterion | Rule |
|---|-----------|------|
| B1 | UI surfaces never poll for state daemon owns | Push via SSE / WebSocket / fsnotify |
| B2 | Polling reserved for external APIs that don't push | Document reason inline |
| B3 | SSE clients auto-reconnect with exponential backoff | 1s → 30s cap |
| B4 | Heartbeat distinct from data frames | `: keepalive` comment, not fake events |
| B5 | Stale data shown to user is labeled | "as of HH:MM" tag |
| B6 | Polling intervals match human perception | Calendar ≥30s, weather ≥10m, market ≥60s |

### Pillar C: Clarity & Feedback

| # | Criterion | Rule |
|---|-----------|------|
| C1 | Every user-initiated action gets confirmation signal | Visual, audio, or both within 300ms |
| C2 | Async work shows state: pending / running / done / failed | Never silent |
| C3 | Errors phrase the *what* + *next action* | "sox not found — install: brew install sox" |
| C4 | One canonical loading indicator per surface | Same shape across HUD widgets |
| C5 | Destructive actions require explicit confirmation | Prompt or attestation gate |
| C6 | Help text scannable in ≤10 seconds | Grouped verb list, 1-line per |
| C7 | Status visible without asking | Ambient HUD / `leah status` first 5 lines |

### Pillar D: Discoverability

| # | Criterion | Rule |
|---|-----------|------|
| D1 | `leah` (no args) shows grouped top-level verbs | Not flat 35-verb dump |
| D2 | Every verb supports `--help` with ≥2 examples | Not just `usage:` line |
| D3 | First-launch wizard runs without flags | Detects missing API key, integrations |
| D4 | HUD reveals available widgets via empty-state tiles | "+ Add weather" not blank em-dash |
| D5 | Voice "what can you do" returns dynamic capability list | Not silence |
| D6 | Integration catalog shows configured-or-not | `leah connect --list` ✓ |
| D7 | New capability surfaces within 1 session | Release tile or audio "new: …" |

### Pillar E: Consistency

| # | Criterion | Rule |
|---|-----------|------|
| E1 | Same verb means same thing across surfaces | accept/reject identical CLI ↔ HUD |
| E2 | Visual identity shared (color/type/spacing) | Single design-tokens file |
| E3 | Voice persona consistent across TTS backends | kokoro↔say preserves tone |
| E4 | Error-code stability | Exit codes and SSE error shapes don't drift |
| E5 | Configuration in one place | `leah config` verb, not env+flags+files |

### Pillar F: Reversibility & Safety

| # | Criterion | Rule |
|---|-----------|------|
| F1 | Destructive action has undo or audit trail | `leah forget --undo`, scrub-via-audit |
| F2 | Accept/Reject reversible within session | ≥5 min undo window, ideally session-scoped |
| F3 | OAuth disconnect clean (token revoked + file removed) | Single `leah disconnect <name>` |
| F4 | Dry-run mode for state-changing CLI | `--dry-run` standardized |
| F5 | Recovery from daemon crash transparent | HUD reconnects, shows "offline" if not |

### Pillar G: Onboarding & First-run

| # | Criterion | Rule |
|---|-----------|------|
| G1 | Zero-config "hello world" works in ≤60s | `brew install leah; leah` → useful output |
| G2 | Each integration enables with one command | `leah connect <name>` |
| G3 | OAuth scope explained in plain language | Not "attest <scope>? [Y/n]" |
| G4 | First voice interaction tutorial < 30s | Spoken or guided |
| G5 | HUD launches at login without manual plist editing | One command |
| G6 | Sample data shown until real data connected | Empty HUD unacceptable |
| G7 | API-key check is first-run gate | Plain error + link, not cryptic 401 |

### Pillar H: Attention & Notification Discipline *(NEW)*

| # | Criterion | Rule |
|---|-----------|------|
| H1 | User can mute / snooze any widget | Per-widget toggle, persists across restart |
| H2 | DND respected (macOS Focus modes) | Notifications suppressed during DND |
| H3 | Notification frequency cap | ≤1 banner per category per 5 min default |
| H4 | Scheduled visibility | "Show news only 7–10am" |
| H5 | Actionable vs decorative clearly separated | Recommendations card ≠ headline ticker |

### Pillar I: Network Transparency & Trust *(NEW)*

| # | Criterion | Rule |
|---|-----------|------|
| I1 | Daemon-down state visible to user | "Leah offline — reconnecting…" not blank |
| I2 | Last-success timestamp on every data surface | Trust requires freshness signal |
| I3 | AI suggestions explain themselves | "Why this?" — pattern, source, confidence |
| I4 | Privacy dashboard: what Leah knows about me | One screen lists profile + integrations |
| I5 | Cost visible alongside spend-causing actions | "$0.0034 charged" inline, not buried in `leah cost` |
| I6 | Stop-the-world kill switch | "leah pause" / HUD pause toggle: stop all background work |

### Pillar J: Accessibility *(NEW)*

| # | Criterion | Rule |
|---|-----------|------|
| J1 | All HUD widgets pass WCAG AA contrast | 4.5:1 text, 3:1 large |
| J2 | Voice errors are spoken, not just printed | Eyes-busy users get audio |
| J3 | CLI output color-independent | `--no-color` works; errors aren't only red |
| J4 | HUD keyboard-navigable | Tab order, focus rings, ESC dismisses |
| J5 | Captions for voice replies (HUD focus panel) | Deaf users see transcript |
| J6 | Live regions used correctly (`aria-live`) | Screen readers announce updates |

### Pillar K: Memory & Personalization Trust *(NEW in v3)*

Leah has `recall`, `memory`, `persona`, `operatormodel`, embeddings table. User TRUSTS what they can SEE. Today they see nothing.

| # | Criterion | Rule |
|---|-----------|------|
| K1 | User can view their full profile in one place | HUD "What Leah knows" screen + `leah profile show` |
| K2 | User can edit any memorized fact | Free-text edit, save, audited |
| K3 | User can delete individual memories without `forget all` | `leah forget <fact-id>` granular |
| K4 | Pattern that produced a recommendation is shown | Every rec card has "based on: <pattern>" |
| K5 | Embedding-search hits show source attribution | `leah recall` already does ✓ but HUD doesn't |
| K6 | Cross-app entity graph browsable | Show who/what/where Leah has linked |
| K7 | Memory aging visible | "last seen 30d ago" tag on stale facts |
| K8 | Wrong memories correctable in 1 step | "no, that's not right" voice intent triggers edit/delete flow |

### Pillar L: Environmental Robustness *(NEW in v3)*

Voice surface today assumes quiet home office. Real users: cafés, cars, kids in background, shared apartments.

| # | Criterion | Rule |
|---|-----------|------|
| L1 | False-wake rate in 60dB ambient noise | <1 false wake per hour |
| L2 | Wake works through music/podcast playback | Echo cancellation enabled |
| L3 | Privacy mode for public consent gates | Silent attestation (gesture/tap) instead of saying "yes Leah" aloud |
| L4 | Network-degraded mode | Cached recommendations + brief work offline |
| L5 | Battery-aware ticking | Reduce polling/wake on low battery |
| L6 | Multi-language hot-switch | At minimum: silent fallback to system locale, not English-only crash |

---

## Part 2 — Audit vs current product

### CLI (`cmd/leah/*`)

| # | Status | Finding (with code evidence) |
|---|--------|------------------------------|
| A6 | 🟢 | Cold-start fast; direct switch dispatch in `main.go:runCommand`. No startup penalty |
| A7 | 🟡 | No standardized spinner. `leah ship`, `leah self-build`, `leah brief` block silently. User can't tell hung vs working |
| C1 | 🟡 | Mixed. `leah connect gmail` prints `ok: gmail authorized → <path>` (`connect.go`). `leah disconnect` prints success. But `leah recall` returns "no matches" only on empty — silent on success otherwise |
| C2 | 🔴 | LLM-calling verbs (ask/recall --llm/self-build/ship) give no progress. User stares at frozen prompt. Cost charged silently — surfaces only via `leah cost` later |
| C3 | 🟢 | Pattern good where present. `voice.Listen` → `"sox not found in PATH — install: brew install sox"` exemplary |
| C5 | 🟢 | **Reviewer correction:** `forget.go` DOES prompt via `forgetAttest()`. Confirms unless `--yes`/`LEAH_FORGET_AUTO_ATTEST=1`. v1 was wrong |
| C6 | 🟡 | **Reviewer correction:** `usage()` IS narrative-grouped (ask/ship/review/status first, then memory ops, integrations, admin). Real defect: 35-line dump regardless. No `--help` for top-level filtering by category |
| D1 | 🟡 | Has light grouping but still 35 verbs. No `leah <category>` namespacing. New user overwhelmed |
| D2 | 🟡 | Some verbs have `usage:` (ship, connect, forget, cost). None have examples. No `leah <verb> --help` consistency — some accept `-h`, some show on no-args |
| D3 | 🔴 | No first-launch wizard. `leah` with no args → `usage()` + exit 2 |
| D6 | 🟢 | `leah connect --list` shows authorized status (`connect.go`) |
| E5 | 🟡 | Config sprawl: `LEAH_STATE_DIR`, `LEAH_PROMPT_DIR`, `LEAH_REVIEWER_PROMPT_DIR`, `LEAH_EMBED_BACKEND`, `LEAH_FORGET_AUTO_ATTEST`, `LEAH_CONNECT_AUTO_ATTEST`, `LEAH_HEALTHCHECK_URL`, `LEAH_BRIEF_DAILY`, `LEAH_BACKUP_LOCAL_PATH`, `LEAH_BACKUP_B2_REPO` + many flags + audit.jsonl on disk. No `leah config` |
| F1 | 🟢 | `audit.jsonl` captures every action with `BlastRadius` field (1/2/3 tiers) |
| F4 | 🟡 | **Reviewer correction:** `forget.go` HAS `--dry-run`. `backup.go` HAS `--restore`. Most verbs (ship/self-build/suggest) lack `--dry-run` |
| G1 | 🔴 | `brew install leah; leah` → usage dump. No API-key check, no setup wizard |
| G7 | 🔴 | First `leah ask` fails on missing `ANTHROPIC_API_KEY` with cryptic library error, not "set ANTHROPIC_API_KEY=… — get one at console.anthropic.com" |
| I5 | 🔴 | Cost charged on every `ask`/`recall --llm`/`ship`/`self-build` and ONLY visible via `leah cost`. User has no inline "this query cost $0.0034" feedback |
| I6 | 🔴 | No `leah pause` / kill switch. To stop autonomous daemon must `launchctl unload` plist |

---

### HUD (`cmd/leah-hud/` + `internal/hud/static/*`)

| # | Status | Finding (with frontend evidence) |
|---|--------|------------------------------------|
| A1 | n/a | HUD always-on, no wake concept |
| A5 | 🔴 | **Confirmed via code read:** `widgets.js` polls each tile on hardcoded TTL (weather 10min, market 60s, news 15min, calendar 30s). `ambient.js` opens EventSource on `/api/events` BUT ALSO `setInterval(pollMetrics, 5000)` against `/api/state`. Both surfaces poll despite SSE existing |
| B1 | 🔴 | Widget refresh is **client-side polling against daemon HTTP endpoints**. SSE channel exists but only `hud.state` rides it. Info-feed never pushed |
| B5 | 🔴 | **Confirmed:** Zero "as of HH:MM" labels in any frontend HTML. Stale market quote indistinguishable from fresh. `widgets.html` placeholders are just `—` |
| C1 | 🔴 | **Confirmed via `recommendations.js`:** accept/reject POSTs, on 204 calls `li.remove()`. No toast, no undo, no transient signal. Once gone, gone. Failure path shows error in tiny `#recs-status` muted text |
| C2 | 🔴 | **Confirmed:** widget tiles never show "loading" or "failed". They start as `class="widget placeholder"` showing em-dash and on fetch error: `widgets.js` does `if (!r.ok) return;` — silently keeps em-dash. User can't distinguish: never loaded / loading / failed / 15-min-stale |
| C4 | 🔴 | **Confirmed:** No canonical spinner/skeleton/pulse. Ambient HUD uses CSS class `ring` for listen/think states but widgets have no equivalent |
| D4 | 🔴 | **Confirmed:** `widgets.html` hardcodes 4 placeholders (weather, AAPL, news, calendar). No "+ Add widget" affordance. Unconfigured = em-dash forever |
| E2 | 🟡 | Three CSS files (`ambient.css`, `focus.css`, `recommendations.css`) all define `:root` color tokens *separately*. `recommendations.css` defines `--accent: #5af2ff` — copy-paste risk |
| F2 | 🔴 | **Confirmed:** Accept/reject in `recommendations.js` is immediate. `li.remove()` runs on 204. No 5s/10s undo window. **Reviewer was right: mis-tap = permanent** |
| F5 | 🔴 | **Confirmed:** `ambient.js` EventSource `es.onerror = () => { es.close(); setTimeout(openStream, 2000); };` — reconnects but UI never shows "offline" state to user. Just silently retries |
| G5 | 🟡 | Launches via `leah-hud` binary + plist install-janitor exists |
| G6 | 🔴 | **Confirmed:** First-launch ambient HUD shows time, but weather/cal/news/market all em-dashes. No sample data, no "connect calendar to see today" CTA |
| H1 | 🔴 | **Confirmed:** No mute, no snooze, no per-widget toggle in any HUD HTML or config |
| H2 | ⚪ | macOS Focus / DND respect unverified — likely not implemented |
| I1 | 🔴 | **Confirmed:** Daemon-down state never shown. Widgets just stay at em-dash. `ambient.js` reconnects EventSource invisibly |
| I2 | 🔴 | No timestamps anywhere. Same as B5 |
| I3 | 🔴 | **Confirmed via `internal/recommend/recommendation.go:31`:** `// Reason/ExpiresAt likewise wait for the surface layer (W19).` Reason field exists in Go struct but is **not wired into HUD JSON or rendered**. `recommendations.js` only renders `r.pattern`, `r.action`, tier class. No "why this suggestion?" |
| J1 | ⚪ | Contrast unverified — `--muted: rgba(255,255,255,0.55)` on dark bg is borderline AA fail |
| J5 | 🔴 | **Confirmed via `focus.html`:** Focus panel renders Reasoner reply as text — but voice reply path (loop.go Speak) does NOT echo to focus panel transcript. Deaf user using voice gets nothing |
| J6 | 🟡 | `aria-live="polite"` on `#recs-list` ✓ and `#response` ✓. `widgets.html` lacks aria-live — silent updates for screen reader |

---

### Voice (`internal/voice/`, `internal/intent/`)

| # | Status | Finding (with code evidence) |
|---|--------|------------------------------|
| A1 | 🔴 | **Confirmed via `wake/wake.go`:** EnergyDetector triggers on RMS threshold, returns `(bool, error)`. **No audio earcon emitted on detect.** Caller in `session.Session` just transitions state. User says "Hey Leah" → silence |
| A2 | 🟡 | `Listen` shells `sox` then `whisper-cli` sequentially (`voice/listen.go`). Subprocess spawn cost + model time. Unmeasured |
| A3 | 🟢 | `intent.Classify` regex sub-ms |
| A4 | 🟡 | TTS via `kokoro` or `say`. `say` blocks playback. No streaming voice |
| A8 | 🟢 | **New finding:** Barge-in IS implemented (`session.Session.Run`, `loop.go` `cancelTurn`). When user speaks during TTS, in-flight reply ctx is cancelled. Good UX, undocumented |
| C1 | 🔴 | No wake earcon. `session.Session` opens with TTS `"Voice session starting. Confirm out loud: yes Leah."` — verbal but heavy. Imagine saying this every wake |
| C2 | 🔴 | If reasoner takes >1s, no "working on it" filler. `loop.go` just waits then speaks reply. Reasoner error → `"I couldn't reason that through, try again."` — generic |
| C3 | 🟡 | `errReasonFallback` is OK but generic. Specific errors (network, budget, attestation) all collapse to same string |
| C5 | 🟢 | Voice session uses attestation phrase `"yes leah"` as gate. Strong consent UX |
| D5 | 🔴 | **Confirmed:** `intent.Classify` returns 4 verbs (ask/ship/review/status). `intents/trip.go` adds trip planning. No `"what can you do"` handler, no enumerate-verbs intent |
| F1 | 🔴 | No "cancel that" voice undo. Session barge-in cancels TTS but not action |
| G4 | 🔴 | No spoken tutorial intent |
| H3 | 🔴 | Voice notifier (`notify/voice.go`) speaks every notification. No throttle. Phone could read 10 headlines in a row |
| I3 | n/a | Recommendations not voiced with reason — same as HUD |
| J2 | 🔴 | `voice/listen.go` error `"sox not found in PATH — install: brew install sox"` is **stderr text**. Eyes-busy voice user hears nothing |

---

### Notification surface (`internal/notify/`)

| # | Status | Finding |
|---|--------|---------|
| H3 | 🔴 | **Confirmed:** `Fanout` calls every notifier (Desktop+Voice+Pushover) on every event. No dedup, no rate-limit, no per-category cap |
| H2 | 🔴 | `Desktop.Notify` shells `osascript -e 'display notification ...'` directly — macOS Focus mode respected by OS, but Leah doesn't *know* if delivered. No Focus-aware silencing of voice/pushover legs |
| C5 | 🟢 | Quote-escape on title/body prevents AppleScript injection (security ✓) |

---

### Brief (`internal/brief/`)

| # | Status | Finding |
|---|--------|---------|
| I3 | 🔴 | Brief renders: yesterday actions, spend, agents, recommendations, mail, calendar. **No "why these recommendations" explanation** |
| I5 | 🟡 | `WeekToDateUSD` + `ProjectedMonthly` shown in brief. Better than CLI |
| C3 | 🟡 | "Silent absence beats noisy 'unavailable'" (per `GatherOpts` comment) — adapter not configured = section omitted. Pragmatic but user never learns Gmail integration exists |
| G6 | 🔴 | Same — if zero integrations configured, brief shows only audit + cost. No "want to add Gmail? leah connect gmail" CTA |

---

### Recommendations engine (`internal/recommend/`)

| # | Status | Finding |
|---|--------|---------|
| I3 | 🔴 | **Confirmed:** `recommendation.go:31` comment: `// Reason/ExpiresAt likewise wait for the surface layer (W19).` Reason field is **TODO**. Suggestions surface as bare `pattern + action` with tier color. User cannot ask "why?" |
| F2 | 🔴 | No undo. `apply.go` is one-way: accept executes, reject quarantines. Mis-tap permanent |

---

### Workspace / multi-context (`cmd/leah/workspace.go`)

| # | Status | Finding |
|---|--------|---------|
| C7 | 🟡 | `leah workspace show` exists. HUD doesn't render active workspace anywhere |
| E1 | 🟡 | Workspace persists in ctxmgr; persona attached. CLI verb `workspace switch <name>` works. **HUD has no workspace switcher** |

---

### Watchdog (`internal/watchdog/`)

| # | Status | Finding |
|---|--------|---------|
| F5 | 🟡 | Heartbeat pings external healthchecks.io URL. **User-facing recovery: zero.** If daemon dies, HUD reconnects silently or stays at em-dash. healthchecks.io pages *operator* but doesn't tell *user* what's happening |
| I1 | 🔴 | No on-device "leah is down" indicator |

---

### Memory & Personalization (`internal/memory`, `persona`, `operatormodel`, `recall`, `embed`) *(NEW in v3)*

| # | Status | Finding |
|---|--------|---------|
| K1 | 🔴 | **No "what Leah knows about me" screen exists.** Profile in `operator_profile` SQLite table. CLI has no `leah profile show`. HUD has no widget. User must `sqlite3` the file |
| K2 | 🔴 | No edit UX. User who notices wrong fact has no fix path except `leah forget <pattern>` |
| K3 | 🟡 | `leah forget <pattern-id>` exists but granularity unclear — pattern IDs not surfaced to user. `leah forget all` is the discoverable path |
| K4 | 🔴 | Recommendation cards show pattern but **no "based on:" attribution chain.** Reason field still TODO at `recommendation.go:31` |
| K5 | 🟢 | `leah recall` shows source tag (audit/contact/project/decision) ✓ |
| K6 | 🟡 | Cross-app entity graph exists (`internal/knowledge` per recent commit) but no UI to browse |
| K7 | 🔴 | No memory aging UI. `last_seen` not surfaced |
| K8 | 🔴 | No "no, that's not right" voice intent. `intent.Classify` has 4 verbs; correction not one of them |

---

### Environmental Robustness (`voice/wake`, `voice/listener`, network paths) *(NEW in v3)*

| # | Status | Finding |
|---|--------|---------|
| L1 | 🔴 | **Confirmed via `wake/wake.go`:** EnergyDetector is pure RMS threshold. Coffee-shop ambient 60–70dB = sustained false-wake. No spectral filter, no keyword model (Porcupine deferred per W13 doc) |
| L2 | 🔴 | No echo cancellation visible. Sox raw `-d` input. Music playing = false wake or missed wake |
| L3 | 🔴 | **Confirmed:** `session.go` attestation phrase `"yes leah"` spoken aloud. Public-space hostile |
| L4 | 🔴 | Recommendations engine is SQLite-backed (W17) — could work offline. Brief calls live regatta CLI + gmail/gcal HTTP — no offline mode |
| L5 | 🔴 | `daemonloop` ticks at fixed cadence regardless of battery state. macOS power-source not consulted |
| L6 | 🔴 | All TTS/STT defaults English. No locale detection. `intent.Classify` regex is English-only |

---

### HUD Form-Factor — flagged for follow-up spec (NEW in v3, moved out of audit in v4)

HUD form-factor critique (detached 720px focus + 320px ambient browser windows vs Granola inline / Apple Intelligence system extension / Limitless overlay) is **out of scope for this audit**. Tracked separately: see follow-up spec `docs/engineer/specs/2026-06-10-hud-form-factor.md` (to be created). One-line summary: Leah's detached-window form may compete with user's primary task; needs separate decision-doc, not audit findings.

---

## Part 3 — Highest-leverage fixes (re-ranked per reviewer)

Ranked by user-visible impact × implementation cost. Reviewer's reordering applied.

### Top blockers (must fix before any consumer launch) — funnel-stage conditional

**Two rankings depending on funnel stage:**

- **Bootstrap stage (current, ~zero users):** Onboarding IS the funnel. First-launch wizard, API-key gate, sample-data HUD = #1. Polling-staleness is rare (no users to hit it 100×/day). Use BOOTSTRAP column below.
- **Post-PMF stage (after first 100 retained users):** Existing-user daily pain dominates. HUD widget state machine + kill polling = #1. Use POST-PMF column below.

**Default to BOOTSTRAP ranking today.** Re-rank to POST-PMF once retention cohort exists.

| Rank | BOOTSTRAP (today) | POST-PMF |
|------|------------------|----------|
| #1 | First-launch wizard + API-key gate (G1, G3, G6, G7, D3) | HUD widget state machine + kill polling (C2, B1, B5, A5, I2) |
| #2 | Voice earcons + lighter attestation (A1, C1, C4, L3) | Voice earcons + lighter attestation |
| #3 | Recommendation explainability + Memory inspector (I3, K1, K4) | Recommendation explainability + Memory inspector |
| #4 | HUD widget state machine + kill polling | First-launch wizard |
| #5 | Undo for destructive actions (F1, F2, C5) | Undo for destructive actions |
| #6 | Daemon-offline indicator (I1, F5) | Daemon-offline indicator |
| #7 | Accessibility minimum (J1, J2, J4, J5) | Accessibility minimum |
| #8 | Environmental robustness (L1, L3) | Environmental robustness |

**Block-detail per item:**

1. **🔴 First-launch wizard + API-key gate (G1, G3, G6, G7, D3)** — `leah` with no args + no config → interactive wizard: check `ANTHROPIC_API_KEY` with plain "get one free at console.anthropic.com", walk user to one integration, launch HUD with sample data. Single change unlocks 5 criteria. **Bootstrap #1 because today onboarding = the funnel.**
2. **🔴 Voice earcons + lighter attestation (A1, C1, C4, L3)** — Three short sounds (wake-ack, processing, error). **Replace heavy verbal attestation with silent gate**: gesture (Touch ID / global hotkey / menubar click) for public-space consent. Verbal opt-in only when user explicitly enables. Café user not embarrassed. AirPods user hears wake-ack.
3. **🔴 Recommendation explainability + Memory inspector (I3, K1, K4)** — Wire the `Reason` field (currently TODO at `internal/recommend/recommendation.go` per baseline SHA — comment "wait for surface layer (W19)") through JSON → JS card "Why this?" disclosure. Add `leah profile show` + HUD "What Leah knows" screen listing patterns, sources, last-seen. Memory user can't see = memory user won't trust.
4. **🔴 HUD widget state machine + kill client polling (C2, B1, B5, A5, I2)** — Replace silent fetch fail in `internal/hud/static/widgets.js`. States: loading skeleton / loaded / stale (TTL exceeded, show "as of HH:MM") / failed (explicit "couldn't load — retry"). Push via SSE — kill TTL polling and `setInterval(load, 15000)` in `recommendations.js`. **Post-PMF #1, bootstrap #4.**
5. **🔴 Undo for destructive actions (F1, F2, C5)** — `leah forget --undo` (last N), HUD recommendation toast with 10s "Undo", voice "cancel that" intent. Eliminates mis-tap data loss.
6. **🔴 Daemon-offline indicator (I1, F5)** — When EventSource fails or `/api/state` 5xx: show "Leah reconnecting…" banner. Don't hide failure behind em-dashes.
7. **🔴 Accessibility minimum (J1, J2, J4, J5)** — WCAG AA contrast pass, voice errors spoken, focus panel keyboard nav, voice replies captioned. **Liability + table stakes — non-negotiable regardless of funnel stage.**
8. **🔴 Environmental robustness (L1, L3)** — Wake detector beyond pure RMS (Porcupine integration). Silent attestation as default. Without these, voice surface is home-office-only — Leah is not a personal assistant if it dies in a Starbucks.

### Major fixes (post-launch)

9. **🟡 Grouped help + every verb has examples (D1, D2, C6)** — `leah` → grouped usage. Every `<verb> --help` shows 2 examples.
10. **🟡 Notification discipline: per-category rate limit + mute (H1, H3)** — `notify.Fanout` adds throttle. Per-category snooze persisted.
11. **🟡 `leah config` unified verb (E5)** — Replace 11 env vars + N flags + JSON files with `leah config set key=value` + `leah config show`.
12. **🟡 `--dry-run` standardized (F4)** — Add to ship/self-build/suggest/disconnect.
13. **🟡 Cost inline (I5)** — After every LLM call, print `($0.0034)` so user sees cost adjacent to query, not buried in `leah cost`.
14. **🟡 Streaming TTS (A2, A4)** — Speak first phoneme while later text generating. Cut perceived voice latency.
15. **🟡 Design tokens file (E2)** — Single `internal/hud/static/tokens.css`. Other CSS files `@import`. No more copy-paste of `:root` vars.
16. **🟡 Spoken capability discovery (D5)** — Voice intent `"what can you do"` → list current categories. Dynamic from CLI verb registry.
17. **🟡 Voice errors spoken not printed (J2)** — `voice/listen.go` errors → also `Speak()` short version. Eyes-busy.
18. **🟡 `leah pause` kill switch (I6)** — Single verb (+HUD button) stops daemonloop, mutes voice, hides recommendations.
19. **🟡 HUD workspace indicator (E1, C7)** — Active workspace pill in ambient HUD header.
20. **🟡 Privacy dashboard (I4)** — `leah show-me-everything` or HUD "What Leah knows" screen: connected integrations, profile patterns, audit row count, embedding count.

### Secondary

- 🟡 OAuth scope explanation — replace `attest <scope>? [Y/n]` with plain English consent screen.
- 🟡 Voice persona regression test — fixed-input → assert tonal characteristics across kokoro/say.
- 🟡 Accessibility audit (J pillar) — separate pass before any consumer ship: contrast, captions, keyboard nav, screen reader.

---

## Part 4 — Sized verdict

**Per two-pass adversarial review:** Would NOT ship Leah to a paying consumer on this UX today.

**Daily-pain blockers (hit user 100×/day):**
- Polling widgets show stale data with no label (B5/I2/A5/B1)
- Widgets have no loading/failed states (C2)
- Daemon-down invisible (I1)
- Recommendation mis-tap permanent (F2)
- No explanation for AI suggestions (I3)
- No memory inspector / can't see what Leah knows (K1, K4)

**One-time blockers (hit user 1× but high-magnitude):**
- First-launch cliff (G1, G7)
- No wake earcon (A1)
- Voice attestation phrase aloud in public (L3)

**Liability blockers (cannot ship without):**
- Accessibility: blind users get blank, deaf users get silence (J1–J6)
- Environmental: voice surface dies in 60dB ambient (L1)

### Ship gate (binding)

**NO consumer ship until BOOTSTRAP #1, #2, #3 + Pillar J (accessibility minimum) all merged and verified.** Path (b) below is contingency only.

**Path (a) — full ship:** Fix all 8 blockers → 8–12 weeks → ship at 100% UX. Competitive risk: Apple Intelligence / Granola ship overlapping surfaces first.

**Path (b) — constrained beta:** Fix bootstrap #1–#3 + accessibility minimum → 3–4 weeks → ship publicly tagged "beta" with documented limitations: voice English-only, public-space mode opt-in beta, memory editor read-only, mobile companion not shipped, ambient HUD form-factor under review.

**Decision required from owner (Tri) BEFORE ship-prep work begins:**
- [ ] Path (a) full OR Path (b) beta?
- [ ] If (b): which limitations are acceptable to document publicly?
- [ ] Target ship date?
- [ ] Reviewer who signs off on each blocker's merge?

**Until decided, the audit's standing recommendation is: BLOCK ship. Pick a path before any user-facing "Leah is ready" announcement.**

### Competitive positioning (why ship at all?)

Leah is not a slower Granola. Differentiation must be explicit before the audit's "ship broken" tradeoff is even rational:

- **vs Apple Intelligence:** runs cross-OS, owns its data layer, scriptable via CLI, no walled garden.
- **vs Granola:** broader scope (full personal-OS, not meeting-only); local-first memory.
- **vs ChatGPT Voice:** persistent memory + cross-app actions, not stateless chat.
- **vs Limitless / Rabbit:** no dedicated device; runs on hardware the user already owns.

If these differentiators don't hold, the UX gaps are unacceptable. If they do, path (b) is defensible because the alternative (waiting 8–12 weeks while competitors take the surface) costs more than the documented beta gaps.

**Open question for owner:** is the differentiation list above the actual positioning, or is the audit guessing? Confirm or rewrite before path (a)/(b) decision.

---

## Part 5 — Open questions (for impl spec, NOT UX audit)

These belong in a follow-up spec, but flagged:

1. SSE multiplex: one channel with kind filter vs Broadcaster fanout per kind?
2. First-launch wizard: CLI-only or HUD setup mode?
3. Earcon set: design from scratch vs adopt macOS system sounds?
4. Undo window duration: 5s vs 10s vs session-scoped?

## Part 6 — Out of scope (intentionally)

- Theming / customization
- Multi-user / shared state
- Mobile companion
- Internationalization
- Adversarial security (separate threat model needed)

---

## Appendix A — Surfaces this audit covered (v3)

CLI (35 verbs, main.go + 30 sibling files) · HUD ambient/widgets/focus/recommendations HTML+JS+CSS · voice wake/listener/session/loop/intents · intent classifier · notify fanout/desktop/voice/pushover · brief generator · recommend engine · watchdog · workspace · cost view · connect/disconnect · memory/persona/operatormodel/embed (read-only via grep — no execution path probed) · knowledge cross-app graph (W30k — package present, UI absent).

**Not audited:** macOS adapter permission flows (TCC prompts), backup/restore restore-time UX, regatta cloud-connect onboarding, attestation device-pairing flow, watchdog page-the-operator integration, plist install-janitor first-run.

## Appendix B — Reviewer fact-checks applied across versions

**v1 → v2 (reviewer #1):**
- C5 verdict 🔴 → 🟢 (forget DOES prompt via `forgetAttest()`)
- C6 "alphabetical" → narrative-grouped but still 35-line dump
- F4 generic → `forget --dry-run` exists, most verbs lack it
- Ranking: HUD push #1 → first-launch #1
- "as of HH:MM" buried in #7 → promoted to top-blocker
- Pillars added: H Attention, I Network/Trust, J Accessibility

**v2 → v3 (reviewer #2):**
- Confirmed v2 claims against code: Reason field unwired ✓, fanout no rate-limit ✓, wake no earcon ✓, ambient.js polls /api/state every 5s alongside SSE ✓, recommendations.js setInterval(load, 15000) ✓, focus.js doesn't render voice replies ✓.
- Ranking RE-INVERTED: first-launch demoted from #1 to #4 (existing-user pain hits 100×/day vs 1× onboarding).
- HUD widget state machine + kill polling → new #1.
- New pillar K: Memory & Personalization Trust (user can't see/edit what Leah knows).
- New pillar L: Environmental Robustness (RMS-only wake = false wakes in 60dB ambient; "yes leah" aloud in public).
- New A9: time-to-first-useful-reply ≤5 min from `brew install`.
- A2/A4 tightened vs 2026 baselines (Granola/Apple Intelligence/ChatGPT Voice).
- HUD form-factor critique added (detached browser window vs inline Apple Intelligence / Granola).
- Accessibility moved from footnote to top-blocker #7 (liability).
- Cost-of-delay path (a) vs (b) added to verdict.

**v3 → v4 (reviewer #3):**
- Baseline SHA pinned (`fc01ee2`) so line refs don't rot.
- 80/20 ranking assumption reframed as funnel-stage-conditional (bootstrap vs post-PMF rankings shown as separate columns; default to bootstrap today).
- Verdict tightened from "Recommendation: path (b)" to "BLOCK ship until #1–#3 + accessibility; path (b) contingency only".
- Decision owner named (operator/Tri). Ship-gate checklist added.
- HUD form-factor critique moved out of audit findings to follow-up spec stub.
- Competitive positioning paragraph added — explicit Leah differentiation vs Apple Intelligence, Granola, ChatGPT Voice, Limitless/Rabbit.
- Verified ✓ all code claims still hold against baseline SHA: `recommendation.go` Reason TODO, `widgets.js` TTL polling, `wake/wake.go` no earcon, `session.go` "yes leah" attestation, `forget.go` does prompt.

**v4 still unresolved:**
- Latency targets aspirational — no Prometheus measurement.
- HUD form-factor decision deferred to separate spec.
- Memory editor read vs read-write scope not decided.
- Multi-language story missing entirely.
- Competitive positioning paragraph is the AUDIT's guess; needs owner confirmation.
## Part 7 — Felt-latency investigation (2026-06-10 follow-up, **amended**)

Investigation pass on perf/responsiveness through the lens of **time from user-action → felt-response**. Four parallel probes ran; adversarial reviewer on PR #220 caught 2 hallucinated file:line claims (P1+P2) that did not survive re-verification against actual code. Table below reflects **verified** state only.

**Amendment note (2026-06-10):** Original Part 7 claimed `recommendations.js:100` had `setInterval(load, 15000)` and `ambient.js:65` had `setInterval(pollMetrics, 5000)`. Re-verification: `recommendations.js` is 135 lines with **zero setInterval calls** (EventSource at line 114); `ambient.js` is 59 lines with only `setInterval(tickClock, 1000)` at line 57 (EventSource at line 47). HUD state push is **already implemented for recs + ambient**. The lag claim was wrong. Issue #210 closed retroactively.

### Verified felt-latency sources

| # | Surface | File:line | Current cadence | Felt symptom | Effort | Δ latency |
|---|---------|-----------|-----------------|--------------|--------|-----------|
| P3 | HUD calendar/market/news/weather | `internal/hud/static/widgets.js:3-8` | Per-tile TTLs: weather 600000ms, market 60000ms, news 900000ms, calendar 30000ms | Stale data shown without "as of HH:MM" label; user can't tell loading/failed/15-min-stale | S | trust signal (couple with B5/I2) |
| P4 | SSE channel | `cmd/leah-hud/app.go` (handleEvents heartbeat-only) | Heartbeat-only ("Real telemetry frames arrive in W35") | Infrastructure exists, daemon→HUD push for non-rec state still missing | M | unlocks future widget push |
| P5 | `leah ask` CLI | `internal/reasoner/anthropic.go:45` | `Messages.New()` blocking, no `cache_control` | User stares at frozen prompt 3-8s, no first-token feedback | S | -3-5s perceived |
| P6 | `leah review` CLI | `internal/reviewer/anthropic.go:44` | Same blocking pattern | Verdict appears all-at-once after full completion | S | -3-5s perceived |
| P7 | Prompt caching | grep `cache_control "ephemeral"` → **zero hits** | System prompts never cached | Repeated input cost on every call (token spend) | S | -90% input cost on repeats (if sys prompt >1024 tok) |

### Refuted / re-scoped from initial audit

| Item | Initial claim | Reality | Action |
|------|---------------|---------|--------|
| HUD recommendations 15s poll (P1) | "setInterval(load, 15000) blocks rec UI" | recommendations.js has NO setInterval; line 114 opens EventSource on `/api/events`. Accept/reject re-fetches once at line 102 | **Already on SSE.** #210 closed. |
| HUD ambient 5s poll (P2) | "setInterval(pollMetrics, 5000) polls /api/state" | ambient.js has NO pollMetrics; line 47 opens EventSource. Only setInterval is 1s clock-tick | **Already on SSE.** No issue filed. |
| Brief feed gather serial | "morning brief stalls behind slowest API, 2-3s" | `internal/brief/feeds.go:77-101` IS serial, but reporters are **orphaned / staged-then-deleted** (not on active path) | Defer until W32 re-wires; trap-issue #214 filed |
| Voice latency budget | "voice loop high felt-lag" | `internal/voice/listener/listener.go:97` `Real.Start()` returns `ErrNotImplemented`; Session.Run unreachable from CLI/daemon | **No current felt-lag — voice non-functional.** Feature-gap, not latency. Blocked on W12. |
| Brief / Trip / HUD make LLM calls | "streaming benefits these surfaces" | Brief has zero LLM; trip W62 is adapter-only; HUD streams from daemon, doesn't call LLM | Drop from streaming surface |

### Ship order by verified felt-latency-per-effort

| Rank | Item | Effort | Notes |
|------|------|--------|-------|
| 1 | Stream `leah ask` + `leah review` (`NewStreaming` swap) + add `cache_control` on System block | S+S | **Now #1 — was #2.** Only verified daily-pain LLM lag. Self-contained per-package. Measure system-prompt tokens first. Issues #212 #213. |
| 2 | "as of HH:MM" freshness label on every widget (P3) — pair with B5/I2 | S | Trust signal. Issue #211. |
| 3 | Knowledge ingest tx-batch (Part 2 cross-app reads) | S | Background → foreground read latency. |
| 4 | SSE telemetry frames (W35) — daemon-pushed state events for non-rec widgets | M | Genuine gap, but recs+ambient already on SSE so smaller delta than originally claimed. |
| 5 | Brief `errgroup` fan-out | S | **Deferred** — orphaned reporters; revisit at W32 re-wire. Trap #214. |
| 6 | Voice whisper-stream (W12) + speculative-intent dispatch | L | Feature-gap not latency-fix. |

### Parallelizable PR slots (file-disjoint, fits 6-cap dispatch rule)

- Slot A: `internal/reasoner/` — stream + cache_control (#212)
- Slot B: `internal/reviewer/` — stream + cache_control (#213)
- Slot C: `internal/hud/static/widgets.js` + `internal/hud/widgets/*` — freshness labels (#211)

### Verified source observations (post-reviewer-pass)

- HUD: `recommendations.js:102/114` (fetch + EventSource), `ambient.js:47/57` (EventSource + clock-tick only), `widgets.js:3-8` (TTLs)
- LLM (verified): `internal/reasoner/anthropic.go:45`, `internal/reviewer/anthropic.go:44`
- Brief (orphan-status confirmed): `internal/brief/feeds.go:77-101`
- Voice (stub-status confirmed): `internal/voice/listener/listener.go:97`

### Cross-ref to Part 3

- Part 3 #1 (HUD widget state machine + kill polling) — **partially-cleared.** Recs+ambient already on SSE; widget freshness labels + W35 telemetry frames remain.
- Part 3 #2 (Voice earcons + attestation) — **decoupled** from latency; voice is feature-gap.
- New: streaming `ask`/`review` + prompt cache — add as Major fix #21.

### Meta-lesson

Two of four investigator-claimed file:line refs (P1 + P2) were hallucinated and survived ranker + initial audit-session Phase 5 scrape. Reviewer on PR #220 caught them. Codified as `feedback_verify_on_path_before_ranking.md` + `feedback_skill_compliance_vs_adversarial.md`. All future "X is slow / X is broken" claims at S+ effort must include either (a) re-verified file:line at PR time or (b) explicit `[unverified]` tag pending probe.
