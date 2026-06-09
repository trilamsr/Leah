---
title: Leah — pattern recognition (audit clustering + skill candidate emitter)
status: draft
phase: design
owner: tri
created: 2026-06-09
parent: 2026-06-09-leah-tier1-self-improvement.md
cross_ref:
  - 2026-06-09-leah-tier1-self-improvement.md §2.8 (recurring-pattern detector)
  - 2026-06-09-leah-tier1-self-improvement.md §2.9 (skill discovery)
---

# Pattern recognition — fast path

Audit-row clustering + skill candidate proposals. Personal use, single operator, weekly cadence. No live detection, no auto-skill creation. Operator reads `~/.leah-state/skill-candidates.md` and cherry-picks.

## 1. Goal

Surface repeated action patterns in the audit log so the operator can decide which to template into reusable skills. "You did X 7 times last week — template it?" — that one prompt. Nothing more.

Non-goals (cut in §6):

- Real-time detection while Leah is running
- Shell-history ingestion
- Auto-creation of skills from candidates
- Embedding-based semantic clustering
- New tables / new storage primitives (reads `audit.jsonl` directly)

## 2. Clustering algorithm

Inputs:
- `auditPath string` — path to `audit.jsonl` (existing `internal/audit/audit.go` `Entry` shape)
- `since time.Time` — clusterer reads entries with `Timestamp >= since`; weekly cron passes `now - 30d`

Cluster key: `(kind, args_hash[:8])` — kind exact, args_hash 8-char prefix.

Rationale for 8-char prefix:
- args_hash is `sha256` hex (32 bytes → 64 chars) per Tier 1 §2.1 design
- 8-char prefix = 32 bits → birthday-collision domain ~`sqrt(2^32) ≈ 65k` events before expected first false-cluster
- 30-day window expected event volume: O(1k-10k). Collision probability for one operator over 30d: negligible (< 0.01% at 10k events)

Threshold: `min_count int` — default `5`. Configurable via env `LEAH_PATTERN_MIN_COUNT`.

Algorithm:

1. Stream-parse `audit.jsonl` line-by-line (no full load — file grows unbounded)
2. Filter `Timestamp >= since`
3. Bucket by `(Kind, ArgsHash[:8])`
4. Emit clusters where `len(bucket) >= min_count`
5. Sort emitted clusters by `count desc`, then `kind asc` (stable ordering for diff-friendly markdown output)

Output `Cluster` struct:

```go
type Cluster struct {
    Kind       string    // e.g. "ask", "ship", "review"
    HashPrefix string    // first 8 chars of args_hash
    Count      int       // event count in window
    First      time.Time // earliest Timestamp in cluster
    Last       time.Time // latest Timestamp in cluster
    Samples    []string  // up to 3 sample `Detail` fields for operator context
}
```

## 3. Skill candidate proposal format

Per-cluster 3-line markdown block (operator-readable, diff-friendly):

```
- **{kind}** × {count} (last {N}d) — hash {prefix}…
  samples: {sample1} | {sample2} | {sample3}
  proposal: template this? `leah skill approve {kind}-{prefix}` to accept, ignore to dismiss.
```

File header (regenerated each weekly run):

```
# Skill candidates — generated {ts}, window {since}..{now}

Cherry-pick entries below. Approval CLI deferred (§6).
Stale entries (no event in window) drop automatically on the next run.
```

Lifecycle:

- **Aging / expiry**: file is fully regenerated each run from `since = now - 30d`. Clusters that fall out of the 30d window disappear from the file automatically — no manual sweep needed
- **Escalation marker**: if a cluster `(kind, hashPrefix)` appears in `≥3` consecutive weekly runs, prefix the bullet with `⬆ recurring (Nw)` where N = consecutive-weeks count. Tracked via sidecar `~/.leah-state/skill-candidates.history.json` (last-N weekly snapshots, max 12 weeks retained). Sidecar is the only state — file itself stays regeneratable
- **File-size bound**: hard cap 200 clusters per file. If more, keep top-200 by `Count desc` and append `# truncated: N more clusters below threshold-200` footer

## 4. Integration

Weekly cron, fires AFTER retro generator (so retro's audit-window summary is consistent with the clusterer's same-window read):

```
# pseudo-cron (real install path in V2 follow-up)
0 9 * * SUN  leah retro --weekly && leah patterns --weekly
```

`leah patterns --weekly` CLI behavior:

1. Resolve `auditPath` from `LEAH_AUDIT_PATH` (default `~/.leah-state/audit.jsonl`)
2. Call `patterns.Detect(auditPath, time.Now().Add(-30*24*time.Hour))`
3. Call `patterns.Propose(clusters)` → markdown string
4. Read prior `~/.leah-state/skill-candidates.history.json`, merge current cluster-keys, compute consecutive-week count per cluster
5. Write `~/.leah-state/skill-candidates.md` (atomic write: tmp + rename)
6. Write updated `~/.leah-state/skill-candidates.history.json` (last-12-weeks snapshots)
7. Emit one `audit.Entry{Kind: "patterns", ArgsHash: hash(window), Outcome: "success", Detail: fmt.Sprintf("%d clusters", len(clusters))}` so the patterns-runner itself is auditable (kind "patterns" only hits threshold after 5 weeks of runs, which IS the meta-signal "you keep running this")

Failure modes:
- Missing `audit.jsonl` → exit 0, log "no audit log yet"
- Corrupt JSON line → skip + count, emit final WARN with skip count (don't fail closed; audit log corruption shouldn't break retro flow)
- Sidecar history corrupt → reset to empty, regenerate (escalation marker just resets to 1w until 3 more weeks accumulate)

## 5. Build order (3 tasks max)

**T1 — `internal/patterns/cluster.go` + tests**
- `Cluster` struct
- `Detect(auditPath string, since time.Time) ([]Cluster, error)`
- Tests: `TestDetectGroupsByKindAndPrefix`, `TestDetectFiltersBelowThreshold`, `TestDetectSkipsCorruptLines`

**T2 — `internal/patterns/proposer.go` + tests**
- `Propose(clusters []Cluster) string` → markdown
- Tests: `TestProposeMarkdownFormat`, `TestProposeEmptyClusters` (returns header-only)

**T3 — `cmd/leah/patterns.go` CLI subcommand + history sidecar wiring**
- CLI: `leah patterns --weekly`
- History sidecar read/write/merge
- Atomic file write
- Audit entry emission

## 6. Cuts (Phase X, reopen triggers cited)

| Cut | Why deferred | Reopen trigger |
| --- | --- | --- |
| Real-time pattern detection | Adds latency to every Leah action; 99% of signal lives in weekly review window | Operator complains weekly cadence misses time-sensitive patterns |
| Shell-history hook (zsh/bash history → audit ingest) | New ingestion path, privacy-sensitive (raw commands ≠ args_hash), needs redaction layer | Tier 1 §2.4 redaction layer ships AND operator wants shell-pattern coverage |
| Auto-skill creation | "Approve" CLI requires skill-template engine; out of scope for initial detection | Operator approves ≥3 candidates manually + asks for one-shot accept |
| Embedding-based semantic clustering | Local embedding model adds dep + cost; prefix-hash clustering covers args-identity case (the common one) | Operator reports "I do similar things with different args and clusterer misses them" |
| New SQLite table for cluster cache | `audit.jsonl` is already the source; caching adds invalidation + schema migration burden | File grows past 1M lines AND `Detect` p99 exceeds 5s on operator's box |
| Privacy redaction of args_hash in markdown | args_hash is already one-way (sha256 prefix); detail samples ARE raw | Tier 1 §2.4 redaction ships; clusterer routes `Detail` through it before writing |

## 7. Dependencies / known gaps

- **Tier 1 redaction layer (§2.4) gap**: `Detail` field samples may contain raw queries, PR titles, branch names. Ships unredacted (single-operator local file, mode 0600). When Tier 1 §2.4 lands, route `cluster.Samples` through redaction before `Propose`.
- **CLI subcommand framework**: `cmd/leah/` layout for subcommands not surveyed in this spec. T3 may need a cobra/flag refactor or fits the existing pattern — surveyor task at T3 dispatch time
- **Cron install**: out of scope. Operator wires `leah patterns --weekly` into their own cron / launchd alongside `leah retro --weekly`

## 8. Adversarial-review record

| Sev | Finding | Resolution |
| --- | --- | --- |
| HIGH | 3-char args_hash prefix = 12 bits, birthday-collision at ~64 events; operator hits this in 30d | §2: 8-char prefix (32 bits, ~65k birthday-domain). Documented rationale inline |
| MED | threshold N=5 may be too high — operator with low activity (e.g. 20 events/30d) sees 0 candidates ever | §2: `min_count` env-configurable (`LEAH_PATTERN_MIN_COUNT`). Default 5; doc note flags low-volume operator should tune down |
| MED | candidates file grows unbounded if operator doesn't read | §3: file fully regenerated each run, 30d window drops stale clusters automatically. Hard 200-cluster cap with truncation footer |
| LOW | where does an approved skill live? | §6 cut: auto-skill creation deferred. Proposes only; approval workflow deferred |
| HIGH | privacy — args_hash is one-way but `Detail` samples are raw | §7 dependency: documented Tier 1 §2.4 redaction-layer gap; ships unredacted under mode-0600 local file; redaction wired at §2.4 land time |
| MED | lifecycle — repeated clusters should escalate vs new noise | §3: `⬆ recurring (Nw)` marker via 12-week history sidecar. ≥3 consecutive weeks triggers escalation prefix |
| LOW | patterns runner is itself an audit-emitting action — recursion / self-cluster risk | §4: documented. Kind "patterns" only hits threshold after 5 weekly runs; that IS the meta-signal (and it's bounded by the weekly cadence) |
