# Latency Instrumentation — Pillar A Histograms

**Date:** 2026-06-10
**Parent:** `docs/engineer/briefs/2026-06-10-ux-audit-cross-surface.md` (Pillar A, A1–A9)
**Status:** spec for implementation; ASPIRATIONAL targets become MEASURED targets on land.

---

## 1. Goal & non-goals

Goal: emit a Prometheus histogram for every Pillar A criterion (A1–A9) so the targets in the UX audit move from aspirational to verifiable; fixes can be regression-tested against p50/p95.

Non-goals: new alerting infra, new Prometheus library (use `internal/obs`), tuning the targets themselves, fixing the UX defects the measurements expose (each surfaces its own follow-up brief), and instrumenting non-Pillar-A latencies (those are deferred).

## 2. Per-criterion histogram spec

All names follow `leah_<surface>_<from>_to_<to>_seconds`. All histograms register through the daemon's shared `*obs.Registry` (the one already wired in `cmd/leah-daemon`). Buckets default to `prometheus.DefBuckets` analogue — `[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]` — except where a tighter band better resolves the target.

### A1 — Wake → first earcon ≤150ms

- **Metric:** `leah_voice_wake_to_earcon_seconds`
- **Labels:** `detector` (`energy`/`porcupine`/future), `backend` (`coreaudio_beep`/`afplay`).
- **Buckets:** `[0.025, 0.05, 0.1, 0.15, 0.2, 0.3, 0.5, 1]` — tight band around the 150ms target.
- **Start:** moment `wake.Detector.Detect` returns `(true, nil)` — `internal/voice/wake/wake.go:45` (the `return rms >= e.threshold, nil` line; line 32 is the func signature).
- **Stop:** earcon player returns from first-sample-out. Earcon doesn't exist yet; see §9 carve-out. Until earcon ships, emit a `leah_voice_wake_detected_total` counter at the same point so the timer is ready to land the moment the player exists.

### A2 — Utterance end → transcript ready ≤600ms p50, ≤1.2s p95

- **Metric:** `leah_voice_utterance_to_transcript_seconds`
- **Labels:** `backend` (`whisper_cli`/`openai_realtime`/`coreml`).
- **Buckets:** `[0.1, 0.25, 0.5, 0.75, 1, 1.5, 2, 3, 5, 10]`.
- **Start:** silence-detector trailing-frame boundary (the moment `listen.Listen` decides utterance is over) — `internal/voice/listen.go` `Listen` body, immediately before transcriber subprocess spawn.
- **Stop:** transcriber returns transcript string. Wrap the subprocess call.

### A3 — Transcript → intent classification ≤50ms

- **Metric:** `leah_intent_classify_seconds`
- **Labels:** `result` (`ask`/`ship`/`review`/`status`/`trip`/`unknown`).
- **Buckets:** `[0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.5]` — sub-ms band; the 50ms target is a regex floor, real value is two orders below.
- **Start:** entry to `intent.Classify` — `internal/intent/intent.go`.
- **Stop:** return.

### A4 — Intent → first audio of reply ≤700ms

- **Metric:** `leah_voice_intent_to_first_audio_seconds`
- **Labels:** `backend` (`kokoro`/`say`/`openai_tts`), `streaming` (`true`/`false`).
- **Buckets:** `[0.1, 0.25, 0.5, 0.7, 1, 1.5, 2, 3, 5]`.
- **Start:** entry to `startTurn` closure — `internal/voice/loop/loop.go:104` (immediately before the `context.WithCancel` at line 105). Note: intent classification is regex-fast (A3) and not on the loop hot path; treat A4 start as the moment a final transcript is handed to the reasoner/TTS chain.
- **Stop:** first PCM frame leaves the speaker subprocess. For `say` (`internal/voice/say.go`) the proxy is `say` exec start; for `kokoro` (`internal/voice/kokoro.go`) and `openai` (`internal/voice/openai.go`), the proxy is first audio chunk written to the player. Each backend needs one observation site; the existing `EmitSpeak` hook in `internal/voice/instrumentation.go:20` is the canonical seam — extend it to take a `firstAudio time.Time`.

### A5 — HUD widget update after state change ≤1s end-to-end

- **Metric:** `leah_hud_state_to_widget_seconds`
- **Labels:** `widget` (`weather`/`calendar`/`market`/`news`/`recommendations`/`focus`).
- **Buckets:** `[0.05, 0.1, 0.25, 0.5, 0.75, 1, 1.5, 2.5, 5, 10]`.
- **Start:** daemon-side state mutation that triggers a push. **NO `Broadcast` seam exists today** — `internal/hud/ipc.go` is client-side only (`Client.PollMetrics`). A5 measurement is BLOCKED on UX-audit blocker #1 (HUD widget SSE push + kill polling) landing first. Until then, emit a `leah_hud_widget_poll_total{widget,outcome}` counter at the existing client poll site so widget-count baseline data accrues.
- **Stop:** (once the SSE emitter ships under blocker #1) SSE write returns from the daemon. Measures daemon→wire, not wire→DOM; browser-side render time is a future companion metric (`leah_hud_widget_dom_paint_seconds`, deferred).

### A6 — CLI first byte stdout ≤200ms (excl. LLM)

- **Metric:** `leah_cli_dispatch_to_first_byte_seconds`
- **Labels:** `verb` (top-level verb), `llm` (`true`/`false`).
- **Buckets:** `[0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.3, 0.5, 1, 2.5]`.
- **Start:** entry to `runCommand` — `cmd/leah/main.go:50`.
- **Stop:** first `os.Stdout.Write` from the dispatched verb. Implementation: wrap `os.Stdout` with a `firstByteRecorder` injected in `runCommand` before dispatch; the recorder calls `Observe` on first write. CLI does not own a daemon Registry — write observations into the daemon by appending to `~/.leah/state/cli_latency.jsonl` (one line per invocation) and have the daemon tail-read on a 5s tick, observing into its registry. See A9 below for shared persistence path.

### A7 — CLI long-running progress within ≤500ms

- **Metric:** `leah_cli_dispatch_to_first_progress_seconds`
- **Labels:** `verb`.
- **Buckets:** `[0.05, 0.1, 0.25, 0.5, 0.75, 1, 2, 5]`.
- **Start:** same as A6 (dispatch entry).
- **Stop:** first progress signal — spinner first frame OR first streamed token OR first log line that the user can see as evidence of work. No canonical spinner exists today (UX-audit C4 blocker); implementation requires `internal/cliprogress` to be created (out of scope of this brief — flagged §9). Until then, emit only for verbs that already stream (`ask`, `ship`).

### A8 — Barge-in TTS cuts off ≤200ms

- **Metric:** `leah_voice_barge_in_cancel_seconds`
- **Labels:** `backend`.
- **Buckets:** `[0.01, 0.025, 0.05, 0.1, 0.15, 0.2, 0.3, 0.5, 1]`.
- **Start:** moment `cancelTurn()` is called — `internal/voice/loop/loop.go:89` (and the equivalent in `session/session.go:87`).
- **Stop:** the `Speak` goroutine returns (ctx-canceled). Add a `tCancelStart` capture inside `cancelTurn`; the speak goroutine reads it on exit and observes.

### A9 — `brew install` → first useful reply ≤5 min

- **Metric:** `leah_onboarding_install_to_first_reply_seconds`
- **Labels:** `outcome` (`useful`/`abandoned`).
- **Buckets:** `[60, 120, 180, 300, 600, 1800, 3600]` — minute-scale.
- **Start:** install-marker timestamp written by post-install hook (`brew` formula `post_install` block writes `~/.leah/state/install_at`).
- **Stop:** first successful verb that produced user-visible output — operationally, first audit-log entry with `BlastRadius >= 1` AND `Outcome == "success"` (per `internal/audit/audit.go:22` — field name is `Outcome`, common success value is `"success"`; verify exact enum at impl time).
- **Persistence:** see §3.

## 3. A9 special case — onboarding e2e timer

A9 spans process boundaries: install runs as `brew`, first reply runs as `leah <verb>`. CLI invocations are ephemeral; observations must persist.

**Decision: derive from existing audit log, not a new persistent timer.**

- `brew` post-install writes `~/.leah/state/install_at` (single line: RFC3339 timestamp). Already a documented Leah state-dir convention; cheap.
- Daemon, on startup AND on every audit-log append, checks: does `install_at` exist AND no `onboarding_first_reply_at` sibling file exist? If yes, scan audit.jsonl for the first row with `BlastRadius >= 1` AND `Outcome == "success"` after `install_at`. If found, observe `(found - install_at).Seconds()` into the histogram, then write `onboarding_first_reply_at` to inhibit re-observation.
- Code lives in `internal/onboarding/measure.go` (new file, ~60 LOC). One observation per machine lifetime.

Rationale: an explicit cross-process timer is more code and another piece of state to corrupt. Audit log already captures "useful" with the precision A9 needs (minutes, not ms). Derivation has the bonus property that re-instrumenting old installs works retroactively from the audit log.

## 4. Code placement

| # | Package | Function | Timer start (file:line) | Timer stop |
|---|---------|----------|-------------------------|------------|
| A1 | `internal/voice/wake` | `EnergyDetector.Detect` → earcon player | `wake.go:45` (return `rms >= e.threshold, nil`) | earcon `Play` returns (player TBD) |
| A2 | `internal/voice` | `Listen` | `listen.go` silence-boundary | transcriber subprocess returns |
| A3 | `internal/intent` | `Classify` | `intent.go` func entry | func return |
| A4 | `internal/voice/loop` | `Run` per-turn | `loop.go:104` (entry to `startTurn` closure, before `WithCancel` at :105) | first PCM frame in `voice.ChainTTS.Speak` |
| A5 | `internal/hud` (SSE emitter does not exist yet) | n/a until UX-blocker #1 lands | interim: `leah_hud_widget_poll_total` counter at client poll site | (when daemon SSE ships) Broadcast write returns |
| A6 | `cmd/leah` | `runCommand` | `main.go:50` entry | `firstByteRecorder.Write` first call |
| A7 | `cmd/leah` | per-verb | `main.go:50` entry | first progress signal (verb-specific) |
| A8 | `internal/voice/loop` + `session` | `cancelTurn` / barge-in | `loop.go:89` / `session.go:86` cancel-fn definition | `Speak` goroutine returns |
| A9 | `internal/onboarding` | `MaybeObserve` | `install_at` file mtime | first audit-log row with `BlastRadius >= 1` AND `Outcome == "success"` after install |

All call sites use the existing instrumentation seam pattern (`internal/voice/instrumentation.go:15` — `OnSpeak` callback in `Config`). For each criterion, add an `OnLatency func(seconds float64, labels map[string]string)` hook to the owning package's `Config`, default no-op, daemon wires it to `registry.Histogram(name, buckets).Observe`. Keeps `internal/intent`, `internal/voice/*`, `internal/hud` registry-free — no import cycle, library-shape preserved.

## 5. Dashboard recommendation

Single Grafana dashboard "Leah — Pillar A Latency". Markdown spec; JSON deferred until panel layout sees a real user.

- **Panel 1 — A1 wake-to-earcon** — heatmap `leah_voice_wake_to_earcon_seconds_bucket`, p50/p95 overlay, threshold line at 0.15s.
- **Panel 2 — A2 STT** — heatmap by `backend` label, p50/p95, threshold lines at 0.6 / 1.2s.
- **Panel 3 — A3 intent** — single-stat p99, threshold 0.05s.
- **Panel 4 — A4 first-audio** — heatmap by `backend × streaming`, threshold 0.7s. Streaming-off vs streaming-on regression should be visible.
- **Panel 5 — A5 HUD push** — heatmap by `widget`, threshold 1s.
- **Panel 6 — A6 CLI first byte** — heatmap by `verb`, threshold 0.2s; `llm=true` excluded.
- **Panel 7 — A7 CLI progress** — heatmap by `verb`, threshold 0.5s.
- **Panel 8 — A8 barge-in** — p99 single-stat, threshold 0.2s.
- **Panel 9 — A9 onboarding** — gauge: most recent observation in minutes. Single-machine, n=1 — not aggregated.
- **Panel 10 — SLO compliance** — table: criterion / p50 / p95 / target / pass-fail.

## 6. Verification plan

For each criterion, validate the metric fires AND the bucket placement is correct in a real local run:

- A1: stub earcon player, fake `Detect` return, sleep 80ms before earcon `Play` return → expect observation in `le="0.1"` bucket.
- A2: run `leah voice` against a pre-recorded WAV, observe → expect `le="2"` or below on whisper-cli laptop. Verify `backend` label.
- A3: `intent.Classify("ship the thing")` in a loop of 1000 → expect all observations in `le="0.005"` bucket; histogram sum / count ≈ 50µs.
- A4: voice session with deterministic `say` backend; observe `streaming="false"` band. Re-run with `kokoro`, observe `streaming="true"` band lower.
- A5: trigger state change via daemon API, watch SSE log for emit, observe → `le="0.5"` typical.
- A6: `time leah status` → expect first-byte ≪200ms. Run with `LEAH_FIRSTBYTE_DEBUG=1` to dump observation point.
- A7: `leah ask "hi"` — expect first-progress (token stream) <500ms.
- A8: voice session; pipe "stop" mid-reply; observe ctx-cancel-to-goroutine-exit duration.
- A9: touch `~/.leah/state/install_at` to 6 minutes ago, write an audit row, restart daemon, expect observation in `le="600"` bucket with `outcome="useful"`.

Scrape `localhost:<daemon_port>/metrics` after each run; grep for the histogram name and confirm `_bucket{le="..."} N`, `_sum N`, `_count N` lines all present.

## 7. TDD plan

One failing test per criterion lands first; impl turns it green. All live in `_test.go` adjacent to the timer call site (existing convention — see `internal/voice/instrumentation_test.go`).

- A1: `TestWakeToEarcon_ObservesLatency` — stub detector + earcon player with controlled clock, assert registry has 1 observation in expected bucket. Currently no earcon → test is `t.Skip("blocked on earcon player W?")` with TODO link until player ships.
- A2: `TestListen_ObservesTranscriptLatency` — fake transcriber that sleeps 250ms, assert `le="0.25"` bucket = 0, `le="0.5"` bucket = 1.
- A3: `TestClassify_ObservesLatency` — call 100×, assert count=100, sum<10ms.
- A4: `TestLoop_IntentToFirstAudioObservation` — stub TTS that records firstAudio at exec entry, fake reasoner with 200ms latency, assert observation in `le="0.25"`.
- A5: `TestBroadcast_ObservesEmitLatency` — assert non-zero observation per call.
- A6: `TestRunCommand_FirstByteObservation` — write a fake `os.Stdout` capture, assert observation.
- A7: `TestRunCommand_FirstProgressObservation` — gated on `internal/cliprogress` landing; skip with TODO until then.
- A8: `TestBargeIn_CancelToExitObservation` — stub TTS that respects ctx, trigger cancel, assert observation <50ms.
- A9: `TestOnboarding_MaybeObserve` — seed `install_at` and audit row, call `MaybeObserve`, assert observation + sentinel file written; second call no-ops.

Each test imports `internal/obs` directly, constructs a `Registry`, asserts via `registry.Snapshot` round-trip (existing pattern in `internal/obs/metrics.go:235`).

## 8. Carve-outs — Pillar A non-blockers today

- **A1 earcon player does not exist.** UX-audit blocker #2 ("voice earcons + lighter attestation") owns shipping it. This brief lands the histogram + a `t.Skip`'d test; landing the player flips the skip to a real assertion. WHY: instrumenting a non-existent code path is a code smell, but pre-wiring the histogram name + bucket choice avoids re-litigating them when the player lands.
- **A4 streaming TTS does not exist for all backends.** `kokoro` streams; `say` does not. The `streaming` label distinguishes them; the non-streaming target is naturally worse and the dashboard surfaces it. No carve-out needed — just honest labels.
- **A5 HUD push does not exist for widgets.** `widgets.js` polls (UX-audit blocker #1). The histogram fires on the SSE path that exists today (state + recommendations). The widget label values land empty for weather/market/news/calendar until polling dies. UX-audit blocker #1 owns the migration; this brief is unblocked because partial coverage is better than none.
- **A7 canonical CLI progress indicator does not exist.** UX-audit C4. Histogram observes for verbs that already stream tokens; remaining verbs skip with TODO until `internal/cliprogress` lands.
- **A9 brew formula post_install hook does not exist.** Trivial follow-up — one-line `File.write("~/.leah/state/install_at", Time.now.utc.iso8601)` in the formula. Brief assumes it; impl PR for A9 must include the formula edit.

## 9. Out of scope (this brief)

Earcon player, canonical CLI spinner, widget SSE migration, brew formula edit, Grafana JSON, browser-side DOM paint timing, alert rules, distributed tracing.

## 10. Acceptance

Brief is "implementable" when:
- every criterion has a histogram name, label set, bucket list, and a precise `file:line` start AND stop;
- every carve-out lists the blocking work and the test posture (skip vs assert);
- no new infra is required — only `internal/obs.Registry` calls.

Reviewer: confirm the OnLatency callback pattern doesn't create a sneak import cycle from `internal/voice/loop` → `internal/obs`; if it does, fall back to a `func(float64)` typed hook with labels resolved at the daemon wiring site.
