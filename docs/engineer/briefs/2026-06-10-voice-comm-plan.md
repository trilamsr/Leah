# Voice-comm delivery plan — W11–W14

Companion to `docs/engineer/specs/2026-06-10-voice-comm.md`. PR-sized waves.
Each wave ships independently green; later waves do not block on earlier
follow-up issues. Per `CLAUDE.md` deletion default — every wave names what
got smaller (or why nothing did).

## Wave 11 — listener + wake skeleton

**Goal:** continuous mic capture lands; energy-trigger fires; wake-phrase
post-filter resolves. No Reasoner wired, no TTS, no state machine. The
output is a debug command that prints "wake!" on detection.

**Files touched:**
- `internal/voice/listener/listener.go` (NEW) — `Listener` interface + sox+whisper-stream impl
- `internal/voice/listener/listener_test.go` (NEW) — fakeExec-driven test
- `internal/voice/wake/wake.go` (NEW) — energy-threshold detector
- `internal/voice/wake/wake_test.go` (NEW) — synthetic RMS-frame test
- `cmd/leah/voice.go` (NEW) — `leah voice start --debug-wake` flag for skeleton-only mode

**Risk:** `whisper-stream` binary is not in current `voice/listen.go`
LookPath catalog. The wave MUST verify it exists (`brew list whisper-cpp`
ships it) before assuming the install hint is accurate. Backout: if
`whisper-stream` is not in the homebrew formula, W11 falls back to
1s-window chunked `whisper-cli` calls and the streaming refactor moves to
a follow-up tracking issue.

**Size:** ~400 LOC across 5 files. One PR.

**Test plan:** all hermetic; doubles for sox+whisper-stream Executor. Manual
operator verification: `leah voice start --debug-wake`, say "hey leah",
expect "wake!" within 500ms.

**Unblocks:** W12 (state machine needs a `Listener` to feed events).

**Deletion:** none — pure addition. Spec defends as A+ via §1 hands-busy
story.

## Wave 12 — session state machine + barge-in TTS cancel

**Goal:** state machine from spec §9 implemented and exercised by doubles.
Barge-in cancels in-flight TTS. No Reasoner yet — `thinking` immediately
transitions to `speaking` with a canned "echo" reply.

**Files touched:**
- `internal/voice/session/session.go` (NEW) — `Session.Run` + state table
- `internal/voice/session/session_test.go` (NEW) — 9 unit tests from spec §10
- `internal/voice/session/states.go` (NEW) — state enum + transition table
- `cmd/leah/voice.go` (EDIT) — wire `--debug-echo` flag for state-machine-only mode

**Risk:** audio-device lock interaction with barge-in. Mitigation: dedicated
test `TestSession_AudioDeviceReleaseAfterBargeIn` (spec §10). If the test
flakes, the wave does not ship — the lock semantics need a real fix, not a
sleep.

**Size:** ~600 LOC. One PR. Largest of the four waves.

**Test plan:** 9 unit + 1 integration test (spec §10). Manual: `leah voice
start --debug-echo`, say "test one", hear "test one" echoed; say "test two"
mid-echo, first echo cuts off, "test two" echoed.

**Unblocks:** W13 (Reasoner swap-in replaces the echo).

**Deletion:** `cmd/leah/listen.go::truncateTranscript` migrates to a shared
helper in `internal/voice/` and the original deletes. Net: -10 LOC.

## Wave 13 — wire Reasoner + audit row

**Goal:** `leah voice start` (no flags) runs the real loop. Reasoner.Ask
replaces echo. Audit row `Kind=voice_turn` per spec §6 lands per turn.
Transcript-toggle file read.

**Files touched:**
- `cmd/leah/voice.go` (EDIT) — drop debug flags, wire real Reasoner +
  dispatcher.Status routing for read-only intents
- `internal/voice/session/session.go` (EDIT) — call Reasoner, write audit row
- `internal/voice/session/transcript_toggle.go` (NEW) — read
  `$HOME/.leah-state/voice.json`; default OFF
- `internal/voice/session/transcript_toggle_test.go` (NEW)

**Risk:** Reasoner latency may blow §5 budget on operator's local model.
Tracking issue `voice/reason-streaming` filed BEFORE this wave merges — the
risk is acknowledged, not hidden. Wave ships at A grade even if first-audio
> 1.5s; A+ requires the tracking issue.

**Size:** ~250 LOC. One PR.

**Test plan:** `TestSession_HappyPath`, `TestSession_ReasonerError`,
`TestSession_TranscriptSuppression` (all from spec §10). Manual: real
end-to-end "Leah, what time is it?" returns Reasoner answer via TTS.

**Unblocks:** W14 (attestation gate locks down the real loop).

**Deletion:** if `cmd/leah/listen.go` push-to-talk is fully subsumed by
`leah voice start --once` (single-turn mode), `listen.go` deletes. This is
a stretch — the conservative path keeps `listen` as a low-overhead PTT
escape hatch. Decision deferred to wave-end with a flag in PR body.

## Wave 14 — attestation gate + idle-timeout polish + secret redaction

**Goal:** spec §7 (attestation) + spec §11 (secret-shape redaction) +
`IdleAfter` escalation to `requires_wake`. Runbook lands.

**Files touched:**
- `internal/voice/session/session.go` (EDIT) — attestation prompt + match
- `internal/voice/session/attest.go` (NEW) — `Attestor` interface +
  phrase-match impl
- `internal/voice/session/redact.go` (NEW) — secret-shape regex + transcript
  sanitizer
- `internal/voice/session/redact_test.go` (NEW)
- `cmd/leah/voice.go` (EDIT) — `leah voice stop` + `leah voice status`
  subcommands
- `docs/engineer/runbooks/voice-comm.md` (NEW) — manual verification checklist
  + threat-model operational notes

**Risk:** runbook cross-link gate (`scripts/check-doc-links.sh`) — if the
runbook links back to the spec and the spec doesn't yet link to the runbook,
the gate fails on the spec PR (already merged at W11). Mitigation: this
wave's PR adds BOTH the runbook AND a one-line link from spec to runbook in
the same diff — the spec edit is additive.

**Size:** ~350 LOC + runbook. One PR.

**Test plan:** `TestSession_AttestationDenied`, secret-redaction unit tests,
`TestSession_IdleTimeout`. Manual: full A+ rubric walk per spec §13.

**Unblocks:** voice-comm is operator-shippable. Tracking issue
`voice/reason-streaming` (W15) becomes the next priority.

**Deletion:** spec §1 echo-loop debug commands (`--debug-wake`,
`--debug-echo`) remove. Net: -40 LOC across `cmd/leah/voice.go`.

## Cross-wave invariants

- No wave bypasses the audit-row schema (spec §6). A wave that "temporarily"
  skips audit is not mergeable.
- No wave introduces a sleep-loop. Per `CLAUDE.md` token-economy + the
  Wave 1-G2 check-no-bare-sleep gate (commit ca2110c), use
  `testutil.Eventually` for state-transition asserts.
- Every wave's PR body answers "what got smaller?" — W11 has no deletion,
  defended as A+ addition; W12–W14 each name a concrete delete.
- Reviewer subagent is dispatched on PR open per `CLAUDE.md` mandate.
  Self-APPROVE is forbidden.

## Tracking issues to file at spec-merge

- `voice/reason-streaming` — stream Reasoner tokens into TTS chunks to
  reclaim the 600ms Reasoner stage from §5 budget.
- `voice/porcupine-wake-evaluation` — measure false-positive rate of the
  energy-threshold detector after 1 week of operator use; promote to
  Porcupine only if FP/hour > 2.
- `voice/openai-realtime-stt` — opt-in path behind
  `LEAH_VOICE_ALLOW_OPENAI_STT=1` for operators on bad-CPU machines.
- `voice/speaker-id` — out-of-scope (§12) but worth a stub for the
  spoofed-operator threat note (§11).

```release-notes
none (internal)
```
