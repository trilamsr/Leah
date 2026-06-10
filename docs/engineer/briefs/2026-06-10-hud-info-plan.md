# HUD UI + info-feeds — combined delivery plan W31–W37

Status: brief (locks W31–W37 sequencing)
Owner: trilamsr
Companions: `docs/engineer/specs/2026-06-10-hud-ui.md`, `docs/engineer/specs/2026-06-10-info-feeds.md`

## Overview

Seven waves. The first three (W31–W33) ship the info-feeds layer all the
way through the morning brief — no HUD dependency, so feeds land first and
prove their cache + synth posture on a surface that already exists. The
back four (W34–W37) ship the HUD: skeleton, focus-panel + reasoner wiring,
info-widget integration, and operator-config. Each wave is sized to a
single reviewable PR; LOC budgets are guidelines, not contracts.

The sequencing reflects `CLAUDE.md` priority UX > performance > long-term:
feeds land first because the morning brief is the operator's existing daily
surface, so synth quality gets measured before the HUD shows it. The HUD
then arrives on a feeds layer that's already been operator-validated.

## W31 — feeds skeleton + weather adapter + cache (≤500 LOC)

- **Goal**: `internal/feeds/` package compiles; weather feed works
  end-to-end via OpenWeatherMap; cache layer functional.
- **Files touched**:
  - `internal/feeds/feeds.go` — `Feed`, `Item`, `Domain`, `Query` interfaces
  - `internal/feeds/cache.go` + `cache_test.go` — SQLite, TTL, SWR, LRU
  - `internal/feeds/weather/openweather.go` + `openweather_test.go`
  - `internal/feeds/synth/weather.go` + `weather_test.go` — rule-based
    `Briefing`
  - `cmd/leah/weather.go` — `leah weather [city]`
  - `cmd/leah/connect.go` — extend with `weather` integration
- **Risk**: SQLite schema-future-proofing — defer until W37 brings
  operator-config schema decisions. MVP: single-table schema with
  domain+key+payload+expires_at.
- **Size**: ≤500 LOC.
- **Test plan**: weather happy + 429 + 5xx + timeout via `httptest`;
  cache TTL + SWR + LRU; synth golden fixtures.
- **Unblocks**: W33 (brief integration), W36 (HUD weather widget).

## W32 — news + market adapters; synthesizers; CLI subcommands

- **Goal**: News and market feeds work; CLI `leah news` and `leah quote`
  return synthesized output.
- **Files touched**:
  - `internal/feeds/news/{hn,reddit,gnews}.go` + tests
  - `internal/feeds/market/{yahoo,alphavantage}.go` + tests
  - `internal/feeds/synth/{news,market}.go` + tests (rule-based)
  - `cmd/leah/{news,quote,info}.go`
  - `internal/feeds/wiki/{wikipedia,ddg}.go` + tests (for `leah info`)
- **Risk**: Yahoo Finance unofficial endpoints drift. Mitigation: Alpha
  Vantage fallback wired in `market.Fetch` from day one.
- **Size**: ~700 LOC across multiple adapters; reviewers can read by
  source.
- **Test plan**: per-adapter `httptest`; market-no-advice lint
  fixture-based; news dedup + ranking golden tests.
- **Unblocks**: W33, W36, voice info-feed routing.

## W33 — morning brief integration (weather + headline + market)

- **Goal**: Morning brief gains weather + top-headline + market summary
  lines after the existing gmail + gcal output (PR #65).
- **Files touched**:
  - `internal/brief/brief.go` — extend `Composer` to call feeds synth
  - `internal/brief/brief_test.go` — golden brief fixtures with feeds
  - `internal/feeds/feeds.go` — `Synthesize` facade if needed
- **Risk**: Brief composition order — operator sensitivity to brief
  length. Mitigation: feeds output is one section; operator can disable
  via config (W37).
- **Size**: ≤300 LOC.
- **Test plan**: golden brief output for cache-hit, cache-miss-with-
  network-stub, and feeds-disabled cases.
- **Unblocks**: brief is now the daily synth-validation surface; HUD
  reuses the same synthesizers.

## W34 — Wails `cmd/leah-hud/` skeleton + ambient panel (≤700 LOC)

- **Goal**: `leah-hud` binary launches a transparent, frameless ambient
  panel showing time, weather, calendar-next, market ticker, headline.
- **Files touched**:
  - `cmd/leah-hud/main.go`
  - `internal/hud/state.go` + `state_test.go` — hidden/ambient/focus FSM
  - `internal/hud/grpc.go` + `grpc_test.go` — `Telemetry.Subscribe` client
  - `internal/hud/static/{index.html,hud.css,hud.js}` — ambient template
  - `internal/hud/render_test.go` — HTML snapshot tests
  - daemon side: `internal/daemon/telemetry.go` — server stream emitting
    `TelemetryFrame` (push-rate ≤500ms)
- **Risk**: Wails webview transparency on macOS 26. Mitigation: smoke-
  test on launch; CI runs HTML snapshot tests only (no real window).
- **Size**: ≤700 LOC.
- **Test plan**: FSM table-driven; gRPC diff render; HTML golden.
- **Unblocks**: W35, W36, W37.

## W35 — focus panel + reasoner integration; voice-summon

- **Goal**: Hotkey summons focus panel; voice query (from W11–W14)
  routes through reasoner; response renders with citations.
- **Files touched**:
  - `internal/hud/state.go` — focus transitions
  - `internal/hud/hotkey.go` + tests — macOS global shortcut
  - `internal/hud/static/{index.html,hud.css,hud.js}` — focus template
  - `internal/hud/reasoner.go` — daemon-side bridge from voice/CLI to HUD
- **Risk**: Depends on voice-comm W11–W14 landing. If voice slips, the
  hotkey-summon path is independently shippable; voice-summon is the
  rider.
- **Size**: ~600 LOC.
- **Test plan**: FSM ambient→focus→ambient; markdown render snapshot;
  citation chip rendering.
- **Unblocks**: W36 (info widgets in focus panel), W37 (operator-config
  hotkey).

## W36 — HUD info-feed widgets (weather, market, headlines, calendar-next)

- **Goal**: Ambient HUD's data rows come from real feeds (W31–W32) +
  gcal (PR #65) via the telemetry stream.
- **Files touched**:
  - `internal/daemon/telemetry.go` — populate `TelemetryFrame` from
    feeds synth output
  - `internal/hud/static/hud.js` — diff-render per field
  - `internal/hud/screencast.go` + tests — screen-recording auto-hide
- **Risk**: Telemetry payload growth — keep frames small (synth output,
  not raw items). Cap at 4KB per frame.
- **Size**: ~400 LOC.
- **Test plan**: telemetry-diff render; screen-recording auto-hide.
- **Unblocks**: W37 operator-config — surfaces are wired, config now
  changes their behavior.

## W37 — operator-config: hotkeys, accent color, source allowlist, blocklist words

- **Goal**: One config file (`~/.leah-state/hud.json` +
  `~/.leah-state/feeds.json`) owns all operator preferences. Live-reload
  on file change.
- **Files touched**:
  - `internal/hud/config.go` + tests — schema, defaults, live-reload
  - `internal/feeds/config.go` + tests — feeds source allowlist,
    location, tracked symbols, blocklist words
  - `cmd/leah/config.go` — `leah config show / edit` helpers
- **Risk**: Schema churn. Mitigation: schema version field; read-side
  migration. Config writes go through the daemon to avoid concurrent-edit
  corruption.
- **Size**: ~500 LOC.
- **Test plan**: config round-trip golden; live-reload race; schema
  version migration.
- **Unblocks**: post-MVP — operator can tune the surface they got.

## Wave dependencies (visual)

```
W31 (feeds: weather + cache)
  └── W32 (feeds: news + market + wiki)
        └── W33 (brief integration)
              └── (synth output proven on existing brief surface)
                    └── W36 (HUD info widgets) — also depends on W34
W34 (HUD skeleton + ambient)
  └── W35 (focus + reasoner + voice-summon) — also depends on voice W14
        └── W36 (HUD info widgets)
              └── W37 (operator-config)
```

## Cross-cutting concerns

- **No AI signatures** in any wave's commits or PRs (per `CLAUDE.md`).
- **Deletion default**: each wave's PR body answers "what got smaller?"
  Feeds layer collapses fetch+cache+synth duplication that would
  otherwise live per-surface (brief, HUD, voice, CLI).
- **Adversarial review**: every PR gets an independent reviewer-subagent
  immediately after `gh pr create`, per `CLAUDE.md`. No self-APPROVE.
- **Operator-attestation** is wired per wave; no surface acquires data
  the operator hasn't attested.
- **Token economy**: `gh` commands use `--json` allowlists; `make check`
  output compressed per `CLAUDE.md`.
