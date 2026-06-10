---
slug: work-tools-plan
status: draft
phase: self-host
owner: leah
---

# Work-tools adapter pack delivery plan (Waves 44-55)

Combined delivery plan for the six work-tools adapters. Specs:

- `docs/engineer/specs/2026-06-10-confluence-adapter.md`
- `docs/engineer/specs/2026-06-10-jira-adapter.md`
- `docs/engineer/specs/2026-06-10-slack-adapter.md`
- `docs/engineer/specs/2026-06-10-notion-adapter.md`
- `docs/engineer/specs/2026-06-10-linear-adapter.md`
- `docs/engineer/specs/2026-06-10-msteams-adapter.md`

All six adopt the same shape as `internal/adapters/gmail` and
`internal/adapters/gcal`: `Attestor` + `TokenSource` + `Transport` +
per-RPC scope + `gateAndToken` ordering (attest BEFORE token load).
Wiring code reuses one consent path across the entire pack.

## Sequencing principle

Adapters that share a module land together; standalone adapters land
solo. One combined `go.mod` tidy after all six scaffolds, then one
wave each for the cross-cutting surfaces (`connect`, `disconnect`,
brief, dispatcher, recommendations). Scaffold-first — no daemon
wiring until the W50 tidy is green.

Pairing logic:

- **Confluence + Jira** share `github.com/ctreminiom/go-atlassian/v2`.
  Land Confluence first (sets the shared HTTP-client helpers), then
  Jira on top.
- **Slack**, **Notion**, **Linear**, **Teams** are each standalone,
  no shared module. Order chosen by complexity ascending: Slack and
  Notion are simplest (single HTTP client surface), Linear adds a
  `go generate` step (genqlient), Teams adds the heaviest SDK
  (`msgraph-sdk-go`) — defer the dep-tree cost to last so a
  late-discovered SDK issue doesn't block the other adapters.

## Wave 44 — Confluence adapter scaffold + shared go-atlassian helpers

**Goal**: Land `internal/adapters/confluence` with the full surface
and test suite from the spec, plus shared `internal/adapters/atlassian`
helpers that Jira will reuse (HTTP client construction, error mapping,
workspace claim check). No daemon, no CLI, no `go.mod` edit (deferred
to W50).

**Files touched**:

- `internal/adapters/atlassian/client.go` (new, ~80 LOC) — shared
  HTTP client + error mapper.
- `internal/adapters/atlassian/client_test.go` (new, ~60 LOC).
- `internal/adapters/confluence/doc.go` (new).
- `internal/adapters/confluence/confluence.go` (new, ~220 LOC).
- `internal/adapters/confluence/confluence_test.go` (new, ~140 LOC).
- `docs/engineer/specs/2026-06-10-confluence-adapter.md` (status ->
  `shipped`).

**Test plan**: seven tests from the spec, hermetic. Failing-test-first
commit captures the red output in the PR body per CLAUDE.md TDD rule.

**Risk**: Low. Pure-Go scaffold behind a `Transport` seam; no real
Atlassian access.

**Size**: M (~500 LOC including tests; spec budget).

**Unblocks**: W45 (Jira), W50 (combined tidy), W51 (`leah connect
confluence`).

## Wave 45 — Jira adapter using shared module

**Goal**: Land `internal/adapters/jira` on top of W44's shared
`atlassian` package. Mirror Jira spec surface (4 RPCs).

**Files touched**:

- `internal/adapters/jira/doc.go` (new).
- `internal/adapters/jira/jira.go` (new, ~180 LOC).
- `internal/adapters/jira/jira_test.go` (new, ~120 LOC).
- `docs/engineer/specs/2026-06-10-jira-adapter.md` (status ->
  `shipped`).

**Test plan**: nine tests from the spec, hermetic. Failing-test-first.

**Risk**: Low. Sits on W44's helpers.

**Size**: S (~300 LOC including tests; spec budget).

**Unblocks**: W50, W51 (`leah connect jira`), W54 (`leah ship --jira`).

## Wave 46 — Slack adapter scaffold

**Goal**: Land `internal/adapters/slack` with the full surface and
test suite from the spec. No daemon, no CLI, no `go.mod` edit.

**Files touched**:

- `internal/adapters/slack/doc.go` (new).
- `internal/adapters/slack/slack.go` (new, ~250 LOC).
- `internal/adapters/slack/slack_test.go` (new, ~200 LOC).
- `docs/engineer/specs/2026-06-10-slack-adapter.md` (status ->
  `shipped`).

**Test plan**: ten tests from the spec, hermetic. Failing-test-first.

**Risk**: Low. Pure-Go scaffold behind `Transport`. Bot Token / User
Token split documented in the spec is captured in the test suite
(`TestSearch_NoUserToken_ErrAuthFailed`).

**Size**: M (~500 LOC including tests; spec budget).

**Unblocks**: W50, W51 (`leah connect slack`), W54 (`leah ship --slack`).

## Wave 47 — Notion adapter scaffold

**Goal**: Land `internal/adapters/notion` with the full surface and
test suite from the spec. Direct HTTP (no SDK adoption) — keeps the
surface small.

**Files touched**:

- `internal/adapters/notion/doc.go` (new).
- `internal/adapters/notion/notion.go` (new, ~230 LOC).
- `internal/adapters/notion/notion_test.go` (new, ~180 LOC).
- `docs/engineer/specs/2026-06-10-notion-adapter.md` (status ->
  `shipped`).

**Test plan**: nine tests from the spec, hermetic. Failing-test-first.

**Risk**: Low. Direct HTTP keeps the dep surface minimal.

**Size**: M (~500 LOC; spec budget).

**Unblocks**: W50, W51 (`leah connect notion`), W54 (`leah ship --notion`).

## Wave 48 — Linear adapter scaffold (genqlient)

**Goal**: Land `internal/adapters/linear` with `genqlient` typed
GraphQL client. Includes `go generate` directive + checked-in
generated artifacts so CI does not need genqlient at build time.

**Files touched**:

- `internal/adapters/linear/doc.go` (new).
- `internal/adapters/linear/linear.graphql` (new) — schema slice
  covering 4 RPCs.
- `internal/adapters/linear/genqlient.yaml` (new) — generator config.
- `internal/adapters/linear/queries.graphql` (new) — query
  definitions.
- `internal/adapters/linear/zz_generated.go` (new, generated) —
  checked in.
- `internal/adapters/linear/linear.go` (new, ~180 LOC) — hand-written
  wrapper.
- `internal/adapters/linear/linear_test.go` (new, ~180 LOC).
- `docs/engineer/specs/2026-06-10-linear-adapter.md` (status ->
  `shipped`).

**Test plan**: ten tests from the spec, hermetic. Failing-test-first.
Additional test: `TestBearer_NoPrefix_InHeader` asserts Linear's
no-`Bearer ` quirk is honored.

**Risk**: Med. First adapter with a code-gen step. Mitigation: check
in the generated file so CI does not need genqlient; document the
regenerate command in `doc.go`.

**Size**: M (~500 LOC including tests + ~300 LOC generated; spec
budget on hand-written portion).

**Unblocks**: W50, W51 (`leah connect linear`), W54 (`leah ship --linear`).

## Wave 49 — Microsoft Teams adapter scaffold

**Goal**: Land `internal/adapters/msteams` with the full surface and
test suite. Heaviest SDK in the pack — landed last so dep-tree
surprises do not block siblings.

**Files touched**:

- `internal/adapters/msteams/doc.go` (new).
- `internal/adapters/msteams/msteams.go` (new, ~240 LOC).
- `internal/adapters/msteams/msteams_test.go` (new, ~220 LOC).
- `docs/engineer/specs/2026-06-10-msteams-adapter.md` (status ->
  `shipped`).

**Test plan**: eleven tests from the spec, hermetic.
Failing-test-first.

**Risk**: Med. Microsoft Graph SDK has a large transitive dep tree
(~12 modules). Mitigation: `Transport` interface keeps the swap door
open to a hand-rolled minimal REST client if W50's tidy surfaces
unacceptable build time.

**Size**: M (~500 LOC including tests; spec budget).

**Unblocks**: W50, W51 (`leah connect msteams`), W54 (`leah ship --msteams`).

## Wave 50 — Combined `go.mod` tidy

**Goal**: Single PR adding every work-tools dependency in one
`go.mod` edit. Mirrors the gmail/gcal pattern (one combined tidy
after parallel scaffolds).

**Files touched**:

- `go.mod` — add require lines for: `go-atlassian/v2 v2.6.1`,
  `slack-go/slack v0.15.0`, `Khan/genqlient v0.7.0`,
  `microsoftgraph/msgraph-sdk-go v1.61.0`,
  `golang.org/x/oauth2 v0.27.0` (if not already required by
  gmail/gcal).
- `go.sum` — `go mod tidy` output.
- `internal/adapters/confluence/confluence.go` — swap `Transport`
  default impl from stub to real go-atlassian-backed client.
- `internal/adapters/jira/jira.go` — same.
- `internal/adapters/slack/slack.go` — swap default `Transport` to
  slack-go-backed client.
- `internal/adapters/linear/linear.go` — wire `genqlient.Client` as
  the default transport.
- `internal/adapters/msteams/msteams.go` — swap default `Transport`
  to msgraph-sdk-go-backed client.
- Notion has no SDK swap (direct HTTP was MVP) — only `oauth2`
  affects.

**Test plan**: existing hermetic tests stay green (they inject fake
`Transport`s). Add one smoke test per adapter that constructs the
default real transport and asserts `New` returns no error with valid
fakes for `Attestor` / `TokenSource`.

**Risk**: Med. Five new modules + their transitive deps in one tidy.
Mitigation: each adapter already lands behind a `Transport` interface
so the real SDK swap is a one-line construction change.

**Size**: M (~200 LOC including smoke tests; mostly `go.sum` churn).

**Unblocks**: W51 (real `connect` flows can exercise the real
transports).

## Wave 51 — `leah connect <tool>` integration (all 6)

**Goal**: Register all six adapters in
`internal/connect/registry.go` (mirror gmail/gcal pattern from PR
#47), implement per-adapter `connect_<name>.go` flows that walk the
operator through credential setup and write a `0600` token file.

**Files touched**:

- `internal/connect/confluence.go` (new) — API-token paste flow.
- `internal/connect/jira.go` (new) — API-token paste flow; shares
  token file with confluence if same workspace.
- `internal/connect/slack.go` (new) — Slack App OAuth flow.
- `internal/connect/notion.go` (new) — integration-secret paste flow.
- `internal/connect/linear.go` (new) — API-key paste flow.
- `internal/connect/msteams.go` (new) — Azure AD device-code OAuth.
- `internal/connect/registry.go` — register all six providers.
- `cmd/leah/connect_confluence.go` ... `cmd/leah/connect_msteams.go`
  (six new files, ~30 LOC each — thin CLI shims).
- Tests: hermetic via injected `Prompt` and `HTTP` fakes.

**Risk**: Med. Six new providers in one wave. Mitigation: each shares
the `Provider` interface from PR #47; the per-provider logic is
narrow (credential paste OR OAuth device-code).

**Size**: L (~800 LOC across files; one PR per pair if review
feedback demands).

**Unblocks**: W52, W53.

## Wave 52 — `leah disconnect <tool>` integration (all 6)

**Goal**: Mirror PR #66 pattern for the work-tools pack. Each
adapter's disconnect path deletes the `0600` token file, writes a
`disconnect_<tool>` audit row, and revokes the credential remotely
where the API supports it (Slack: `auth.revoke`; Microsoft: token
revoke endpoint; others: no-op remote, local delete only).

**Files touched**:

- `internal/connect/disconnect_confluence.go` ... (six new files).
- `internal/connect/disconnect_test.go` — one table-test covering all
  six tools.
- `cmd/leah/disconnect.go` — already accepts a provider name via PR
  #66; verify the new providers route.

**Risk**: Low. PR #66 set the shape; this wave is wiring.

**Size**: M (~400 LOC).

**Unblocks**: nothing — disconnect is a terminal flow. Required for
operator-trust UX (no token can be stranded).

## Wave 53 — Morning brief enrichment

**Goal**: Wire all six adapters into the daemon's morning brief
(mirrors PR #65 brief-wire pattern from gmail/gcal). Each adapter
contributes a small section to the brief.

**Files touched**:

- `cmd/leah-daemon/brief.go` — extend the brief composer.
- `cmd/leah-daemon/build_confluence.go` (new) — adapter construction.
- `cmd/leah-daemon/build_jira.go` (new).
- `cmd/leah-daemon/build_slack.go` (new).
- `cmd/leah-daemon/build_notion.go` (new).
- `cmd/leah-daemon/build_linear.go` (new).
- `cmd/leah-daemon/build_msteams.go` (new).
- `internal/brief/work_tools.go` (new) — fan-in helper that pulls
  per-adapter summaries.
- Tests: hermetic; each adapter section short-circuits to "no data"
  when the token file is absent (so the brief works on a partially-
  connected setup).

**Risk**: Med. First wave where all adapters meet the real
`Attestor`. Each per-tool RPC fires a real attestation prompt at
brief-render time — UX must batch these so the operator sees one
prompt per session, not one per tool.

**Size**: L (~700 LOC across files; may split per tool if review
feedback demands).

**Unblocks**: W55 (recommendation engine consumes the same per-tool
data).

## Wave 54 — Dispatcher write-paths (`leah ship --<tool>`)

**Goal**: Operator-facing CLI write paths for the four tools with
MVP write surfaces: Slack (`PostMessage`), Jira (`CreateIssue` /
`Comment`), Notion (`CreatePage`), Linear (`CreateIssue` /
`Comment`), Teams (`PostMessage`). Confluence has no write surface
in MVP — `leah ship --confluence` is intentionally NOT in this wave.

Depends on PR #54 (selfbuild sentinel) for the dispatcher's
attestation-gate plumbing.

**Files touched**:

- `internal/dispatcher/slack_post.go` (new).
- `internal/dispatcher/jira_post.go` (new) — handles both
  `CreateIssue` and `Comment`.
- `internal/dispatcher/notion_post.go` (new).
- `internal/dispatcher/linear_post.go` (new) — handles both
  `CreateIssue` and `Comment`.
- `internal/dispatcher/msteams_post.go` (new).
- `cmd/leah/ship.go` — extend with `--slack`, `--jira`, `--notion`,
  `--linear`, `--msteams` flag paths.
- Integration tests under `internal/dispatcher/` cover happy path +
  attestation-denied + rate-limited per adapter.

**Risk**: Med-High. First place rate limits land in the dispatcher's
real attestation path. Mitigation: each adapter's rate-limit logic
already covered by W44-W49 unit tests; dispatcher integration just
verifies the path is connected.

**Size**: L (~800 LOC; one PR per tool if review demands).

**Unblocks**: W55.

## Wave 55 — Recommendation-engine adapters

**Goal**: Tie all six tools into the recommendation engine from PR
#69 (`learn-recommend-apply`). Each tool gains a
`Recommendation.Source = "<tool>"` so the engine can surface
"consider posting this in #eng" or "consider creating a Jira issue"
suggestions, gated by the same attestation pool.

Depends on PR #69 having merged.

**Files touched**:

- `internal/recommend/sources/confluence.go` (new).
- `internal/recommend/sources/jira.go` (new).
- `internal/recommend/sources/slack.go` (new).
- `internal/recommend/sources/notion.go` (new).
- `internal/recommend/sources/linear.go` (new).
- `internal/recommend/sources/msteams.go` (new).
- `internal/recommend/registry.go` — register all six sources.
- Tests: hermetic via mock adapter clients.

**Risk**: Low. PR #69 set the `Source` shape; this wave is wiring.

**Size**: M (~600 LOC).

**Unblocks**: nothing further in this pack. Future per-tool tuning
(e.g. learn which channel a user posts to most often) reopens as
separate work.

## Cross-cutting decisions

- **One canonical adapter shape**: `Attestor` + `TokenSource` +
  `Transport` + per-RPC scope + `gateAndToken` ordering across all
  six. No per-tool ad-hoc adapter design. The reviewer can read one
  spec and review the others by exception.
- **Adopt-over-build per CLAUDE.md**: official or community-maintained
  SDK where active + pinned (`go-atlassian`, `slack-go`, `msgraph-sdk-go`,
  `genqlient`); direct REST only where the SDK adds modeling overhead
  for surface we defer (Notion).
- **One combined `go.mod` tidy** (W50) after all six scaffolds — mirrors
  the gmail/gcal pattern. Avoids the parallel-rebase storm.
- **Token files** all live at `$HOME/.leah-state/secrets/<name>-token.json`
  mode `0600`. Attestation pool (`internal/attestation/pool.go`, PR
  #67) provides the prompts.
- **Hashed audit fields**: `channel_hash`, `query_hash` everywhere
  channel/query is a workspace topology hint. Plaintext NEVER appears
  in audit-derived UI.
- **No webhooks / push** in any adapter MVP. Pull only across the
  pack. The Events API / Graph subscriptions / Linear subscriptions
  / Slack Socket Mode reopen as a unified push wave once the pull
  surface is observed in real use.
- **Conservative rate limits** (per spec): Slack 1/sec/channel; Jira
  10/min writes; Notion 3/sec writes; Linear 10/min writes; Teams
  4/sec posts. Operator-configurable limits deferred.

## UX > performance > long-term trade-offs

- **UX wins**: API-token (Confluence/Jira/Notion/Linear) over OAuth
  where the worse setup is one-time paste and the worse runtime
  forever is the OAuth redirect dance.
- **UX loses**: Teams' Azure AD app registration is unavoidable for
  Microsoft Graph; W51 mitigates with a step-by-step guide. Slack's
  Slack App creation is the runner-up; W51 ships a copy-paste install
  URL.
- **Performance**: rate limits cap dispatcher burst throughput but
  match each vendor's published quotas — avoids 429 retry storms.
- **Long-term**: not scaffolding threading / replies / reactions /
  attachments across the pack keeps the consent surface narrow.
  Each future RPC requires a new `Scope*` constant the reviewer can
  flag.

## Anti-goals (none of these land in W44-W55)

- Webhooks / push subscriptions across any tool.
- Threading / replies in any messaging tool (top-level posts only).
- Reactions / emoji / GIFs.
- File uploads / attachments.
- Cross-tool search (single-tool queries only MVP).
- Multi-workspace / multi-tenant fan-out.
- OAuth flows where API-token works (Confluence/Jira/Notion/Linear).
- Operator-configurable rate limits.
- Recipient / channel allow-listing.
- Full-body audit capture (operator-toggle deferred per-tool).
- Block-tree mutations (Notion), transitions (Jira), cycles
  (Linear), meetings or calls (Teams).
