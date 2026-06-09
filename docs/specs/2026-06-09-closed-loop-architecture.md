---
title: Leah — closed-loop architecture (self-observation → self-diagnosis → self-build)
status: living
owner: tri
created: 2026-06-09
---

# Closed-loop architecture

How all the pieces fit. Operator's north-star: Leah notices her own bugs + dispatches regatta to fix them.

## The 4 layers

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
                          │                                              
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

## The 5 surfaces

How operator interacts with the loop:

| Surface | Reads from | Writes to |
|---|---|---|
| CLI (`leah ask/ship/review/status/contact/project/decision/ctx/mistake/retro/patterns/suggest/self-build`) | Memory, audit, regatta state | Audit, memory, regatta issues |
| Daemon (`leah-daemon`) — per-30s tick | Regatta state | Audit (state-transition rows), heartbeat |
| Daemon weekly tick (4 tasks) | Audit (last 7d), memory | Skill candidates file, retro file, operator profile, outcome resolutions |
| JARVIS UI dashboard (127.0.0.1:8080) | Audit, memory, regatta, budget, heartbeat | None (read-only Phase 1) |
| Voice / push notifications | Daemon transitions | None |

## The closed loop — Leah-fixes-Leah

This is the concrete sequence operator's directive enables:

1. **Observe**: `internal/obs` captures a panic in `internal/reasoner/anthropic.go::Complete` — stack trace written to `~/.leah-state/panics/2026-06-09T13-42-00-reasoner-complete.txt` + `leah_panic_total{package=reasoner}` increments.

2. **Remember**: Audit row written: `{kind: "reasoner.panic", outcome: "failed", detail: "see panics/2026-06-09T13-42-00-..."}`. Daemon weekly tick reads audit + sees the panic outcome.

3. **Decide**: `selflearn.Resolver` notices `panic` outcomes are increasing week-over-week. Wave 3 hook (deferred): builds a regatta-issue-body from the panic file + last 10 slog lines + which-goroutine + memory snapshot. Surfaces to operator: "Leah noticed N panics in reasoner this week — want me to dispatch a self-build fix?"

4. **Act**: Operator says yes → `leah self-build "fix the reasoner.Complete panic at &lt;stack&gt; — context attached"` → SelfBuild reads the panic file + audit context + drafts a Leah-feature spec (which is really a bug-fix spec) → `gh issue create` against trilamsr/Leah → regatta picks up → opens PR with fix.

5. **Validate**: Leah's independent reviewer subagent reads the PR diff vs the linked issue (which has the panic context) → verdict APPROVE/REVISE/BLOCK → operator-attestation-question gate (Wave 2-G) forces operator to answer a random question in PR comment → operator merges.

6. **Verify-after-shipping**: Next 30s daemon poll → CI ran → new binary deployed (manual for now; CI deploy = Wave X) → next reasoner.Complete call → no panic → `leah_panic_total{package=reasoner}` flatlines.

7. **Retro**: Following weekly retro reports: "Leah shipped 1 self-fix this week: reasoner.Complete panic. Panic rate dropped from N/wk to 0."

## Where we are

### Shipped
- Layer 1 audit (MVP-5) ✅
- Layer 1 obs (Wave 2-K, in flight)
- Layer 2 memory (Wave 1-A) ✅
- Layer 2 ctxmgr (Wave 1-C) ✅
- Layer 3 patterns (Wave 1-D) ✅
- Layer 3 selflearn resolver+mistake+retro (Wave 1-B) ✅
- Layer 3 operatormodel (Wave 2-J, in flight)
- Layer 4 ship dispatcher (MVP-5) ✅
- Layer 4 reviewer independent subagent (MVP-5) ✅
- Layer 4 selfbuild dispatcher (Wave 1-E) ✅
- Surface: CLI (Wave 2-G, in flight)
- Surface: daemon per-30s + weekly tick (Wave 2-H) ✅
- Surface: JARVIS UI dashboard (Wave 2-I) ✅

### In flight (parallel agents)
- Wave 2-G: CLI wiring + schema reconciliation + operator-attestation gate
- Wave 2-J: operator-model + recommendation
- Wave 2-K: observability layer

### Wave 3 (next)
- `leah suggest` CLI (operator-model surfacer; deferred from J to avoid main.go collision)
- `obs` daemon wiring (start metrics snapshotter + write panic files; deferred from K)
- Bug-fix-self-build hook: selflearn.Resolver detects increasing-panic-rate → drafts regatta-issue-body with panic context → notify operator
- Daily morning brief (cron 8am — reads audit + agents + operator-model)
- Voice PTT (whisper.cpp local)
- `leah recall <query>` semantic search

### Wave X — never (without explicit re-evaluation)
- Multi-user / SaaS
- Autonomous merge of self-build PRs (operator-merge is the load-bearing safety control)
- Autonomous money movement
- Holographic 3D UI / WebGL

## Risk + adversarial discipline

Self-modifying systems need extra scrutiny. The structural safeguards:

1. **Independent reviewer subagent** on every PR (regatta gate enforces canonical agent-id; Leah dispatches separate Anthropic call)
2. **Operator-attestation question** in self-build PR bodies (Wave 2-G ships this) — forces operator to answer a random question in PR comment before merge
3. **Operator-merge mandatory** — `gh pr merge` is NEVER autonomous for self-build PRs
4. **`leah self-build` repo-locked** to trilamsr/Leah (rejects --repo override)
5. **Build precondition** — SelfBuild won't dispatch if `go build ./...` fails locally first
6. **Rate limit** — max 3 self-build dispatches per 24h (prevents runaway)
7. **Forbidden globs** in self-build prompt — `~/.aws/`, `~/.ssh/`, `~/.netrc`, `os.Environ()` iteration, `go.mod`/`go.sum` direct edits
8. **Audit + obs trail** — every self-build dispatch + every panic + every metric gets recorded; operator can run `leah retro` to see what changed
9. **Specs adversarially reviewed before code** — Wave 1 agents all included self-critique sections; Wave 2-J + 2-K critique their own specs before scaffolding

## Cross-spec links

- Memory v4: `2026-06-09-memory-m2-minimal.md`
- Self-learn: `2026-06-09-self-learning-personal.md`
- Context manager: `2026-06-09-context-manager.md`
- Pattern recognition: `2026-06-09-pattern-recognition.md`
- Self-build: `2026-06-09-self-building-via-regatta.md`
- JARVIS UI: `2026-06-09-jarvis-ui.md`
- Operator model: `2026-06-09-operator-model.md` (Wave 2-J, in flight)
- Observability: `2026-06-09-observability.md` (Wave 2-K, in flight)
- Roadmap: `2026-06-09-roadmap-overview.md`
- Phase X deferrals: `2026-06-09-leah-phase-x-multi-operator-roadmap.md`
- This doc: `2026-06-09-closed-loop-architecture.md`
