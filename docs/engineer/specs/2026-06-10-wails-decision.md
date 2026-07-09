# Wails native-window wrapping — defer decision (W38)

Date: 2026-06-10
Scope: cmd/leah-hud entrypoint
Status: ADR — defer

## 1. Question

Spec §3 picks Wails as the long-term native-window framework for the HUD.
The HUD ships today as a localhost HTTP server (W34–W37). Do we add
`github.com/wailsapp/wails/v3` to `go.mod` now and wrap `cmd/leah-hud` in
a native window, or stay on pure HTTP until later?

## 2. Decision

Defer. Stay on localhost HTTP. Revisit when Wails v3 hits beta.

## 3. State of Wails (as of 2026-06-10)

- v3 line: alpha — latest is `v3.0.0-alpha.96` (2026-05-25).
- v3 docs explicitly say "alpha. API reasonably stable, apps in
  production." No beta tag yet.
- Daily release cadence since alpha.10 — breaking changes land
  continuously.
- v2 is stable but uses the old declarative single-window API that v3
  abandons; adopting v2 means a second migration when v3 betas.

## 4. Cost of adopting now

- Native build toolchain becomes a CI dependency: macOS WebKit,
  Linux WebKit2GTK, Windows WebView2. Current CI runs `go test ./...`
  only.
- `wails3 init` adds a `wails.json` + `frontend/` shape that splits
  the current single-package HUD into a Go-side + JS-side build.
- Alpha churn forces re-pinning at least monthly; each bump risks
  reviewer time and rebuild breakage.
- go.sum grows substantially (Wails pulls webview2, go-webkit2,
  syscall shims, dev-server tooling).

## 5. Cost of waiting

- Pure HTTP HUD already renders in the operator's browser. No user-
  visible feature is blocked.
- The ambient/focus/widgets/operator-config HTML+CSS+JS asset tree
  embedded via `internal/hud/static` is Wails-ready — Wails v3
  consumes an `embed.FS` for static assets the same way the current
  `http.FileServer` does. No throwaway work today.
- Future migration is a `main.go` swap (replace `http.ListenAndServe`
  with `application.New(...).Run()`) plus a `wails.json` — the asset
  tree carries over.

## 6. Decision rule

Per README.md: UX > performance > long-term benefits, default simpler.
A browser tab is the same UX as a frameless WebView for the MVP-5
operator (single user, local machine). Performance is identical
(both render in WebKit/Chromium). The long-term benefit (native
chrome, dock icon, single-instance) is real but unrealized until
Wails ships beta.

## 7. Revisit trigger

Reopen this ADR when any of:
- Wails v3 tags `v3.0.0-beta.1` (or later) on GitHub.
- An operator-facing feature requires native window APIs the browser
  can't provide (system tray, global hotkey OS-side, dock badge).
- A second consumer of the HUD assets (e.g. mobile shell) needs a
  shared native runtime.

## 8. Out of scope

Wails v2 adoption. The v2 API is legacy; adopting it now would force
a second migration when v3 betas. If a native shell becomes urgent
before v3 beta, the cheaper path is a stock `webview/webview_go`
wrapper (~200 LOC, no framework lock-in) — not v2.
