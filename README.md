# Leah

Personal AI chief-of-staff. MVP-5: 5 CLI commands dispatching GitHub PRs through regatta with independent reviewer + per-process cost ceiling + JSONL audit.

- Specs: `docs/specs/`
- MVP-5 plan: `docs/plans/2026-06-09-leah-mvp5.md`
- Phase X deferred items: `docs/specs/2026-06-09-leah-phase-x-multi-operator-roadmap.md`

## First-time install (Mac)

### 1. Build the binary

    cd ~/code/leah   # or wherever you cloned
    go build -o leah ./cmd/leah

Optional — install to `$PATH`:

    install -m 0755 leah /usr/local/bin/leah

If installed to `$PATH`, also set absolute prompt dirs (see env vars below).

### 2. Install required CLIs

    # gh — GitHub CLI
    brew install gh
    gh auth login

    # regatta — operator's own PR orchestrator
    # (assumes you cloned regatta separately; cd into it and `make install` or your usual)
    regatta --version

### 3. Configure env vars

Add to `~/.zshrc` or `~/.bashrc`:

    # required — Reasoner backend
    export ANTHROPIC_API_KEY=sk-ant-...

    # optional — per-process $ ceiling (default $5)
    export LEAH_BUDGET_DOLLARS=5

    # optional — model selection (defaults below)
    export LEAH_MODEL=claude-sonnet-4-6           # Reasoner (ask + ship draft)
    export LEAH_REVIEWER_MODEL=claude-sonnet-4-6  # independent reviewer subagent

    # if leah binary is on $PATH, point to repo prompts:
    export LEAH_PROMPT_DIR=$HOME/code/leah/prompts
    export LEAH_REVIEWER_PROMPT_DIR=$HOME/code/leah/reviewer-prompts

    # optional — state dir (default ~/.leah-state)
    export LEAH_STATE_DIR=$HOME/.leah-state

    # optional — phone push notifications
    export LEAH_PUSHOVER_USER=...     # sign up at https://pushover.net
    export LEAH_PUSHOVER_TOKEN=...    # create application token

    # optional — daemon heartbeat (when watcher runs > 5min)
    export LEAH_HEALTHCHECK_URL=https://hc-ping.com/<uuid>
    # one-shot setup: ./scripts/healthcheck-setup.sh

`source ~/.zshrc` to load.

### 4. Obtain credentials

| Env var | Where to get |
|---|---|
| ANTHROPIC_API_KEY | https://console.anthropic.com → API Keys |
| LEAH_PUSHOVER_USER + TOKEN | https://pushover.net → register + create app |
| LEAH_HEALTHCHECK_URL | https://healthchecks.io → free account + run `./scripts/healthcheck-setup.sh` |

### 5. Verify

    leah version             # prints 0.0.1-mvp5
    leah                     # prints usage
    leah status              # prints "no activity" first time

## Commands

### `leah ask "<query>"`

Direct query to Reasoner. BR=0 (read-only). Charges per-call against budget.

    leah ask "what regatta CLI subcommands exist"

Output: Reasoner answer to stdout. One audit row written.

### `leah ship <repo> "<intent>"`

Files a regatta issue + starts watcher loop (polls every 30s up to 60min OR until agent state terminal). BR=3. On merge/escalation/failure → desktop notification.

    leah ship trilam/regatta "fix the prwatch retry-budget bug"
    leah ship trilam/leah "add LEAH_MODEL_HAIKU env for triage tier"

Output:
- URL of newly-filed issue
- Then watcher silently polls; pings healthchecks.io URL each poll (if configured)
- On agent terminal state: desktop push via macOS notification + (if Pushover configured) phone push
- Audit row written immediately + watcher updates on terminal state

### `leah review <repo> <pr#>`

Spawns independent reviewer subagent against a real PR. Reads diff via `gh pr diff`, dispatches subagent with `reviewer-prompts/independent-reviewer.md`, parses verdict, validates canonical agent-id shape per regatta allowlist.

    leah review trilam/regatta 1112

Output:
- Subagent review body to stdout
- `Verdict: APPROVE|REVISE|BLOCK  Agent-id: <id>` summary line
- Audit row with verdict + agent-id

Does NOT post review to PR — operator reads + decides.

### `leah status [--json]`

Last 20 audit entries. Default = table. `--json` = pretty array.

    leah status
    leah status --json | jq '.[] | select(.outcome=="failed")'

### `leah version`

Prints `0.0.1-mvp5`.

## State + audit

All state lives in `$LEAH_STATE_DIR` (default `~/.leah-state/`):

- `audit.jsonl` — every action one line; append-only

Inspect:

    cat ~/.leah-state/audit.jsonl | jq .
    leah status                                  # last 20 human-readable
    leah status --json                           # all readable, JSON

## Backup

Personal-use Phase 1 backup: `restic` to local USB + offsite (Backblaze B2).

    # one-time
    brew install restic
    restic -r /Volumes/USB-drive/leah-backup init
    restic -r b2:leah-offsite:state init   # B2 bucket

    # cron daily
    0 3 * * * restic -r /Volumes/USB-drive/leah-backup backup ~/.leah-state ~/code/leah/prompts ~/code/leah/reviewer-prompts
    15 3 * * * restic -r b2:leah-offsite:state backup ~/.leah-state

Restore drill quarterly:

    mkdir /tmp/leah-restore && cd /tmp/leah-restore
    restic -r /Volumes/USB-drive/leah-backup restore latest --target .
    ls ~/.leah-state

(Phase X spec covers Litestream + SQLCipher bifurcation if multi-Mac sync needed.)

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `ANTHROPIC_API_KEY not set` | env var missing | `export ANTHROPIC_API_KEY=...` then `source` rc |
| `read system prompt: no such file` | leah on `$PATH` w/o LEAH_PROMPT_DIR | set absolute `LEAH_PROMPT_DIR` |
| `gh: not authenticated` | gh login expired | `gh auth login` |
| `regatta: command not found` | regatta CLI not on PATH | install regatta + verify with `regatta --version` |
| `budget exceeded: spent=$X attempted=$Y ceiling=$Z` | per-process cap hit | raise `LEAH_BUDGET_DOLLARS` OR investigate why cost spiked |
| `reviewer agent-id ... does not match canonical allowlist` | subagent returned wrong-shape ID | re-run; if persistent, file issue |
| `pushover credentials not set` (on `leah ship`) | LEAH_PUSHOVER_USER/TOKEN missing | set them OR ignore — desktop push still works |
| watcher exits after 60min with no notification | regatta agent didn't reach terminal state | check `regatta agents list --json` directly |
| `osascript: ...` errors | macOS notification permission denied | System Settings → Notifications → Terminal (or wherever leah runs) → Allow |

## Always-on daemon

`leah-daemon` polls regatta state every 30s + notifies on terminal agent transitions (`merged` / `escalated` / `failed` / `stuck` / `killed`). Runs independent of CLI commands. Cold-start seeds state without notifying (avoids restart-flood); heartbeats `LEAH_HEALTHCHECK_URL` per tick (skipped silently if unset).

### Run manually

    go build -o leah-daemon ./cmd/leah-daemon
    ./leah-daemon                              # foreground, 30s poll
    LEAH_DAEMON_POLL_SECONDS=10 ./leah-daemon  # 10s poll

### launchd install (macOS)

    install -m 0755 leah-daemon /usr/local/bin/leah-daemon
    cp scripts/leah.plist ~/Library/LaunchAgents/com.tri.leah.plist
    # Edit plist EnvironmentVariables to inject LEAH_HEALTHCHECK_URL + LEAH_STATE_DIR
    launchctl load ~/Library/LaunchAgents/com.tri.leah.plist

Stop with `launchctl unload ~/Library/LaunchAgents/com.tri.leah.plist`. Logs at `/tmp/leah.stdout.log` + `/tmp/leah.stderr.log`.

## What's NOT in MVP-5

These exist in spec but deferred. See `docs/specs/2026-06-09-leah-phase-x-multi-operator-roadmap.md` for full list + reopen triggers.

- Memory layer (contacts, threads, projects, decisions) — comes in M2
- Voice (push-to-talk + TTS) — M4
- Email + calendar adapters (Gmail Pub/Sub + gcal sync) — M5
- Tier 1 self-improvement (Sunday review, A/B harness) — M3
- Workspace dimension (multi-context isolation) — when 2nd workspace needed
- Cedar policy engine + bifurcated SQLCipher backup — deferred for personal use
- Multi-device tokens + iOS Shortcut bridge — when iPhone-secondary milestone reached

## Architecture

11 internal packages:

- `internal/audit` — JSONL append-only logger
- `internal/budget` — per-process $ ceiling (atomic Charge)
- `internal/reasoner` — Anthropic SDK wrapper + Client interface
- `internal/intent` — regex intent classifier (ask/ship/review/status)
- `internal/ghclient` — gh CLI wrapper (CreateIssue / ViewPR / ListPRsForBranch)
- `internal/regattaclient` — `regatta agents list --json` wrapper
- `internal/notify` — macOS osascript + Pushover HTTP
- `internal/dispatcher` — Ask / Ship / Status orchestration
- `internal/reviewer` — independent reviewer subagent + PostReview canonical-ID gate
- `internal/watchdog` — healthchecks.io heartbeat
- `internal/daemonloop` — always-on regatta-state poll + terminal-transition notifier

CLI surface: `cmd/leah/main.go`. Daemon: `cmd/leah-daemon/main.go`.

Prompts: `prompts/` (Reasoner) + `reviewer-prompts/` (subagent — separate dir per independence-principle).

## Development

    ./scripts/check.sh   # build + test + vet + lint
    go test ./...
    golangci-lint run ./...

Integration tests (require ANTHROPIC_API_KEY):

    go test -tags integration ./internal/reasoner/ -v
    go test -tags integration ./internal/reviewer/ -v

CLAUDE.md at repo root has operator-rules for any agent (main + subagents).

## License

Private; not licensed for external use. Personal-use only.
