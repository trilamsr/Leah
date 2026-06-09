# Leah — Operating Principles

Twelve principles. Every decision (feature scope, design, code-org, dispatch, merge) is checked against these. When two principles collide, the one earlier in this list wins.

The first principle is the anchor: **operator is the customer**. Every other principle bends to operator UX. If a principle below ever appears to contradict #1, re-read #1.

---

## 1. Operator is the customer

Leah has exactly one user: the operator. Single-tenant forever. Every feature passes the test "does the operator need this to keep dispatching their real work unattended?" — if no, defer or delete.

- **Example.** `cmd/leah/main.go` is loopback-only by default; `internal/web` binds `127.0.0.1` (`docs/specs/2026-06-09-jarvis-ui.md`). Multi-user auth never landed because no operator needs it.
- **Anti-example.** Adding `tenant_id` to `internal/memory/schema.sql`, a billing webhook, or an "invite a teammate" CLI flag. All three are explicitly Phase-X per `docs/specs/2026-06-09-leah-phase-x-multi-operator-roadmap.md`.
- **Enforcement.** When a spec or PR introduces a Phase-X token, the operator rejects on review. No mechanical gate yet — first repeat triggers `scripts/check-phase-x-leak.sh` (port from regatta).

## 2. Default simpler

Pick the simplest viable option. Three similar lines beat a premature abstraction. Do not pre-build interfaces, tier systems, or lint scripts for drift that has not happened.

- **Example.** `internal/audit/audit.go::Logger.Append` re-opens the JSONL file every call instead of holding a long-lived `*os.File`. O_APPEND syscall serialization is the locking primitive. Two fewer fields, zero close-on-exit logic.
- **Anti-example.** Wrapping `audit.Logger` in a `Sink` interface with `JSONLSink`, `S3Sink`, `KafkaSink` impls because "we might want remote audit later." Wait for the second sink; build the abstraction then.

## 3. Reversibility shapes risk

Every action carries a blast radius (0 read-only, 1 local-state write, 3 external write, 4 self-modifying). Higher blast radius requires more friction: confirmation, attestation, operator-merge, or refusal.

- **Example.** `ARCHITECTURE.md` CLI table assigns `leah ask` BR=0, `leah ship` BR=3, `leah self-build` BR=4. `internal/dispatcher/selfbuild.go` adds an operator attestation footer that must be answered in a PR comment before merge (see `prompts/self-build-attestations.txt`).
- **Anti-example.** Letting `leah self-build` open a PR and enable automerge in the same call. Self-modifying + zero operator window = unbounded blast radius.

## 4. Independent review for self-modification

Code that modifies Leah is never reviewed by the agent that wrote it. A fresh-slot reviewer subagent runs before merge and its agent-id is verifiable.

- **Example.** `internal/reviewer/postreview.go::ValidateAgentID` rejects any reviewer-id that does not match the canonical regex `^(a[0-9a-f]{16}|cavecrew-reviewer-[a-z0-9-]+)$`. Self-tagged tokens like `self` or the operator login fail closed.
- **Anti-example.** The author of a PR pasting `Reviewer-recommendation: APPROVE` into their own PR body. Mechanically rejected by the regex; culturally rejected because the verdict is meaningless when the prompt and the grader share a context.

## 5. Adopt before build

Before writing a new package, check whether an existing CLI or library already solves the problem. Prefer wrapping a subprocess over re-implementing in Go. Cite the version, license, and the survey row that justified the choice.

- **Example.** `internal/ghclient/ghclient.go` and `internal/regattaclient` shell out to `gh` and `regatta` instead of vendoring their Go libraries. `internal/voice` shells out to `kokoro` instead of porting the TTS model. `docs/research/2026-06-09-adopt-vs-build-survey.md` is the survey.
- **Anti-example.** Re-implementing `gh issue create` against the GitHub REST API "for type safety." The wrapper is 30 lines and behaves identically to operator muscle memory; the rewrite is 300 lines, two more deps, and one more auth flow to keep happy.

## 6. Earn complexity

Every interface ships with at least two real implementations (production + test fake) AND a caller that distinguishes between them. Single-impl interfaces with no test fake get inlined.

- **Example.** `internal/notify/desktop.go::Executor` interface has `ShellExec` (production, calls `osascript`) and is stubbed by a fake `Executor` in tests so the test does not actually shell out. The interface earns its keep.
- **Anti-example.** Defining a `Clock` interface with one `time.Now()` method and one impl, used only in two files. Inline `time.Now()` or pass a `Now func() time.Time` field (the pattern `audit.Logger` uses).

## 7. Audit everything; redact what crosses the wire

Every operator-facing action emits one JSONL row in `audit.jsonl`. Every cloud call gets the body redacted in the audit row — the audit log answers "what did Leah do," not "what did the LLM see."

- **Example.** `internal/audit/audit.go::Entry` records `Kind`, `ArgsHash` (not raw args), `BlastRadius`, `Outcome`, `CostDollars`, `Detail`. The reasoner prompt body is not stored in the audit row; it lives only in the slog stream gated by `LEAH_LOG_PROMPTS`.
- **Anti-example.** Logging the full Anthropic request payload into `audit.jsonl`. The file is mode 0600 but it's still on local disk forever, and the operator reads it during retros — secrets do not belong there.

## 8. TDD before merge

Write the failing test first. Commit log on every load-bearing PR shows the RED commit before the GREEN. No mocks of the thing under test.

- **Example.** `internal/budget/budget_test.go` exercises real `Charge` calls and asserts on `Spent()` — no mock budget. `internal/audit/audit_test.go` writes to a temp file and reads it back, asserting on the bytes — no mock filesystem.
- **Anti-example.** A "test" for `dispatcher.SelfBuild` that mocks `Reasoner`, `GHClient`, and `RegattaClient` all at once — the only thing exercised is the wiring, not the behavior. Replace with a thin integration test using fakes that produce realistic bytes.

## 9. Cost is observable

Every cloud call charges the per-process `budget.Budget`. The dollar cost appears in the audit row and is summed in the weekly retro. The operator can answer "did this week cost more than it produced?" from one file.

- **Example.** `internal/budget/budget.go::Budget.Charge` serializes on a mutex; `internal/reasoner/anthropic.go` calls `Charge` on every successful response. `cmd/leah/cost.go` aggregates the audit-row dollar column.
- **Anti-example.** A new LLM call site that skips `budget.Charge`. Even if the operator pays the bill, the retro silently under-reports spend and the per-process ceiling no longer holds.

## 10. Operator-merge mandatory on self-build

Self-build PRs never automerge. The operator answers an attestation question in a PR comment before merge. The attestation is sampled from `prompts/self-build-attestations.txt` so the same question is not memorized.

- **Example.** `internal/dispatcher/selfbuild.go::pickAttestationQuestion` rotates the question per dispatch. The PR body's "Operator merge attestation" footer names the GitHub login that must answer.
- **Anti-example.** `leah self-build` calls `gh pr merge --auto` after issue creation. Zero operator window between APPROVE and merge = independent review is decorative.

## 11. Cite the shared state, not the local worktree

Every repo-internal reference in a spec, brief, or PR body resolves against `git ls-tree origin/main` — never the author's dirty worktree. Numeric and version claims are paired with the exact command that produced them.

- **Example.** `ARCHITECTURE.md` Package map cites packages that exist on `main`, not a half-merged feature branch. When a spec says "N internal packages," the next line shows the exact command — e.g. `ls internal/ | wc -l` → `24` at HEAD `7b6c227`.
- **Anti-example.** A spec citing `internal/foo/` that only exists in the author's local worktree. The next operator clones a fresh repo, the link 404s, the spec is unverifiable.

## 12. Specs describe current state, forward-looking only

Specs document what is true now and what is planned next. They do not narrate the history of how the design got here ("Wave 1-A landed X then Wave 2-G changed Y"). History lives in `git log` and in dated `docs/research/*.md` retro files.

- **Example.** `ARCHITECTURE.md` is one snapshot of "where things live today," with no wave numbers in section headers. Retros under `docs/research/2026-06-09-*-retro-audit.md` carry the historical narrative.
- **Anti-example.** A spec that opens with "Originally Wave 1 shipped this as a Sink interface; Wave 2 collapsed it; Wave 3 re-expanded it." The reader has to mentally diff three revisions to learn what is true now. Move that to the retro; keep the spec one-shot readable.

---

## How to use these principles

- **Designing a feature.** Walk through 1–12 in order. The first principle that says "no" wins. Document the rejection.
- **Reviewing a PR.** Same walk. A finding cites the principle number (`violates §3: BR=4 action with no attestation gate`).
- **Adding a new principle.** Requires the operator's signoff AND a worked example that the existing 12 fail to cover. Default is to fold the new lesson into an existing principle's body rather than expand to 13.
