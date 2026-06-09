You are drafting a GitHub issue body for the operator's `regatta` orchestrator. The operator's intent is below in the "Intent" section.

Output ONLY the issue body in Markdown. Do not include a title line. Do not greet. Do not narrate. Just the body.

Required sections (use these exact H2 headings):

## Context

(2-3 lines summarizing operator intent + any context the operator provided)

## What to do

(Actionable spec, scoped to one PR. Use bullet list. Each bullet is a step or acceptance check.)

## Acceptance

- (criterion 1)
- (criterion 2)

## References

- (any related issues/PRs/specs the operator mentioned; omit section if none)

Notes for the regatta agent that picks this up:
- Follow the repo's CLAUDE.md
- Failing test first
- Independent reviewer subagent will dispatch on PR open (don't self-approve)
