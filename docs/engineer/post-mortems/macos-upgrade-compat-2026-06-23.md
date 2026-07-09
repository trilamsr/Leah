# macOS upgrade compat audit 2026-06-23 — Sonoma 14 / Sequoia 15 / Tahoe 16

Scope: native macOS app slice (`app/Leah/Sources/**`) + darwin-tagged Go bridges (`internal/vision/**`). Deployment target `app/Leah/Package.swift:6` = `.macOS(.v14)`.

Sources: Apple "macOS Sequoia 15 Release Notes" (developer.apple.com/documentation/macos-release-notes/macos-15-release-notes), Sparkle 2.6.4 release (github.com/sparkle-project/Sparkle/releases/tag/2.6.4), VNRecognizeTextRequest documentation (developer.apple.com/documentation/vision/vnrecognizetextrequest).

## Coverage matrix

Legend: `OK` = same behavior; `OK*` = OK with caveat; `BREAK` = known runtime/API regression; `?` = needs hardware test.

| # | Surface                                   | 14 (Sonoma) | 15 (Sequoia) | 16 (Tahoe) |
|---|-------------------------------------------|-------------|--------------|------------|
| 1 | NSPanel collection behavior (HUD + focus) | OK          | OK*          | ?          |
| 2 | Sparkle 2.6.x updater                     | OK          | OK*          | ?          |
| 3 | Keychain kSecAttr* (BYOK)                 | OK          | OK           | OK         |
| 4 | Vision OCR (VNRecognizeTextRequest)       | OK          | OK*          | ?          |
| 5 | AVCaptureDevice (mic auth only)           | OK          | OK*          | ?          |
| 6 | CGDisplayStream live-screen               | OK*         | BREAK        | BREAK      |
| 7 | NSStatusItem hexagon                      | OK          | OK*          | ?          |
| 8 | Carbon hot-key (⌥Space, dashboard)        | OK          | OK           | OK*        |

## Per-category findings

### 1. NSPanel Space affinity

`app/Leah/Sources/LeahApp/AmbientHUDWindow.swift:54` and `app/Leah/Sources/LeahUI/FocusPanel.swift:31` both set `collectionBehavior = [.fullScreenAuxiliary, .moveToActiveSpace, .stationary]`. Neither uses `.canJoinAllSpaces` — the Sequoia semantic change to that flag (now requires entitlement on multi-display setups, per macOS 15 release notes "AppKit") is **not** load-bearing here.

`.moveToActiveSpace` + `.fullScreenAuxiliary` continue to work on 15. Caveat (`OK*`): Sequoia tightened the rule that NSPanels with `.nonactivatingPanel` styleMask must not also rely on `windowDidResignKey` for dismissal in Stage Manager — the `FocusPanel` does exactly this (`FocusPanel.swift:71`). Stage Manager users on 15 may see the focus panel dismiss spuriously when a tile transition steals key state.

Fix sketch: gate `windowDidResignKey` dismissal behind a "user clicked outside" detector (`NSEvent.mouseLocation` outside `panel.frame`) instead of trusting key-loss alone.

### 2. Sparkle updater

`app/Leah/Package.swift:11` pins `sparkle-project/Sparkle, from: "2.6.0"`. `app/Leah/Sources/LeahUpdate/Updater.swift:23-28` instantiates `SPUStandardUpdaterController` with a delegate that allow-lists channels.

Sparkle 2.6.0 supports macOS 10.13+. **Caveat** (`OK*`): Sequoia's stricter Sandbox/SMJobBless deprecation requires Sparkle ≥ 2.6.2 for clean installer behavior on signed apps; the `from: "2.6.0"` lower bound permits resolution to a pre-2.6.2 cached graph in a frozen-lockfile environment. Tahoe is unverified — Sparkle's Sequoia-compatible release line (2.6.4) postdates Tahoe DB1.

Fix sketch: tighten to `from: "2.6.4"`; add Tahoe smoke-test once DB ships.

### 3. Keychain BYOK

`app/Leah/Sources/LeahAuth/Keychain.swift` uses `kSecAttrAccessibleWhenUnlocked` (lines 33, 40) and does **not** set `kSecAttrSynchronizable`. iCloud Keychain visibility changes in macOS 15 do not apply — Leah's BYOK item is local-only and per-user. `OK` across 14/15/16.

No fix needed.

### 4. Vision OCR locales

`internal/vision/ocr/ocr_bridge_darwin.m:11-13` instantiates `VNRecognizeTextRequest` with `.accurate` level + `usesLanguageCorrection = YES` but **never sets `recognitionLanguages`**. macOS 15 added 10+ recognition languages (per macOS 15 release notes "Vision") — defaulting to the system-preferred list will silently widen coverage on 15+, which may shift confidence scores for mixed-script captures.

Caveat (`OK*`): no breakage on 14, but the OCR confidence-threshold callers downstream (`internal/vision/ocr/ocr.go`, `internal/vision/router/router.go`) may receive different blocks on 15 vs 14 for the same input. No deterministic-output guarantee.

Fix sketch: pin `req.recognitionLanguages = @[@"en-US"]` for v1.1 stability; widen explicitly in v1.2 with operator opt-in.

### 5. AVCaptureDevice permissions

`app/Leah/Sources/LeahUI/Wizard/MicStep.swift:50` calls `AVCaptureDevice.requestAccess(for: .audio)` from a button action. `app/Leah/Sources/LeahUI/Settings/PermissionsPane.swift:43` reads status. **Camera is never requested** — the camera-capture path in `internal/vision/capture/screen_darwin.go:25-27` is a stub returning "pending AVCaptureDevice bridge".

Sequoia's continuous-access prompt cadence change (now re-prompts monthly for screen+camera continuous access, per macOS 15 release notes "TCC") affects screen and camera. Mic is unaffected. Since Leah's camera path is unimplemented, the cadence change has no impact today.

Caveat (`OK*`): when camera capture lands (T03 follow-up), the monthly re-prompt must be handled — store last-prompted timestamp and surface a soft notice rather than a silent stream stall.

### 6. CGDisplayStream live-screen

`internal/vision/capture/screen_darwin.go:18-23` returns errors for both `Screenshot` and `StartLiveScreen` — both unimplemented. The error strings reference `CGDisplayCreateImage` and `CGDisplayStream`, both of which Apple **deprecated in macOS 14.4** and the recommended replacement is `ScreenCaptureKit` (`SCStream`, `SCStreamConfiguration`).

`BREAK` on 15/16: shipping the planned CGDisplayStream bridge today would land on a deprecation that emits warnings on 14, still functions on 15, but per Apple's "Sequoia and later" guidance may be removed by Tahoe.

Fix sketch: skip CGDisplayStream entirely; implement T04 directly against ScreenCaptureKit (`SCStream` for live, `SCScreenshotManager.captureImage` for snap). Adds a `ScreenCaptureKit` framework link in `internal/vision/capture/` cgo LDFLAGS or a Swift bridge.

### 7. NSStatusItem hexagon

`app/Leah/Sources/LeahUI/MenubarItem.swift:51` requests `NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)`. `app/Leah/Sources/LeahUI/MenubarHexagon.swift:30` marks the rendered image `isTemplate = true`.

Sequoia tightened menubar item width when the "Show full menubar in fullscreen" toggle is off — squareLength items render at 22pt instead of 24pt. The hexagon is drawn into an 18×18 NSImage (`MenubarHexagon.swift:8`) which fits both sizes.

Caveat (`OK*`): if a user is on Sequoia with the new "auto-hide menubar in fullscreen" enabled, the hexagon click-target shrinks. No visual breakage but tap precision degrades. Tahoe unverified.

Fix sketch: none needed for v1.1; track tap-target complaints post-ship.

### 8. Carbon hot-key API

`app/Leah/Sources/LeahUI/HotkeyManager.swift:41-48` uses `RegisterEventHotKey` (Carbon HIToolbox). `app/Leah/Sources/LeahApp/LeahApp.swift:76-101` repeats the pattern for the dashboard hotkey.

`RegisterEventHotKey` is deprecated in name but Apple has shipped no replacement; it remains supported through macOS 15. **Caveat** (`OK*` on 16): Carbon HIToolbox is the longest-running deprecation in macOS history; Tahoe DB1 has not removed it but Apple's Carbon-removal roadmap is unstated. No drop-in replacement — `NSEvent.addGlobalMonitorForEvents` requires Accessibility but does not block the key from reaching the focused app.

Fix sketch: keep Carbon for v1.1; track Tahoe DB notes for removal signal.

## Recommendations

**Fix BEFORE v1.1 ship:**

- (#4 Vision OCR) Pin `recognitionLanguages = ["en-US"]` in `ocr_bridge_darwin.m:11`. One-line change, prevents silent confidence-score drift on 15.
- (#2 Sparkle) Bump pin from `2.6.0` to `2.6.4` in `Package.swift:11`. One-line, resolves Sequoia installer compatibility.
- (#6 CGDisplayStream) Do **not** implement live-screen against CGDisplayStream — go straight to ScreenCaptureKit. Affects T04 brief; skip the deprecated path.

**Defer to v1.2:**

- (#1 NSPanel) Stage Manager dismissal hardening — only matters for Stage Manager users (small share).
- (#5 AVCaptureDevice) Camera re-prompt UX — only matters once camera capture lands.
- (#7 NSStatusItem) Menubar tap-target on Sequoia auto-hide menubar — cosmetic.

**No action:**

- (#3 Keychain) Not affected.
- (#8 Carbon hotkey) Works through 16.

## Test gap list (need 15/16 hardware)

1. NSPanel Stage Manager dismissal regression (#1) — needs Sequoia + Stage Manager on.
2. Sparkle 2.6.x updater install on Sequoia signed+notarized app — needs Sequoia machine + a real appcast item.
3. Vision OCR confidence-score parity (#4) — same fixture image OCR'd on 14 vs 15, diff the `TextBlock.Confidence` values.
4. Menubar hexagon at 22pt on Sequoia auto-hide menubar (#7) — visual check.
5. Carbon `RegisterEventHotKey` smoke test on Tahoe DB (#8) — when DB is publicly available.
6. ScreenCaptureKit replacement implementation for T04 (#6) — write the bridge before claiming live-screen ships.
