# Scoping subagent must verify against current tree, not memory

A general-purpose "next-wave scoper" returned a dispatch table inventing
`connect-linear`, `connect-notion`, `connect-slack`, `connect-jira` as
new providers — all four were already shipped via the `newTokenPaste`
substrate at `internal/connect/registry.go:26-31`. The scoper read
research docs + memory but never grepped `registry.go` for existing
provider rows.

Why: subagent prompts that lean on memory/docs without forcing a
`grep <symbol>` against the current tree produce phantom dispatch
candidates. Cost: 1 wasted worktree spawn + 1 hollow "already shipped"
return.

How to apply: any scoping prompt MUST include an explicit
"grep/ls/read [target file] FIRST before listing candidates" step,
and require evidence-of-absence per candidate (e.g. "registry.go does
not contain X").

Anchor: `internal/connect/registry.go:26-31` lists confluence / jira /
slack / notion / linear / msteams as `newTokenPaste(...)` rows. Grep:
`grep -E "newTokenPaste|name:" internal/connect/registry.go`.
