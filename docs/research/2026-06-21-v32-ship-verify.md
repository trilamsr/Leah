# v3.2 Spec Ship-Readiness Verification — Phase 1 Gate

**Spec:** `docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md` (2724 lines, v3.2)
**Auditor question:** can the engineer assigned to Phase 1 start writing Swift on Monday?
**Verdict:** **NEEDS-7-DECISIONS-FIRST** (spec is otherwise complete; the gaps are decision-shaped, not draft-shaped).

---

## Phase 1 — per-deliverable score

| # | Deliverable | Score | Blocker (1 line) |
|---|---|---|---|
| 1 | Daemon ↔ LLM streaming (Claude Sonnet + Anthropic SDK + prompt caching) | **BLOCKED** | Model id never named (spec says "Lock model id in `internal/reasoner/`" — *this audit is that lock*); SDK language unspecified (Go vs Swift); API key custody undefined (Keychain mentioned only for integration tokens, never the Anthropic key). |
| 2 | Knowledge store (sqlite-vec) | **PARTIAL** | sqlite-vec extension load path spec'd (§17.10) but **embedding model is explicitly deferred to "§17.x LLM provider"** — that section does not exist. Embedding dim, provider, on-device vs cloud are all open. |
| 3 | Memory pipeline | **BLOCKED** | One sentence in §19 ("auto-capture conversation + state"). No schema, no event taxonomy, no retention policy, no test plan. §16 has zero coverage. |
| 4 | Focus panel + ⌥Space + streaming text | **READY** | §17.2 NSPanel mask locked, §6.1 hotkey locked, §10.7 streaming envelope locked. §13.4 wireframe present. §16.5 streaming test specified. |
| 5 | Menubar item | **READY** | §4.2 template-image + shape-based state locked; trivial. |
| 6 | 3-step wizard | **READY** | §8.2–§8.4 + §13.11 wireframes; §8.7 error matrix; §19 explicitly trims to 3 steps. |
| 7 | Settings (General + Privacy + Permissions) | **READY** | §9.2 IA tree exhaustive; §9.3 status glyphs locked; §17.11 persistence locked. |
| 8 | Developer ID + notarization + Sparkle | **PARTIAL** | §17.1 enumerates entitlements + tools; §17.1 names `https://updates.leah.app/appcast.xml` — but **DNS/hosting custody undefined**; Sparkle EdDSA key "offline-backed" without naming the backup procedure or custodian. |
| 9 | Dark mode | **READY** | §3.1 tokens locked; `#08090C` locked v3.2. |
| 10 | 3 widget primitives (stat / table / list) | **READY** | §10.1 catalog + §10.7 envelope + §16.2 schema tests all present. |

---

## Phase 2 / 3 sequencing

Phase 2 depends cleanly on Phase 1: ambient HUD reuses streaming substrate (#4); 10 widget types reuse envelope (#10); light mode is palette swap on dark tokens (#9). **One hidden coupling:** Phase 2's "registry hot-reload via fsnotify" (§19) implies Phase 1 ships a registry — but §19 Phase 1 says "Hardcoded into panel; no adapter registry yet." That means Phase 2 ships the registry on day 1, not Phase 1 — fine, but the Phase 1 adapter abstraction must be designed to be replaced, not extended.

Phase 3 (voice/TTS/wake-word/Touch ID/Dashboard) is genuinely independent of Phase 1 internals. No coupling concerns.

---

## Cross-cutting opens (decisions still needed before Monday)

1. **Framework pick (SwiftUI+AppKit vs Wails vs webview_go).** §17.2 explicitly defers ("Framework pick is deferred"). Cannot write Swift on Monday if the answer might be Go+webview. Spec's own §17.2 note flags webview as likely-blocked by backdrop-filter — bias toward SwiftUI+AppKit but **make the call**.
2. **Anthropic model id.** Lock the exact string (e.g. `claude-sonnet-4-5-20250929`). Spec says "lock in `internal/reasoner/`" — that's a placeholder, not a lock.
3. **Anthropic SDK language.** Go (server-side daemon) vs Swift (Anthropic ships a Swift SDK as of 2025). Couples to #1.
4. **Anthropic API key custody.** macOS Keychain (`SecItemAdd` with `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`) vs env var vs settings.json. Spec is silent.
5. **Embedding model.** Voyage / OpenAI text-embedding-3 / on-device Core ML / Anthropic-when-shipped. Drives sqlite-vec column dimension — must lock before any indexing code.
6. **Sparkle EdDSA private key custody.** Who holds the key, where the offline backup lives, rotation procedure. Single point of failure for every future update.
7. **App architecture diagram.** None exists in spec. For 10wk solo, fine to sketch on a napkin — but the daemon ↔ HUD ↔ launchd ↔ Sparkle process topology should be drawn once before code.

Also-noted (non-blocking for Monday but file before Phase 1 ends):
- **GitHub Releases appcast URL** (`https://updates.leah.app/appcast.xml`): DNS owner + hosting (Cloudflare Pages? GH Pages? direct GH release asset?) — `updates.leah.app` not registered today.
- **Logging destination** (already specced — §17.9 OSLog + `~/Library/Logs/Leah/leah.log` rotated 10MB×5). READY.
- **Entitlements.plist** (already specced — §17.1 table of 7 entries). READY.

---

## Test plan ship-readiness (Phase 1 slice)

| Test | Automatable in CI | Reproducible perf budget | Measurable "done" |
|---|---|---|---|
| §16.1 visual contract | Yes (snapshot) | n/a | Yes |
| §16.2 schema validation | Yes | n/a | Yes |
| §16.5 streaming test | Yes (fake LLM driver spec'd) | n/a | Yes |
| §16.7 cross-doc parity | Yes (`make check-spec-parity`) | n/a | Yes |
| §16.9 perf (cold-launch p50 <300ms, hotkey p95 <100ms, mount <120ms warm, idle RAM <80MB, idle CPU <0.5%) | Yes via `xctrace`/`vmmap`/`powermetrics` | **Yes — all budgets numeric** | Yes |
| Memory pipeline tests | **NONE SPECIFIED** | n/a | **No** — deliverable #3 has no acceptance criteria |
| Knowledge store / sqlite-vec tests | **NONE SPECIFIED** | n/a | **No** — deliverable #2 has no acceptance criteria |
| Daemon ↔ Anthropic SDK integration | **NONE SPECIFIED** | n/a | **No** — deliverable #1 has no acceptance criteria beyond §19 ship-criterion prose |

Phase 1 "done" measurability: §19 ship-criterion ("operator types 'what's the status of MAY-19?' and gets the real answer in <3s") is a **demo gate**, not a CI gate. Need a scripted end-to-end test that hits Linear + GitHub + renders to widget — currently nowhere in §16.

---

## Final verdict

**NEEDS-7-DECISIONS-FIRST.**

The spec is unusually thorough on UI/UX surface contracts, NSPanel masks, streaming envelope, entitlements, and perf budgets — those are ship-ready. The blockers cluster at the boundary between "Leah-the-product" and "the LLM substrate that makes Leah work": model id, SDK pick, API key custody, embedding model, framework pick, Sparkle key custody, and a one-page architecture diagram. All 7 are decision-shaped (≤1 day of operator decisions), not draft-shaped (no new design work). After they're answered, the engineer can start Swift Monday — and three of the 10 Phase 1 deliverables (#1, #2, #3) need acceptance criteria written into §16 before they're truly done-able.

Estimated unblock cost: **1 decision-day + 1 spec-amendment-day** = ship-ready by Wednesday.
