# Leah — Architecture

Single-file architecture overview. Operator-facing reference for "where does X live, what depends on Y, what writes to Z". The full design rationale lives under `docs/specs/`; this doc is the map.

## Goal

Leah is a single-operator personal AI chief-of-staff that closes the loop on her own evolution: she observes her own behavior (audit + obs), remembers what happened (memory.db + ctxmgr), decides what to do next (patterns + selflearn + operatormodel), and dispatches regatta to ship her own next feature (dispatcher.SelfBuild). Tri runs Leah; Leah runs Leah. Multi-user / SaaS / autonomous money or merge are explicitly out of scope (see `docs/specs/2026-06-09-leah-phase-x-multi-operator-roadmap.md`).

## The 4 layers

Ported from `docs/specs/2026-06-09-closed-loop-architecture.md`:

```
┌─────────────────────────────────────────────────────────────────────┐
│  Layer 4 — ACT (regatta dispatch)                                   │
│  internal/dispatcher/{ship,selfbuild}.go                            │
│  → leah self-build "<bug-fix>" or "<feature>"                       │
│  → gh issue create against trilamsr/Leah                            │
│  → regatta picks up → opens PR → leah review (independent subagent) │
│  → operator merges                                                  │
└─────────────────────────────────────────────────────────────────────┘
                          ▲
                          │ recommend
┌─────────────────────────┴───────────────────────────────────────────┐
│  Layer 3 — DECIDE (operator-model + patterns + self-learn)          │
│  internal/operatormodel/ — Wave2-J                                  │
│  internal/patterns/ — Wave1-D                                       │
│  internal/selflearn/ — Wave1-B                                      │
│  → operatormodel.Recommend(profile, ctx, time) → []Recommendation   │
│  → patterns.Detect(audit) → []Cluster → skill candidates            │
│  → selflearn.Resolver.Run() → back-fills outcome                    │
│  → selflearn.Retro.Generate(week) → markdown report                 │
│  Plus daemon weekly cron fires all 3 → state files in ~/.leah-state │
└─────────────────────────────────────────────────────────────────────┘
                          ▲
                          │ read
┌─────────────────────────┴───────────────────────────────────────────┐
│  Layer 2 — REMEMBER (memory + ctxmgr + audit)                       │
│  internal/memory/ — Wave1-A (contact/project/decision)              │
│  internal/ctxmgr/ — Wave1-C (active context + history)              │
│  internal/audit/ — MVP-5 (JSONL append-only)                        │
│  → Single memory.db (modernc.org/sqlite) at ~/.leah-state/          │
│  → schema v4: contact, project, decision, mistake_log,              │
│    operator_profile, context, operator_state, context_switch_log    │
└─────────────────────────────────────────────────────────────────────┘
                          ▲
                          │ write
┌─────────────────────────┴───────────────────────────────────────────┐
│  Layer 1 — OBSERVE (audit + obs)                                    │
│  internal/audit/ — MVP-5 (every action one line)                    │
│  internal/obs/ — Wave2-K (slog + metrics + panic recovery)          │
│  → Every action → audit row (user-facing semantics)                 │
│  → Every internal call → slog + metrics (operational semantics)     │
│  → Every panic → ~/.leah-state/panics/<ts>.txt + slog ERROR         │
│  → Trace correlation via obs.WithTrace(ctx, traceID)                │
└─────────────────────────────────────────────────────────────────────┘
                          ▲
                          │ instrument
              ┌───────────┴────────────┐
              │  Leah internals        │
              │  internal/reasoner/    │
              │  internal/budget/      │
              │  internal/dispatcher/  │
              │  internal/daemonloop/  │
              │  internal/ghclient/    │
              │  internal/regattaclient│
              │  internal/reviewer/    │
              │  internal/watchdog/    │
              │  internal/notify/      │
              │  internal/web/         │
              └────────────────────────┘
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
| `reviewer` | 4 | Independent reviewer subagent + canonical agent-id gate | `budget`, Anthropic SDK |
| `reasoner` | infra | Main Anthropic-backed LLM surface; charges budget per Ask | `budget`, `obs`, Anthropic SDK |
| `ghclient` | infra | `gh` CLI wrapper (`CreateIssue` / `ViewPR` / `ListPRsForBranch`) | `os/exec` |
| `regattaclient` | infra | `regatta agents list --json` wrapper | `os/exec` |
| `notify` | infra | macOS osascript banner + Pushover phone push (both satisfy `Notifier`) | `os/exec`, `net/http` |
| `watchdog` | infra | healthchecks.io per-tick liveness ping | `net/http` |
| `daemonloop` | infra | Per-30s regatta poll + weekly-cron tick wrapper | `audit`, `obs`, `regattaclient` |
| `intent` | infra | Regex classifier (`ask`/`ship`/`review`/`status`); not currently wired | — |
| `web` | surface | JARVIS dashboard HTTP server (loopback-only `/api/state` + `/dashboard`) | `audit`, `budget`, `memory`, `regattaclient` |

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
2. **Reasoner draft** — `dispatcher.SelfBuild.Run` calls `reasoner.Reasoner.Ask` with the self-build system prompt (`prompts/self-build-feature.md`). Anthropic SDK → text + dollar cost. `budget.Budget.Charge` enforces the per-process ceiling.
3. **Clarify gate** — `selfbuild.isClarifyResponse` checks for `## Clarifying questions` without `## Title`; if hit, prints questions + writes `outcome=clarify` audit row + returns `ErrSelfBuildClarify` (no issue filed).
4. **Attestation question** — `SelfBuild.pickAttestationQuestion` picks one line from `prompts/self-build-attestations.txt` and appends an "Operator merge attestation" footer naming the operator login that must answer in a PR comment.
5. **Inner Ship** — `SelfBuild` constructs an inner `dispatcher.Ship` with `Repo=trilamsr/Leah`, the pre-drafted spec wrapped in a `passthrough` Reasoner (so Ship doesn't re-call the LLM), and forwards watcher fields.
6. **Issue create** — `Ship.Run` writes the body to a tmp file, calls `ghclient.Client.CreateIssue` with label `ready-for-agent`. Returns URL.
7. **Audit** — `audit.Logger.Append` writes a `kind=ship outcome=pending BR=3` row (Ship) followed by `kind=self-build outcome=dispatched BR=4` (SelfBuild) with `prompt_sha` + `attestation_question` in detail.
8. **Watcher** — `Ship.watch` polls `regattaclient.Client.List` every 30s, pings `watchdog.Heartbeat`, fires `notify.Desktop` on terminal transition.
9. **Regatta picks up** — outside Leah's process: regatta sees the labelled issue + opens a PR.
10. **Review** — operator runs `leah review trilamsr/Leah <PR#>`. `reviewer.Reviewer.Review` dispatches `AnthropicSubagent` with the reviewer prompt + diff + linked-issue body. Parses `Reviewer-recommendation:` + `Reviewer-agent-id:` lines; `ValidateAgentID` rejects non-canonical IDs.
11. **Operator merge** — manual `gh pr merge` (automerge is BANNED on self-build PRs per `docs/specs/2026-06-09-closed-loop-architecture.md` §risk). Attestation comment required.
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
go test -tags integration ./internal/reasoner/ -v   # requires ANTHROPIC_API_KEY
go test -tags integration ./internal/reviewer/ -v   # requires ANTHROPIC_API_KEY
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

Authoritative design docs under `docs/specs/`:

- Closed-loop architecture: [`docs/specs/2026-06-09-closed-loop-architecture.md`](docs/specs/2026-06-09-closed-loop-architecture.md)
- Leah overview: [`docs/specs/2026-06-09-leah-overview.md`](docs/specs/2026-06-09-leah-overview.md)
- Fast-path roadmap: [`docs/specs/2026-06-09-roadmap-overview.md`](docs/specs/2026-06-09-roadmap-overview.md)
- Memory M2: [`docs/specs/2026-06-09-memory-m2-minimal.md`](docs/specs/2026-06-09-memory-m2-minimal.md)
- Self-learning: [`docs/specs/2026-06-09-self-learning-personal.md`](docs/specs/2026-06-09-self-learning-personal.md)
- Context manager: [`docs/specs/2026-06-09-context-manager.md`](docs/specs/2026-06-09-context-manager.md)
- Pattern recognition: [`docs/specs/2026-06-09-pattern-recognition.md`](docs/specs/2026-06-09-pattern-recognition.md)
- Self-building via regatta: [`docs/specs/2026-06-09-self-building-via-regatta.md`](docs/specs/2026-06-09-self-building-via-regatta.md)
- JARVIS UI: [`docs/specs/2026-06-09-jarvis-ui.md`](docs/specs/2026-06-09-jarvis-ui.md)
- Operator model: [`docs/specs/2026-06-09-operator-model.md`](docs/specs/2026-06-09-operator-model.md)
- Observability: [`docs/specs/2026-06-09-observability.md`](docs/specs/2026-06-09-observability.md)
- Phase X deferrals: [`docs/specs/2026-06-09-leah-phase-x-multi-operator-roadmap.md`](docs/specs/2026-06-09-leah-phase-x-multi-operator-roadmap.md)
- Tier 1 self-improvement: [`docs/specs/2026-06-09-leah-tier1-self-improvement.md`](docs/specs/2026-06-09-leah-tier1-self-improvement.md)
- Tier 2 SWE productivity: [`docs/specs/2026-06-09-leah-tier2-swe-productivity.md`](docs/specs/2026-06-09-leah-tier2-swe-productivity.md)
- Tier 3 schedule + comms: [`docs/specs/2026-06-09-leah-tier3-schedule-comms-multi-account.md`](docs/specs/2026-06-09-leah-tier3-schedule-comms-multi-account.md)
- Remaining tiers reordered: [`docs/specs/2026-06-09-leah-remaining-tiers-reordered.md`](docs/specs/2026-06-09-leah-remaining-tiers-reordered.md)

Operator rules for any agent (main + subagents): [`CLAUDE.md`](CLAUDE.md).
