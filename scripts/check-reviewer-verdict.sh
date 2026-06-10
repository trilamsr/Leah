#!/usr/bin/env bash
# check-reviewer-verdict.sh — fail when a load-bearing PR body does not
# carry a `Reviewer-recommendation: APPROVE` token paired with an
# independent `Reviewer-agent-id:`.
#
# Originated in regatta; ported to leah 2026-06-10. The drift pattern
# this closes is observed in leah PR #222 (confluence) and PR #224
# (facetime) where an agent self-merged a load-bearing PR with no
# independent adversarial pass — CLAUDE.md "TDD + review" mandates
# reviewer on every load-bearing PR but enforcement was operator
# discretion. This gate makes the rule machine-enforced.
#
# Gate inputs:
#   --pr <number>           fetch body via `gh pr view`
#   --body-file <path>      read body from a local file (CI + fixtures)
#   --load-bearing          treat the PR as load-bearing (CI also flags
#                           when the changed-paths heuristic matches)
#   --changed-paths-file <path>  newline-delimited file of changed paths.
#                           Any path matching the load-bearing list
#                           sets load-bearing=1 AND bypasses the
#                           release-notes category auto-skip.
#   --skip                  short-circuit pass (operator-discretion escape)
#   --pr-author <login>     PR author login. When provided AND the PR
#                           carries Reviewer-recommendation: APPROVE on a
#                           load-bearing surface, the script also enforces
#                           that a bare `Reviewer-agent-id:` line exists AND
#                           its value differs from <login>.
#   --automerge-enabled     signals that `autoMergeRequest != null` on the
#                           PR. When the PR is also load-bearing AND
#                           `Reviewer-agent-id:` is present, the gate
#                           fails closed — agent-written APPROVE plus
#                           agent-enabled automerge leaves zero operator
#                           window between APPROVE-token landing and merge.
#
# Operator escape (rare): include `<!-- reviewer-skip-justified: <reason
# ≥4 chars> -->` in the PR body to bypass the self-tag mismatch check
# for trivial doc/typo/dep-bump cases. The token-present check still
# runs — the escape only relaxes author ≠ Reviewer-agent-id.
#
# Operator-opened bypass: when the PR body carries
# `<!-- operator-opened: <reason ≥4 chars> -->`, the author IS the
# operator by definition; the author-mismatch check would always fail.
# Still requires a Reviewer-agent-id from the canonical allowlist.
#
# Auto-skip: release-notes prefix in [CHORE]/[DOCS]/[CI]/[NONE]/[CHANGE].
# NOT applied when --changed-paths-file flags a load-bearing surface
# above — those surfaces are themselves the category being reviewed.
#
# Pass: body contains `Reviewer-recommendation: APPROVE` ON ITS OWN
# line (case-insensitive, leading/trailing whitespace allowed). Tokens
# inside ```-fenced code blocks are stripped before the scan so stale
# examples or draft snippets cannot beat the bare footer token.
#
# Fail: token absent OR equals REVISE/BLOCK when --load-bearing.
#
# Exit codes:
#   0 pass / category-exempt / not load-bearing.
#   1 missing token on load-bearing PR.
#   2 REVISE / BLOCK recommendation.
#   3 usage error.

set -uo pipefail

RV_LIB_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/lib/reviewer-verdict"
# shellcheck source=lib/reviewer-verdict/args.sh
. "$RV_LIB_DIR/args.sh"
# shellcheck source=lib/reviewer-verdict/body-source.sh
. "$RV_LIB_DIR/body-source.sh"
# shellcheck source=lib/reviewer-verdict/path-classifier.sh
. "$RV_LIB_DIR/path-classifier.sh"
# shellcheck source=lib/reviewer-verdict/category-skip.sh
. "$RV_LIB_DIR/category-skip.sh"
# shellcheck source=lib/reviewer-verdict/token-extract.sh
. "$RV_LIB_DIR/token-extract.sh"
# shellcheck source=lib/reviewer-verdict/verdict.sh
. "$RV_LIB_DIR/verdict.sh"

rv_parse_args "$@"

if [ "$SKIP" -eq 1 ]; then
  exit 0
fi

rv_resolve_body
rv_classify_paths
rv_category_skip
rv_extract_tokens
rv_decide_verdict
