---
name: audit-session
description: End-of-session audit + handoff for any agent operating in the leah repo. Use when the user says "audit session", "end session", "wrap up", "before we stop", "what did we miss", "before signing off", or any phrasing that asks Claude to validate the session's work before exit. Runs Phase 0 (cross-session handoff continuity check) + 9 main phases (PR audit, reviewer-comment audit, issue audit, doc audit, code audit + orphan scan, worktree cleanup, learning + memory, cost + budget, NEXT-SESSION HANDOFF) + A1/A2 (roadmap + autonomy-lever, with operator-redirect count) and writes a single consolidated handoff file the next session reads to pick up exactly where this one left off. Default = silent pass per phase; ONE operator hand-back at end. Auto-file ONLY mechanically-derivable trackers (parity, self-tag, REVISE-slip, self-approve-after-amend). Phase 5 carves out instrumentation fan-out PRs from the quality-unverified scan and runs an orphan-package scan (catches v3.3.0-style inert-ship: packages merged but never wired into the composition root). Phase 7 cross-refs the learn-from-mistakes skill — surfaces unsaved learnings if pushback/rollback events fired without that skill activating.
---

# audit-session

End-of-session validator + handoff. Catches what slipped, codifies what was learned, writes the next session's pickup file.

## Activation

**Explicit.** "audit session" / "end session" / "wrap up" / "before we stop" / "before signing off" / "what did we miss" / `/audit-session`.

**Implicit.** Self-offer (don't start) when: "I'm done" / "back tomorrow" / ≥30min idle + ≥1 PR open / harness session-end signal.

## Inputs

- `$SESSION_START` — ISO. Default 8h ago.
- `$GIT_AUTHOR` — git `user.email`.
- `$HANDOFF_DIR` — `.claude/session-handoffs/`. Created if missing. Override via `SESSION_HANDOFF_DIR` env var. Next-session BOOT step reads from the same path; both sides must use the same `$HANDOFF_DIR` resolution. Path is gitignored (operator-local handoffs).

## Default behavior

- 9 phases sequential.
- Phase 0 + 9 main phases sequential.
- Silent per phase when clean; one line + action on finding.
- After phase 9: ONE consolidated hand-back (≤30 lines).
- Auto-file ONLY mechanically-derivable trackers. Never auto-close, auto-merge, auto-edit CLAUDE.md.

## Phase-completion ledger (init)

Failure mode addressed: skill caller has been printing the consolidated hand-back after running ~40% of phases. The summary is a deliverable, so a free-form "audit complete" claim is a falsifiable overclaim. Fix: ledger file, scored at hand-off boundary, blocks the summary until every phase is marked `done` or `skipped:<reason>`.

```bash
mkdir -p "$HANDOFF_DIR"
ledger="$HANDOFF_DIR/phase-ledger.txt"
: > "$ledger"
for p in 0 1 2 3 4 5 5_5 6 7 8 9 A1 A2; do
  printf 'phase%s=pending\n' "$p" >> "$ledger"
done
```

Each phase block ends with a portable `mark_done <N>` invocation (defined below). A deliberately skipped phase MUST be marked `phase<N>=skipped:<reason>` instead of `=done`.

```bash
# Portable: avoid sed -i differences across BSD/GNU. Rewrites the line in place.
mark_done() {
  local n="$1" ledger="$HANDOFF_DIR/phase-ledger.txt" tmp
  tmp=$(mktemp)
  awk -v n="$n" '$0 ~ "^phase" n "=pending$" { print "phase" n "=done"; next } { print }' \
    "$ledger" > "$tmp" && mv "$tmp" "$ledger"
}

# Intentional skip — completion gate accepts =skipped:<reason> the same as =done.
# Without this helper, a skipped phase left untouched would block summary emission
# and force the caller to hand-edit the ledger, defeating the structural guarantee.
mark_skipped() {
  local n="$1" reason="${2:-unspecified}" ledger="$HANDOFF_DIR/phase-ledger.txt" tmp
  tmp=$(mktemp)
  awk -v n="$n" -v r="$reason" '$0 ~ "^phase" n "=pending$" { print "phase" n "=skipped:" r; next } { print }' \
    "$ledger" > "$tmp" && mv "$tmp" "$ledger"
}
```

## Phase 0: cross-session continuity

Per session retrospective 2026-06-10: the prior session wrote `2026-06-10T21-skill-test-handoff.md` but the current session never opened it. Handoff written ≠ handoff read; the audit-session contract was satisfied while the actual continuity goal slipped.

```bash
last_handoff=$(ls -t "$HANDOFF_DIR"/*-session-handoff.md 2>/dev/null | head -1)
if [ -z "$last_handoff" ]; then
  exit 0  # no prior handoff — first session in this dir, silent pass.
fi
# Was the handoff read this session? Search transcript / conversation for the filename.
basename=$(basename "$last_handoff")
if [ -n "${CLAUDE_TRANSCRIPT:-}" ] && [ -r "$CLAUDE_TRANSCRIPT" ] && grep -q "$basename" "$CLAUDE_TRANSCRIPT"; then
  exit 0  # handoff filename referenced — assume read, silent pass.
fi
# Cannot prove it was read. Surface in hand-back (per Hard Nos: no auto-file).
echo "unread_prior_handoff=$last_handoff" >> "$HANDOFF_DIR/phase0-flags.txt"
```

Silent pass when no prior handoff exists OR transcript shows it was referenced. Surface in hand-back ONLY when prior handoff exists and was NOT referenced. Never auto-file an issue (per Hard Nos `NO auto-file on uncertain detection`).

`mark_done 0`

## Phase 1: PR audit

```bash
gh pr list --author "@me" --search "updated:>=$SESSION_START" \
  --json number,state,mergedAt,mergeStateStatus,statusCheckRollup,body -L 50 \
  > "$HANDOFF_DIR/prs.json"
```

Rules:
- `mergedAt==null AND state==OPEN` → log + offer operator merge. NOT auto-merge.
- `state==MERGED AND body~/Reviewer-recommendation: (REVISE|BLOCK)/` → file `[SESSION-AUDIT][post-merge] PR#<N>`.
- `mergeStateStatus IN (BLOCKED,DIRTY,UNSTABLE)` → file `[SESSION-AUDIT][automerge-stall] PR#<N>`.
- `Reviewer-agent-id:` matches PR author login → file `[SESSION-AUDIT][self-approve-leak]` per CLAUDE.md "Never self-approve".
- **Self-approve-after-amend (binding gate, per `feedback_no_self_approve_after_edits` + S5 reflexion-loop spec `docs/engineer/specs/2026-06-10-reflexion-loop.md`).** The canonical verdict set is `clear-to-merge` | `block-on-findings` | `re-spawn-design`. Verdict strings appear in TWO surfaces — (a) inline review/comment bodies on GitHub, AND (b) this session's transcript when reviewers were spawned as inline Agent tool calls whose output never reached GitHub. **Both surfaces must be scanned** — defaulting to (a) alone silently passes reviewer-runs that never posted (the common case in this codebase).

  Per-PR algorithm:

  ```bash
  events=()  # each: <iso-timestamp>\t<verdict>\t<agent-id-or-source>

  # Source (a) — GitHub review/comment bodies, timestamped by submittedAt/createdAt.
  jq -r '
    (.reviews // [])[] | select(.body | test("block-on-findings|clear-to-merge")) |
      "\(.submittedAt)\t\((.body | capture("(?<v>block-on-findings|clear-to-merge)").v))\tgh-review-\(.author.login)",
    (.comments // [])[] | select(.body | test("block-on-findings|clear-to-merge")) |
      "\(.createdAt)\t\((.body | capture("(?<v>block-on-findings|clear-to-merge)").v))\tgh-comment-\(.author.login)"
  ' "$HANDOFF_DIR/pr-$N.json" >> "$HANDOFF_DIR/pr-$N-events.tsv"

  # Source (b) — session transcript. Resolution: line-order ≈ wall-clock order, so
  # synthesize a monotonic timestamp from `mergedAt - (line_count - line_number) seconds`
  # which preserves ordering across the merged event list (a-events stay at their real
  # ISO time; b-events sort into the same chronological slot relative to mergedAt).
  if [ -n "${CLAUDE_TRANSCRIPT:-}" ] && [ -r "$CLAUDE_TRANSCRIPT" ]; then
    total=$(wc -l < "$CLAUDE_TRANSCRIPT")
    merged_epoch=$(date -j -f '%Y-%m-%dT%H:%M:%SZ' "$(jq -r .mergedAt "$HANDOFF_DIR/pr-$N.json")" '+%s')
    grep -nE "PR\s*#?$N\b" "$CLAUDE_TRANSCRIPT" |
      grep -E 'block-on-findings|clear-to-merge' |
      awk -F: -v t="$total" -v m="$merged_epoch" '{
        ts = m - (t - $1);
        printf "%s\t%s\ttranscript-line-%d\n", strftime("%Y-%m-%dT%H:%M:%SZ", ts), ($0 ~ /block-on-findings/ ? "block-on-findings" : "clear-to-merge"), $1
      }' >> "$HANDOFF_DIR/pr-$N-events.tsv"
  else
    # Fallback: $CLAUDE_TRANSCRIPT unset or unreadable → main thread surfaces
    # the gap interactively in the consolidated hand-back instead of auto-filing
    # an issue. Phase 7 cross-ref still records "transcript unavailable" so the
    # operator knows the GH-only path is the only signal.
    echo "transcript_unavailable" >> "$HANDOFF_DIR/pr-$N-events.tsv"
  fi

  # Walk forward. For each block-on-findings event, require a later clear-to-merge
  # from a distinct agent-id (cavecrew-reviewer-* OR a[0-9a-f]{16}) before mergedAt.
  sort "$HANDOFF_DIR/pr-$N-events.tsv" |
    awk -F'\t' 'BEGIN{block=""} $2=="block-on-findings"{block=$3; next}
                $2=="clear-to-merge" && block!="" && $3!=block {block=""}
                END{if(block!="") exit 1}'
  ```

  Exit 1 → file `[SESSION-AUDIT][self-approve-after-amend] PR#<N>`. "transcript_unavailable" sentinel → main thread surfaces gap in hand-back, never auto-files (per Hard Nos `NO auto-file on uncertain detection`). (Does NOT detect `re-spawn-design` slips — those rebuild the change wholesale and bypass the simple later-clear-to-merge heuristic; flag as future-lever in Phase A2.)

`mark_done 1`

## Phase 2: Reviewer-comment audit

ONE cached `gh pr view N --json comments,reviews` per PR (cache to avoid re-fetching).

```bash
for n in $(jq -r '.[].number' "$HANDOFF_DIR/prs.json"); do
  gh pr view "$n" --json comments,reviews > "$HANDOFF_DIR/pr-$n-comments.json"
done
```

- Grep `HIGH|CRITICAL|MED|🔴|🟡` in review bodies. Cross-ref open `[REVIEWER #<N>]` issues. Missing → file.
- Merged PR with `Reviewer-recommendation: REVISE` token unaddressed → `[BYPASS-AUDIT]` issue.

`mark_done 2`

## Phase 3: Issue audit

```bash
gh issue list --author "@me" --search "created:>=$SESSION_START" -L 30 \
  --json number,title,state,labels,body > "$HANDOFF_DIR/issues.json"
```

Decision:
- Body references PR# now MERGED + title lacks `(partial)` → close-candidate. List for operator.
- Duplicate titles → flag.
- `depends on #N` for OPEN N → orphan log.

`mark_done 3`

## Phase 4: Doc audit

```bash
git log --since "$SESSION_START" --author "$GIT_AUTHOR" -- CLAUDE.md \
  | grep -oE 'feedback_[a-z_]+' | sort -u > "$HANDOFF_DIR/new-rules.txt"

# Gate-boundary propagation. A rule that lives only in CLAUDE.md / MEMORY.md
# never reaches the subagent prompt — so the dispatch-template fan-out is the
# load-bearing surface. Surface every missing rule as a [GATE-BOUNDARY-GAP]
# line; the hand-back lists them so the operator can patch the templates
# (skill cannot auto-edit dispatch-templates per Hard Nos).
: > "$HANDOFF_DIR/gate-boundary-gaps.txt"
while read rule; do
  [ -z "$rule" ] && continue
  if ! grep -l "$rule" docs/engineer/dispatch-templates/*.md > /dev/null 2>&1; then
    # Enumerate the templates so the hand-back line names every target file.
    for tmpl in docs/engineer/dispatch-templates/*.md; do
      echo "[GATE-BOUNDARY-GAP] feedback_${rule#feedback_} missing from $tmpl" \
        >> "$HANDOFF_DIR/gate-boundary-gaps.txt"
    done
  fi
done < "$HANDOFF_DIR/new-rules.txt"
```

MEMORY.md sync: diff new `feedback_*` slugs vs MEMORY.md index → write missing index lines via Edit. Memory dir is operator-personal; do NOT `cat` slug files (read-on-demand only).

Pointers freshness: `git diff --stat origin/main -- docs/engineer/`; ≥3 doc moves → flag stale-pointer audit.

**Phase 4 v1.1 spec-parity check (binding when session touches `docs/superpowers/specs/2026-06-22-leah-phase4-design.md`).** The Phase 4 design spec has 9 §-sections (§1 Voice, §2 Sync, §3 Learn, §4 Vision, §5 A2A, §6 Attest, §7 Plugin, §8 Budget, §9 Supervisor). Each must back into an `internal/<pkg>/` directory with ≥1 non-test file. The pure-spec ship-with-no-code failure mode (audit-recommended-not-autonomous) is the symptom this catches.

```bash
spec_pkgs=(voice sync learn vision a2a attest plugin budget supervisor)
: > "$HANDOFF_DIR/phase4-parity.txt"
for p in "${spec_pkgs[@]}"; do
  non_test=$(find "internal/$p" -name '*.go' ! -name '*_test.go' 2>/dev/null | head -1)
  [ -z "$non_test" ] && echo "[PHASE4-PARITY] internal/$p missing non-test file" >> "$HANDOFF_DIR/phase4-parity.txt"
done
```

Surface every line in hand-back. Do NOT auto-file (per Hard Nos) — a missing pkg may be intentional drop or pending implementation; operator decides.

**Phase 4 dispatch-template harness verification.** Five templates must parse + paths must resolve: `{implementer,implementer-adapter,reviewer,designer,triage}.md`. Spec-parity script lives at `scripts/check-phase4-parity.sh` (when present).

```bash
for t in implementer implementer-adapter reviewer designer triage; do
  [ -r "docs/engineer/dispatch-templates/$t.md" ] || \
    echo "[DISPATCH-TEMPLATE-MISSING] $t.md" >> "$HANDOFF_DIR/phase4-parity.txt"
done
[ -x scripts/check-phase4-parity.sh ] && scripts/check-phase4-parity.sh \
  >> "$HANDOFF_DIR/phase4-parity.txt" 2>&1 || true
```

`mark_done 4`

## Phase 5: Code audit (signal mining)

Scrape session transcript for: `TODO|FIXME|smell|dead|simplif|refactor|over-broad|bloat|unused|hallucinat|stale`. Cross-ref this-session-filed issues + open `followup` issues.

Unfiled → operator hand-back list. **Do NOT auto-file** (noise).

**Quality-unverified scan (per session retro 2026-06-10: 21/22 PRs merged on verbal `clear-to-merge` without GH-posted reviews — Phase 1 detection missed them because reviewers ran inline).** For each session PR, check `gh pr view <N> --json reviewDecision,reviews,comments`:

```bash
for n in $(jq -r '.[].number' "$HANDOFF_DIR/prs.json"); do
  # Carve-out: instrumentation fan-out PRs are pure-add by template design.
  # Skip when title matches the wiring pattern to avoid >50% noise.
  title=$(gh pr view "$n" --json title --jq .title)
  if echo "$title" | grep -qE '^(feat|test)\(.*\): (wire|add).*Metrics'; then
    continue
  fi
  review_count=$(gh pr view "$n" --json reviews,comments --jq '[(.reviews // [])[], (.comments // [])[]] | length')
  if [ "$review_count" = "0" ]; then
    echo "$n quality-unverified: no GH-posted reviews/comments" >> "$HANDOFF_DIR/quality-unverified.txt"
  fi
done
```

Quality-unverified PRs (after carve-out) surface in hand-back per-PR list. Do NOT auto-file (per Hard Nos `NO auto-file on uncertain detection` — inline-only review is acceptable when transcript can prove it; this scan only catches PRs where neither GH nor transcript shows review evidence).

Deletion debt:
```bash
# Refresh origin/main first — local ref can lag the remote if no fetch
# fired this session, undercounting pure-add commits merged after the
# last fetch. Read-only; safe in primary checkout.
git fetch origin main --quiet

# --merges filter misses squash-and-merge (the common GH path), which
# lands as a single non-merge commit on origin/main. Use --first-parent
# against origin/main to enumerate squash-merged commits and --no-merges
# to drop true merge commits if any exist; --shortstat surfaces ins/del.
git log --since "$SESSION_START" --author "$GIT_AUTHOR" \
  --first-parent --no-merges --shortstat origin/main \
  | awk '/insertions/ && !/deletion/ {print}'
```
≥3 pure-add commits on origin/main → `[DELETION-DEBT]` audit issue.

`mark_done 5`

## Phase 5.5: orphan-package scan (inert-ship guard)

Failure mode this catches: v3.3.0 shipped 3 packages (TTS providers, KG ingestor, MCP bridge) that compiled, passed tests, and merged — but were never imported from `cmd/leah-daemon` or `cmd/leah`. The release tagged inert. Phase 1 + Phase 5 quality-unverified scans missed it because the PRs themselves were green; the gap was at the composition-root boundary, not inside the package. Mechanical fix: every non-`cmd/` package must have at least one non-test importer in the module.

```bash
# Imports list — exclude test packages so test-only imports do not mask production-zero callers.
go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}' ./... 2>/dev/null \
  | sort -u > /tmp/imported

go list ./... 2>/dev/null | sort -u > /tmp/all-packages

# Subtract imported from all, then drop cmd/ entrypoints (they are roots, not callees).
orphans=$(comm -23 /tmp/all-packages /tmp/imported | grep -v "^github.com/trilam/leah/cmd/")
if [ -n "$orphans" ]; then
  {
    echo "Orphan packages (zero non-test callers, not cmd/ entry):"
    echo "$orphans"
  } > "$HANDOFF_DIR/orphans.txt"
fi
```

Surface in hand-back as `[ORPHAN-CANDIDATE] x N` where N is the orphan line count. Do NOT auto-file — some orphans are intentional (future-wired, dev-only entrypoints reachable via `go run`, test-only helpers consumed across module boundaries). Operator triages: wire it, delete it, or annotate it.

**Phase 4 producer wiring (composition-root verification).** Lesson reinforced post-v3.3.0: 3 wiring gaps (TTS / KG / MCP bridge) shipped because orphan-scan ran AFTER tag. Phase 4 added 7 producer surfaces that must each be constructed in the composition root and invoked from ≥1 IPC handler — checking import-graph alone is not enough (a producer can be imported but never instantiated).

```bash
phase4_producers=(
  "learn.Recommender"
  "budget.Runtime"
  "sync.Discovery"
  "a2a.Server"
  "plugin.Host"
  "supervisor.Status"
  "vision.Router"
)
root="cmd/leah-daemon"
# Composition-root file: prefer composition_root.go when present, else main.go.
# Both are valid — the spec calls for composition_root.go but main.go is the
# current home of the wiring surface; tolerate either to avoid false alarms.
comp_root="$root/composition_root.go"
[ -r "$comp_root" ] || comp_root="$root/main.go"

# wirePhase4Producers entry-point check — spec calls for a single named hook
# invoked from main() so the wave can be re-ordered without churning main.go.
if [ -r "$comp_root" ] && ! grep -q 'wirePhase4Producers' "$comp_root" "$root/main.go" 2>/dev/null; then
  echo "[PHASE4-WIRING] wirePhase4Producers not invoked from $root/main.go" \
    >> "$HANDOFF_DIR/phase4-wiring.txt"
fi

for prod in "${phase4_producers[@]}"; do
  # Constructed in composition root?
  if ! grep -rq "$prod" "$root/"*.go 2>/dev/null; then
    echo "[PHASE4-WIRING] $prod not constructed in $root/" >> "$HANDOFF_DIR/phase4-wiring.txt"
    continue
  fi
  # Invoked by ≥1 IPC handler? Heuristic: referenced from any ipc_*.go file
  # in the daemon package. False-negative when the producer is passed through
  # a struct field rather than a direct ref — operator triages in hand-back.
  if ! grep -rq "$prod" "$root"/ipc_*.go 2>/dev/null; then
    echo "[PHASE4-WIRING] $prod constructed but no ipc_*.go reference" \
      >> "$HANDOFF_DIR/phase4-wiring.txt"
  fi
done
```

Every `[PHASE4-WIRING]` line surfaces in hand-back. Do NOT auto-file (per Hard Nos `NO auto-file on uncertain detection` — indirect refs via struct fields produce false negatives; operator triages).

**Phase 4 frozen-enum-files delta.** Phase 4 added Kind enum entries to the already-frozen `internal/obs/events.go` and `internal/ipc/frame.go` for the sync / recommend / plugin / a2a / vision surfaces. Single-owner-per-dispatch rule (CLAUDE.md `Dispatch parallelism`) applies — concurrent edits race. Scan for parallel touches:

```bash
git log --since "$SESSION_START" --author "$GIT_AUTHOR" --name-only --pretty=format:'%H' \
  -- internal/obs/events.go internal/ipc/frame.go \
  | awk 'NF==1 && length($0)==40 {sha=$0; next} /events.go|frame.go/ {print sha"\t"$0}' \
  | sort -u > "$HANDOFF_DIR/frozen-enum-touches.txt"
[ "$(wc -l < "$HANDOFF_DIR/frozen-enum-touches.txt")" -ge 2 ] && \
  echo "[FROZEN-ENUM-RACE] ≥2 commits touched frozen-enum files this session" \
    >> "$HANDOFF_DIR/phase4-wiring.txt"
```

`mark_done 5_5`

## Phase 6: Worktree + branch cleanup

```bash
git worktree list --porcelain | awk '/^worktree/ {print $2}' | while read wt; do
  br=$(git -C "$wt" rev-parse --abbrev-ref HEAD 2>/dev/null)
  [ "$br" = "main" ] && continue
  merged=$(gh pr list --head "$br" --state merged --json mergedAt --jq '.[0].mergedAt')
  [ -n "$merged" ] && echo "worktree $wt: branch $br merged; safe remove"
done

git branch --merged main | grep -v '^\* main' | grep -v 'main$' | head -20
```

Decision: hand back commands; NO auto-remove. Per CLAUDE.md `Worktree discipline`: primary checkout stays on main; flag if not.

`mark_done 6`

## Phase 7: Learning + memory

**Scope:** universal patterns + operator-personal `feedback_*` rules. Phase 7 writes memory + CLAUDE.md candidates; never auto-edits CLAUDE.md.

- **Twice-burned scan.** Transcript grep `same|again|twice|retry|second time|broken again`. Cluster by root cause. ≥2 + no `feedback_*` entry → new candidate.
- **Repeated operator directive.** ≥2 user turns with same phrasing → queue codification candidate.
- **Trap projection.** Trap operator hit ≥2 → propose worker-side fix at gate / prompt / dispatch-template boundary.
- **Self-approve-after-amend gate.** Per memory `feedback_no_self_approve_after_edits` + S5 reflexion-loop (`docs/engineer/specs/2026-06-10-reflexion-loop.md`): scan transcript for `block-on-findings` reviewer verdicts → confirm re-spawned reviewer returned `clear-to-merge` before merge → if not, file `[SESSION-AUDIT][self-approve-after-amend]` retroactively.

**Cross-ref `learn-from-mistakes` skill (binding):** Scan transcript for friction triggers — user pushback (`no`, `don't`, `stop`, `revert`, `undo that`), in-session rollback, test failing twice for related reasons, rediscovered ruled-out answer. For each trigger, verify `learn-from-mistakes` skill activated (look for "/learn" invocation or `feedback_*.md` write in session). If trigger fired WITHOUT skill activation → surface the unsaved learning candidate in the consolidated hand-back so the operator can decide whether to capture or drop. Do NOT auto-invoke the skill (per Hard Nos); only surface the gap.

Write: `feedback_*.md` stubs under `~/.claude/projects/<hash>/memory/`; MEMORY.md index lines via Edit; CLAUDE.md candidate list into handoff file (NOT auto-edit).

`mark_done 7`

## Phase 8: Cost + budget audit

```bash
gh api rate_limit --jq '.resources.core | {remaining,reset}'
```

Log-only unless rate-remaining <500 OR explicit budget cap hit. One-line summary into handoff: `N PRs merged, M issues, K subagents, GH remaining Y`.

`mark_done 8`

## Phase 9: Next-session handoff

THE LOAD-BEARING PHASE. Writes `$HANDOFF_DIR/<ISO>-session-handoff.md`. Next session reads this BEFORE doing anything else.

Schema:

```yaml
---
session_id: <uuid>
session_start: <ISO>
session_end: <ISO>
operator: $GIT_AUTHOR
exit_reason: clean | interrupt | bottleneck | budget-cap
next_session_first_action: <one-line>
---

## Open PRs at exit
- #<N> <title> — <state> <mergeStateStatus> — <one-line why-open>

## Open issues filed this session
- #<N> <title> — <surface> [<labels>]

## Open bottleneck (if any)
<paragraph: bottleneck-resolution loop attempt count + last error + next thing to try>

## Active worktrees + branches
- <worktree-path> (<branch>) — <status, last commit sha>

## Pending operator decisions
1. <decision the next session should NOT make autonomously>

## Memory deltas
- new `feedback_<slug>` files written
- CLAUDE.md candidates for next codification round
- dispatch-template parity drift items

## Unsaved learning candidates (from Phase 7 cross-ref)
- <friction trigger> → <draft learning> — surface only, no auto-write

## Roadmap delta vs UX-audit blockers / current wave brief
- blockers progressed: <list>
- new findings that move the roadmap: <list>

## Next-session quick-start (paste verbatim)
```bash
cd <primary-checkout>
git fetch origin main && git pull --ff-only
cat .claude/session-handoffs/<this-file>
```

## Top 3 things that went wrong + how I'd avoid them
1. <lesson> — <prevention>
```

`mark_done 9`

## Phase A1: roadmap audit (MANDATORY when session touched UX-audit blockers or wave brief)

Re-read `docs/engineer/briefs/2026-06-10-ux-audit-cross-surface.md` (binding ship-gate) + current wave brief. Compare against session evidence. Find:

- **Blockers marked CLEARED that the session saw drift back open.** File `[ROADMAP-DRIFT]` issue + propose status flip.
- **Blockers marked IN-FLIGHT that didn't move this session.** Log only.
- **Operator priority reorder** signals in user turns ("do X first now", "skip Y") → write reorder candidate.

`mark_done A1`

## Phase A2: more-autonomy lever scan (MANDATORY when session hit any operator-bottleneck)

Scan the session for places where the operator was a bottleneck — an action the agent could have done by itself if it had X capability. Categorise:

- **Boot-time precondition gap** — failure mode that should refuse-at-boot. File `[AUTONOMY-LEVER]` issue with proposed gate.
- **Reviewer prompt gap** — finding type the adversarial reviewer should have caught. File issue with proposed dispatch-template addition.
- **Self-improve detector gap** — observed pattern that should self-trigger but didn't.
- **CI gate gap** — drift that a `scripts/check-*.sh` script should mechanically catch.
- **Operator-decision gap** — decision the operator made that could be encoded as a rule.

**Operator-redirect count (per session retro 2026-06-10).** Each operator turn that asks "are we stuck?", "how do we resolve X?", "what now?", or otherwise redirects the main thread is an autonomy signal. The rate-per-hour is the metric; the absolute count is the audit-trail.

```bash
# Source (a) — session transcript when available.
if [ -n "${CLAUDE_TRANSCRIPT:-}" ] && [ -r "$CLAUDE_TRANSCRIPT" ]; then
  # Pattern is uncertainty-on-main-thread: "are we stuck", "how do we / can we", "what now",
  # NOT directives like "wait for CI" or "wait on merge" — those are operator instructions,
  # not redirects. Reviewer 2026-06-10 flagged bare `wait\b` as false-positive on directives.
  redirects=$(grep -ciE '\b(are we stuck|are we stuck\?|how do we|how can we|what now|what next|why are we|did we (just|already))' "$CLAUDE_TRANSCRIPT" || true)
else
  # Source (b) — fallback: scan conversation context. Same pattern as Phase 1
  # detection algorithm's transcript-unavailable sentinel. Surface in hand-back
  # with "transcript_unavailable" marker; do NOT auto-file.
  redirects="transcript_unavailable"
fi
echo "operator_redirects=$redirects" >> "$HANDOFF_DIR/phase-a2-flags.txt"
```

Surface in hand-back as one line. ≥5 redirects/hour signals a recurring bottleneck — propose an `[AUTONOMY-LEVER]` issue. <5 → log only.

For each finding write an `[AUTONOMY-LEVER]` issue with: surface, smallest implementer brief (file:line + 1-line fix), estimated operator-touch reduction.

`mark_done A2`

## Hand-off summary (consolidated operator output)

**Completion gate.** Before emitting the hand-back, count `=pending` rows in the ledger. If non-zero, the skill refuses to finalize — names every pending phase so the operator sees exactly which steps were skipped instead of a free-form "audit complete" claim.

```bash
pending=$(grep -c '=pending$' "$HANDOFF_DIR/phase-ledger.txt")
if [ "$pending" -ne 0 ]; then
  echo "audit-session: phase-ledger has $pending pending entries; cannot finalize hand-back."
  echo "still pending:"
  grep '=pending$' "$HANDOFF_DIR/phase-ledger.txt" | sed 's/=pending$//' | sed 's/^/  - /'
  exit 1
fi

# Phase 4 gate-boundary-gaps must reach the operator. The Phase 4 block wrote
# every missing-from-template slug to gate-boundary-gaps.txt; emit each line
# verbatim under FILED so the operator sees which dispatch-template needs the
# patch (skill itself cannot auto-edit dispatch-templates per Hard Nos).
gbg_count=0
if [ -s "$HANDOFF_DIR/gate-boundary-gaps.txt" ]; then
  gbg_count=$(wc -l < "$HANDOFF_DIR/gate-boundary-gaps.txt" | tr -d ' ')
  echo "--- gate-boundary-gaps (Phase 4) ---"
  cat "$HANDOFF_DIR/gate-boundary-gaps.txt"
fi

# Phase 5.5 orphan candidates. First line of orphans.txt is the header banner;
# subtract it from the count so [ORPHAN-CANDIDATE] x N reflects actual package count.
orphan_count=0
if [ -s "$HANDOFF_DIR/orphans.txt" ]; then
  orphan_count=$(( $(wc -l < "$HANDOFF_DIR/orphans.txt" | tr -d ' ') - 1 ))
  echo "--- orphan-candidates (Phase 5.5) ---"
  cat "$HANDOFF_DIR/orphans.txt"
fi

# Phase 4 parity (Phase 4 doc-audit extension) + wiring (Phase 5.5 extension).
# Emit verbatim so the operator sees the failing §-section / producer pair.
phase4_parity_count=0
if [ -s "$HANDOFF_DIR/phase4-parity.txt" ]; then
  phase4_parity_count=$(wc -l < "$HANDOFF_DIR/phase4-parity.txt" | tr -d ' ')
  echo "--- phase4-parity (Phase 4) ---"
  cat "$HANDOFF_DIR/phase4-parity.txt"
fi
phase4_wiring_count=0
if [ -s "$HANDOFF_DIR/phase4-wiring.txt" ]; then
  phase4_wiring_count=$(wc -l < "$HANDOFF_DIR/phase4-wiring.txt" | tr -d ' ')
  echo "--- phase4-wiring (Phase 5.5) ---"
  cat "$HANDOFF_DIR/phase4-wiring.txt"
fi
```

After all 9 phases + A1/A2 run, emit ONE consolidated hand-back block (the `[GATE-BOUNDARY-GAP] x N` count below = `gbg_count`; the individual lines emitted above this block name each template that needs the patch):

```
audit-session: <N PRs merged, M issues, K subagents this session>

OPEN (operator action):
- merge: PR#<N> CLEAN+APPROVE, waiting on you
- decide: <one-line decision>

FILED (auto):
- [POST-MERGE-AUDIT] x N
- [REVIEWER #PR] x N
- [SELF-APPROVE-AFTER-AMEND] x N
- [AUTONOMY-LEVER] x N
- [GATE-BOUNDARY-GAP] x N (from Phase 4 — feedback_* rules missing from dispatch-templates; surfaced for manual patch, not auto-edited)
- [ORPHAN-CANDIDATE] x N (from Phase 5.5 — packages with zero non-test, non-cmd importers; surfaced for triage, not auto-filed — catches the v3.3.0 inert-ship failure mode where shipped packages never reached the composition root)
- [PHASE4-PARITY] x N (from Phase 4 — Phase 4 design-spec §-section without backing `internal/<pkg>/` non-test file; surfaced for manual triage)
- [PHASE4-WIRING] x N (from Phase 5.5 — Phase 4 producer not constructed in `cmd/leah-daemon/` OR constructed but no `ipc_*.go` reference; surfaced for triage — catches the v3.3.0 inert-ship recurrence at the producer-instantiation level)
- [FROZEN-ENUM-RACE] x N (from Phase 5.5 — ≥2 session commits touched `internal/obs/events.go` or `internal/ipc/frame.go`; single-owner-per-dispatch rule violated, surfaced for review)
- [DISPATCH-TEMPLATE-MISSING] x N (from Phase 4 — one of `{implementer,implementer-adapter,reviewer,designer,triage}.md` absent from `docs/engineer/dispatch-templates/`)

CANDIDATES (next-session promote):
- CLAUDE.md: <slug>
- feedback_: <slug>
- learn-from-mistakes unsaved: <slug>
- dispatch-template reorder: <one-line>

HANDOFF: .claude/session-handoffs/<file>.md
NEXT-SESSION FIRST ACTION: <from frontmatter>
```

## Hard nos

- NO auto-merge any PR.
- NO auto-close any issue.
- NO auto-edit CLAUDE.md.
- NO `git gc --prune=now` or reflog-destructive cleanup.
- NO auto-invoke other skills (including learn-from-mistakes — only surface gaps).
- NO unbounded CI poll.
- NO auto-file on uncertain detection (e.g. Phase 1 transcript-unavailable sentinel → surface gap in hand-back, never auto-file).
- NO "audit-session complete" claim unless `phase-ledger.txt` has 0 `=pending` entries. Free-form completion language is the failure mode this ledger exists to block.
