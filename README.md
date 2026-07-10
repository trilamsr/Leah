# Leah

Personal chief-of-staff for macOS — ask questions, ship code, run adapters — offline-first, from your menubar. Single-operator macOS native app + CLI + always-on daemon. Phase 3 ship — v3.3.0; see `CHANGELOG.md` for the latest delta.

- Architecture: `ARCHITECTURE.md`
- Specs: `docs/specs/` (design rationale) + `docs/superpowers/designs/` (UI design)
- House rules: see [House rules](#house-rules) below
- Phase plans: `docs/superpowers/plans/2026-06-22-leah-macos-native-phase{2,3,4}.md`
- Work tracker: [Linear — Leah](https://linear.app/themaydow/project/leah-a8d553e8cc88)

## What runs where

| You interact with | It is | Runs |
|---|---|---|
| `leah` | CLI | on-demand |
| Menubar app | Native macOS app | always |
| `leah-daemon` | Background service | always |
| HUD panel (⌥Space) | Ask/answer overlay | on-demand |

## Install

```sh
brew tap trilamsr/leah && brew install leah && leah init
```

`leah init` walks the first-launch wizard: prompts for your API key, installs the launchd daemon plist, and offers to connect Gmail / Google Calendar.

| Credential | Where to get |
|---|---|
| `ANTHROPIC_API_KEY` | your API provider dashboard |
| `LEAH_ELEVENLABS_API_KEY` | https://elevenlabs.io → API |
| `LEAH_PUSHOVER_*` | https://pushover.net → register + app token |
| `LEAH_HEALTHCHECK_URL` | https://healthchecks.io → run `scripts/healthcheck-setup.sh` |

## What Leah does now (Phase 3, v3.3.0)

Closed-loop core, observe → remember → decide → act, with a native macOS HUD + voice on top. Highlights:

- **Native macOS HUD** — SwiftUI app (`app/Leah/`) with NSPanel hotkey, NSStatusItem, Settings panes, widget tile registry. AF_UNIX socket transport between daemon and HUD via length-prefixed JSON frames (`internal/platform/ipc/frame.go`).
- **Voice** — Wake-word (`wake-leah.mlmodel` + VAD gate + per-app suppression, opt-in), push-to-talk (Fn / right-⌘), TTS subsystem (ElevenLabs Flash v2.5 cloud primary, Apple Ava Premium fallback) with daemon-side privacy classifier. §17.17.
- **CLI surface** — ~50 subcommands. Daily drivers: `ask`, `ship`, `review`, `call`, `brief`, `find`, `recall`, `connect <integration>`, `ctx`, `status`, `cost`, `retro`, `self-build`, `self-build-status`, `news`, `paper`, `quote`, `watch`, `open`, `inbound`, `purge`. `leah` with no args prints the full list.
- **Memory** — `contact`, `project`, `decision`, `mistake`. SQLite at `~/.leah-state/memory.db` (`internal/memory/`). Typed-attestation gate on `leah purge` CLI; Touch ID gate on Settings → Memory → Purge in the native app per §17.13.
- **Context manager** — `leah ctx new/switch/show/history/list`; single-active-context; per-switch audit row.
- **Self-build dispatcher** — `leah self-build "<intent>"` files a `[SELF-BUILD]` issue against `trilamsr/Leah` (repo hard-locked); regatta picks up → PR → `leah review` independent subagent verdict → operator merges. Automerge banned on self-build.
- **Independent reviewer** — `leah review <repo> <pr#>` runs an independent reviewer subagent with a separate prompt + model (`LEAH_REVIEWER_MODEL`); validates agent-id against the canonical allowlist.
- **JARVIS dashboard (daemon)** — `leah-daemon --dashboard 127.0.0.1:8080` serves `/dashboard` HTML + `/api/state` JSON. Loopback-only. Distinct from the §4.7 SwiftUI dashboard inside the app.
- **§4.7 dashboard surface (app)** — `app/Leah/Sources/LeahUI/Dashboard/` reuses Phase 2 widget adapters (memory + agenda + briefs + news + knowledge).
- **Push-source substrate** — `internal/platform/macos/{mail,contacts,focus,activeapp}/push.go` plus knowledge/memory deltas fan out to HUD via IPC `push.*` frames.
- **Knowledge graph** — KG-backed citations join the answer-engine streaming path (`internal/thinking/knowledge/` → source repo file/line OR Linear issue ID).
- **MCP publish** — `internal/platform/mcp/server.go` publishes Leah's tools to peer agents read-only, gated behind `LEAH_MCP_PUBLISH=1`.
- **Eval pipeline** — `internal/platform/eval/` runs the canonical trace set on pre-commit + nightly; delta table persisted by `internal/platform/eval/store.go`.
- **Bandit recommender** — Beta-posteriors wired into the `internal/thinking/recommend/` ranker behind `LEAH_RECOMMEND_BANDIT=1`.
- **Sparkle updates** — auto-appcast generation, EdDSA verify on install, Settings → Advanced → "Use rollback channel for updates". Key custody runbook: `docs/runbooks/sparkle-key-custody.md`.
- **Daemon weekly tick** — Sunday-9am cron fires resolver back-fill, pattern detect → `skill-candidates.md`, retro generate → `retro-YYYY-WW.md`, operatormodel profile rebuild.
- **Operator model** — `operatormodel.UpdateProfile` rebuilds time-of-day / cadence / context-transition signals from last 30 days; `Recommend()` ranks candidates.
- **Observability** — `internal/platform/telemetry` slog daily-rotated JSONL logs, in-process metrics, `SafeGo`/`SafeRun` panic-recovery into `~/.leah-state/panics/`.
- **Adapters shipped** — Gmail, Google Calendar, Discord, Maps, Flights, iMessage, FaceTime, TMDB. First-launch auth via `leah connect <integration>` (browser OAuth device-code default; MCP fallback when integration is MCP-only).
- **Backup** — `restic` to local USB + Backblaze B2; `leah backup` + restore drills.

State lives in `$LEAH_STATE_DIR` (default `~/.leah-state/`): `audit.jsonl`, `memory.db`, `panics/`, `retro-*.md`, weekly tick outputs.

## Daemon install (launchd)

```sh
install -m 0755 leah-daemon /usr/local/bin/leah-daemon
cp scripts/leah.plist ~/Library/LaunchAgents/com.leah.daemon.plist
# Edit plist EnvironmentVariables to inject LEAH_HEALTHCHECK_URL + LEAH_STATE_DIR
launchctl load ~/Library/LaunchAgents/com.leah.daemon.plist
```

Stop: `launchctl unload ~/Library/LaunchAgents/com.leah.daemon.plist`. Logs at `/tmp/leah.stdout.log` + `/tmp/leah.stderr.log`.

## Development

Manual build from source (contributors / no-brew hosts):

```sh
git clone https://github.com/trilamsr/Leah ~/code/leah && cd ~/code/leah

# 1. Build CLI + daemon + HUD app
go build -o leah ./cmd/leah
go build -o leah-daemon ./cmd/leah-daemon
make app-build

# 2. Install required CLIs
brew install gh
gh auth login

# 3. Configure env vars in ~/.zshrc
export ANTHROPIC_API_KEY=sk-ant-...                # required
export LEAH_BUDGET_DOLLARS=5                       # optional, default $5
export LEAH_PROMPT_DIR=$HOME/code/leah/prompts     # required if leah is on $PATH
export LEAH_STATE_DIR=$HOME/.leah-state            # optional, default ~/.leah-state
export LEAH_PUSHOVER_USER=...                      # optional, phone push
export LEAH_PUSHOVER_TOKEN=...
export LEAH_HEALTHCHECK_URL=https://hc-ping.com/<uuid>  # optional, daemon heartbeat
# Phase 3 — voice/TTS:
export LEAH_ELEVENLABS_API_KEY=...                 # optional, cloud TTS primary
# Phase 3 — features off by default:
export LEAH_MCP_PUBLISH=0                          # 1 enables peer-agent MCP publish
export LEAH_RECOMMEND_BANDIT=0                     # 1 enables bandit recommender

source ~/.zshrc

# 4. Verify
leah version
leah                 # prints usage
leah status          # last 20 audit rows
./leah-daemon        # foreground; or install via launchd (see above)
```

Dev loop:

```sh
./scripts/check.sh                  # build + test + vet + lint
go test ./...
golangci-lint run ./...
make smoke                          # phase 1 e2e
make phase2-smoke                   # phase 2 e2e
bash scripts/dev/phase3-smoke.sh    # phase 3 e2e (ten invariants)
make dev                            # build daemon + app, wait for socket, tail logs
make dev-stop                       # kill daemon + quit app
```

Sign + notarize for distribution: `docs/runbooks/signing-and-notarization.md`.

Integration tests (require `ANTHROPIC_API_KEY`):

```sh
go test -tags integration ./internal/thinking/reasoner/ -v
go test -tags integration ./internal/thinking/reviewer/ -v
```

## What's NOT in scope

Deferred to Phase 4+ or explicitly deferred forever.

- Multi-user / SaaS — never without explicit re-evaluation.
- Autonomous money movement, autonomous merge — never without explicit re-evaluation.
- Workspace dimension (multi-context isolation) — when 2nd workspace needed.
- Cedar policy engine + bifurcated SQLCipher backup — deferred for personal use.
- Multi-device tokens + iOS Shortcut bridge — when iPhone-secondary milestone reached.

## Operator runbooks

Task-specific operator docs live under [`docs/operator/`](docs/operator/):

- [`install-brew.md`](docs/operator/install-brew.md) — Homebrew tap + formula install path
- [`release-runbook.md`](docs/operator/release-runbook.md) — tag → sign → notarize → Sparkle appcast
- [`reproducible-build.md`](docs/operator/reproducible-build.md) — deterministic build verification

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `ANTHROPIC_API_KEY not set` | env var missing | `export ANTHROPIC_API_KEY=...` then `source` rc |
| `read system prompt: no such file` | `leah` on `$PATH` w/o `LEAH_PROMPT_DIR` | set absolute `LEAH_PROMPT_DIR` |
| `gh: not authenticated` | gh login expired | `gh auth login` |
| `regatta: command not found` | regatta CLI not on PATH | install regatta + verify with `regatta --version` |
| `budget exceeded: ...` | per-process cap hit | raise `LEAH_BUDGET_DOLLARS` OR investigate why cost spiked |
| `reviewer agent-id ... does not match canonical allowlist` | subagent returned wrong-shape ID | re-run; if persistent, file issue |
| `socket did not appear` (`make dev`) | daemon failed to start | check `/tmp/leah-dev.log`; common cause: port conflict or missing `ANTHROPIC_API_KEY` |
| `osascript: not allowed assistive access` | inject-* scripts need Accessibility | System Settings → Privacy & Security → Accessibility → add Terminal |
| `pushover credentials not set` (on `leah ship`) | `LEAH_PUSHOVER_*` missing | set them OR ignore — desktop push still works |

## House rules

- Decision priority: UX > performance > long-term benefits. Default simpler; three similar lines beat a premature abstraction.
- Single-operator forever. Reject any feature that only pays off multi-user.
- Blast radius shapes friction. Higher-rank actions require more gate: notify → approve → 2FA → refuse.
- Adopt before build. Check if `gh`, `regatta`, or another CLI already does it before adding a package.
- Audit everything; redact cloud crossings. Every operator-facing action lands one JSONL row.
- Cost is observable. Every LLM/TTS/embedding call routes through `budget.Charge`.
- Root cause only; deletion default (every PR answers "what got smaller?"); drop ceremony.
- Comments: WHY not WHAT. Default to no comment. Test/Fuzz/Benchmark godocs 1 line max.
- TDD: failing test first; capture failing output; then impl; then green.
- No AI signatures anywhere (no Co-Authored-By, no "Generated with").

## License

Private; not licensed for external use. Personal-use only.
