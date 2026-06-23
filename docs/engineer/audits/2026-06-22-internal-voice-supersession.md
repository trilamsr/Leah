# internal/voice supersession audit

Scope: does `internal/tts/` (Phase 3 Task 1) supersede `internal/voice/`, and what — if any — of `internal/voice/` can now be deleted? Baseline: origin/main @ 3fc41f1.

## Verdict

PATH B. `internal/voice/` is NOT entirely superseded. The two packages cover different shapes:

- `internal/tts/` — §17.17 contract: `Provider.Speak(ctx, text, voice) (AudioStream, error)` + privacy classifier. Per-utterance routing between ElevenLabs (cloud, Phase 3 Task 2) and Apple Ava (local, Phase 3 Task 3). HUD-bound audio bytes.
- `internal/voice/` — legacy TTS chain (`say(1)` / Kokoro / OpenAI), the `Listen` shell wrapper, voice loop instrumentation (`TurnInstrumentation`, `TurnTimer`), and the voice-loop substrate subpackages (`loop`, `listener`, `session`, `wake`, `intents`).

The Phase 3 plan calls this out explicitly: docs/superpowers/plans/2026-06-22-leah-macos-native-phase3.md L82 — "We add `internal/tts/` as the §17.17-shaped surface and leave `internal/voice/` for legacy `say(1)` / Kokoro fallbacks."

A blanket delete PR is wrong. A targeted prune is possible (see "What got smaller" below).

## Live non-test callers of `internal/voice` (parent package)

| Caller | Symbols used | Migration path |
|---|---|---|
| `cmd/leah/listen.go:45` | `voice.Listen`, `voice.ShellExec`, `voice.ListenConfig` | Keep. Shell-driven `whisper`/`ffmpeg` transcription — orthogonal to TTS, no §17.17 replacement. |
| `cmd/leah-daemon/comms.go:55` | `voice.NewTTS`, `voice.Synthesizer` | Keep until Discord notify path migrates to `tts.Provider` (out of Phase 3 scope; ElevenLabs adapter is HUD-only, not bytes-to-Discord). |
| `cmd/leah-daemon/instrumentation.go:41,46,85` | `voice.ChainTTS`, `voice.Bind`, `voice.SelfChecker` | Keep until chain itself is removed. `voice.Bind` registers chain metrics; `SelfChecker` powers `/health` voice probe. |
| `cmd/leah-daemon/main.go:109,110` | `voice.ChainTTS`, `voice.NewTTS` | Keep. Chain is the legacy-side TTS fallback for non-§17.17 callers (notify, recommend). |
| `internal/notify/discord.go:20,33` | `voice.Synthesizer`, `voice.ErrSynthesizeUnsupported` | Keep. Discord bot delivers MP3 attachments via the chain's `Synthesize([]byte)` shape — `tts.Provider.Speak` returns a stream and is HUD-bound, wrong shape. |
| `internal/notify/voice.go:13,19` | `voice.TTS`, `voice.NewTTS` | Keep. Server-side push notify announcer — not HUD-bound. |
| `internal/recommend/voice.go:31,38` | `voice.TTS` | Keep. Bandit-recommendation announcer; same shape as notify. |

All seven callers are load-bearing today. None are dead.

## Live subpackages

| Subpackage | Non-test external callers | Verdict |
|---|---|---|
| `internal/voice/listener/` | `cmd/leah-daemon/instrumentation.go:31` (`listener.RegisterMetrics`) | LIVE — metric registration. |
| `internal/voice/loop/` | `cmd/leah-daemon/instrumentation.go:32` (`voiceloop.RegisterMetrics`) | LIVE — metric registration only. Substrate not yet wired into daemon main. |
| `internal/voice/session/` | None outside `internal/voice/` | UNWIRED substrate. Imports `voice.TurnTimer` / `voice.TurnInstrumentation` — those exist only to feed this. |
| `internal/voice/wake/` | None outside `internal/voice/` | UNWIRED substrate. |
| `internal/voice/intents/` | None outside `internal/voice/` | UNWIRED substrate. |

`session`, `wake`, `intents` have zero non-test importers anywhere in the repo. They are scaffolding for the voice loop that Phase 3 Task 4 (`tts.speak` IPC) does NOT consume — Phase 3 wires TTS through the daemon IPC handler, not through `voice/session.Run`.

These three subpackages are candidates for either (a) a parallel delete PR if Phase 3 confirms they are no longer the integration target, or (b) deferred to the wave that builds the daemon-side voice-loop driver. Decision belongs to the operator / Phase 3 lead, not this audit.

## Symbols in `internal/voice/` parent — dead vs live

DEAD outside the package (used only by `_test.go` or by the unwired `session`/`wake`/`intents` subpackages, which themselves have zero external callers):

- `voice.TurnInstrumentation`, `voice.NewTurnInstrumentation` — used by `voice/session/session.go:68` only.
- `voice.TurnTimer`, `voice.NewTurnTimer`, `TurnTimer.Mark*` / `Finish` / `BargeIn` — used by `voice/session/session.go:96` only.
- `voice.BargeInCancelBuckets` — exported only for tests.
- `voice.EmitSpeak` — no non-test callers found.
- `voice.StreamToolUseSuppressedHook` — no non-test callers found.

LIVE (named caller exists outside the parent package):

- `voice.TTS` (interface) — notify, recommend.
- `voice.Synthesizer` (interface) — notify/discord, daemon/comms.
- `voice.ErrSynthesizeUnsupported` — notify/discord.
- `voice.NewTTS` — daemon/main, daemon/comms, notify/voice.
- `voice.ChainTTS` — daemon/main, daemon/instrumentation.
- `voice.Bind` — daemon/instrumentation.
- `voice.SelfChecker` — daemon/instrumentation.
- `voice.Listen`, `voice.ShellExec`, `voice.ListenConfig` — cmd/leah/listen.

## What got smaller (deletion-default scope)

A minimal targeted prune PR — independent of the substrate question — could delete the five DEAD parent-package symbols above plus the file or subset of `instrumentation.go` that owns them. Rough size:

- `internal/voice/instrumentation.go` L71–268 (TurnInstrumentation + TurnTimer + BargeInCancelBuckets + EmitSpeak + StreamToolUseSuppressedHook) ≈ 200 LOC.
- Cascades: drop `internal/voice/session/`, `internal/voice/wake/`, `internal/voice/intents/`, and the bulk of `internal/voice/loop/` (loop only exists to drive session) — ≈ 2.5k LOC across files + tests.

Total deletable if substrate is confirmed unused: ~2.7k LOC. PR is file-disjoint from Phase 3 Task 2/3/4 work (none of them touch `voice/session`, `voice/wake`, `voice/intents`).

Recommend: file as a Phase 3 follow-up issue, blocked on confirmation from Phase 3 owner that the daemon-side voice loop will not be assembled out of `voice/session` + `voice/wake` + `voice/intents`. If the answer is "those were scaffolding for a different architecture that §17.17 supersedes," the delete is a single ~2.7k-LOC PR.
