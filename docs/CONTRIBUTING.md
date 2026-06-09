# Contributing to Leah

Personal-use repo — this is mostly a note to future-Tri. Read [`CLAUDE.md`](../CLAUDE.md) first; what's below is the working-author counterpart.

## Repo layout

```
.
├── cmd/
│   ├── leah/             # CLI entrypoint + per-verb files (ask, ship, review, ctx, ...)
│   └── leah-daemon/      # Always-on poller + dashboard host + weekly tick
├── internal/             # 18 packages — see ARCHITECTURE.md for the full map
│   ├── audit/            # Layer 1 — JSONL action log
│   ├── obs/              # Layer 1 — slog + metrics + panic recover
│   ├── budget/           # Per-process $ ceiling
│   ├── memory/           # Layer 2 — schema.sql owner (v4)
│   ├── ctxmgr/           # Layer 2 — single-active-context
│   ├── selflearn/        # Layer 3 — resolver / mistake / retro
│   ├── patterns/         # Layer 3 — audit clusterer
│   ├── operatormodel/    # Layer 3 — behavior observer + recommender
│   ├── dispatcher/       # Layer 4 — ask / ship / selfbuild / status
│   ├── reviewer/         # Layer 4 — independent subagent + canonical-id gate
│   ├── reasoner/         # Anthropic-backed main LLM
│   ├── ghclient/         # gh CLI wrapper
│   ├── regattaclient/    # regatta CLI wrapper
│   ├── daemonloop/       # per-30s + weekly tick engine
│   ├── notify/           # macOS desktop + Pushover
│   ├── watchdog/         # healthchecks.io ping
│   ├── intent/           # regex verb classifier
│   └── web/              # JARVIS dashboard (loopback only)
├── docs/
│   ├── specs/            # Authoritative design docs (closed-loop, memory, selflearn, ...)
│   ├── plans/            # Phase plans (MVP-5, etc.)
│   └── research/         # Adopt-vs-build surveys
├── prompts/              # Reasoner system prompts
├── reviewer-prompts/     # Independent reviewer subagent prompts (separate dir per independence-principle)
├── scripts/
│   ├── check.sh          # Pre-push: build + test + vet + lint
│   ├── healthcheck-setup.sh
│   └── leah.plist        # launchd template
├── ARCHITECTURE.md       # Single-file architecture overview
├── CLAUDE.md             # Agent operating rules (universal)
└── README.md             # Operator runbook
```

## TDD discipline (per CLAUDE.md `feedback_tdd_discipline`)

Failing test FIRST. Order matters; the commit log must show the failing test landed before the impl. Concretely:

1. Write the test against the not-yet-existing behavior. Run it. Capture the failing output.
2. Commit the failing test (or stage it locally) with the failing output in the commit body or PR description.
3. Write the minimum impl. Run the test green.
4. Commit the impl with the now-passing reference.

Tests for selflearn, patterns, operatormodel, ctxmgr, memory are the load-bearing examples — every public function has at least one table-driven case.

Skip TDD only when the change is a pure doc edit, dep bump, or `[CHORE]` shuffle with no behavior delta.

## Independent reviewer subagent (per CLAUDE.md TDD+review section)

Any load-bearing change ships with an adversarial review pass. Load-bearing = anything under `internal/dispatcher`, `internal/selflearn`, `internal/memory`, `internal/reviewer`, `internal/obs`, or any change to the schema / `prompts/` / `reviewer-prompts/`.

- Spawn a fresh subagent (separate session) and pass it the diff + the linked spec.
- Reviewer recommendation: APPROVE / REVISE / BLOCK on a single line, plus a canonical agent-id matching `^(a[0-9a-f]{16}|(cavecrew-reviewer|designer|triage|implementer|reviewer)-[a-z0-9]+(-[a-z0-9]+)*)$`.
- NEVER write your own APPROVE — author writing own approval is zero adversarial pass (CLAUDE.md `feedback_no_self_tagged_approve`).
- HIGH / MED findings: fix inline or file a tracker issue + cite # before merging.

Mechanical reviewer (Leah itself): `leah review trilamsr/Leah <PR#>` dispatches the Anthropic subagent against the diff for self-build PRs. Use it on every self-build before the operator-merge step.

## Commit message conventions

Match the existing log:

```
<type>(<pkg>): <imperative summary>

<optional body explaining why, not what>
```

Types observed: `feat`, `fix`, `refactor`, `docs`, `chore`, `ci`.

Examples from `git log`:

- `feat(obs): structured logs + metrics + panic recovery (Wave2-K)`
- `fix(ui): consistent budget format + status header + detail truncation`
- `refactor(memory,selflearn): consolidate mistake_log into memory schema v3`
- `docs(specs): closed-loop architecture meta-doc`
- `chore: gitignore .env (secrets local-only)`
- `ci: bump golangci-lint-action v6 → v7`

Multi-pkg changes: comma-separate inside the parens (`refactor(memory,selflearn): ...`). No package qualifier for repo-wide `chore` / `ci`.

Subject under 72 chars. Body wraps at 80. Use the body for the "why" — the diff already shows the "what".

## Pre-push

```bash
./scripts/check.sh
```

That runs `go build ./...`, `go test ./...`, `go vet ./...`, and `golangci-lint run ./...` if installed. Fix everything before pushing — there is no second-chance CI cleanup loop. Integration tests live under `-tags integration` and need `ANTHROPIC_API_KEY`; run them when changes touch `internal/reasoner` or `internal/reviewer`.

## No AI signatures (per CLAUDE.md `feedback_no_signatures`)

Zero `Co-Authored-By: Claude`, zero `🤖 Generated with`, zero "written by Claude" anywhere — commits, PR bodies/titles, code comments, generated docs. The output is yours; sign it yourself or not at all.

## Pointers

- Architecture map: [`../ARCHITECTURE.md`](../ARCHITECTURE.md)
- Agent operating rules (universal): [`../CLAUDE.md`](../CLAUDE.md)
- Design specs: [`./specs/`](./specs/)
- Roadmap: [`./specs/2026-06-09-roadmap-overview.md`](./specs/2026-06-09-roadmap-overview.md)
- Phase X deferrals: [`./specs/2026-06-09-leah-phase-x-multi-operator-roadmap.md`](./specs/2026-06-09-leah-phase-x-multi-operator-roadmap.md)
- Operator runbook: [`../README.md`](../README.md)
