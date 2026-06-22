# Phase 2 dev loop

How to develop a Phase 2 feature with live runtime feedback — edit code, observe the running app, bug-fix, repeat.

## Quick start

```
make dev          # build daemon + app, wait for socket, tail logs
# ... edit code ...
make dev-stop     # kill daemon + quit app
make dev          # reboot with new build
```

## Typical agent loop

```
# 1. Boot
make dev

# 2. Trigger the focus panel
scripts/dev/inject-hotkey.sh space        # sends ⌥Space

# 3. Type a query
scripts/dev/inject-text.sh "what is leah"

# 4. Screenshot result
scripts/dev/screenshot.sh /tmp/leah-shot.png

# 5. Inspect logs
scripts/dev/tail-logs.sh --duration 5s

# 6. Send a raw IPC frame
scripts/dev/ipc-send.sh '{"kind":"ask","turn_id":"dev-1","seq":0,"payload":{"text":"hello"}}'

# 7. Edit, make dev-stop, make dev, repeat
```

## Scripts

| Script | Purpose |
|--------|---------|
| `screenshot.sh [PATH]` | Capture full screen (or `--window WINID`) to PATH; prints dims |
| `inject-hotkey.sh [KEY]` | Send ⌥Space (default) or esc/return/tab via System Events |
| `inject-text.sh "TEXT"` | Keystroke TEXT into focused window |
| `ipc-send.sh JSON` | Write one frame to the daemon socket, print responses until turn.end |
| `tail-logs.sh` | Stream unified app + daemon logs via `log stream`; `--file` tails `/tmp/leah-dev.log`; `--duration Ns` exits after N s |

## Permissions

Three macOS permission gates can block scripts:

**Accessibility** — required by `inject-hotkey.sh` and `inject-text.sh` to send keystrokes via System Events. Grant in System Settings → Privacy & Security → Accessibility → add Terminal / iTerm2 / Claude Code.

**Screen Recording** — required by `screenshot.sh`. Grant in System Settings → Privacy & Security → Screen Recording. If absent, the script exits non-zero; `dev-harness_test.sh` soft-skips rather than fails.

**Apple Events** — required by `dev-stop` (`osascript … quit app "Leah"`). Usually granted alongside Accessibility. If the dialog appears, click OK once.

## Files written by `make dev`

| Path | Content |
|------|---------|
| `/tmp/leah-dev-daemon.pid` | PID of the background daemon process |
| `/tmp/leah-dev.log` | Combined daemon stdout + `log stream` output |
| `~/Library/Caches/Leah/leah.sock` | Unix socket the daemon binds |
| `~/.leah-state-dev/` | Dev sandbox state directory |

## Troubleshooting

`socket did not appear` — daemon failed to start; check `/tmp/leah-dev.log`. Common cause: port conflict or missing `ANTHROPIC_API_KEY`.

`osascript: not allowed assistive access` — Accessibility permission not granted; see above.

`ipc-send.sh: socket not found` — daemon not running; `make dev` first.
