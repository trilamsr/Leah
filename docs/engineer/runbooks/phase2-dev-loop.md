# Phase 2 dev loop

Reference for iterative debugging of leah-daemon and the SwiftUI HUD during development.

## Tools

`scripts/dev/` contains three helpers: `lldb-attach.sh`, `diag-state.sh`, and `repl.sh`. The `scripts/smoke/` directory contains `diag-state.go`, which `diag-state.sh` delegates to.

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

## Bug-fix workflow with breakpoints

Reproduce the bug first. Run `make dev` to start the daemon, then use `inject-hotkey.sh` and `inject-text.sh` to drive the app into the failing state. Run `screenshot.sh` to capture the wrong UI state. Run `scripts/dev/diag-state.sh` to confirm what the daemon sees at that moment — the response payload carries `clients`, `conversation`, `memory_stats`, `pending_tts`, `daemon_uptime_s`, and `last_error`.

Once the wrong state is confirmed, attach the debugger. Run `scripts/dev/lldb-attach.sh leah-daemon` — this finds the running daemon PID and drops into an lldb session showing the current process status. Set a breakpoint at the suspicious function (`br set -n <function-name>`), then trigger the bug again via `inject-text.sh`. Step through the frames (`thread step-in`, `thread step-over`) to find the bad value, then patch.

Permission gotchas: Accessibility and Apple Events permissions are required for the inject scripts (grant in System Settings > Privacy & Security). Screen Recording permission is required for `screenshot.sh`. For lldb specifically: attaching requires either a SIP-compatible (debug) build or Developer Mode enabled. Check with `csrutil status` — if SIP is fully enabled, lldb can only attach to processes you built and signed with a debug entitlement. For a production-signed app you cannot attach lldb at all; build an unsigned dev daemon instead (`make build-daemon-dev` or equivalent) and run that locally.

## Scripts

| Script | Purpose |
|--------|---------|
| `screenshot.sh [PATH]` | Capture full screen (or `--window WINID`) to PATH; prints dims |
| `inject-hotkey.sh [KEY]` | Send ⌥Space (default) or esc/return/tab via System Events |
| `inject-text.sh "TEXT"` | Keystroke TEXT into focused window |
| `ipc-send.sh JSON` | Write one frame to the daemon socket, print responses until turn.end |
| `tail-logs.sh` | Stream unified app + daemon logs via `log stream`; `--file` tails `/tmp/leah-dev.log`; `--duration Ns` exits after N s |
| `diag-state.sh` | Query daemon for live diag.state — prints clients, conversation, memory_stats, uptime |
| `lldb-attach.sh PROC` | Find PID of PROC and drop into lldb session |

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

## Run Phase 2 e2e

`make phase2-smoke` runs `scripts/smoke/phase2-e2e.sh`, the single-script end-to-end gate for Phase 2. It boots a fresh `leah-daemon` against a sandbox state dir (`~/.leah-state-e2e/`) and asserts the eight runtime invariants the plan lists: widget.mount streamed for a widget-shaped ask, pin persistence across daemon restart, the 2-pin cap (third Pin returns a `notification.toast`), toast frame shape (`level=info` and non-empty `text`), no crash across a dark/light `AppleInterfaceStyle` toggle, fsnotify 200 ms debounce coalescing a 3-write burst into one `PinnedChanged`, BGE backend selection routing to `embeddings_bge_small_en_v1_5_384`, and `scripts/check-spec-parity.sh` exit 0.

The script is macOS-only — on Linux it prints a `skip:` line and exits 0 so the same CI workflow can host it without branching. Invariant 1 SKIPs without `ANTHROPIC_API_KEY`, invariant 7 SKIPs without `LEAH_EMBED_MODEL_PATH` (or `LEAH_MODEL_DIR`). On failure the script exits with the first failing step number and prints `STEP=N` to stderr; daemon log is at `/tmp/leah-phase2-e2e.log`. `make phase2-smoke-stop` force-cleans state if a run aborts.

## Troubleshooting

`socket did not appear` — daemon failed to start; check `/tmp/leah-dev.log`. Common cause: port conflict or missing `ANTHROPIC_API_KEY`.

`osascript: not allowed assistive access` — Accessibility permission not granted; see above.

`ipc-send.sh: socket not found` — daemon not running; `make dev` first.
