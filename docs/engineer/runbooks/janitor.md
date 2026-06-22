# worktree janitor runbook

## What it does

A launchd agent sweeps `.claude/worktrees/agent-*/` every 5 minutes and
prunes any worktree whose branch is either merged into `origin/main` or
no longer present on `origin`. Wave-9 V4 saw 354 MB and 129 stale index
locks accumulate before this gate landed.

## Install / uninstall

```
make install-janitor    # bootout unloads the prior version, bootstrap
make uninstall-janitor  # loads from the .plist — plist edits take effect
                        # on every re-install, and double-install is safe
```

Paths:
- plist: `~/Library/LaunchAgents/com.leah.worktree-janitor.plist`
- log: `~/.leah-state/janitor.log`
- script: `scripts/leah-worktree-janitor.sh` (run from repo root)

## Verify it's loaded

```
launchctl print gui/$(id -u)/com.leah.worktree-janitor | head -20
```

The `state = running` or `state = waiting` line means the agent is
registered. `StartInterval = 300` is the 5-minute tick.

## Manual sweep (no launchd)

```
bash scripts/leah-worktree-janitor.sh
```

Safe at any time. Exits 0 even when offline — fetch failure short-circuits
the sweep, so a captive-portal wifi does not produce a spurious log error.

## Tests

```
bash scripts/leah-worktree-janitor_test.sh
```

Covered regressions:
- T1 merged branch pruned
- T2 upstream-deleted branch pruned
- T3 live branch preserved (the destructive-regression guard)
- T4 non-`agent-*` worktrees ignored
- T5 offline run exits 0 and prunes nothing

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| log file empty | RunAtLoad is false; first tick is +5min | wait, or `bash scripts/leah-worktree-janitor.sh` once |
| `bootstrap` fails with "already exists" | prior install lingering | `make uninstall-janitor && make install-janitor` |
| worktree skipped "locked or busy" | another git op holding `.git/index.lock` | rerun after the op finishes; janitor retries every 5 min |
| nothing pruned despite merged PR | `git fetch origin` fails (offline / auth) | check `~/.leah-state/janitor.log`, restore network |
