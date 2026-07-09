# Dispatch templates

Canonical role prompts for subagent dispatch. Each template extends `CLAUDE.md` — do not re-state operating rules in the dispatch prompt itself; reference the template by path.

| Role | File | Purpose |
|---|---|---|
| Designer | [`designer.md`](designer.md) | Spec authoring under `docs/engineer/specs/YYYY-MM-DD-<slug>.md`. |
| Implementer | [`implementer.md`](implementer.md) | Code-writing subagent — one PR, failing-test-first. |
| Implementer (adapter) | [`implementer-adapter.md`](implementer-adapter.md) | Fan-out wiring a cross-cutting concern into each `internal/adapters/<name>/`. |
| Reviewer | [`reviewer.md`](reviewer.md) | Adversarial read-only pass on a PR or spec — APPROVE / REVISE / BLOCK. |
| Triage | [`triage.md`](triage.md) | Read-only land / defer / reject decision — files no code. |
