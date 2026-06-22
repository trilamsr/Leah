# Phase 1 end-to-end smoke

Validates the daemon ↔ LLM ↔ IPC path end-to-end: boots `leah-daemon`, sends
an `ask` frame over the Unix socket, and asserts `prose.delta` + `turn.end`
frames come back.

## Prerequisites

- Go toolchain in PATH
- `ANTHROPIC_API_KEY` set to a valid key

## Run

```sh
make smoke
# or directly:
bash scripts/smoke/phase1-e2e.sh
```

## Without an API key

The script exits 0 and prints:

```
SKIP: ANTHROPIC_API_KEY not set, partial smoke only
daemon build ok
```

This confirms the daemon compiles but skips the live IPC assertion.

## Pass output

```
phase1 e2e ok
```

## Fail modes

| Symptom | Cause |
|---|---|
| `socket did not appear` | Daemon failed to start — check stderr for build or init errors |
| `dial: …` | Socket vanished between bind and connect — rerun |
| `read: EOF` | Daemon closed the connection before sending `turn.end` |
| `missing frames` | `prose.delta` or `turn.end` never arrived — check API key and model access |
| `ANTHROPIC_API_KEY required` | Key unset when calling `phase1-e2e.sh` directly with a fake key |

## Files

| Path | Role |
|---|---|
| `scripts/smoke/phase1-e2e.sh` | Runner: starts daemon, waits for socket, invokes IPC client |
| `scripts/smoke/ipc-ping.go` | Go client compiled at runtime via `go run` |
| `scripts/smoke/phase1-e2e_test.sh` | Test harness: invokes runner, asserts `phase1 e2e ok` |
