# Leah — Architecture

Single-file architecture overview. Operator-facing reference for "where does X live, what depends on Y, what writes to Z". Reflects v3.3.0 (Phase 3 shipped — voice + push-source + KG citations + MCP publish + Sparkle EdDSA). Phase-1 layers 1–4 are unchanged; Phase-2 layer 5 (UI shell) and Phase-3 layer 6 (Voice + Push) are tracked under "Phase 2 + 3 deltas". This doc is the map; Phase 2/3 implementation plans under `docs/superpowers/plans/` are the design rationale.

## Goal

Leah is a single-operator personal chief-of-staff that closes the loop on her own evolution: she observes her own behavior (audit + obs), remembers what happened (memory.db + ctxmgr), decides what to do next (patterns + selflearn + operatormodel), and dispatches regatta to ship her own next feature (dispatcher.SelfBuild). Tri runs Leah; Leah runs Leah. Multi-user / SaaS / autonomous money or merge are explicitly out of scope.

## Layer model (Phase 1 → Phase 3)

Phase 1 stood up Layers 1–4 (audit / memory / decide / dispatch). Phase 2 added Layer 5 (UI shell — widget envelope + HUD tile registry + Ambient + notification toast + pin flow + light mode + BGE ONNX retrieval). Phase 3 added Layer 6 (Voice + Push — ElevenLabs/Apple TTS chain + wake-word + VAD + suppression + push-to-talk + push-source IPC fan + KG citation join + MCP read-only publish + Sparkle EdDSA verify + Dashboard).

```
┌─────────────────────────────────────────────────────────────────────┐
│  Layer 4 — ACT (regatta dispatch)                                   │
│  internal/actions/dispatcher/{ship,selfbuild}.go                            │
│  → leah self-build "<bug-fix>" or "<feature>"                       │
│  → gh issue create against trilamsr/Leah                            │
│  → regatta picks up → opens PR → leah review (independent subagent) │
│  → operator merges                                                  │
└─────────────────────────────────────────────────────────────────────┘
                          ▲
                          │ recommend
┌─────────────────────────┴───────────────────────────────────────────┐
│  Layer 3 — DECIDE (operator-model + patterns + self-learn)          │
│  internal/thinking/operatormodel/ — Wave2-J                                  │
│  internal/thinking/patterns/ — Wave1-D                                       │
│  internal/thinking/learn/ — Wave1-B (recommend + retro; single-tenant repo)  │
│  → operatormodel.Recommend(profile, ctx, time) → []Recommendation   │
│  → patterns.Detect(audit) → []Cluster → skill candidates            │
│  → learn.Resolver.Run() → back-fills outcome                        │
│  → learn.Retro.Generate(week) → markdown report                     │
│  Plus daemon weekly cron fires all 3 → state files in ~/.leah-state │
└─────────────────────────────────────────────────────────────────────┘
                          ▲
                          │ read
┌─────────────────────────┴───────────────────────────────────────────┐
│  Layer 2 — REMEMBER (memory + ctxmgr + audit)                       │
│  internal/memory/ — Wave1-A (contact/project/decision)              │
│  internal/platform/activectx/ — Wave1-C (active context + history)  │
│  internal/platform/audit/ — MVP-5 (JSONL append-only)                        │
│  → Single memory.db (modernc.org/sqlite) at ~/.leah-state/          │
│  → schema v4: contact, project, decision, mistake_log,              │
│    operator_profile, context, operator_state, context_switch_log    │
└─────────────────────────────────────────────────────────────────────┘
                          ▲
                          │ write
┌─────────────────────────┴───────────────────────────────────────────┐
│  Layer 1 — OBSERVE (audit + obs)                                    │
│  internal/platform/audit/ — MVP-5 (every action one line)                    │
│  internal/platform/telemetry/ — Wave2-K (slog + metrics + panic recovery) │
│  → Every action → audit row (user-facing semantics)                 │
│  → Every internal call → slog + metrics (operational semantics)     │
│  → Every panic → ~/.leah-state/panics/<ts>.txt + slog ERROR         │
│  → Trace correlation via obs.WithTrace(ctx, traceID)                │
└─────────────────────────────────────────────────────────────────────┘
                          ▲
                          │ instrument
              ┌──────────────┴──────────────┐
              │  Leah internals             │
              │  internal/thinking/reasoner/│
              │  internal/platform/budget/           │
              │  internal/actions/dispatcher/       │
              │  internal/platform/daemonloop/       │
              │  internal/actions/ghclient/         │
              │  internal/actions/regattaclient/    │
              │  internal/thinking/reviewer/│
              │  internal/platform/watchdog/         │
              │  internal/actions/commsout/ │
              │  internal/platform/web/              │
              └─────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│  Layer 6 — VOICE + PUSH (Phase 3)                                   │
│  internal/actions/tts/{elevenlabs,apple} + classifier.go + provider.go │
│  internal/input/voice/ — wake-word + VAD + suppression                    │
│  app/Leah/Sources/LeahAudio, LeahWake — playback + wake adapter     │
│  app/Leah/Sources/LeahUI/PushToTalk.swift — Fn / right-⌘ PTT        │
│  cmd/leah-daemon/pushsource_runtime.go — fans push.* IPC stream     │
│  → `tts.speak` → classifier picks cloud (ElevenLabs Flash v2.5) or  │
│    local (Apple Ava Premium) → `tts.cloud.frame` chunks or          │
│    `tts.apple.speak` / `tts.local{,.prewarm}` → LeahAudio plays     │
│  → `push.{mail,contacts,focus,activeapp}` → HUD widgets subscribe   │
└─────────────────────────────────────────────────────────────────────┘
                          ▲
┌─────────────────────────┴───────────────────────────────────────────┐
│  Layer 5 — UI (Phase 2)                                             │
│  internal/input/hud/ — tile registry + recommendations + pinned + focus   │
│  app/Leah/Sources/LeahUI — MenubarItem, HotkeyManager, FocusPanel,  │
│    Notifications, Settings/, Dashboard/, Wizard/                    │
│  app/Leah/Sources/LeahWidgets — Phase-2 widget surface              │
│  app/Leah/Sources/LeahUpdate — Sparkle EdDSA verify + rollback      │
│  app/Leah/Sources/LeahAuth — Touch ID gate (memory purge + telem)   │
│  → Daemon emits widget.{mount,update,stale,error,dismiss,unmount}   │
│    + notification.toast → HUD subscribes via LeahIPC                │
└─────────────────────────────────────────────────────────────────────┘
                          ▲
```

## Package map

Every `internal/*` package, what it does, and what it depends on.

| Package | Layer | Purpose | Depends on |
|---|---|---|---|
| `audit` | 1 | JSONL append-only operator-action log (`~/.leah-state/audit.jsonl`) | — |
| `obs` | 1 | slog + in-process metrics + `SafeGo` panic recovery + per-trace logger | — |
| `budget` | infra | Per-process $ ceiling enforced via mutex-guarded `Charge` | — |
| `memory` | 2 | SQLite KB for `contact` / `project` / `decision`; owns `schema.sql` (v4) | `modernc.org/sqlite`, `oklog/ulid` |
| `ctxmgr` | 2 | Single-active-context + switch history; shares `memory.db` | `modernc.org/sqlite` |
| `selflearn` | 3 | Outcome resolver, mistake log, weekly retro markdown | `audit`, `memory` |
| `selflearn/rules` | 3 | Per-kind world probes (e.g. `RegattaPR` polls `gh pr view`) | `audit`, `selflearn` |
| `patterns` | 3 | Audit clustering → `skill-candidates.md` for operator review | `audit` |
| `operatormodel` | 3 | Time-of-day / cadence / context-transition observers + ranked recs | `audit`, `ctxmgr`, `memory` |
| `dispatcher` | 4 | Orchestrations: `Ask` / `Ship` / `SelfBuild` / `Status` | `audit`, `budget`, `ghclient`, `regattaclient`, `obs` |
| `reviewer` | 4 | Independent reviewer subagent + canonical agent-id gate | `budget`, cloud LLM SDK |
| `reasoner` | infra | Main reasoning surface (cloud LLM); charges budget per Ask | `budget`, `obs`, cloud LLM SDK |
| `ghclient` | infra | `gh` CLI wrapper (`CreateIssue` / `ViewPR` / `ListPRsForBranch`) | `os/exec` |
| `regattaclient` | infra | `regatta agents list --json` wrapper | `os/exec` |
| `notify` | infra | macOS osascript banner + Pushover phone push (both satisfy `Notifier`) | `os/exec`, `net/http` |
| `watchdog` | infra | healthchecks.io per-tick liveness ping | `net/http` |
| `daemonloop` | infra | Per-30s regatta poll + weekly-cron tick wrapper | `audit`, `obs`, `regattaclient` |
| `intent` | infra | Regex classifier (`ask`/`ship`/`review`/`status`); not currently wired | — |
| `web` | surface | JARVIS dashboard HTTP server (loopback-only `/api/state` + `/dashboard`) | `audit`, `budget`, `memory`, `regattaclient` |

## Phase 2 + 3 deltas

The Phase-1 table above is unchanged. Phase 2 + 3 added the packages, targets, and IPC kinds below; everything Phase 1 still works unmodified.

### New Go packages

| Package | Layer | Purpose | Cross-ref |
|---|---|---|---|
| `hud` | 5 | Tile registry + recommendations + pinned widgets + focus-state + push-source widget stream; daemon-side state HUD subscribes to | Phase 2 §HUD recompose |
| `tts` | 6 | `provider.go` chain (cloud → local fallback) + `classifier.go` (privacy gate; daemon-only) | Phase 3 §17.17 |
| `tts/elevenlabs` | 6 | ElevenLabs Flash v2.5 SSE client; emits `tts.cloud.frame` chunks | Phase 3 §17.17 |
| `tts/apple` | 6 | Apple Ava Premium voice trigger; emits a single `tts.apple.speak` IPC (HUD does playback) | Phase 3 §17.17 |
| `voice` | 6 | TTS listener + duplex + loop (`internal/input/voice/{listener,duplex,loop}`) | Phase 3 §voice |
| `embed` | 2/5 | Embedding backend selector (`SelectGenerator`); accepts `LEAH_EMBED_BACKEND=hash|openai|bge`, default `hash` (deterministic 256d). `bge` loads BGE-small-en-v1.5 via ONNX Runtime (CGo only). Per-(model,dim) physical tables (`embeddings_bge_small_en_v1_5_384`, …) keep cloud↔local toggles re-embed-free. Voyage path scaffolded in table names only, no generator implemented | Phase 2 §retrieval |
| `eval` | 3 | Closed-loop bootstrap traces + harness + LLM judge + scheduler + store (Phase-3 quality gate) | Phase 3 §eval |
| `mcp` | 4 | Read-only MCP server: tool publish, A2A card, self-build A2A, redaction; mutations not exposed | Phase 3 §MCP publish |
| `knowledge` | 2 | KG citation join over memory.db + embeddings; feeds streaming answer-engine | Phase 3 §KG citations |

### New Swift targets (`app/Leah/Package.swift`)

| Target | Purpose |
|---|---|
| `LeahApp` | Executable; menu-bar + global hotkey + Sparkle wiring |
| `LeahUI` | Menubar hexagon, HotkeyManager, FocusPanel (NSPanel), Notifications, PushToTalk, plus `Settings/`, `Dashboard/`, `Wizard/` subtrees |
| `LeahWidgets` | Phase-2 widget rendering surface (consumes `widget.*` IPC envelopes) |
| `LeahAudio` | Cloud-frame audio `Player` + Apple `AppleSpeech` driver |
| `LeahWake` | Wake-word adapter; bundles `Resources/Models/wake-leah.mlmodel` |
| `LeahAuth` | Touch ID gate (memory purge + telemetry-toggle per §17.13) |
| `LeahUpdate` | Sparkle delegate w/ EdDSA verify + rollback channel |
| `LeahIPC` | AF_UNIX framed IPC client (frame kinds below) |

### New IPC frame kinds (`internal/platform/ipc/frame.go`)

| Kind | Direction | Purpose |
|---|---|---|
| `widget.mount` / `widget.update` / `widget.stale` / `widget.error` / `widget.dismiss` / `widget.unmount` | daemon → HUD | Phase-2 widget lifecycle envelope |
| `notification.toast` | daemon → HUD | Phase-2 toast surface |
| `tts.speak` / `tts.cancel` | HUD → daemon | Operator request |
| `tts.cloud.frame` | daemon → HUD | ElevenLabs chunked audio frames (LeahAudio.Player) |
| `tts.apple.speak` | daemon → HUD | Apple Ava local-synthesis trigger (LeahAudio.AppleSpeech); emitted by `cmd/leah-daemon/tts_handler.go` |
| `tts.local` / `tts.local.prewarm` | daemon → HUD | Direct `internal/actions/tts/apple` provider path: prewarm Ava voice model + speak (literals, not constants in `frame.go`) |
| `tts.speak.done` / `tts.speak.err` / `tts.cancel.ok` | daemon → HUD | Terminal acks |
| `push.mail` / `push.contacts` / `push.focus` / `push.activeapp` | daemon → HUD | Push-source fan-out (string-literal kinds emitted by `cmd/leah-daemon/pushsource_runtime.go`, not constants in `frame.go`) |

### Sequencing

- HUD subscribes to daemon IPC on launch; `widget.*` envelopes drive `hud.registry` → `LeahWidgets` rendering.
- `tts.speak` flows HUD → daemon → `tts.classifier` → privacy gate → either ElevenLabs (`tts.cloud.frame` chunks → `LeahAudio.Player`) or Apple (`tts.apple.speak` → `LeahAudio.AppleSpeech`); terminal `tts.speak.done` / `.err` always returns.
- Push sources (`mail`, `contacts`, `focus`, `activeapp`) run inside `leah-daemon`; `pushsource_runtime.go` fans each producer's deltas into the public `push.*` namespace HUD widgets subscribe to.
- KG citation join (`internal/thinking/knowledge`) runs inside the answer-engine streaming path; citations stream alongside Reasoner tokens.

### Storage — embedding namespace

`memory.db` schema v9 (latest additive marker: Phase 1 conversation auto-capture). Embedding storage landed in v5. Current writes route through `internal/platform/embed.tableName(model, dim)` into **per-(model, dim) physical tables** lazily created by `ensureTable` — `embeddings_bge_small_en_v1_5_384`, `embeddings_hash_*`, etc. Each table carries its own `idx_<table>_model` index. The legacy v5 `embedding` table (single-table layout with `(model, dim)` columns + `idx_embedding_model`) is preserved but frozen; new writes never land there. Cloud↔local toggles therefore never trigger re-embedding.

### Per-feature env gates

| Env | Default | Effect |
|---|---|---|
| `LEAH_EMBED_BACKEND` | `hash` | `hash` / `openai` / `bge` — selects `internal/platform/embed.SelectGenerator` provider. `bge` requires CGo + `LEAH_EMBED_MODEL_PATH` (or `LEAH_MODEL_DIR`). Unknown values error out — there is no auto-fallback |
| `LEAH_RECOMMEND_BANDIT` | off | Arms the recommendation bandit in `internal/thinking/recommend` (UCB exploration) |
| `LEAH_EVAL` | off | Enables `internal/platform/eval` closed-loop quality harness + scheduler |
| `LEAH_MCP_PUBLISH` | off | Mounts `internal/platform/mcp` server on the daemon (read-only tools + A2A card) |

## CLI command surface

`cmd/leah/main.go` + per-verb files (`contact.go`, `ctx.go`, `decision.go`, `mistake.go`, `patterns.go`, `project.go`, `retro.go`, `selfbuild.go`).

| Command | What it does | Blast radius |
|---|---|---|
| `leah ask "<query>"` | Direct Reasoner query, stdout response | 0 (read-only) |
| `leah ship <repo> "<intent>"` | Draft regatta issue + create + watch 60min for terminal state | 3 |
| `leah review <repo> <pr#>` | Independent reviewer subagent on a real PR (verdict + agent-id to stdout) | 0 (no PR comment posted) |
| `leah status [--json]` | Tail last 20 audit rows | 0 |
| `leah contact <add\|list\|show>` | Memory KB CRUD on `contact` rows | 1 |
| `leah project <add\|list\|show>` | Memory KB CRUD on `project` rows | 1 |
| `leah decision <add\|list\|show>` | Log + recall recorded decisions | 1 |
| `leah ctx <new\|switch\|show\|history\|list>` | Active-context lane manager | 1 |
| `leah mistake add` | Operator-annotated negative outcome → `mistake_log` | 1 |
| `leah retro [--week YYYY-WW]` | Render weekly retro markdown to stdout | 0 |
| `leah patterns [--weekly]` | Skill-candidate clusters from audit | 0 |
| `leah self-build "<intent>"` | Repo-locked feature-spec dispatch against `trilamsr/Leah` | 4 |
| `leah version` | Print version string | 0 |

Daemon: `cmd/leah-daemon/main.go` runs the per-30s poll + (optional) `--dashboard 127.0.0.1:8080` JARVIS server + weekly tick (resolver / patterns / retro / operatormodel).

## Data flow — `leah self-build "fix bug X"`

End-to-end example showing every package the flow touches.

1. **CLI entry** — `cmd/leah/main.go::main` dispatches `self-build` → `cmd/leah/selfbuild.go::runSelfBuild`.
2. **Reasoner draft** — `dispatcher.SelfBuild.Run` calls `reasoner.Reasoner.Ask` with the self-build system prompt (`prompts/self-build-feature.md`). Cloud LLM SDK returns text + dollar cost. `budget.Budget.Charge` enforces the per-process ceiling.
3. **Clarify gate** — `selfbuild.isClarifyResponse` checks for `## Clarifying questions` without `## Title`; if hit, prints questions + writes `outcome=clarify` audit row + returns `ErrSelfBuildClarify` (no issue filed).
4. **Attestation question** — `SelfBuild.pickAttestationQuestion` picks one line from `prompts/self-build-attestations.txt` and appends an "Operator merge attestation" footer naming the operator login that must answer in a PR comment.
5. **Inner Ship** — `SelfBuild` constructs an inner `dispatcher.Ship` with `Repo=trilamsr/Leah`, the pre-drafted spec wrapped in a `passthrough` Reasoner (so Ship doesn't re-call the LLM), and forwards watcher fields.
6. **Issue create** — `Ship.Run` writes the body to a tmp file, calls `ghclient.Client.CreateIssue` with label `ready-for-agent`. Returns URL.
7. **Audit** — `audit.Logger.Append` writes a `kind=ship outcome=pending BR=3` row (Ship) followed by `kind=self-build outcome=dispatched BR=4` (SelfBuild) with `prompt_sha` + `attestation_question` in detail.
8. **Watcher** — `Ship.watch` polls `regattaclient.Client.List` every 30s, pings `watchdog.Heartbeat`, fires `notify.Desktop` on terminal transition.
9. **Regatta picks up** — outside Leah's process: regatta sees the labelled issue + opens a PR.
10. **Review** — operator runs `leah review trilamsr/Leah <PR#>`. `reviewer.Reviewer.Review` dispatches `AnthropicSubagent` with the reviewer prompt + diff + linked-issue body. Parses `Reviewer-recommendation:` + `Reviewer-agent-id:` lines; `ValidateAgentID` rejects non-canonical IDs.
11. **Operator merge** — manual `gh pr merge` (automerge is BANNED on self-build PRs; blast radius = 4). Attestation comment required.
12. **Daemon transition** — `daemonloop.Loop.tick` notices the regatta agent state went `merged` → writes `kind=daemon.transition outcome=observed` audit row + fires `Notifier.Notify`.
13. **Weekly retro** — next Sunday 9am, `daemonloop` fires `selflearn.Resolver` (back-fills the original `ship` row's outcome via `selflearn/rules.RegattaPR`), `patterns.Detect`, `selflearn.Retro.Generate`, `operatormodel.UpdateProfile`. Reports land under `~/.leah-state/`.

## State files

All persistent state lives under `$LEAH_STATE_DIR` (default `~/.leah-state/`, mode 0700).

| Path | Owner | Format | Purpose |
|---|---|---|---|
| `audit.jsonl` | `audit.Logger` | JSONL, append-only | Every operator action one line; read by status/retro/patterns/operatormodel/web |
| `memory.db` | `memory.Store` (shared with `ctxmgr`, `selflearn.MistakeStore`, `operatormodel`) | SQLite (modernc), WAL mode | Schema v4: contact, project, decision, mistake_log, context, operator_state, context_switch_log, operator_profile, operator_profile_meta |
| `last-weekly.txt` | `daemonloop` | Single RFC3339 line | Tracks last weekly-tick fire to gate the 7d cadence |
| `skill-candidates.md` | `patterns.Propose` via daemon weekly | Markdown | Cherry-pickable list of repeat-action clusters surfaced for operator |
| `retro-YYYY-WW.md` | `selflearn.Retro.Generate` via daemon weekly | Markdown | Per-ISO-week retro: actions, cost, wins, mistakes |
| `panics/<ts>-<name>.txt` | `obs.SafeRun` | Plain text | Stack trace + goroutine name on recovered panic; selflearn (Wave 3) reads these to draft self-bug issues |
| `logs/leah-YYYY-MM-DD.jsonl` | `obs.dailyRotator` | JSONL (slog) | Daily-rotated structured log fanned in alongside stderr |

## Build + dev

Build:

```bash
go build -o leah ./cmd/leah
go build -o leah-daemon ./cmd/leah-daemon
```

Tests:

```bash
go test ./...
go test -tags integration ./internal/thinking/reasoner/ -v   # requires ANTHROPIC_API_KEY
go test -tags integration ./internal/thinking/reviewer/ -v   # requires ANTHROPIC_API_KEY
```

Lint:

```bash
golangci-lint run ./...
```

Pre-push (composite):

```bash
./scripts/check.sh   # go build + go test + go vet + golangci-lint (if installed)
```

## Cross-refs

- Ship log: [`CHANGELOG.md`](CHANGELOG.md)
- House rules for any agent (main + subagents): [`README.md` § House rules](README.md#house-rules).
