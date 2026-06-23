# Phase 4 wiring checklist

Composition-root verification for v1.1. Each Phase 4 producer must be constructed and held in `cmd/leah-daemon/`. The sibling agent owns the wiring edits to `cmd/leah-daemon/main.go`; this file records the expected end-state so orphan-scan after wave close has a target.

Layout: one row per producer. `Constructor` is the canonical entry point; `Wires into` names the daemon file that should call it. A row is "wired" when the constructor is referenced from the listed file outside `_test.go`.

| Deliverable | Package | Constructor | Wires into |
|---|---|---|---|
| 1. Voice frontier | `internal/voice` | `voice.NewChain(backends...)` | `cmd/leah-daemon/ipc_voice.go` |
| 1. Voice frontier | `internal/voice` | `voice.NewTurnInstrumentation(reg, path)` | `cmd/leah-daemon/instrumentation.go` |
| 2. Multi-device sync | `internal/sync` | `sync.NewMTLSConfig(sharedKey)` | `cmd/leah-daemon/main.go` (peer transport bring-up) |
| 2. Multi-device sync | `internal/sync` | `sync.NewAttemptCounter()` | `cmd/leah-daemon/main.go` |
| 3. Recommend pass-2 | `internal/recommend` | `recommend.NewSignalDispatcher(engine, bus, ctxFn)` | `cmd/leah-daemon/recommend.go` |
| 3. Recommend pass-2 | `internal/recommend` | `recommend.NewSQLiteEngine(path, logger)` | `cmd/leah-daemon/recommend.go` |
| 3. Recommend pass-2 | `internal/recommend` | `recommend.NewVoiceAnnouncer(tts, attest)` | `cmd/leah-daemon/recommend.go` |
| 3. Recommend pass-2 | `internal/recommend` | `recommend.NewMacosMirrorSource(seam, opts)` | `cmd/leah-daemon/recommend.go` |
| 4. Vision | `internal/vision` | `vision.NewEngine()` | `cmd/leah-daemon/ipc_vision.go` |
| 4. Vision | `internal/vision` | `vision.NewMemConsent()` (swap to sqlstore in prod) | `cmd/leah-daemon/ipc_vision.go` |
| 5. A2A (inbound MCP + Leah-to-Leah) | `internal/a2a` | `a2a.NewServer(id, consent, budget, caps, handler)` | `cmd/leah-daemon/inbound.go` (alongside MCP server) |
| 5. A2A | `internal/a2a` | `a2a.NewClient(id, caps, name)` | `cmd/leah-daemon/inbound.go` |
| 5. A2A | `internal/a2a` | `a2a.NewConsentStore(db)` | `cmd/leah-daemon/inbound.go` |
| 6. Continuous attestation | `internal/attest` | `attest.NewVerifier(cfg)` | `cmd/leah-daemon/attestation.go` |
| 6. Continuous attestation | `internal/attestation` | `attestation.Load(path, scopes...)` | `cmd/leah-daemon/attestation.go` |
| 7. Plugin SDK | `internal/plugin` | `plugin.NewHost(cfg)` | `cmd/leah-daemon/main.go` |
| 7. Plugin SDK | `internal/plugin` | `plugin.NewSandbox(policy)` | called inside `plugin.Host`; no direct daemon ref needed |
| 7. Plugin SDK | `internal/plugin` | `plugin.NewQuotaMeter(clock)` | `cmd/leah-daemon/main.go` (or via `plugin.NewHost`) |
| 8. Privacy budget runtime | `internal/budget` | `budget.NewRuntime(db)` | `cmd/leah-daemon/main.go` (shared across a2a + recommend + vision) |
| 9. Watchdog supervisor | `internal/watchdog` | `watchdog.New()` | `cmd/leah-daemon/main.go` |
| 9. Watchdog supervisor | `internal/supervisor` | `supervisor.New(cfg)` | `cmd/leah-daemon/main.go` |

Cross-cutting wires (single shared instance flows through multiple producers):

- `budget.Runtime` is the single privacy-budget ledger; `a2a.NewServer` takes it as `b`, recommend's pass-2 dispatcher consults it before publishing, vision OCR debits per-frame. One `budget.NewRuntime(db)` call at daemon start, passed by reference.
- `supervisor.Supervisor` owns process lifecycle for daemon-managed children; `watchdog.Heartbeat` is the in-process liveness ping. Both live on the root `daemon` struct.
- `a2a.NewServer` requires the daemon's ed25519 signing key (same identity used for inbound MCP attestation). Key custody per §17.19 of predecessor spec.

Verification command (run after composition-root agent finishes):

```sh
for ctor in \
  voice.NewChain voice.NewTurnInstrumentation \
  sync.NewMTLSConfig sync.NewAttemptCounter \
  recommend.NewSignalDispatcher recommend.NewSQLiteEngine recommend.NewVoiceAnnouncer recommend.NewMacosMirrorSource \
  vision.NewEngine vision.NewMemConsent \
  a2a.NewServer a2a.NewClient a2a.NewConsentStore \
  attest.NewVerifier attestation.Load \
  plugin.NewHost plugin.NewQuotaMeter \
  budget.NewRuntime \
  watchdog.New supervisor.New ; do
  hits=$(grep -lE "\b${ctor}\(" cmd/leah-daemon/*.go 2>/dev/null | grep -v _test.go | wc -l | tr -d ' ')
  printf '%-44s %s\n' "$ctor" "${hits:-0} prod refs"
done
```

A row that reports `0 prod refs` is an orphaned Phase 4 producer — package merged but never wired. v3.3.0 shipped with 3 such gaps (TTS / KG / MCP) before the post-tag audit caught them; this checklist is the pre-tag guard for v1.1.
