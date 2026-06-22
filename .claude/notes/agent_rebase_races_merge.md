# Agent rebase + push races mid-flight auto-merge

A finalizer agent (a64f4bca) rebased PR 340's branch to inject 3
extra lesson files, then pushed — but PR 340 had already squash-
merged at 04:57:26Z. The rebased branch became orphan. Finalizer
correctly recovered by opening PR 345 with the 3 extra files off
main.

Why: agents that rebase-then-push to an in-flight PR branch race
the auto-merge resolution. Auto-merge wins, push lands on a branch
GH no longer cares about. Recovery costs an extra PR cycle.

How to apply: while a PR has auto-merge armed and CI green, prefer
commit-on-top-of-origin/<branch> over rebase. Only rebase when CI
demands it (DIRTY conflict). For multi-commit additions during the
in-flight window, branch off origin/main and open a fresh PR
instead of mutating the in-flight branch.

Anchor: PR 340 merged at 2026-06-22T04:57:26Z; finalizer agent
(a64f4bca8471cb0c4) attempted to push to its already-merged
branch and correctly opened PR 345 off main as recovery. Grep
regression check: search session transcript for `git push` events
that come after a `gh pr view --json state` showing MERGED.
