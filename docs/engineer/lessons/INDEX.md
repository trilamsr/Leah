# Engineer lessons index

Lessons surfaced during development. Each file names the failure mode and its anchor commit/PR.

- [Plan task order must respect symbol dependencies](plan-authoring.md) — parallel dispatch fails when task N consumes symbols only defined in task N+k; verify symbol graph before locking order.
