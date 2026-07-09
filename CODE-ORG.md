# Leah — Code Organization & Engineering Rules

Twelve sections of binding engineering rules. Every section cites at least one concrete file:line in the current codebase. When a rule appears to conflict with `PRINCIPLES.md`, the principle wins.

Audience: any agent (main session, subagent, CI) and the operator. Rules below override personal style.

---

## 1. Package boundaries

- **One responsibility per package.** `internal/audit` owns the JSONL log. `internal/budget` owns the per-process dollar ceiling. `internal/reasoner` owns the Anthropic call. Do not add a second responsibility to an existing package; spawn a new sibling under `internal/` (see `ARCHITECTURE.md` Package map).
- **No cross-layer back-imports.** The 4-layer stack in `ARCHITECTURE.md` (ACT → DECIDE → REMEMBER → OBSERVE) is one-directional. `internal/audit` must not import `internal/dispatcher`; the dispatcher writes audit rows, never the reverse.
- **Tests next to code.** Every `foo.go` has a sibling `foo_test.go` in the same directory (`internal/audit/audit_test.go`, `internal/budget/budget_test.go`). No `_test/` subdirectories.
- **Internal-only.** Everything lives under `internal/` so the public Go module surface stays empty. Reopen-trigger: an external caller asks for a stable Go API. Until then, nothing is exported across module boundaries.

## 2. Error handling

- **`%w` wraps, `%v` does not.** `internal/audit/audit.go::Append` returns `fmt.Errorf("open audit log: %w", err)`. Callers can `errors.Is` / `errors.As`. Using `%v` here silently drops the cause and breaks downstream type-switch.
- **Typed errors when callers branch.** `internal/dispatcher/selfbuild.go` returns `ErrSelfBuildClarify` so `cmd/leah/selfbuild.go` can distinguish "operator needs to clarify" from "Anthropic 500." Add a typed error the moment a second caller wants to branch.
- **Never swallow.** A function either handles an error (logs + recovers) or returns it. `_ = thing.Close()` in a `defer` is the only acceptable silent ignore, and only when the Close is on a read-only file (`audit.go::Append`).
- **errcheck is mandatory.** `scripts/check.sh` runs `golangci-lint` which runs `errcheck`. The recent `cmd/leah/cost.go` errcheck fix (commit `7b6c227`) is the canonical example of what gets caught.

## 3. Comments

- **WHY not WHAT.** Default to no comment. A clear name and signature need no preface. `internal/budget/budget.go::Budget.Spent` has no body comment because the name and the locked read are self-explanatory.
- **Package doc on the first .go file.** One sentence stating purpose, scoped tightly. Good: `internal/audit/audit.go` opens with "Package audit is Leah's append-only JSONL log of operator-facing actions." Bad: a 20-line ASCII-art block.
- **Exported godoc: symbol-name first AND WHY.** `internal/budget/budget.go::DefaultCeiling` reads `// DefaultCeiling is the fallback per-process $ ceiling when LEAH_BUDGET_DOLLARS is unset or invalid.` Names the symbol; explains why it exists; one sentence.
- **Test godocs: 1 line max.** `// TestFoo asserts O on I.` No multi-paragraph rationale on a `*_test.go` symbol. Rationale belongs in a spec or in the PR body.

## 4. Tests

- **TDD.** Failing test commits before implementation commits. `git log --reverse` on a feature branch shows the RED commit first.
- **Race-clean.** Every test passes under `go test -race ./...`. `internal/budget/budget_test.go` deliberately races two goroutines through `Charge` to prove the mutex holds.
- **No mocks of the thing under test.** `internal/audit/audit_test.go` writes to a real `t.TempDir()` file and reads the bytes back. Mocking `os.OpenFile` here would test the mock, not the package.
- **Table-driven for ≥3 cases.** `internal/reviewer/postreview_test.go` table-drives the `ValidateAgentID` accept/reject cases. One row per shape; one t.Run per row for `-run` filtering.
- **Integration behind build tags.** `internal/reasoner/anthropic_test.go` runs under `-tags integration` because it requires a real `ANTHROPIC_API_KEY`. `scripts/check.sh` skips tagged tests; the operator runs them locally before release.

## 5. CLI design

- **One subcommand per file under `cmd/leah/`.** `cmd/leah/main.go` dispatches; the implementation of `leah ship` lives in a sibling file, not in main. Today the verbs are spread across `selfbuild.go`, `contact.go`, `ctx.go`, etc. — same pattern.
- **`--json` for every read verb.** `leah status --json` already does it (`cmd/leah/main.go::main`). Add `--json` to any new read verb so the operator can pipe into `jq`.
- **`usage()` is the source of truth for help text.** `cmd/leah/main.go::usage` prints the full verb list. Adding a verb without updating `usage()` is a bug, not an omission.
- **Exit codes are stable.** `0` success, `1` runtime error, `2` usage error. `cmd/leah/main.go` uses `os.Exit(2)` for missing args and `os.Exit(1)` for runtime failures. Scripts depend on this; do not invert.

## 6. Subprocess pattern

- **`Executor` interface for every shell-out.** `internal/ghclient/ghclient.go::Executor` and `internal/notify/desktop.go::Executor` are the canonical shape: one method, `Run(ctx, args...) ([]byte, error)`. Production impl `ShellExec` calls `exec.CommandContext`; test impl is a struct literal.
- **Tests use the fake.** `internal/ghclient/ghclient_test.go` never invokes `gh`. The fake records args and returns a fixture. CI passes without `gh` installed.
- **Capture stderr on ExitError.** Wrap with the command name so the operator sees `gh issue create: <stderr>`, not a bare `exit status 1`.
- **Context-bounded.** Every subprocess call takes a `context.Context` and uses `exec.CommandContext`. No bare `exec.Command` — leak risk on daemon shutdown.

## 7. State on disk

- **Everything under `$LEAH_STATE_DIR` (default `~/.leah-state/`).** Directory is mode 0700. Files are mode 0600. `internal/audit/audit.go::Append` opens with `0o600` explicitly. No state outside this root.
- **SQLite via `modernc.org/sqlite` (pure Go, no cgo).** `internal/memory/memory.go` imports `modernc.org/sqlite`; this keeps `go build` cgo-free and reproducible. Adding a cgo dep requires operator signoff.
- **JSONL for append-only.** Audit, daily logs (`internal/obs/rotator.go`), and any future event stream. Newline-delimited JSON, one record per line, no array brackets.
- **Schema migrations are idempotent.** `internal/memory/schema.sql` uses `CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS`. Version gate via `schema_meta` row: read current version, apply only the deltas needed. Never destructive on startup.
- **ULIDs for primary keys.** `internal/memory/memory.go::newULID` wraps `oklog/ulid`. Time-ordered, lexicographically sortable, no central registry.

## 8. Adopt over build

- **Check the survey first.** `docs/research/2026-06-09-adopt-vs-build-survey.md` is the registry of "did we already evaluate this?" Adding a new dep or wrapping a new subprocess requires a row.
- **Wrap subprocesses; do not reimplement.** `internal/ghclient` wraps `gh`; `internal/regattaclient` wraps `regatta agents list`; `internal/voice` wraps `kokoro`. Each wrapper is ≤200 LOC and tested against the `Executor` fake.
- **Thin Go shim.** A wrapper exposes only the verbs Leah uses. `ghclient` does not surface every `gh` subcommand — just `CreateIssue`, `ViewPR`, `ListPRsForBranch`. New verb = new method, not a "generic gh runner."
- **Pin versions.** `go.mod` pins `anthropic-sdk-go v1.49.0`, `oklog/ulid/v2 v2.1.1`, `modernc.org/sqlite v1.52.0`. Bumps go through `golangci-lint` and the full test suite; no auto-bump.

## 9. Concurrency

- **`sync.Mutex` over channels for shared state.** `internal/budget/budget.go::Budget` uses `sync.Mutex` to serialize `Charge`. Channels are for cross-goroutine signaling, not protecting a field.
- **One goroutine for cron.** `internal/daemonloop` runs one ticker per cadence. No parallel "weekly resolver" + "weekly retro" goroutines — they fire in sequence on the same tick.
- **`obs.SafeGo` for every new goroutine.** `internal/obs/recover.go::SafeGo` recovers panics, writes a stack trace to `panics/<ts>.txt`, and logs an ERROR. Bare `go fn()` is a bug; the daemon dies silently otherwise.
- **`go test -race ./...` on every commit.** Run locally before push. Not yet wired into `scripts/check.sh` (today: plain `go test ./...`); reopen-trigger to add `-race` is the first race-bug that lands on `main`. A race detected locally is a real defect; do not retry around it.

## 10. Observability

- **`slog` on load-bearing transitions.** Log on dispatch entry, terminal-state observation, and error returns. Operator can grep one structured field in `~/.leah-state/logs/leah-YYYY-MM-DD.jsonl` and reconstruct a session.
- **`obs.Registry` for counters.** `internal/obs/metrics.go::Registry.Counter` returns the shared in-process counter for a name; no Prometheus dep, no hidden globals.
- **Trace correlation via `obs.WithTrace(ctx, traceID)`.** `internal/obs/logger.go::WithTrace` returns a child context carrying the trace ID; downstream slog calls read it back via `TraceID(ctx)`. Every CLI invocation can stamp one ULID at `main` so the daily log is greppable per-session.
- **Panic file is the source of truth for crashes.** `~/.leah-state/panics/<ts>-<goroutine-name>.txt`. `internal/selflearn` (planned) reads these to draft self-bug issues.

## 11. Security

- **No secrets in code.** `.env` is gitignored. Anthropic key reads from `ANTHROPIC_API_KEY`. Pushover key reads from `PUSHOVER_*`. Future secrets go through the OS keychain (`security` on macOS) when possible.
- **Redact LLM bodies in audit.** `internal/audit/audit.go::Entry` does not have a `prompt_body` field. Bodies live in the slog stream gated by `LEAH_LOG_PROMPTS=1` (default off). Audit rows show `args_hash`, not args.
- **Self-build never touches forbidden globs.** `prompts/*.md`, `go.mod`, `go.sum`, `.github/`, `.env*`, `~/.ssh/`, `~/.aws/`, `~/.npmrc`, `$HOME/.netrc`, and `os.Environ()` iteration are off-limits to self-build PRs (see `prompts/self-build-feature.md` §"Allowed prefixes" + §6 clarify-trigger list). Reviewer subagent re-checks; pre-merge `git diff` operator scan is the backstop.
- **Loopback only by default.** `internal/web` binds `127.0.0.1`. The daemon does not listen on `0.0.0.0` or any LAN interface unless `--bind` is passed explicitly. Reopen-trigger: operator hosts Leah on a Tailscale-style mesh; until then, 127.0.0.1.

## 12. Commit hygiene

- **Subject ≤50 chars, Conventional-Commits-style.** `feat(pkg): ...`, `fix(pkg): ...`, `docs(pkg): ...`, `chore: ...`, `test(pkg): ...`, `refactor(pkg): ...`. Recent examples: `fix(lint): errcheck — _, _ = on cost.go fmt.Fprintf` (`7b6c227`), `fix(web): TTL-cache Snapshot to absorb 3s dashboard poll cost (H4)` (`17b3ea9`).
- **Body explains WHY.** The diff shows WHAT. The body says "the dashboard polled every 3s and re-walked the audit log; TTL cache cuts walk cost by N%." If the WHY is obvious from the subject, omit the body.
- **No AI signatures.** No `Co-Authored-By: Claude`, no `🤖 Generated with ...`, no "written by Claude" anywhere. `CLAUDE.md` already states this; this section is the reminder.
- **One concern per commit.** A `fix:` commit does not also re-format an unrelated file. Re-formatting belongs in its own `chore: gofmt` commit. Makes `git log --oneline` legible and `git revert` safe.
- **Push from worktrees only.** Primary checkout stays on `main` and is read-only (`git fetch`, `git log`, `git status`). Feature work lives in `.claude/worktrees/agent-<id>/` per `CLAUDE.md` Worktree discipline.

---

## How to use this doc

- **Writing new code.** Read the section for the surface you are touching. Cite the rule number in the PR body if the choice is non-obvious.
- **Reviewing a PR.** A finding cites the section number (`violates §2: %w dropped, cause unrecoverable`). Reviewer comment-sweep checks §3 (comments) on every load-bearing PR.
- **Adding a new rule.** Same procedure as `PRINCIPLES.md`: requires the operator's signoff AND a concrete repo example. Default is to fold into an existing section.
