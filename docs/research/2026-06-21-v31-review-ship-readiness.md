# v3.1 spec ship-readiness review

> Date: 2026-06-21
> Reviewer lens: senior eng manager, 5× Developer ID + Sparkle shipper
> Spec under review: `docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md` (2564 lines, v3.1)
> Question: is this executable on Monday, or does it need more decisions?

---

## 1. Ship verdict

**DOWNSCOPE-FIRST.**

v3.1 is an outstanding **design canon** — palette, motion, IA, brand voice, anti-patterns, ASCII wireframes — and a notably-rigorous adversarial-folded document. As **a Monday-morning engineering brief, it is not yet executable**.

The single blocking gap: **framework is still deferred** (SwiftUI vs Wails vs webview_go). Until that's picked, the rest of the spec is partially-bilingual — every implementation section names primitives twice ("NSPanel … Wails alternative …"), every motion section caveats ("CAEmitterLayer … or Wails equivalent"), and there is no app skeleton anyone can `swift build` against. §17.2 even self-flags the Wails path as likely-broken (no cross-window `backdrop-filter` sampling), which means the deferral is in practice a choice between SwiftUI/AppKit-native vs giving up real glass blur — the spec's #1 visual signature.

In addition: 7 of the 10 ship-blockers below have implementation-level decisions that are referenced but not made (knowledge store backend, telemetry posture, log destination, settings persistence format, LLM provider choice, Sparkle update URL/EdDSA key custody, marketing-hero screenshot source assets).

The honest read: this is a great **v3 design lock**. To ship v1 on a reasonable horizon, cut surfaces and pick the framework BEFORE Monday.

---

## 2. Open ship-blockers

### B1 — Framework pick (CRITICAL, blocks everything)
- **Decision needed:** SwiftUI+AppKit (native) vs Wails v3 (Go+webview) vs webview_go (raw).
- **Impact:** Every implementation directive in §17.2 is bifurcated. NSPanel masks, `CAEmitterLayer`, `NSVisualEffectView`, `CALayer` sprite-sheets, `MenuBarExtra` are SwiftUI-native; the Wails fallbacks named alongside (`WindowSetAlwaysOnTop`, `backdrop-filter`) are either lower-quality or, per §17.2 self-note, broken for cross-window blur. **You cannot start the focus-panel ticket without this.**
- **Recommendation:** SwiftUI + AppKit interop. The spec's signature motion (gold seam unfold, hex sigil rotate, ProMotion-budget compliance), real `backdrop-filter` glass, and template menubar tinting are all SwiftUI/AppKit-native. Wails buys ~30% of Go-reuse and loses real glass.

### B2 — LLM provider + streaming format
- **Decision needed:** Claude API vs OpenAI vs local (Ollama)? Streaming wire (SSE vs Anthropic event-stream)? Caching strategy (prompt caching enabled, cache-key shape)?
- **Impact:** §10.7 specifies the daemon→UI streaming protocol but the daemon→LLM streaming is undefined. Current code (`internal/reasoner/reasoner.go`) uses Anthropic — confirm that's the v1 choice and lock the model id, otherwise §5.3 thinking-ring durations are guesswork.
- **Recommendation:** Lock Claude `claude-sonnet-4-5` (or whatever current Sonnet) + prompt caching ON + Anthropic SDK streaming. Add to §17.

### B3 — TTS provider + privacy posture
- **Decision needed:** §2.7 names ElevenLabs cloud as default, Samantha as fallback. But the marketing thesis is "presence on the operator's machine" — sending mic+text→cloud TTS for every utterance contradicts that, and there's no privacy-disclosure copy in the wizard.
- **Impact:** Privacy story, MAU bill (ElevenLabs ~$5–22/mo per heavy user), latency floor (cloud TTS = 200–600ms first-audio), offline-day UX.
- **Recommendation:** Decide one of (a) ElevenLabs cloud + explicit wizard disclosure + cost-per-user model, (b) on-device only (Samantha + premium voices), (c) ElevenLabs Flash + 12 cached canned-line set. v1 cut: ship Samantha-only; "Leah voice canon" is v2.

### B4 — Wake-word ML model (named but not bundled)
- **Decision needed:** Spec L507 says `wake-leah.mlmodel` (Core ML) but doesn't name the training pipeline or source — Picovoice (per-seat license), Porcupine, OpenWakeWord, custom training? Auto-memory cite: Picovoice ~3-5% CPU sustained, OpenWakeWord ~8%; perf budget breaks if wrong choice.
- **Impact:** No engineer can write `internal/wake/` without this. Licensing alone could be a 2-week procurement loop.
- **Recommendation:** v1 = wake-word OFF feature-flagged. Ship without `internal/wake/`. Re-decide post-launch.

### B5 — Knowledge store backend (zero hits in spec)
- **Decision needed:** The widget catalog references "knowledge timeline" (§4.7) and a Knowledge widget type, but spec has **zero** mention of `sqlite-vec`, embeddings, vector store, FTS5, or any persistence schema for it. Current code uses SQLite (per `internal/knowledge/storage.go`) — confirm + spec the schema.
- **Impact:** Knowledge widget can't be built; "dashboard memory" view can't be built.
- **Recommendation:** Reuse existing `internal/knowledge/` SQLite + FTS5. Document the schema in §10.6 adapter table.

### B6 — Sparkle infrastructure (URL + EdDSA key + channel)
- **Decision needed:** §17.1 names `https://updates.leah.app/appcast.xml`. **Who owns that domain, who hosts the appcast, who custodies the EdDSA private key, which channel(s) — stable only? stable+beta?** This is normally a 1-week ops loop (DNS, S3+CloudFront or equivalent, KMS for key, build-time signing in CI).
- **Impact:** Cannot ship auto-update without it. Cannot release the binary publicly without `updates.leah.app` existing.
- **Recommendation:** Stable-only at v1. Use GitHub Releases as the appcast host (drop custom domain) — `https://github.com/<org>/leah/releases/latest/download/appcast.xml`. Generate EdDSA key now; commit public key into bundle.

### B7 — Telemetry / crash-reporting posture (zero direct hits)
- **Decision needed:** Any telemetry? Sentry? Self-hosted crash logs? Nothing? Without it, you have no signal on real-world crashes post-ship.
- **Impact:** Privacy story, EULA, wizard copy, post-launch ops.
- **Recommendation:** v1 ship zero telemetry + ONE in-app "Send diagnostic bundle" button (operator-initiated, OSLog dump + crash reports tarball, opens Mail.app). No background phone-home.

### B8 — Logging destination
- **Decision needed:** Spec doesn't say where Leah logs go. OSLog (unified logging, sysdiagnose-discoverable, recommended) vs stdout vs file in `~/Library/Logs/Leah/`?
- **Impact:** Debuggability, privacy (OSLog redaction policy), disk footprint.
- **Recommendation:** OSLog (`os.Logger(subsystem: "app.leah", category: …)`) with `.privacy(.private)` on user content. Standard macOS answer.

### B9 — Settings persistence format
- **Decision needed:** Spec names `pinned-widgets.json` (Application Support), `widget-registry.json` (Application Support), `leah.sock` (Caches). But where do **user preferences** live? UserDefaults (`~/Library/Preferences/app.leah.plist` — standard Mac answer, CloudKit-syncable later) vs custom JSON?
- **Impact:** Settings pane (§9) cannot be wired without this.
- **Recommendation:** UserDefaults for all scalar prefs; JSON files only for structured state (pins, registry).

### B10 — Marketing hero composition source assets
- **Decision needed:** §13.14 names the screenshot that sells the product. Who produces it? There is no Figma file, no asset bundle, no SVG of the mark linked, no `~/Library/Application Support/Leah/assets/` listing.
- **Impact:** Cannot ship landing page on launch day. Mark SVG/PDF must be produced before anyone builds the wizard (96px hero size).
- **Recommendation:** Spawn a 2-day "produce mark + 4 hero screenshots" task BEFORE Monday. Designer or AI-generated, sign-off by owner.

---

## 3. Engineer-weeks estimate

Assuming **SwiftUI+AppKit native** (B1 resolved), 1 senior macOS engineer, no surprises:

| Surface | Weeks | Notes |
|---|---|---|
| Skeleton + entitlements + Sparkle wiring + DMG | 1.5 | Hardened Runtime, codesign, notarytool, EdDSA appcast, DMG layout |
| Ambient HUD (panel + NSPanel masks + drag/snap + Dynamic Type reflow) | 1.5 | per-monitor sticky, Spaces behavior, occlusion-based animation halt |
| Menubar item (template image + state shape) | 0.5 | trivial in `MenuBarExtra` |
| Focus panel (summon flourish, NSPanel key-handling, prior-app capture-restore, streaming render, tile interleave) | 2.5 | gold-seam transform animation is the hard part; streaming text + tile interleave is non-trivial |
| Widget protocol + 13 adapters (JSON-Schema validator, IPC framing, adapter registry, cached-last-good) | 3.5 | ~0.25/widget for 13 + 0.25 for protocol |
| Wizard (5 steps, TTS warm-up, accessibility-grant flow, mic-prompt, integration card) | 1.5 | Accessibility-grant dance + integration auth is fiddly |
| Settings pane (8 sections, IA, live preview, Things-style sidebar, search) | 2.0 | Live-preview on Appearance, search-in-settings |
| Dashboard | 1.5 | Memory + agenda + briefs + news + knowledge views; can reuse widget adapters |
| Notification widget stack (toast queue, 2-cap, coalesce timer) | 0.5 | |
| Voice subsystem (TTS pre-warm + pipeline; STT/Apple Speech; barge-in plumbing) — WITHOUT wake-word | 1.5 | Wake-word deferred per B4 recommendation |
| Light-mode parity (palette swap, KVO observer, cross-fade, mark emboss two-direction) | 1.0 | Every surface tested at both palettes |
| Accessibility (VoiceOver labels, Dynamic Type, reduce-motion, reduce-transparency, focus rings) | 1.5 | Genuinely-comprehensive a11y is always ~10% of total |
| Test infra (visual contract — need to PICK a tool, schema tests, lifecycle, streaming, security, a11y, perf, power) | 2.0 | No visual-diff tool named in §16; XCUITest+snapshot OR swift-snapshot-testing pick needed |
| Telemetry / logging / settings persistence (per B7/B8/B9) | 0.5 | |
| Buffer (notarization rejections, codesign edge cases, App-Store-style review-hell) | 1.5 | First Developer-ID notarization on a new bundle id always has 1–2 reject cycles |
| **TOTAL** | **22.5 eng-weeks** | ≈ **5.5 calendar months solo** |

**Parallelism:** with 2 senior macOS engineers + 1 designer, achievable in **~14 calendar weeks (3.5 months)** — widget adapters parallelize cleanly, surfaces are mostly independent.

**This is v3, not v1.** No v1 ships in <8 weeks at this scope.

---

## 4. Recommended v1 cut-line

Ship surfaces in this order; treat the rest as v2.

### v1 (target: 6 weeks, 1 eng)
- **Skeleton + Developer ID + notarization + Sparkle (GitHub Releases as appcast host)** — 1.5 wks
- **Menubar item only** (no ambient HUD) — 0.5 wk
- **Focus panel** with `⌥Space` summon + streaming text response (no widgets, no tiles, no gold-seam flourish — just cross-fade in) — 1.5 wks
- **Wizard** (3 steps: welcome, hotkey+accessibility, mic) — 1.0 wk
- **Settings** (3 sections: General, Permissions, About) — 0.5 wk
- **3 widget types** out of 13: Calendar, Weather, ASK-response (the prose-only one). Hardcoded into the panel, no adapter registry yet. — 1.0 wk

**v1 is: a tiny native panel that summons on ⌥Space, talks to your Go daemon, streams an answer, ships signed + notarized + Sparkle-updatable.** Ship this and you have a real product on macOS.

### v1.1 (next 4 weeks)
- Ambient HUD (the 280×84 surface)
- Gold-seam flourish + sigil animations
- Light-mode parity
- Notification widget stack
- 5 more widget types (Stock, News, Reminder, Mail, Task)

### v2 (next 8 weeks)
- Dashboard
- Pin-to-ambient
- Widget gallery overlay
- Remaining widget adapters
- Voice TTS canon (after B3 decided)
- Wake-word (after B4 decided)
- Multi-monitor / fullscreen-secondary-display
- Settings live-preview pane

### Phase-to-v3
- ElevenLabs cloud TTS
- Knowledge widget + dashboard memory view
- Plugin/extensibility

---

## 5. Risks at deploy

1. **Notarization first-submission rejection.** New `app.leah.*` bundle id, no prior notarization history → 1–2 reject cycles likely on hardened-runtime entitlements (esp. AppleEvents for Calendar/Mail/Reminders). **Mitigation:** notarize a stub the week before v1 ready-to-ship to flush issues.
2. **Sparkle EdDSA key custody.** Lose this key, you can never push another update to existing installs. **Mitigation:** generate now, store in 1Password + offline backup; document in runbook.
3. **`updates.leah.app` is vaporware.** Spec names it; DNS + hosting doesn't exist. **Mitigation:** use GitHub Releases as appcast URL for v1.
4. **AppleEvents permissions UX.** Calendar/Mail/Reminders access prompts are infamously bad (system modal that the user can dismiss without granting + then can't easily re-trigger). Spec lazy-prompts; lazy-prompts trigger Apple's "weird permission popup mid-task" experience. **Mitigation:** Wizard step 4 actually requests the chosen integration's TCC permission inline so the prompt is in-context.
5. **Mic permission denial → app appears broken.** Wizard step 3 needs a no-mic-fallback path (currently spec'd as "lazy retry" but no UI shown). **Mitigation:** "Voice unavailable — use the panel" CTA shown if denied.
6. **Webview backdrop-filter fallback** (if anyone pushes Wails despite §17.2 self-warning): real glass blur impossible, brand promise breaks.
7. **Inter font bundling vs Söhne aspiration drift.** Inter ships OFL — fine. But §3.3 leaves Söhne as "optional post-launch upgrade" — once shipped on Inter, switching font is a visual-regression review for every surface. **Mitigation:** commit to Inter, drop Söhne from the spec entirely.
8. **First-launch Accessibility-permission dance** — global hotkey silently fails in third-party apps without Accessibility grant. Spec acknowledges this; needs an end-to-end test (operator clicks hotkey in Safari, sees no panel, finds Settings deep-link, grants permission, returns) before ship.
9. **MacBook Intel users** — spec assumes M-series perf budgets; CAEmitterLayer + ProMotion-budget reasoning doesn't hold on Intel. Either drop Intel from supported matrix or run the spec's perf-budget tests on a 2018 Intel MacBook Pro before claiming v1 supports it.
10. **No visual-diff tool picked.** §16.1 "Visual contract tests" exists as a heading but the tool (`swift-snapshot-testing`? Pointfree? Percy? XCUITest+manual?) is unnamed. Defaults to "ship it and pray."

---

## 6. Top 10 ambiguities

1. **Framework deferred** (§17.2). Engineer must invent: which framework. **The biggest one.**
2. **"Tasteful," "polished," "regal"** — spec uses the words "consistent" 5×, "polished" 2×, "regal" 8× without quantifying. Engineer asks: "is this regal enough?" Answer is "ask the designer," and there's no designer named.
3. **Visual-diff tool unnamed** (§16.1). Engineer must invent: `swift-snapshot-testing` vs `iOSSnapshotTestCase` vs custom XCUITest+ImageMagick diff.
4. **Knowledge widget data shape** (§10.1 reference, §10.6 silent). Engineer must invent: schema, query API, indexing strategy.
5. **Dashboard memory view rendering** (§4.7 prose, no widget protocol cited). Engineer must invent: are these widgets-on-canvas (reuse §10) or a separate view? "Where the operator goes to look, not to ask" tells you intent, not architecture.
6. **Toast queue source-of-truth** (§4.4). Spec says daemon-pushed (MAY-19 B1/B5 substrate). Engineer must invent: what's the message shape? Where's the protocol? "Notification widget catalog" doesn't exist.
7. **Settings live-preview** (§4.6) — preview pane on Appearance section. Engineer must invent: same-process render-at-scale (tricky — `NSVisualEffectView` doesn't render well inside another `NSVisualEffectView`) vs separate-process child.
8. **CLI ↔ GUI parity** (§6.8) — claims every GUI affordance has a CLI equivalent. Engineer must invent: which `leah <verb>` invocations correspond to which GUI surfaces; this is multiple sprints of new CLI surface area not specced.
9. **TTS canned-line cache vs real-time** (§2.7). Spec implies real-time cloud TTS; that's ~300ms latency floor + cost-per-utterance + bandwidth. Engineer must invent: cache strategy, pre-warmed common-utterances list, offline-when-cached behavior.
10. **Mark emboss spec drift** (§3.6 vs §18). Spec L240 says §3.6 vs §18 had a two-direction-emboss conflict that §3.6 "canonicalizes" — but §18 still contains the old variant per change log. Engineer must invent which is true OR check §18 directly. (Pure spec-hygiene cleanup, but representative of how cross-section reconciliations leak.)

---

## Notes on the spec's strengths (so the verdict reads fairly)

- The adversarial-review fold pattern (v2 → v3 → v3.1) is genuinely best-in-class spec hygiene. Most companies don't get this far.
- §1 operator-decision override table is the cleanest disagreement-with-defaults section I've seen in a design doc.
- §15 anti-patterns is rare and valuable — most specs only say what to build, not what to refuse.
- WCAG contrast computations baked into every color token is excellent.
- Distribution + entitlements lock in §17.1 is exactly the right level of detail.

The verdict isn't "this is a bad spec." The verdict is **"this is a v3 design lock masquerading as a v1 build brief."** Cut surfaces, pick the framework, ship the menubar+panel slice first.
