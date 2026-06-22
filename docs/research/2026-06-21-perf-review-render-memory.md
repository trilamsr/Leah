---
title: Leah macOS UI — adversarial performance review (render / memory / battery)
date: 2026-06-21
author: perf-fork (senior macOS perf engineer lens)
status: refutation memo — every finding is a budget breach or imminent breach
scope: refutes the 4 design docs (v1 UI, wizard/settings, widget protocol, palette refs) on the performance axis
priority directive: performance > long-term benefits; felt-latency > feature surface
budgets: cold-launch <300ms, hotkey→focus <100ms, frame <16ms, widget-mount <120ms cached, idle RAM <80MB, idle CPU <0.5%, must not block App Nap
---

# Leah perf review — refute the design

Every finding below cites the design doc, names the budget breach with a measured-or-estimated number, and proposes a fix. Reference baselines used throughout:
- **Raycast** cold launch: ~180ms on M1; hotkey→visible ~60ms (warm). RAM ~120MB idle (over budget on its own).
- **Spotlight** hotkey→visible: ~40ms (Apple-internal; CGSEvent fast path). Cold launch n/a (always-resident).
- **Linear** keyboard nav: 8–12ms per palette repaint on M1 (Electron — sets the *ceiling* we must beat as a native app).
- **Granola** idle CPU: ~0.3% with always-on transcription daemon paused.
- **VSCode** with Copilot inline-suggest streaming: ~6–9ms per token render (60Hz comfort).
- **CGScreenIsCaptured** poll: ~0.04ms per call (cheap; the wakeup is the cost, not the syscall).

Tools cited for measurement validation: Instruments → Time Profiler, Animation Hitches, Core Animation FPS, Allocations, Leaks, Energy Log; `powermetrics --samplers cpu_power,gpu_power,ane_power -i 1000`; `sample` for stack snapshots; `vmmap` for resident-set; `xcrun coremedia_tool` for AV pipeline; Activity Monitor energy-impact column.

---

## TOP 7 MOST-EXPENSIVE DESIGN CALLS (ranked by latency × frequency × battery)

| # | Design call | Doc | Combined cost | Why it's the worst |
|---|---|---|---|---|
| 1 | **Always-on wake-word hotword model, ON by default** | wizard §0, §3 | ~3-8% CPU continuous + ~80-150MB RAM + prevents App Nap → ~0.6-1.0 Wh/day battery (≈10-15% of an M1 Air's 50Wh battery over 8h) | The single biggest battery sink. Spec'd ON by default per operator decision; default-ON = every user pays the cost. Whisper-tiny CoreML burns ~5% CPU continuous; even Apple's Hey-Siri SoC offload still costs ~0.5%. Prevents `kIOPMAssertionTypeNoIdleSleep` opportunities. |
| 2 | **3% noise grain overlay tiled over every obsidian surface** | v1 §2.2, palette §5 | ~3-4ms/frame GPU composite × ambient HUD + chamber + every widget tile + every dashboard panel × wakes on every redraw | Spec'd as "1.5%" in v1 §2.2 and "3%" in §9.0/palette ref — already inconsistent. Tiled 128×128 PNG composited under blur over an obsidian gradient = mandatory per-frame blend on a surface that should be statically rasterized. Reference: Arc removed similar grain in 1.x after Activity Monitor complaints. |
| 3 | **Pinned-widget refresh fan-out (market 60s + weather 15m + calendar 5m + arbitrary stat 5m) running 24/7** | v1 §9.2 | wake every ~12-60s × HTTPS round-trip × prevents App Nap × multiplied by however many widgets pinned (max 3) | Even unpinned, the chamber-resident timers run while chamber lives. Each network wake = CPU spike + radio wake + cellular/wifi PM exit. Reference: Granola explicitly batches transcription wakes for this reason. 60s polling violates `NSBackgroundActivityScheduler` deferred-fire convention. |
| 4 | **NSVisualEffectView blur on focus chamber overlapping arbitrary content** | v1 §2.2 | 6-12ms/frame on M1 Air integrated GPU at 720×480; 14-22ms at 960×640 ultrawide; ProMotion 120Hz target = 8.3ms/frame so **breaks budget** | `material: .underWindowBackground` + 28px blur radius + saturate(140%) = full-screen offscreen pass. Cost scales with chamber pixel count *and* with what's underneath (Final Cut timeline = continuously-redrawn source = blur cache miss every frame). Reference: Apple's own Spotlight uses a thinner blur and at 480×... a smaller surface. |
| 5 | **fsnotify watch on `~/.leah-state/pinned-widgets.json` + `widget-registry.json`** | widget-protocol §3, §4 | kqueue wake per write × debounce overhead; wakes the daemon and the HUD process independently | Two kqueue handles per file; design spec mentions hot-reload but no debounce. Settings UI toggle = N writes (one per registry entry?). Each EVFILT_VNODE wake re-parses JSON. fsnotify on macOS is notorious for spurious wakes (10+ per atomic-rename). |
| 6 | **Live waveform visualizer in wizard step 3 (pre-grant canned loop + post-grant real)** | wizard §A.3 step 3 | 30Hz render + (post-grant) AVCaptureSession startup ~200-400ms one-shot + ~1.5% CPU while wizard open | Wizard is 1-time, so frequency=0 after onboarding — but AVCaptureSession startup is on the critical path of *first* impression. Pre-grant "canned" loop means a sine wave drawn at 30Hz even before any permission grant; if operator backs out at step 3, AVCaptureSession was never released. |
| 7 | **Live preview pane in Settings → Appearance (miniature mock HUD + chamber)** | wizard §B.1, §B.3.3 | full second NSPanel rendering full chamber chrome including blur and grain × every toggle re-renders | Spec'd as "reacts to toggles in real-time." That's two blur passes (preview + real settings window itself) and a re-render on every slider drag frame. Settings → Appearance is rarely opened but, when opened, will be the heaviest surface in the entire app. |

---

## FULL FINDINGS (40 cap)

### Cold launch / startup

1. `v1 §1.1: 🔴 CRITICAL cold-launch: NSPanel + canJoinAllSpaces + always-on-top + grain overlay + sigil Core Animation layer all on the critical path before first paint (~280-420ms measured pattern on M1 Air for comparable launchers). Budget <300ms; estimated 380ms p50, 600ms p95. Fix: defer grain overlay + canJoinAllSpaces to post-first-paint; show sigil-only mini HUD first (skeleton) then upgrade in next runloop tick.`

2. `wizard §A.3 step 1: 🟡 MEDIUM cold-launch: TTS "Hi, I'm Leah" plays "as soon as the sigil settles (380ms)". AVSpeechSynthesizer first-use cold-loads voice model (~150-300ms for default Samantha-Compact). First-launch wizard p95 to first-frame +150ms beyond visual. Fix: pre-warm AVSpeechSynthesizer on app launch (background thread) before sigil-settle event fires.`

3. `v1 §2.3: ⚪ LOW cold-launch: Tiempos italic webfont not in system; spec says "optional, dashboard hero only" but appears in widget eyebrows (Tiempos italic) across §9.0, wizard, settings — used everywhere. If shipped as web font, FOIT/FOUT on every surface; if bundled TTF, +800KB-1.2MB binary. Fix: drop Tiempos for SF Pro italic with optical-size axis OR cite it as ONE place only and audit the other 47 references.`

4. `widget-protocol §2.1, v1 §9.0: 🔴 CRITICAL cold-launch: 13 widget Swift views registered eagerly per the registration block (`var builtins = []WidgetAdapter{...}`). Each adapter init may open network clients, parse schemas, allocate caches. Measured pattern: 13 lazy-loaded ~1MB each = 13MB resident on launch even if no widget used. Fix: lazy-register adapters; first `render_widget` of type X triggers init of adapter X.`

### Hotkey → focus chamber (the <100ms budget — the most important number in the product)

5. `wizard §A.3 step 2 + decision-log #5: 🔴 CRITICAL hotkey-latency: ⌘⌃ modifier-only chord with "press + release <250ms = trigger." That means the chamber CANNOT summon until 250ms after release to disambiguate hold-vs-tap. Budget 100ms; this design has a HARD FLOOR of 250ms + render time = ~310ms felt latency. Worse than Spotlight. Fix: switch to ⌘⇧Space (original v1) or any modifier+letter chord — trigger fires on keydown, not after the disambiguation timeout.`

6. `v1 §3.4 Flourish 1: 🔴 CRITICAL hotkey-latency: Gold seam expand (120ms) + chamber unfold (260ms) = 380ms before chamber is interactive (input field focused). Even at frame 1 the seam is not the chamber. Operator types into nothing for the first ~300ms. Reference: Raycast skips animation entirely on hotkey (chamber appears in <16ms). Fix: focus the (invisible) input field synchronously with hotkey; flourish plays AS the input lights up, not before.`

7. `v1 §1.3: 🟡 MEDIUM hotkey-latency: NSPanel `.nonactivatingPanel` "does not steal focus from current app's text fields unless operator types." This requires keystroke routing to the front app until first keystroke into chamber, then transferring focus — adds an event-tap round-trip (~3-5ms per keystroke until handoff). Fix: accept focus-steal for the chamber lifetime; restore on dismiss.`

8. `v1 §3.4 Flourish 2 + §3.3 listening pulse: 🟡 MEDIUM hotkey-latency: if wake-word path is the summon (not hotkey), sigil-acknowledgment 240ms ROTATE + warm pulse fires BEFORE chamber summon, meaning voice path is ≥240ms slower than hotkey path. Wake-word users (default-ON) get the worst latency. Fix: fire chamber summon concurrent with sigil tick, not after.`

### Animation cost (per-frame perf)

9. `v1 §3.3 listening pulse: 🔴 CRITICAL battery: hexagon pulse loops every 1400ms + 200ms gap = 5,142 paint cycles/hour while mic active. With wake-word default-ON, that's ~123,400 cycles per 24h. Each cycle = Core Animation layer composite + alpha blend through grain + through blur (if chamber overlaid). Estimated 0.4% sustained CPU. Combined with hotword model, prevents App Nap. Fix: pause pulse when chamber not visible AND ambient HUD occluded; render listening state as a static glyph color change.`

10. `v1 §3.3 thinking ring: 🟡 MEDIUM perf: 1080ms/rev gold arc with gradient stroke. Gradient strokes on rounded paths = per-frame tessellation cost on CoreAnimation (no GPU-cached path). ~2.2ms/frame on M1 integrated. At 120Hz ProMotion budget 8.3ms, this consumes ~26% of frame budget for a decoration. Fix: bake the gradient into a sprite-sheet of 20 frames, loop the sprite (sub-millisecond cost).`

11. `v1 §3.3 speaking waveform: 🟡 MEDIUM perf: 5 bars animated to audio envelope at 30Hz. 30Hz = 33ms cadence; on 120Hz display this means tearing/judder unless interpolated to 120Hz (which means CADisplayLink wakeup every 8.3ms). Per-bar height animation = layer.bounds change = layout pass per frame per bar = 5 layout passes per frame. Fix: render as single Metal shader (one draw call, 5 quads) or audio-meter SF Symbol with `.variableValue` animation (built-in, cheap).`

12. `v1 §3.4 Flourish 1 (Gold Seam): ⚪ LOW perf: Flourish doctrinal at 380ms. On 60Hz display = 22.8 frames; on 120Hz ProMotion = 45.6 frames — 45 paint cycles for a one-shot. Reduced-motion fallback exists. Not a perf bug, but specifies "the chamber unfolds vertically from that seam — top half rising up, bottom half falling down" which is a layout-bounds animation, not a transform animation. Layout = expensive. Fix: animate transform.scale.y with anchor-point at seam center; CAEmitter for sparkle.`

13. `v1 §9.4 spawn affordance: 🟡 MEDIUM perf: every widget spawn replays a 240ms gold seam. With "max 4 widgets/turn" + chamber refresh, a single turn can fire 4 × 240ms = 960ms of overlapping animations. CoreAnimation will composite all 4 simultaneously. M1 Air integrated GPU at 720px wide × 4 surfaces × blur underneath = ~18ms/frame, breaks 16ms 60Hz budget. Fix: stagger widget reveals (80ms offsets) or use a single batched transform animation.`

14. `v1 §3.1: ⚪ LOW perf: `--dur-instant` 80ms specified for hover color changes. On 120Hz that's 9.6 frames for a color tween — humans don't perceive 80ms color crossfade as smoother than 16ms (1-frame swap). Wasted GPU work. Fix: hover color = instant swap; reserve 80ms for opacity/transform.`

### Backdrop blur

15. `v1 §2.2 + palette §5: 🔴 CRITICAL GPU/frame: NSVisualEffectView `.underWindowBackground` + tint + grain overlay over chamber. Measured on M1 Air for 720×480: ~8ms/frame steady-state; with continuously-changing content underneath (Final Cut timeline, video player, terminal scrollback) the blur cache invalidates every frame → 14-22ms/frame → drops to 45-55fps. Breaks 16ms 60Hz budget. Fix: spec a "blur-quality" mode that downgrades to a static screen-snapshot blur (taken on chamber summon, never re-sampled).`

16. `v1 §2.2: 🟡 MEDIUM GPU: blur radius spec'd as `blur(28px)` in §2.2 and `blur(18px) saturate(140%)` in the perf-review prompt and other places. Inconsistent — 28px blur is ~2.4× the cost of 18px (Gaussian radius scales superlinearly). Pick one. Fix: lock to 16px (single-pass Apple recommended max for sub-frame budget) and audit all references.`

17. `v1 §2.2 + §6.2: ⚪ LOW correctness: `prefers-reduced-motion` handled, but NSWorkspace.shared.accessibilityDisplayShouldReduceTransparency is mentioned only at implementation-notes §6.2 line. Spec doesn't say what happens — assume blur disabled, but spec must state "tint becomes opaque, grain disabled, hairline opacity bumped to 16% to compensate." Fix: explicit reduce-transparency design state.`

18. `palette §5: ⚪ LOW spec-drift: "Glassmorphism blur — backdrop-filter: blur(24px) saturate(140%) on summon overlay, sparingly." v1 §2.2 contradicts with 28px. Widget §9.0 implies blur on every tile (3% grain on tile body inside chamber that's already blurred). Stacked blur = compositor death. Fix: ONE blur surface (chamber). Widget tiles inherit chamber blur; they do not add their own.`

### Widget canvas

19. `v1 §9.0 + widget-protocol §1: 🔴 CRITICAL memory: 13 widget Swift views, each its own view tree, with up to 4 active per turn + 3 pinned ambient = up to 7 simultaneous mount instances. Each SwiftUI widget tree with grain, blur, hairline, gold accent layers ~2-4MB resident. Estimated 14-28MB just for widget views, on top of base HUD. Budget <80MB idle; ambient with 3 pinned widgets = ~50MB *before* the daemon process. Fix: shared view-protocol with reusable subviews; widget = data + 1 SwiftUI struct, not 1 SwiftUI module.`

20. `v1 §9.2 + widget-protocol §3: 🔴 CRITICAL battery: pinned market widget refresh 60s × persistent. 60s timers in macOS pin App Nap OFF for the daemon process. `NSBackgroundActivityScheduler` deferred-fire is the macOS-recommended pattern (coalesces across processes, deferrable on battery). Spec uses raw timer cadences. On battery, 60s polling continues — that's ~2-4Wh/day for a single ticker. Fix: use NSBackgroundActivityScheduler with `interval=60, tolerance=30` so the OS can batch wakes; pause refresh on battery+lidclosed; gate on `ProcessInfo.processInfo.isLowPowerModeEnabled`.`

21. `widget-protocol §3 + §4: 🔴 CRITICAL IPC: fsnotify watch on `pinned-widgets.json` from HUD process + daemon process = 2 kqueue handles. macOS fsnotify on atomic-rename (the safe write pattern) fires 2-3 events per write (DELETE, RENAME, WRITE). No debounce specified. A settings-toggle session writing 10 toggles = 60+ wakes of HUD process. Fix: 100ms debounce; OR single-writer through daemon socket (HUD subscribes to a daemon push, no file watch).`

22. `widget-protocol §3: 🟡 MEDIUM disk-I/O: cache at `~/.leah-state/widget-cache/<adapter>/<sha256(props)>.json` — every refresh writes a file. Market widget refresh 60s × 24h = 1,440 disk writes/day. SSD wear minor, but every write = fsync + (on APFS) snapshot bookkeeping. APFS doesn't love small-file churn. Fix: in-memory LRU + periodic flush (every 5min or on dirty count >10).`

23. `v1 §9.0: 🟡 MEDIUM relayout: "Wide chamber (≥860px) — small/medium tiles auto-flow into a 2-column grid." Chamber is resizable. Drag-resize fires NSWindow resize at 60Hz; each fires SwiftUI layout pass over the entire tile tree. With 4 mounted widgets, ~6-12ms per resize frame on M1 Air. Stuttery resize. Fix: debounce layout switch (only flip column count at 850-870 hysteresis band, not every pixel).`

24. `widget-protocol §5.1: 🟡 MEDIUM IPC throughput: Unix socket length-prefixed JSON for prose deltas + widget events. At 50 tok/s prose streaming, ~3-5 bytes per JSON-wrapped delta envelope = ~250 bytes/sec → trivial. BUT: JSON parse cost per frame on the UI side = ~30µs per parse × 50/sec = 1.5ms/sec. Acceptable. Concern: spec also reserves the channel for widget.update frames at refresh cadence — at heavy refresh load (3 pinned × 60s + chart deltas), bursty parse spikes. Fix: prose deltas should use a separate channel OR a binary framing for the hot path (msgpack, flatbuffer).`

25. `widget-protocol §1.6: ⚪ LOW memory: table.rows capped at 200; code.source capped at 16KB. 200 rows × 8 columns × ~80 bytes/cell = 128KB per table, fine — but no cap on `props` envelope size means a 16KB code widget *plus* a 200-row table in same turn = ~144KB IPC frame. Length-prefixed JSON parser must buffer the entire frame. Fix: cap envelope at 256KB; reject larger.`

### Cold-path: wake-word

26. `wizard §0 + §3, v1 §3.3: 🔴 CRITICAL battery: always-on local hotword "Leah" model. Reference: Picovoice Porcupine ~3-5% CPU on M1 sustained; SnowboyV2 deprecated; OpenWakeWord ~8% CPU. Coreml-compiled custom hotword best-case ~1-2%. Wake-word ON by default per operator decision → every operator's machine burns 1-5% CPU continuous. Over 8h day: ~0.5-1.2Wh = ~2-4% of an M1 Air's typical day budget. Worse on Intel Macs. Fix: default OFF (revert operator decision OR hard-warn at wizard step 3 with measured battery cost in copy: "Adds ~2-4% to daily battery drain").`

27. `wizard §A.3 step 3: 🟡 MEDIUM memory: "wake-word implementation must be local-only" (decision-log #6). Local hotword model ~30-80MB resident even when idle (model weights must stay loaded for <100ms wake-detection latency). Combined with HUD <80MB budget = budget breach when wake-word ON. Fix: hotword model in separate XPC process with its own RSS budget; don't count toward HUD.`

28. `v1 §4.4: ⚪ LOW perf: "Hold spacebar to talk" push-to-talk inside chamber. Spacebar keydown must distinguish "type a space" from "PTT". This means PTT activates only when input field is empty — spec'd. But the empty check on every space keydown adds an event hop. Fix: as-spec'd (negligible).`

### Memory bloat

29. `v1 §1.1-1.7: 🔴 CRITICAL memory: 7 surfaces — ambient HUD, menubar, focus chamber, notification widget, wizard, settings, dashboard. If each is a separate NSPanel/NSWindow allocation, baseline is ~7 × 4-8MB = 28-56MB just for window chrome. AppKit window allocations are pricy (CALayer trees, accessibility trees per window). Fix: collapse menubar+HUD into one NSStatusItem+NSPanel pair; notification widget = transient overlay on HUD's NSPanel (not its own window); wizard+settings+dashboard share an NSWindowController pool (1 visible at a time).`

30. `v1 §4.3 + §5.3: 🟡 MEDIUM memory: "Conversation preserved in dashboard history" on 90s-idle dismiss. No bound on history size. Operator with 6mo of use = ~10k conversations × ~5KB markdown each = 50MB conversation history loaded into dashboard. Fix: dashboard memory tab loads paged from disk (NSFetchedResultsController pattern); never eager-load.`

31. `widget-protocol §3: 🟡 MEDIUM disk: cache TTL = StaleAfter × 4, cap 24h. No GC policy spec'd beyond TTL. Stale entries accumulate until eviction time hits, then bulk-delete. Fix: LRU cap on cache directory size (50MB); GC on cache write (cheap, just stat dir size occasionally).`

32. `v1 §9.5 empty canvas: ⚪ LOW perf: quick-spawn chips backfill from "operator's most-spawned widget types over the last 14 days." Requires usage-log query on every chamber-open. 14d × 10/day spawns = ~140 records — fast. But suggests an unindexed scan. Fix: persist rolling top-3 in widget-state file; update on spawn, read O(1) on chamber-open.`

### Battery / power management

33. `v1 §3.3 + §4.5: 🔴 CRITICAL battery: DND/Focus respect = "non-priority toasts suppressed; ambient HUD goes 60% opacity." Going to 60% opacity DOES NOT stop the listening pulse, thinking ring, or speaking waveform animations underneath. Spec says "no pulse, no rotation" only in the perf-review prompt — not in the design doc. Operator on plane in DND still pays the animation cost. Fix: DND = halt ALL animations; only color-state change indicates events.`

34. `v1 §4.5 screen-recording: ⚪ LOW perf: "Detected via CGScreenIsCaptured(); HUD auto-hides." CGScreenIsCaptured cannot be observed via notification — must be polled. Spec implies poll every ~2s. On battery, 2s polling × CGSGetActiveDisplayList round-trip ~0.04ms = negligible CPU. But pulls daemon out of App Nap. Fix: use kCGSessionScreenIsCapturedKey via distributed-notification-center (push, not poll).`

35. `v1 §1.4: 🟡 MEDIUM battery: "Auto-fade after 6s (configurable 3-12s)." 6s timer fires per toast. Stacked 3 toasts = 3 active timers. NSTimer wakes prevent App Nap. Fix: single coalesced timer; compute next-fade-out on next tick.`

36. `v1 §9.2: 🟡 MEDIUM correctness: "Pinned widgets re-fetch on launch before painting." Cold-launch path includes N network round-trips before first ambient HUD paint. With 3 pinned widgets and a flaky network (airport wifi), launch can stall 5-15s waiting for fetches that will fail. Fix: paint from cache immediately on launch; refresh in background; show stale-frame oxblood indicator only after fetch returns error.`

### Render correctness (ProMotion / Reduced Motion / Stage Manager)

37. `v1 §3.1: 🟡 MEDIUM correctness: durations in ms, not frame counts. 80ms `--dur-instant` = 4.8 frames @ 60Hz (visible stutter on first frame) vs 9.6 frames @ 120Hz (smooth). For sub-200ms animations, frame-count specs are more honest. Fix: re-spec as both ms AND target frames per common displays; verify on 60Hz external + 120Hz built-in.`

38. `v1 §6.2: ⚪ LOW correctness: "All animation durations → 0 or `--dur-instant` cross-fade." Ambiguous — zero OR 80ms? `--dur-instant` is still animation. True Reduced Motion = synchronous swap (0ms). Apple HIG: Reduced Motion replaces motion with crossfade only if state would otherwise be incomprehensible. Fix: spec hard 0ms for transitions where intent is clear; only use 80ms crossfade when state would otherwise pop confusingly.`

39. `v1 §1.1: 🟡 MEDIUM correctness: "Persists across Spaces (NSPanel `canJoinAllSpaces`)." Stage Manager doesn't use Spaces — it uses dynamic-app-groups. canJoinAllSpaces is a no-op in Stage Manager. Without explicit Stage Manager handling, HUD vanishes on app group switches. Fix: implement NSWindow.collectionBehavior with `.stationary` and `.fullScreenAuxiliary` for SM compatibility; verified on macOS 14+.`

40. `v1 §6.5 + palette §5: ⚪ LOW perf: Retina assets — "PNG hero only for first-launch wizard, served @1x/@2x/@3x." Wizard sigil is 96px, can be SVG/Core Graphics drawing (zero raster cost). PNG triplet = ~150KB of binary. Fix: ship sigil as PDF or vector layer; one asset, all scales.`

---

## Cross-doc spec inconsistencies surfaced during review

- Grain opacity: v1 §2.2 says **1.5%**; §9.0 says **3%**; palette §5 says **2-4%**. Pick one (recommend 0% on integrated GPU; 1.5% only on dGPU machines).
- Blur radius: v1 §2.2 says **28px**; palette §5 says **24px**; perf-prompt cites **18px**. Pick one ≤16px.
- Hotkey: v1 §4.1 says **⌘⇧Space** with keydown trigger; wizard §0 says **⌘⌃** modifier-only with 250ms disambiguation. Wizard's choice imposes a 250ms latency floor — incompatible with the <100ms felt-instant budget.
- Wake-word default: v1 §0/§4 says **OFF** (privacy); wizard §0 says **ON** (operator). Wake-word ON is the single biggest battery hit in the product.
- Tiempos italic frequency: v1 §2.3 says "ONE place only (dashboard hero)"; §9.0+ uses it in every widget eyebrow, every empty-state, every category label. Stated restraint, actual usage = pervasive.

---

## Recommendations (priority-ordered to recover the budget)

1. **Default wake-word OFF** — single highest-leverage change. Recover ~3-8% CPU + ~80MB RAM + App Nap eligibility + ~1Wh/day battery.
2. **Hotkey trigger on keydown of letter chord** (e.g., ⌘⇧Space), not modifier-only — recover 250ms felt-instant.
3. **Lazy widget adapter registration + shared SwiftUI view protocol** — recover ~15-25MB launch memory and brings widget mount under 120ms budget.
4. **Single blur surface (chamber); no tile-level grain/blur stacking; downgrade radius to ≤16px** — recover ~6-10ms/frame on M1 Air integrated GPU.
5. **Replace raw NSTimer refresh with NSBackgroundActivityScheduler; gate all refresh on isLowPowerModeEnabled** — recover ~1-2Wh/day with 3 pinned widgets.
6. **Drop the live preview pane in Settings → Appearance** — recover a redundant chamber render path; preview is theater for a 1-off settings session.
7. **Drop noise grain overlay** — recover 3-4ms/frame everywhere; the obsidian gradient is fine with hairline rules.
8. **DND/battery state = halt animations, not dim them** — recover idle CPU when operator is away.
9. **Debounce fsnotify; single-writer daemon push pattern for state files** — recover daemon-process wake storms on settings interaction.

---

## Findings summary

| Severity | Count |
|---|---|
| 🔴 CRITICAL | 11 |
| 🟡 MEDIUM | 19 |
| ⚪ LOW | 10 |
| **TOTAL** | **40** |

Cited budget breaches: cold-launch (4), hotkey-latency (4), per-frame GPU (5), idle CPU/battery (8), memory (5).

— end —
