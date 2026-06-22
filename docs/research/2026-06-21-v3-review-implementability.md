# Leah macOS Native UI — v3 spec implementability review

> Reviewer: senior macOS engineer (ex-Apple AppKit, Linear Desktop, Granola). Read against `docs/superpowers/specs/2026-06-21-leah-macos-native-ui-design.md` (2330 lines).
> Lens: "what bites me Monday when I start building this." No taste-level critique — only places the spec is wrong, unbuildable, or under-specified for an engineer to ship.

---

## Counts

| Severity | Count |
|---|---:|
| 🔴 BLOCKER | 8 |
| 🟡 RISK | 27 |
| ⚪ NOTE | 14 |
| **Total** | **49** |

| Category | Count |
|---|---:|
| A. AppKit/SwiftUI primitive wrong or impossible | 11 |
| B. Edge cases missing | 13 |
| C. Protocol + IPC realism | 9 |
| D. Build-time blockers (entitlements, sandbox, signing, fonts, models, min-OS) | 11 |
| E. Test plan realism | 5 |

---

## Top 7 most-likely-to-bite-you-Monday

1. **Hardened-Runtime + sandbox block the global hotkey loop the spec assumes is free.** `⌥Space` from anywhere requires either a registered `NSEvent.addGlobalMonitorForEvents` (which is sandbox-allowed but cannot consume the event — `Space` will still type a space into the foreground app) OR the Carbon `RegisterEventHotKey` API (consumes the event, but requires Accessibility permission AND is allowed only for non-sandboxed processes for arbitrary keys; sandboxed Mac App Store apps cannot do this). Spec § 6.1 just says "requires macOS Accessibility permission" and assumes consumption works. **Decision needed before Monday: Mac App Store distribution (no) vs. Developer ID + notarization (yes). The spec doesn't pick one.** Without the pick you can't write entitlements.plist and you can't compile.
2. **`NSPanel` cannot both `canJoinAllSpaces` AND remain non-activating AND take key on summon.** § 4.1 says ambient HUD is `canJoinAllSpaces` non-activating; § 4.3 says the focus chamber "IS a regular key-window NSPanel" that "takes key on summon" and "returns key to prior app on dismiss" (Spotlight pattern). Spotlight doesn't do this with a single NSPanel — Spotlight uses a dedicated `LSUIElement` agent. The chamber-and-HUD-as-one-app design needs a deliberate `NSWorkspace.shared.frontmostApplication` capture-and-restore dance that the spec doesn't describe. "Returns key to prior app on dismiss" is not free in AppKit — it's `NSApp.hide(nil)` + tracked-prior-app `activate(options:)`, and `activate(options:)` is deprecated on macOS 14+ in favor of `NSWorkspace.shared.open` patterns. This is buildable but is *not* "use NSPanel."
3. **`SCShareableContent` observers (§ 6.5, decision #12) replacing `CGScreenIsCaptured` is wrong-API.** `SCShareableContent` enumerates *shareable* windows/displays — it doesn't tell you whether the screen is *currently being captured.* The right notification is `CGDisplayStream` + `CGDisplayRegisterReconfigurationCallback`, or the public-API replacement that landed in macOS 14.4: `CGDisplayStreamFrameStatusFrameComplete` is read-only. The actually-correct macOS-13+ API is the `NSWorkspace.shared.notificationCenter` `NSWorkspaceScreenIsBeingCapturedNotification` (since macOS 12.1, augmented in 13). Spec names the wrong observer.
4. **Frame-budget claim "sprite-sheet sub-millisecond" for thinking ring (§ 5.3) ignores `CADisplayLink`.** macOS has *no* `CADisplayLink` (iOS-only); the equivalent is `CVDisplayLink` (deprecated macOS 15) or `NSView.displayLink(target:selector:)` (macOS 14+). A 20-frame sprite-sheet animated by `CAKeyframeAnimation` on `contents` re-uploads textures every frame on Apple Silicon if not pre-uploaded as `CGImage` w/ `IOSurface` backing. Spec asserts a perf number without naming the timing primitive.
5. **Unix socket length-prefixed JSON at `~/Library/Application Support/Leah/leah.sock` (§ 17 / § 10.7) will fail App Sandbox out of the box.** Sandboxed apps cannot create AF_UNIX sockets in `~/Library/Application Support/` — the sandboxed path is the container, not the user's Library. Either (a) the daemon is *not* sandboxed and lives outside the container (then the UI's connect() needs a temporary-exception entitlement OR XPC), or (b) move the socket to `$XDG_RUNTIME_DIR` equivalent which on macOS is `(getconf DARWIN_USER_TEMP_DIR)`. The spec puts it in the wrong place.
6. **The widget JSON schema bakes `"$schema": "http://json-schema.org/draft-07/schema#"` and assumes runtime validation, but doesn't name a Go validator.** `internal/widget/schema_test.go` is referenced (§ 16.2) but Go has no canonical draft-07 validator that is also fast enough for hot-path widget.mount validation (xeipuuv/gojsonschema is the common pick but allocates aggressively). Monday-blocker: pick library, lock version, get a sample bench.
7. **"Conversation history preserved 24 h on disk" + "5 min idle shrinks to pill" + Esc-does-not-cancel-LLM (§ 6.3) creates a streaming-state-machine the spec doesn't draw.** If the user types a prompt, hits Esc, switches apps for 5 min, the LLM stream completes, the chamber shrinks to a pill, then the user clicks the pill — does the pill show the complete response (yes per spec) but where did the streaming render? Did it render into a hidden chamber? Did it queue deltas? Spec § 5.5 state diagram doesn't include the "chamber-hidden-but-stream-in-flight" state. This is buildable but every engineer will draw a different diagram.

---

## Hard prerequisites — collected single list

**Minimum macOS:** 13.0 (Ventura). Drivers:
- `SCShareableContent` API only on 12.3+ (spec already gates).
- SF Symbols variable-value (waveform) only on macOS 13+ (spec doesn't gate).
- NSPanel `canJoinAllSpaces` exists pre-13 but `NSWindow.CollectionBehavior.canJoinAllApplications` (used for cross-app-Space float) needs 13+.
- `NSAccessibility.reduceMotion` notification name: macOS 12+.

**Entitlements (App Sandbox + Hardened Runtime):**
- `com.apple.security.app-sandbox` (decision: ON or OFF — spec doesn't choose)
- `com.apple.security.device.audio-input` (mic)
- `com.apple.security.network.client` (LLM API, web fetch for image/citation widgets)
- `com.apple.security.network.server` (if daemon listens — only needed for non-unix-socket transport)
- `com.apple.security.files.user-selected.read-write` (image / file widgets the LLM dereferences? — spec hand-waves)
- `com.apple.security.personal-information.calendars` (Calendar adapter)
- `com.apple.security.temporary-exception.files.absolute-path.read-write` for `~/Library/Application Support/Leah/leah.sock` IF sandboxed — confirm with Apple before shipping
- `com.apple.security.automation.apple-events` (if Calendar adapter uses EventKit, not needed; if AppleScript, yes)
- Hardened-Runtime exclusions: `com.apple.security.cs.disable-library-validation` likely required if loading wake-word ML model from on-disk; `com.apple.security.cs.allow-unsigned-executable-memory` if model uses MLIR-style runtime codegen.

**Permissions (TCC, prompted lazily):**
- Microphone (`AVCaptureDevice.requestAccess(for: .audio)`)
- Accessibility (`AXIsProcessTrustedWithOptions` — there is no programmatic grant; spec acknowledges)
- Screen Recording (`CGRequestScreenCaptureAccess` — needed for SCShareableContent observation of captured streams)
- Calendar (`EKEventStore.requestAccess` — replaced by `requestFullAccess` on macOS 14+; spec uses the older API name implicitly)
- Notifications (`UNUserNotificationCenter.requestAuthorization`) — needed for in-Leah notification widget if it falls back to system notifications during DND/fullscreen

**Fonts (licensing — confirm before bundling):**
- **Söhne** — commercial license from Klim Type Foundry (~$600/style for desktop, **does not include app bundling** by default; requires separate App License). Spec lists Inter as fallback; treat Söhne as aspirational until licensing closed.
- **Tiempos Headline Italic** — also Klim, same licensing constraint. Used in ONE place (Dashboard "Today" header). If licensing blocks, fall back to **New York** (Apple-bundled, ships with macOS, free) — visually adjacent.
- **Inter** — OFL, bundle freely.
- **JetBrains Mono** — OFL, bundle freely.

**ML models on disk:**
- Wake-word model (default OFF, but if opted in) — spec doesn't name. Common picks: `openWakeWord` (~1 MB per phrase), `porcupine` (paid). File location: must be inside app bundle `Resources/` OR downloaded into `Application Support/Leah/models/` post-launch with progress indicator — spec doesn't say.

**Code signing + notarization:**
- Developer ID Application certificate.
- Notarization (`xcrun notarytool`) for distribution outside MAS.
- Sparkle DSA / EdDSA keypair for auto-update — spec doesn't mention auto-update at all (NOT in scope, but worth flagging).

---

## A. AppKit/SwiftUI primitive issues

`§ 4.1 ambient HUD`: 🔴 A BLOCKER: spec says HUD "Floats above normal windows; below fullscreen apps unless `always-on-top` toggled" + `canJoinAllSpaces`. `NSPanel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]` gets you above fullscreen apps; `.stationary` keeps it from joining mission control. Spec needs to enumerate the actual `NSWindow.CollectionBehavior` mask — current wording produces conflicting masks.

`§ 4.3 focus chamber`: 🔴 A BLOCKER: "regular key-window NSPanel" — `NSPanel.becomesKeyOnlyIfNeeded = false` plus `worksWhenModal = true`, plus `level = .modalPanel`. None of these are listed. Without the level set, the chamber will sit at `.normal` and lose to any opened modal dialog.

`§ 4.4 notification widget — max 2 visible, stacks above HUD`: 🟡 A RISK: stack management with overflow collapse and a single coalesced NSTimer (spec § 4.4 perf note) is fine, but spec doesn't say whether toasts are NSPanels or one NSPanel with multiple NSView children. NSPanel-per-toast lets you place independently but each panel adds an `NSWindowController` and gets a separate `NSWindowDidChangeOcclusionStateNotification` — drains battery. Pick: one panel, internal layout.

`§ 5.3 listening pulse — "pause when chamber not visible AND ambient HUD occluded"`: 🟡 A RISK: occlusion detection via `NSWindow.occlusionState` only fires when the system is sure (>75% covered). A HUD that's 70% covered keeps the animation running. Spec asserts a battery recovery number ("0.4% sustained CPU") that depends on a heuristic the OS won't always trigger.

`§ 5.3 speaking waveform — "OR a single Metal shader (one draw call, 5 quads)"`: 🟡 A RISK: 5 quads in a single Metal draw is correct but mixing a Metal-backed `CAMetalLayer` inside an `NSVisualEffectView` hierarchy disables the vibrancy on that layer's superview. Spec doesn't say the waveform isn't INSIDE the chamber's vibrancy region — but the chamber's vibrancy is § 3.2's whole point.

`§ 5.4 Flourish 1 — "transform.scale.y with anchor-point at seam center, NOT a layout-bounds animation"`: 🟡 A RISK: `NSWindow` frame animations DO trigger layout. `CALayer.transform` only animates the layer, not the window frame. To do the seam-and-unfold flourish without a window-frame animation, you have to create the chamber at final size (already laid out) and animate the content view's transform — meaning the chamber is fully visible behind a mask for the duration. Spec implies a window-grows-from-zero flourish. Pick one.

`§ 6.5 fullscreen + secondary monitor opt-in — "relocate to chosen secondary monitor"`: 🟡 A RISK: monitoring `NSApplication.didChangeScreenParametersNotification` works; but the "fallback if the named monitor disconnects mid-session: dim+shrink to corner orb (default)" requires you to detect the *named* monitor by `NSScreen.deviceDescription[.screenNumber]`, which is unstable across reconnects. Spec needs to say the matching key (model name? UUID via `IOServicePort`? — there isn't a clean one).

`§ 7.2 chamber — "input field gains focus synchronously with keydown"`: 🟡 A RISK: NSWindow `makeFirstResponder:` is synchronous, but `NSApp.activate(ignoringOtherApps: true)` is not — activation runs async via the WindowServer. If the user types within ~50 ms of hotkey-release, first keystrokes will land in the prior app. Spotlight handles this via a private API (`_setActivationPolicy:`). The spec's "<100ms felt-instant" claim collides with this.

`§ 13.5 gold seam flourish — diagrammed bands "from screen-vertical-center"`: ⚪ A NOTE: "screen" here means which screen? Multi-monitor with chamber on secondary, seam at "screen vertical center" wants the chamber's *parent screen* not main. Make it explicit.

`§ 4.2 menubar — "NSStatusItem hit-area is system-padded to ≥24 × 24 pt"`: ⚪ A NOTE: this is true but the menubar height is fixed at 24 pt on standard menubar, 32 pt on the "Larger" accessibility setting. A 24×24 hit area is the *floor*, not always achievable above the menubar's intrinsic height. Spec wording reads as if you control it; you don't.

`§ 17 — "SVG; render via Core Animation for the rotation/pulse flourishes at 60 fps"`: ⚪ A NOTE: AppKit has no SVG renderer. You'd use SF Symbols (custom symbol JSON template) or precompose to PDF (vector PDF assets) or convert SVG → CGPath at build time. "SVG" in an implementation note is hand-wavy.

---

## B. Edge cases missing

`§ 5.5 state diagram`: 🔴 B BLOCKER: app-backgrounded-mid-stream — what happens? The state machine doesn't cover (a) chamber dismissed via Esc with stream in-flight, (b) app backgrounded, (c) stream completes, (d) user re-summons via hotkey. The shrink-to-pill diagram assumes the chamber was visible.

`§ 7.3 chamber states`: 🔴 B BLOCKER: "Network drops mid-LLM-stream" not enumerated. Daemon-down state covers process-down; network-down mid-stream needs its own row. Today's failure mode: partial response renders, no caption, no retry.

`§ 4.3 chamber lifetime`: 🟡 B RISK: daemon restart mid-session — does HUD reconnect transparently? Spec implies yes (unix socket reconnect loop) but never specifies the reconnect cadence, the deltas-buffered-while-disconnected policy, or the "stale tile" indicator.

`§ 6.5 multi-monitor`: 🔴 B BLOCKER: "two displays, one disconnects" — ambient HUD on the disconnected display goes where? Spec only handles this for the fullscreen + secondary case. Default case undefined → HUD invisible at offscreen coordinates.

`§ 6.7 wake-word + macOS sleep/wake`: 🟡 B RISK: wake-word listener resumes after wake? Hotkey re-registers after wake? Spec doesn't say. `RegisterEventHotKey` survives sleep but Accessibility-derived listeners often don't.

`§ 3.6 appearance auto-switch`: 🟡 B RISK: spec says cross-fade per surface over 240 ms — but `NSApp.effectiveAppearance` KVO fires synchronously, and if the chamber is mid-streaming-response when appearance changes, text re-renders with new colors mid-paragraph. Spec doesn't say "freeze response until cross-fade complete."

`§ 3.3 typography`: 🟡 B RISK: "Tiempos italic" rendering Cyrillic/CJK — Tiempos Headline doesn't ship CJK glyphs. The Dashboard "Today" header for a Japanese operator renders as `.notdef` boxes. Spec doesn't define a fallback chain.

`§ 4.1 HUD position "100 px from edge" + § 11.5 multi-monitor`: 🟡 B RISK: 4K external (220 ppi) + 1080p built-in (110 ppi) — "100 px" in spec is in points or pixels? § 4.1 uses "px" loosely. NSWindow API is points. Be explicit.

`§ 5.3 speaking waveform + § 11.3 VoiceOver`: 🟡 B RISK: VoiceOver active — per-token streaming announce is killed (§ 15 anti-pattern). Sentence-batched announce is mentioned but the implementation rule is not given (when does a sentence end mid-stream — punctuation lookahead? Spec doesn't say).

`§ 6.1 hotkey relaunch / restart`: 🟡 B RISK: HUD restores last position on relaunch (state file in `~/Library/Application Support/Leah/`) but spec doesn't define behavior when the saved position is on a now-disconnected monitor at relaunch time. Default: HUD renders offscreen.

`§ 6.5 lockscreen`: ⚪ B NOTE: spec doesn't cover lockscreen. Daemon should suspend HUD render, suspend wake-word, keep API calls in-flight. Currently undefined.

`§ 7.3 + § 17 state-path migration`: ⚪ B NOTE: existing browser-HUD users have `~/.leah-state/` (per repo). Migration path to `~/Library/Application Support/Leah/` is not in the spec — first-launch detection + one-time move needs an owner.

`§ 6.7 permission revoked mid-session`: 🟡 B RISK: user revokes mic in System Settings while wake-word is opted in. Spec says menubar shows "WAKE-WORD PAUSED · LOW POWER" for battery case but does not enumerate the permission-revoked case. AVAudioSession on macOS doesn't fire a clean "permission revoked" — you discover it on the next `requestAccess`.

---

## C. Protocol + IPC realism

`§ 10.7 transport`: 🔴 C BLOCKER: Unix socket placement (see Top-7 #5). Move to `(getconf DARWIN_USER_TEMP_DIR)/leah.sock` or use XPC. As specified, App Sandbox containers can't bind there.

`§ 10.7 envelope size cap 256 KB`: 🟡 C RISK: 256 KB hard cap collides with `image` widget that the daemon dereferences and returns as a `blob URL` (presumably `data:` URI). A 4K JPEG is easily >256 KB encoded. Either: (a) image data goes out-of-band (separate file path, daemon serves via local HTTP), or (b) the cap is per-frame and image frames don't carry the bytes — spec is silent.

`§ 10.7 backpressure`: 🟡 C RISK: length-prefixed JSON over AF_UNIX has no backpressure signaling. If the UI is slow (Reduce Motion off, big widget tree), the kernel buffer fills and daemon `write()` blocks. Spec doesn't say whether daemon uses non-blocking writes + queue + drop-policy, or blocks. Drop-policy matters for `prose.delta` (drop = lost tokens = corrupted text).

`§ 10.7 widget validation`: 🟡 C RISK: "daemon validates, UI also validates" is implied but not stated. Double-validation cost on hot path. Single-side validation needs a trust direction (daemon is trusted, UI just renders); spec doesn't pick.

`§ 10.7 tool-call args contain URL → daemon fetches → returns blob URL to HUD → HUD renders`: 🟡 C RISK: blob URL TTL? Cache invalidation? If the same URL is requested twice in one turn (LLM repeats), is it deduplicated? `~/Library/Application Support/Leah/widget-cache/<adapter>/<sha256(props)>.json` is the cache (§ 17) but `props` includes refresh timing — the same image will get a different sha256 across refreshes.

`§ 6.7 wake-word model loading`: 🟡 C RISK: model file lives where? Spec mentions on-device fine-tune nightly during charging. Fine-tuning a wake-word model on-device requires either Core ML model + `MLUpdateTask` (works, but slow), or shipping a TF Lite runtime (license + binary size). Spec hand-waves.

`§ 5.3 TTS audio`: 🟡 C RISK: streamed or batched? Where does playback halt on operator interrupt? `AVSpeechSynthesizer.stopSpeaking(at: .immediate)` works but if the TTS is from a remote API (ElevenLabs, OpenAI), playback runs through `AVAudioPlayer` and stopping mid-buffer creates a click. Spec doesn't say which TTS engine.

`§ 10.7 telemetry frames vs widget mount frames — multiplexed on one socket`: 🟡 C RISK: spec § 10.7 says "Envelope size cap 256 KB" but also that the same channel carries prose deltas, widget events, AND telemetry (§ 10.8 references `internal/obs/`). Telemetry frames have completely different shape — needs a `kind` discriminator at envelope level. Spec shows envelope only for widget envelope.

`§ 17 fsnotify on registry`: ⚪ C NOTE: `fsnotify` on macOS is `FSEvents` under the hood. `FSEvents` is path-based, not file-handle-based — if the registry file is replaced (rename + move on top), the watcher may miss the change. Standard fix is to watch the parent directory + filter. Worth a one-line note.

---

## D. Build-time blockers

`§ 6.1 + § 17`: 🔴 D BLOCKER: spec doesn't choose between Mac App Store (sandboxed) and Developer ID + notarization (un-sandboxed). Every other decision (entitlements, hotkey API, socket placement, model loading) cascades from this. Pick before Monday.

`§ 3.3 typography`: 🔴 D BLOCKER: Söhne license. Klim's standard app license is per-quarter MAU-tiered. For an indie / personal-use app, Inter fallback is fine — spec already allows. But the spec language ("Söhne (Inter fallback if license unavailable)") needs to flip default to Inter, with Söhne as a post-launch upgrade. Otherwise an engineer ships an unlicensed font.

`§ 3.3 Tiempos`: 🔴 D BLOCKER: same Klim issue. Use **New York Italic** (system-bundled, free, visually adjacent serif italic) — spec doesn't list this fallback.

`§ 6.1 hotkey + § 6.6 Accessibility permission`: 🟡 D RISK: `RegisterEventHotKey` is part of Carbon's `HIToolbox` framework which is technically deprecated in macOS 14 docs. Replacement is `NSEvent.addGlobalMonitorForEvents` + `addLocalMonitorForEvents` *combined* — but global monitor cannot consume events. Spec is silent on this collision.

`§ 5.3 SF Symbols variable-value waveform`: 🟡 D RISK: variable values land macOS 13+. Spec doesn't gate a minimum macOS version. Pick 13.0 minimum or downgrade to a static waveform on macOS 12.

`§ 6.5 ScreenCaptureKit`: 🟡 D RISK: macOS 12.3+ already gated in spec, but ScreenCaptureKit requires Screen Recording permission EVEN to *enumerate* shareable content. Spec doesn't list Screen Recording in the lazy-prompt set (§ entitlement section is silent).

`§ 17 NSBackgroundActivityScheduler`: ⚪ D NOTE: not mentioned in spec but the "fine-tune nightly during charging" rule needs it. NSBackgroundActivityScheduler is sandbox-allowed and is the right primitive.

`§ 16.10 powermetrics in tests`: 🟡 D RISK: `powermetrics` requires root. CI cannot run this without sudo. Spec asserts a test ("`powermetrics --samplers cpu_power -i 1000`") that won't run on standard GitHub Actions runners.

`§ 16.10 simulate os_memory_pressure`: 🟡 D RISK: there is no public API to inject memory pressure. The Apple-recommended tool is `memory_pressure -l critical` (also requires root). Spec needs to say "manual local test" or pick a stub.

`§ 16.9 frame budget Instruments`: ⚪ D NOTE: Instruments templates don't run headless in CI. Either spec a Metal-frame-counter shim or mark this test "local-only."

`§ 17 Hardened Runtime + audio device`: ⚪ D NOTE: audio capture is fine under Hardened Runtime — no exception needed. But the spec doesn't say so explicitly; engineer will spend a half day confirming.

`§ 17 missing entry`: 🟡 D RISK: no entry for auto-update (Sparkle / EdDSA). Spec is "design lock" but ships-as-personal-use means update strategy matters. Add or explicitly mark out of scope.

---

## E. Test plan realism

`§ 16.1 visual contract / snapshot tests for ASCII wireframes`: 🟡 E RISK: snapshot-testing ASCII art is performative — ASCII art doesn't encode color, blur, motion, or focus state. The only mechanically-verifiable thing is layout grid units. Spec should call this what it is (layout-grid regression check) or drop it.

`§ 16.5 streaming test "Driver injects synthetic LLM stream"`: 🟡 E RISK: the test asserts frame ORDER but not frame TIMING. A correct daemon could emit all frames in 1 µs — passing the test but failing the design intent (prose chunks interleaved with widget mounts as the LLM streams them, not after). Add a per-frame minimum-delay assertion.

`§ 16.8 VoiceOver smoke test`: 🟡 E RISK: VO automation requires `AXUIElement` queries plus `osascript` to drive VO commands. There's no Swift-side VO automation API. CI'll need a macOS runner with VO permission granted, which Apple Silicon GH runners can't grant. Local-only.

`§ 16.7 cross-doc parity check`: ⚪ E NOTE: `make check` rule cross-referencing § 10.1 widget catalog ⊇ protocol widget list — fine, mechanically derivable, ship.

`§ 16.10 App Nap eligibility test`: ⚪ E NOTE: verifiable only via `Energy Log` post-hoc; not a unit/integration test. Move to manual-test checklist.

---

## Additional findings beyond the 5 categories

`§ 8.3 wizard step 2`: 🟡 RISK: wizard says re-recording the hotkey gives "real-time conflict feedback as keys are pressed." Implementing this requires reading the key event before the OS dispatches it — fine inside a wizard textfield (local monitor), but the conflict check itself ("Apple-documented system shortcuts") requires either a curated list (where? — spec doesn't reference) or the now-removed HIToolbox enum (spec acknowledges removal). The list needs a maintainer.

`§ 9.5 destructive memory-purge "typed PURGE"`: ⚪ NOTE: locale issue — non-English speakers asked to type a Latin-script all-caps word. Workable but consider localizing the confirmation string per locale (e.g., 削除 / SUPPRIMER).

`§ 10.8 security — open_url verb`: 🟡 RISK: "`open_url` requires the URL be in a tile already rendered" — the daemon needs to maintain a per-turn rendered-URL set keyed by tile id. Spec doesn't specify the lifetime (per-turn? per-chamber-session? until pinned?) or what happens after the tile is dismissed.

`§ 10.9 extensibility — "lazy-register"`: ⚪ NOTE: adapter init on first render means the first invocation of a widget pays the init cost — which is exactly when the LLM is mid-stream and the user is watching. Spec asserts this is a perf win (boot-time savings) but doesn't acknowledge the first-render tax. Probably right call; document the trade.

`§ 11.2 reduced motion`: 🟡 RISK: spec says "Flourish 2 → opacity fade only (NOT color-shift — semantic-color)" but Flourish 2 *is* a color-shift (gold → glow → gold). Under reduced motion this collapses to nothing. Spec's reduced-motion replacement is "sigil color shifts to `--gold-glow` for 200 ms then back" (§ 5.4) — which IS a color shift, contradicting § 11.2.

`§ 17 NSVisualEffectView fallback for Wails/webview "backdrop-filter polyfill"`: 🟡 RISK: there is no backdrop-filter polyfill that matches NSVisualEffectView. Safari `backdrop-filter: blur()` is real but doesn't sample from windows behind the browser — only from same-document content. A webview-based chamber will not have real glass blur against the desktop. Decide before picking the framework.

`§ 4.1 HUD per-monitor sticky + § 11.5 each monitor remembers HUD anchor + scale mode`: ⚪ NOTE: "remembers" → state file schema needs per-monitor identity. As above, monitor identity across reconnect is unstable. Pick a degraded-match policy ("fuzzy match by resolution + name" → fallback to default position).

`§ 16.2 schema_test.go — golden good/bad props table`: ⚪ NOTE: gold-standard. No issues.

`§ 6.4 push-to-talk Fn (or ⌥)`: 🟡 RISK: `Fn` key has NO accessible event on macOS without private API. NSEvent doesn't expose Fn modifier reliably — `event.modifierFlags.contains(.function)` works on internal keyboards but is inconsistent on external. `⌥` already conflicts with the global hotkey (`⌥Space`). Pick a different PTT key (e.g., right-Cmd alone).

`§ 18 versioning + change log + § 14 open decisions log`: ⚪ NOTE: 90+ rows in the decisions log — at this density, the spec doubles as its own ADR ledger. Worth splitting for readability; doesn't affect implementability.

`§ 6.7 wake-word phrase "Hey Leah" — 2-word reduces false-pos ~80%`: ⚪ NOTE: citation needed; voice-assistant literature varies wildly by SNR. Treat as design intuition not measured fact.

`§ 7.1 ambient HUD row 2 — predictable source rotation by time of day`: ⚪ NOTE: timezone — what defines AM/PM/Evening? `Calendar.current` is sandbox-allowed; spec doesn't say. Default fine.

---

## Final notes for the implementer

This spec is unusually rigorous on visual contract, color tokens, and motion durations. Where it breaks down for build-time is the **sandboxing posture** (MAS vs Developer ID — undefined), the **process boundary between daemon and UI** (single app? launchd agent? XPC service?), and the **streaming-state-machine edge cases** around dismiss + cancel + idle + reconnect.

Recommended single biggest unblock before Monday: write `entitlements.plist` + decide distribution channel. Every other decision rebases on this one.
