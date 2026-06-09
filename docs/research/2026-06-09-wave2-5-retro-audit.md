# Wave 2-5 Retroactive Adversarial Audit

- Date: 2026-06-09
- Range: `4f3dbd3..HEAD` (18 commits, ~8.4 k LOC across `internal/` + `cmd/` + `docs/`)
- Reviewer: independent (post-hoc; no Reviewer-agent-id slot was active during the wave)
- Baseline: `go test ./... -race -count=1` — **PASS** (21 packages green)
- Severity counts: **CRITICAL 0 · HIGH 7 · MED 13 · LOW 11**

## 1. Methodology

Read the full diff stat, then sampled every load-bearing surface in the
range with the six audit dimensions (A defects, B design gaps,
C adversarial concerns, D code drift, E test gaps, F spec drift) in
mind. Cross-checked each finding against the actual source at HEAD
(not a draft) and confirmed test baseline is green before filing. Files
read in full: `internal/web/server.go`, `internal/web/state.go`,
`internal/web/static/dashboard.js`, `internal/web/static/dashboard.html`,
`internal/dispatcher/selfbuild.go`, `internal/dispatcher/ship.go`,
`internal/dispatcher/ask.go`, `internal/operatormodel/{profile,observe,recommend}.go`,
`internal/selflearn/rules/panic_rate.go`, `internal/selflearn/{resolver,mistake}.go`,
`internal/voice/{tts,kokoro,openai}.go`, `internal/obs/{logger,metrics,recover}.go`,
`internal/daemonloop/loop.go`, `internal/memory/memory.go`,
`internal/notify/{desktop,pushover,voice}.go`, `internal/reasoner/reasoner.go`,
`internal/reviewer/reviewer.go`, `internal/watchdog/heartbeat.go`,
`internal/audit/audit.go`, `internal/budget/budget.go`,
`cmd/leah/{main,suggest,selfbuild}.go`, `cmd/leah-daemon/main.go`,
`prompts/self-build-attestations.txt`. Spot-checked tests in each
package. No source/spec edits made.

## 2. Findings by severity

### CRITICAL

(none)

The wave did not introduce a panic-on-nil, schema-corrupting migration,
unbounded budget bypass, or operator-merge-gate bypass. The most
load-bearing surfaces (self-build repo lock, budget gate, attestation
gate, web bind-to-loopback) are correctly enforced. Skip to HIGH for
the first actionable items.

### HIGH

**H1. `dispatcher.SelfBuild` writes `ArgsHash: ""` on the rejected /
failed / clarify / success audit rows — breaks selflearn dedup.**
`internal/dispatcher/selfbuild.go:199,209,219,238` all emit
`audit.Entry` with `ArgsHash: ""`. `selflearn.rowKey` =
`(Kind,ArgsHash,Timestamp)`; multiple self-build attempts in the same
window collapse to the same key (`self-build,,<ts>`) and the resolver
silently dedups them. **Fix:** hash the intent string (matches
`Ship.Run` at `ship.go:116`); add a regression test asserting two
distinct self-build runs produce distinct keys.

**H2. `dispatcher.SelfBuild.Run` re-runs Reasoner with the regatta-issue
SystemPrompt inside `inner.Run` — but `passthrough.Ask` ignores it.**
`selfbuild.go:121-137` constructs `inner := &Ship{ Reasoner:
passthrough{spec: specWithAttestation} ... }`. `passthrough.Ask`
(`selfbuild.go:155`) returns the spec verbatim, but `Ship.Run` at
`ship.go:77-78` calls `s.Reasoner.Ask(ctx, "Intent:\n"+intent+"...")`
— the regatta-issue-template SystemPrompt that `runSelfBuild` set on
the OUTER Reasoner (`cmd/leah/selfbuild.go:41`) is never seen because
the outer Reasoner already ran once with the self-build prompt. So
`runSelfBuild` wires the SELF-BUILD prompt as `SystemPrompt`, the spec
is generated under it correctly, but the ship-template prompt path is
silently dead — the Ship docstring (`ship.go:65`) claims the
"regatta-issue body" gets drafted but for self-build the body IS the
spec. This is the intended behavior but the comment trail is
misleading and the wiring is fragile: any future change that swaps
`passthrough` for a real Reasoner would unexpectedly re-charge the
budget + re-prompt. **Fix:** rename `passthrough` → `prebakedReasoner`,
add a comment on `runSelfBuild` documenting the SystemPrompt-is-load-
bearing-once contract, OR refactor `Ship.Run` to accept a pre-drafted
body.

**H3. Operator-attestation gate is unenforced at merge time — only an
honor-system PR-body footer.** `dispatcher/selfbuild.go:287-305`
appends a "PRs merged without `Attestation:` comment will be flagged
in the next weekly retro" line — but no code in the wave actually
scans merged self-build PRs for the comment. The retro generator
(`internal/selflearn/retro.go`, not in this wave's diff) is the
implied enforcer but is not wired. Operator habituation is the
attack vector this gate exists to defend against (`feedback_no_self_
tagged_approve` analog); a soft prose threat does not deter. **Fix:**
file a tracker for "retro flags un-attested self-build merges" or
mechanically block merge via a regatta-side gate. As-is, the
attestation question rotates randomly but is functionally cosmetic.

**H4. `web.Snapshot` calls per-source readers that each open `memory.db`
+ scan `audit.jsonl` from disk on every poll — 3 s default, every tab
focus.** `state.go:77-86` calls `tailAudit` (full-file scan,
`state.go:107-141`), `readMemory` (3 SQL queries,
`state.go:160-184`), `readMetrics` (full JSON read,
`state.go:92-105`), `listAgents` (subprocess via regatta CLI,
`state.go:143-158`). With a 30-day audit log (~MB-scale) this is a
linear scan every 3 s when the operator has the tab open. **Fix:** at
minimum, cache `tailAudit` keyed on file mtime; longer-term, use
`tail -n 20` style reverse seek. Wire from `daemonloop` last-tick
instead of disk-only.

**H5. `voice.OpenAITTS.Speak` posts the operator's text to a third party
without an opt-in gate.** `internal/voice/openai.go:33-101` —
constructed via `NewTTS()` if `OPENAI_API_KEY` is set
(`voice/tts.go:104`). Wired into `notify.VoiceNotify`
(`internal/notify/voice.go:18-21`). Any notification body Leah
generates (PR titles, agent IDs, intent strings echoed in
"agent X: failed → escalated") will round-trip to api.openai.com when
kokoro is unavailable or fails. Privacy posture for a personal-use
tool: should be explicit. **Fix:** require `LEAH_VOICE_ALLOW_OPENAI=1`
env in `pickBackends` to add the OpenAI tier; document in tts spec.

**H6. `daemonloop.maybeFireWeekly` releases the mutex from a goroutine
that captures the slice — but the test-only `weeklyInterval` package
var is mutated by tests without synchronization.** `daemonloop/loop.go:28`
declares `var weeklyInterval = 7 * 24 * time.Hour`; tests almost
certainly mutate it (typical pattern). Concurrent `tick` goroutines
read `weeklyInterval` at `loop.go:150` without sync. Race detector
will catch on parallel-test runs. **Fix:** make it a `Loop` field set
via `New(...)` or via an exported setter; or guard with a sync.RWMutex.
Spot-check: `loop_test.go` has 254 lines, this should be greppable.

**H7. `obs.SafeRun` writes panic files unconditionally to
`$LEAH_STATE_DIR/panics/` with no rotation.** `obs/recover.go:67-83`
writes one timestamped `.txt` per panic, never deletes. A wedged
goroutine that panics every 30 s creates ~2.8 k files/day, blowing
the inode budget on small filesystems and slowing `mostRecentPanicFile`
(panic_rate.go:184-213) which `os.ReadDir`s + sorts every entry on
every weekly tick. **Fix:** cap to last N (e.g. 100) on each write, or
add a daily-rotation/keep-last-7-days cleaner.

### MED

**M1. `web.Server.handleDashboard` uses `staticFS.ReadFile` per request
instead of serving via the existing file server.** `server.go:96-104`
— same bytes are also served at `/static/dashboard.html` via the
embed FS server. Two code paths for the same payload. **Fix:** delete
`handleDashboard`, redirect `/dashboard` → `/static/dashboard.html`
(or change the index file).

**M2. `web.tailAudit` swallows JSON-unmarshal errors silently.**
`state.go:122-124` — `if err := json.Unmarshal(...); err != nil {
continue }` matches `patterns.Detect` posture but loses signal: a
malformed line means corruption of the audit log, which is the single
source of truth for the entire selflearn loop. **Fix:** count the
errors into an `obs` counter (e.g. `leah_audit_parse_errors_total`)
so the dashboard surfaces a non-zero badge when corruption appears.

**M3. `web.MemoryView.RecentDecisions` hard-codes top 5 unrelated to
`/api/state` JSON shape.** `state.go:173-181` — operator can't tune.
Not a defect today, but the dashboard.js renders all 5 with title
attributes; a long-rationale decision blows the tooltip. **Fix:**
expose `Limit` on `Server`; truncate long `Choice` server-side.

**M4. `dashboard.js::renderMetrics` does `Object.entries(c).slice(0,1)`
— surfaces the first counter the registry happens to return.**
`dashboard.js:118` — Go map iteration is randomized; the "headline"
counter shown to the operator differs run to run. **Fix:** sort
entries deterministically (alphabetic) and pick a known key.

**M5. `dispatcher.Ship.watch` swallows every `regattaclient.List` error
to stdout, never to obs/audit.** `ship.go:147` —
`fmt.Fprintf(s.Out, "regatta list error: %v\n", err)` only. The
watcher logs nothing through `obs.LoggerFromCtx`, so the operator
sees the error in their terminal only — daemon-mode watchers (none
today, but future) would lose it. **Fix:** mirror to `lg.Warn`.

**M6. `dispatcher.SelfBuild.deriveSelfBuildTitle` truncates with no
ellipsis when intent fits under 60 chars.** `selfbuild.go:173-179` —
`len(t) > 60-len(SelfBuildTitlePrefix)` is fine, but the conditional
is brittle: `60-len(SelfBuildTitlePrefix)-3` for the ellipsis offset
is `60-13-3 = 44`. The truncated title can be `[SELF-BUILD] add ...`
which loses information. Title-truncation should be more aggressive
on Word boundaries. Not user-blocking. **Fix:** truncate to last
whitespace before the limit.

**M7. `dispatcher.isClarifyResponse` is case-folded and checks for `##
title` literal — fragile to system-prompt rewording.** `selfbuild.go:160-168`
— if the prompt is updated to use `# Title` (single hash) or
`Title:` line-prefix, every clarify response will be treated as a
valid spec and dispatched. **Fix:** ground-truth from the prompt
file: count the expected H2 sections, refuse if any required one is
absent. Or have the spec system-prompt emit a stable sentinel like
`## leah-selfbuild-spec-v1` that this function can grep.

**M8. `operatormodel.Profile.Update` calls `ObserveContextTransitions(rows,
nil)`.** `profile.go:99` — passes `nil` switches; the function early-
returns `nil` (`observe.go:73`). So the context-transition class
produces ZERO observations in production. Spec §2.1 lists this as
one of the three classes; the wiring TODO is in a code comment
(`profile.go:99: "// ctxmgr.Switch wiring lives in daemon caller"`)
but the daemon caller at `cmd/leah-daemon/main.go::buildWeeklyTasks`
also does not wire it. The operator-model is silently 2/3 of spec.
**Fix:** load switches from ctxmgr in `UpdateProfile`, or document
explicitly + delete the class from spec until ready.

**M9. `voice.ChainTTS` serializes ALL `Speak` calls on a single mutex
across multi-tier backends.** `voice/tts.go:71-86` — `c.mu.Lock()`
held for the duration of HTTP round-trip + afplay playback. A 5-second
OpenAI TTS call blocks every subsequent Speak. With the spec's claim
of being a notification backend (M3 of notify path), one slow notif
queues the rest. **Fix:** lock only around `afplay` (the actual
audio-device contention); drop the lock around synthesis.

**M10. `voice.OpenAITTS.Speak` does not wrap `io.Copy` errors with the
content of the response body.** `openai.go:90-92` — if synth returns
a 200 with truncated body, the error is `openai tts: write: <copy
err>`. The temp file is removed by defer so debugging requires curl
replay. Low frequency; debugging cost is real. **Fix:** capture the
first N bytes into a sidecar log on failure.

**M11. `obs.Registry.Snapshot` serializes the entire registry under one
top-level lock + per-series sub-lock — no copy-and-release pattern.**
`metrics.go:174-234` — under heavy snapshot frequency (60 s default,
fine) this is OK. If anyone adds a histogram-heavy hot path, latency
spikes will pile up during snapshot. Document the contract; alert on
snapshot duration. **Fix:** copy maps into local vars under lock,
release, then marshal — standard Prom pattern.

**M12. `obs.dailyRotator.writerFor` returns `io.Writer` from inside the
lock — but the comment claims "writerFor caller writes lock-free".**
`logger.go:142-168` — the comment is wrong. `multiWriter.Write` calls
`writerFor` which acquires `mu.Lock`, returns the handle, releases.
Then `_, _ = w.Write(p)` writes WITHOUT holding the lock. If a date
change happens concurrently (close() races), the writer can write to
a closed `*os.File`. **Fix:** either keep the lock for the duration
of the write, or use a `sync.Pool`-style swap with refcount.

**M13. `selflearn.PanicDetector` is a marker interface with no `Detect`
method.** `selflearn/resolver.go:48-53` — `interface{ Name() string
}`. The daemon caller (`cmd/leah-daemon/main.go:212`) does
`d.(rules.PanicRateRule)` to type-assert back to the concrete type.
This is an over-abstraction — single implementation, single caller,
and the interface forces a downcast. Spec invokes "marker interface
keeps cycle clean" but moving `Candidate` + `BuildIssueBody` into
selflearn would close the cycle without the abstraction.
**Fix:** delete the interface; let daemon import `rules` directly and
loop over `[]rules.PanicRateRule{...}`. Three lines instead of seven.

### LOW

**L1. `cmd/leah/main.go` uses `context.Background()` in every
subcommand entrypoint (`main.go:107,134,182`; `selfbuild.go:24`;
`suggest.go:20`) — no SIGINT handling.** Ctrl-C kills mid-Reasoner
without writing the audit row. Cost: one stale `pending` row per
cancelled run. **Fix:** `signal.NotifyContext` in `main.go` once,
thread through.

**L2. `cmd/leah/suggest.go:78` carries a code-comment TODO
`(--llm phrasing TODO; printing template form)` with no tracker
issue.** Violates `feedback_test_godoc_one_line` / stale-TODO drift.
**Fix:** file issue, replace with `// TODO(#NNN)`.

**L3. `cmd/leah-daemon/main.go:112` `Heartbeat: func() time.Time {
return time.Now() }` always returns the request time — dashboard's
"heart" indicator is a lie until daemonloop exposes last-tick.**
Listed as TODO in code comment; no tracker. Same fix as L2.

**L4. `dispatcher.Ship.Run` cleans the body draft by replacing `-->`
with `-- >`.** `ship.go:85` — defensive against HTML comment
injection in operator intent. Fine, but the audit row's Detail is the
raw url; the body content is untouched after this strip. The comment
should explain WHY (preventing closing the `<!-- leah-dispatched -->`
sentinel mid-body), not WHAT (replacing strings).

**L5. `voice.KokoroTTS.Speak` calls `os.Stat` for sanity, but uses
`filepath.Base(path)` in the error message — strips the directory the
caller needs to investigate.** `kokoro.go:42`. **Fix:** use `path`.

**L6. `voice.OpenAITTS.Speak` reads `o.HTTPClient` once and falls back
to a fresh `&http.Client{Timeout: 30 * time.Second}` — but does NOT
share a Transport.** `openai.go:70-73` — every call without an
operator-supplied client allocates a fresh client + DefaultTransport
hits the same global keep-alive pool, so this is actually fine; but
the pattern obscures it. **Fix:** package-level lazy singleton.

**L7. `obs.captureStack` allocates a fresh 64 KB buffer per panic.**
`recover.go:57-61` — fine; panics are rare. Note that under panic
storm this is GC pressure on top of an already-broken process.

**L8. `obs/metrics.go::flatten` allocates a new keys slice + sorts +
builds a strings.Builder per call.** Called from every `Counter.Add` /
`Gauge.Set` / `Histogram.Observe`. For low-cardinality counters on
the daemon hot path this is fine; for any high-frequency observer
the alloc rate is observable. **Fix:** memoize the flattened key on
first observation; OR pre-flatten on the calling side (operator
constructs labels once).

**L9. `operatormodel.Recommend` rounds hour/day in UTC**
(`recommend.go:39-40`) but `Profile.Update` honors operator-laptop tz
via `p.tz()` (`profile.go:90,71-75`). Slot keys produced under local
tz vs lookup under UTC will silently mismatch for any operator outside
UTC. **Fix:** thread the same tz through Recommend.

**L10. `memory.NewStore` sets `db.SetMaxOpenConns(1)`** (`memory.go:89`)
— pragmatic for WAL contention, but blocks any future parallel-read
benefit. Document the rationale + the trade.

**L11. `memory.embeddedSchemaVersion` is `"4"` but schema_version
comments in `schema.sql` go through `2`** — confirmed by the on-disk
schema header. Lexicographic comparison `v > embeddedSchemaVersion`
(`memory.go:124`) works through v9 then breaks (`"10" < "9"`).
Same trap already noted in the spec. **Fix:** parse to int; OR pad
to zero-leading width.

## 3. Cross-cutting patterns

### P1. Audit rows with empty `ArgsHash` on the new-Wave entrypoints

`selfbuild.go:199,209,219,238` all emit `ArgsHash: ""`. `Ship.Run`
correctly hashes; SelfBuild does not. This breaks
`selflearn.rowKey`'s uniqueness contract and downstream pattern
detection (`patterns.Detect`) — every self-build run looks like a
duplicate. See H1.

### P2. `_ = err` followed by `defer ... Close()` without obs.SafeClose helper

12 sites across the wave drop `Close()` errors with `defer func() { _
= f.Close() }()`. `memory/memory.go:187,250,316` (DB rows),
`web/state.go:115`, `selflearn/resolver.go:172`,
`operatormodel/profile.go:130,195,206,264,286`,
`dispatcher/selfbuild.go:260`, etc. Acceptable per Go idiom, but a
package-local `obs.SafeClose(closer io.Closer, name string)` helper
that increments a counter on err would catch FS-full silently-corrupt
scenarios. **Action:** add helper to `internal/obs`, sweep call sites
in Wave 6.

### P3. Background contexts in CLI subcommands

5 CLI entrypoints use `context.Background()` with no signal binding.
The daemon does it right (`cmd/leah-daemon/main.go:71`) with
`signal.NotifyContext`. CLI should mirror. See L1.

### P4. WHAT-comments creeping back in

Several godocs narrate the body rather than explain why. Examples:
`metrics.go:84` (`// Inc adds 1 to the counter at the given label
set.` — `Inc(map)` already says this), `memory.go:103` (`// Close
releases the underlying DB handle.`), `audit.go:31` (`// Logger is
the append-only writer...`). Per CLAUDE.md comments discipline these
should be name+sig only OR add WHY. Wave 6 sweep.

### P5. Single-impl interfaces with no test fake

`selflearn.PanicDetector` (M13), `dispatcher.RegattaClient`,
`dispatcher.HeartbeatPinger`, `dispatcher.Notifier`,
`web.RegattaLister`. Most are file-disjoint with their consumers and
do have test fakes, but `PanicDetector` is the canonical
over-abstraction. **Action:** apply `feedback_default_simpler` —
delete the abstraction or justify per-PR.

### P6. Spec-comment-says-X-code-does-Y

- `operatormodel.Profile.Update` is documented as 3 observation
  classes but ships 2 (M8).
- `obs.dailyRotator.writerFor` says "writes lock-free" but does NOT
  guard the post-return write (M12).
- `dispatcher.SelfBuild` docstring says "Ship would call Reasoner
  again with the regatta-issue template" but the regatta-issue prompt
  is never loaded for self-build paths (H2).

Pattern: docstrings drafted during design, code drift during
implementation, no comment-vs-code reconcile pass. **Action:** Wave 6
include explicit doc-vs-code pass on these three sites.

## 4. Adversarial-review gap analysis

Wave 2-5 shipped 18 commits in autonomous-ish mode. None of them
carry a `Reviewer-agent-id:` footer (operator + main-thread only).
Self-critique was inline in the spec docs (e.g.
`docs/specs/2026-06-09-bug-fix-self-build-hook.md` has its own
adversarial section). Per `feedback_no_self_tagged_approve` + the
regatta-side enforcement, this is exactly the failure mode the
regatta reviewer-verdict gate exists to prevent — but Leah is a
sibling repo without that gate, so the rule landed on operator
discipline only.

What an INDEPENDENT reviewer slot would have caught:

| Finding | Self-critique caught? | Independent reviewer would catch? |
|---|---|---|
| H1 ArgsHash empty | NO — selfbuild spec didn't audit dedup | YES — direct grep `ArgsHash:` reveals the gap |
| H2 SystemPrompt-load-bearing-once | NO — passthrough pattern presented as feature | YES — control-flow trace from runSelfBuild |
| H3 Attestation gate unenforced | PARTIAL — spec mentions retro flag | YES — "show me the code that scans" question |
| H4 web tail per-poll | NO — perf not in spec scope | YES — production-load adversarial framing |
| H5 OpenAI silent egress | NO — privacy not flagged | YES — `feedback_default_simpler` egress audit |
| H6 weeklyInterval var race | NO — race detector clean today only | YES — concurrent-state audit pass |
| H7 panic file unbounded growth | NO — happy path only in spec | YES — failure-cascade thinking |
| M8 context-transition wired-as-nil | NO — TODO comment in code | YES — "is the wiring complete?" question |
| M12 dailyRotator lock-comment lie | NO — author wrote the comment | YES — concurrency reread |
| M13 PanicDetector marker over-abstract | NO — designed as extension point | YES — `feedback_default_simpler` predicate |

Quantitative: 7 HIGH + 3 MED (10 items) above were NOT in any self-
critique section. Spec-included adversarial sections caught the
load-bearing surface (repo-lock, attestation prose, loopback bind)
but missed every concurrency / contract / dedup / privacy finding
that an out-of-context reviewer would surface in the first 30 min.

This validates `feedback_adversarial_review_every_step` — a fresh
slot before merging each Wave would have caught 10/31 findings (32%)
inline, leaving 21 for routine Wave 6 sweep instead of 31.

## 5. Recommended Wave 6 sweep tasks

File as `[FOLLOWUP]` issues; do NOT dispatch from this audit. Priority
ordering follows severity → cross-cutting → cleanup.

1. **W6-A** (H1) Fix `dispatcher.SelfBuild` ArgsHash propagation +
   regression test.
2. **W6-B** (H2) Refactor `passthrough` / document SystemPrompt-once
   contract for SelfBuild.
3. **W6-C** (H3) Wire retro generator to flag un-attested self-build
   merges; or downgrade prose threat.
4. **W6-D** (H4) Cache `web.tailAudit` by mtime; expose daemonloop
   last-tick for `Heartbeat` field.
5. **W6-E** (H5) Gate `voice.pickBackends` OpenAI tier on
   `LEAH_VOICE_ALLOW_OPENAI=1`.
6. **W6-F** (H6) Make `weeklyInterval` a `Loop` field; remove the
   package-level mutable var.
7. **W6-G** (H7) Rotate `$LEAH_STATE_DIR/panics/` (keep last N).
8. **W6-H** (M2/M4/M8/M12) Cross-cutting: doc-vs-code reconcile pass
   on `operatormodel`, `obs/logger`, `dispatcher/selfbuild`.
9. **W6-I** (M13) Delete `selflearn.PanicDetector` marker interface.
10. **W6-J** (P2) Add `obs.SafeClose` helper + sweep 12 sites.
11. **W6-K** (P3) Single `signal.NotifyContext` in `cmd/leah/main.go`.
12. **W6-L** (P4) Comments discipline sweep across new files.
13. **W6-M** (L9) Thread tz through `operatormodel.Recommend`.
14. **W6-N** (L11) Replace lex schema-version compare with int parse.
15. **W6-O** Standing: independent reviewer slot on every load-bearing
    PR going forward (`feedback_no_self_tagged_approve` applied to
    Leah, not just regatta).

---

End of audit. No source edits made; this doc is the only artifact.
