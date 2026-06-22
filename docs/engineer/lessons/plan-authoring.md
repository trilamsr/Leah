# Plan task order must respect symbol dependencies

If task N references a type/function defined in task N+k (k>0), parallel
dispatch fails with "undefined: X" diagnostics when worktrees compile.
Plan-authoring agents sometimes order tasks by feature (router, streaming,
handler) without verifying the symbol graph. Each task's "Interfaces
(Consumes)" block must list ONLY symbols from earlier tasks. Verify by
grepping the brief for symbol names against the symbol table of brief
1..N-1 before locking order.

Anchor: Phase 1 Task 4 (Haiku router) consumed `AnthropicClient`,
`CompleteResult`, `buildSystemBlock`, `RecordCacheOutcome` which Task 3
(Sonnet streaming) defines. Task 4 ran first → compile errors caught only
by reviewer. See `.superpowers/sdd/progress.md` Phase 1 ledger.
