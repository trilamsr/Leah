# Leah

Personal AI chief-of-staff. MVP-5 scope: dispatches GitHub PRs through regatta with independent reviewer + cost ceiling + JSONL audit.

See `docs/specs/` for full design; `docs/plans/2026-06-09-leah-mvp5.md` for the MVP-5 plan.

## Build

    go build -o leah ./cmd/leah

## Setup

1. Set environment:

       export ANTHROPIC_API_KEY=sk-ant-...
       export LEAH_PUSHOVER_USER=...           # optional, for phone push
       export LEAH_PUSHOVER_TOKEN=...
       export LEAH_HEALTHCHECK_URL=https://hc-ping.com/<uuid>  # optional
       export LEAH_BUDGET_DOLLARS=5             # per-process ceiling, default 5

2. Ensure `gh` CLI is authenticated:

       gh auth status

3. Ensure `regatta` CLI is in PATH:

       regatta --version

4. (Optional) Install launchd plist:

       cp scripts/leah.plist ~/Library/LaunchAgents/com.tri.leah.plist
       launchctl load ~/Library/LaunchAgents/com.tri.leah.plist

## Run

    leah ask "what does regatta currently support"
    leah ship "fix the prwatch retry-budget bug"
    leah review 1112
    leah status
