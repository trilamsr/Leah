---
slug: notion-adapter
status: draft
phase: self-host
owner: leah
---

# Notion adapter (MVP)

## 1. Goal

Give Leah a narrow, auditable Notion interface so the operator can pull
recent pages from selected databases for downstream synthesis and write
back new pages without leaving the terminal. MVP exposes four RPCs
(`ListDatabases`, `QueryDatabase`, `GetPage`, `CreatePage`) — page
edits, block-tree mutations, rich-text deltas, comments, and the
push-subscription surface defer to follow-up waves once the
operator-attestation flow is observed in real use. Shape mirrors
`internal/adapters/gmail` (Attestor + per-RPC scope + `gateAndToken`
ordering) so wiring code reuses one consent path across the work-tools
pack.

## 2. Dependency (adopt-over-build)

Two candidates evaluated:

- `github.com/jomei/notionapi` — community SDK; ~1.8k stars; active.
- Direct REST against `https://api.notion.com/v1`.

MVP picks **direct REST**. Rationale: Notion's API is small (4
endpoints cover our MVP surface) and the SDK adds modeling overhead
for block types we explicitly defer. The `Transport` interface keeps
the door open for `notionapi` if a future caller wants the typed
block tree. Adopt-over-build still applies — the standard library's
`net/http` is the adoption target here.

- Direct HTTP via `net/http`.
- Companion runtime: `golang.org/x/oauth2` (BSD-3-Clause) for token
  refresh once OAuth lands; bypassed when the API-token (integration
  secret) mode is used.

MVP defers the `go.mod` edit to the W50 combined tidy. The require
line is `golang.org/x/oauth2 v0.27.0` only — no notion-specific
module.

If a caller later needs the typed surface:

- Module: `github.com/jomei/notionapi`
- Tag: `v1.13.3`
- License: MIT.

## 3. Surface

```go
package notion

const (
    ScopeListDatabases = "notion:list_databases"
    ScopeQuery         = "notion:query_database"
    ScopeGetPage       = "notion:get_page"
    ScopeCreate        = "notion:create_page"
)

var (
    ErrAttestationDenied = errors.New("notion: attestation denied")
    ErrAuthFailed        = errors.New("notion: auth failed")
    ErrNotFound          = errors.New("notion: not found")
    ErrRateLimited       = errors.New("notion: rate limited")
    ErrInvalidPage       = errors.New("notion: invalid page payload")
)

type DB struct {
    ID    string
    Title string
    URL   string
}

type Page struct {
    ID    string
    Title string
    URL   string
    DB    string // parent database ID; empty if page-as-page
}

type Filter struct {
    PropertyName  string
    PropertyValue string // simplest single-property equality filter MVP
}

type Props struct {
    Title  string
    Fields map[string]string // property name -> string value MVP
}

type Attestor interface {
    Attest(ctx context.Context, scope string) error
}

type TokenSource interface {
    Token(ctx context.Context) (string, error)
}

type Transport interface {
    ListDatabases(ctx context.Context, bearer string) ([]DB, error)
    QueryDatabase(ctx context.Context, bearer, dbID string, f Filter) ([]Page, error)
    GetPage(ctx context.Context, bearer, id string) (Page, error)
    CreatePage(ctx context.Context, bearer, dbID string, p Props) (Page, error)
}

type Config struct {
    Attestor    Attestor
    TokenSource TokenSource
    Transport   Transport
}

type Client struct { /* unexported */ }

func New(cfg Config) (*Client, error)
func (*Client) ListDatabases(ctx context.Context) ([]DB, error)
func (*Client) QueryDatabase(ctx context.Context, dbID string, f Filter) ([]Page, error)
func (*Client) GetPage(ctx context.Context, id string) (Page, error)
func (*Client) CreatePage(ctx context.Context, dbID string, p Props) (Page, error)
```

`New` fail-closes on nil deps.

## 4. Operator-attestation flow

Mirror of gmail's `gateAndToken`. Per-call gate ordering is
load-bearing:

1. `QueryDatabase`, `GetPage`, `CreatePage` validate the relevant ID
   args are non-empty. `CreatePage` additionally validates
   `p.Title != ""`. A typo MUST NOT consume an attestation prompt.
2. `Attestor.Attest(ctx, scope)` runs. A non-nil return aborts with
   `ErrAttestationDenied` (wrapped). **No bearer token is loaded.**
3. Only on consent does `TokenSource.Token` execute and the transport
   issue the call.
4. Each RPC writes an audit row `Kind: "notion_<op>"` with `success
   bool` and `page_id` / `db_id`. Property values from `CreatePage` are
   NEVER recorded — see threat model.

Attestation pool reuses `internal/attestation/pool.go` (PR #67).

## 5. Token storage

Credential lives at `$HOME/.leah-state/secrets/notion-token.json`
(path-only). File mode MUST be `0600`.

Notion has two credential modes:

- **Internal Integration Secret** (MVP default): operator creates an
  integration at `https://www.notion.so/my-integrations`, copies the
  secret, manually grants the integration access to each database via
  the database's "..." -> "Connections" menu. UX win: no OAuth dance;
  granular per-DB sharing matches Notion's own access model.
- **OAuth (public integrations)**: deferred. Requires a Notion-side
  app review for distribution; not worth the cost for the self-host
  use case.

Token file shape:
`{"integration_secret": "secret_...", "workspace_id": "..."}`.

Token refresh deferred (integration secrets do not expire).

## 6. Future wiring (NOT in this MVP)

- `cmd/leah/notion.go` — `leah connect notion` subcommand; walks
  operator through integration creation + per-DB share.
- `cmd/leah-daemon/brief.go` — morning-brief task: "pages updated in
  databases you watch since yesterday".
- `internal/dispatcher/notion_post.go` — `leah ship --notion <db>` for
  operator-authored page creation.
- Recommendation engine: `Recommendation.Source = "notion"`.

## 7. Threat model

| Surface | Mitigation |
| --- | --- |
| Token in panic traces | `gateAndToken` returns the bearer only after attestation passes; never logs the token. |
| Token-on-disk | `0600` mode required at write time. |
| Scope creep | 4 RPCs MVP. New RPC = new `Scope*` constant + PR. |
| Attestor-bypass | `New` refuses construction without an `Attestor`. |
| Cross-workspace leakage | `workspace_id` recorded in token file and audit rows. |
| Page-body / property leak in audit | Audit row stores `page_id` / `db_id` and `success bool`; NEVER `title`, `property values`, or block text. `Detail` field excludes user-authored content. |
| Spam vector | Conservative MVP rate limit: max **3 writes per second** (matches Notion's published per-integration quota). 4th write within 1s returns `ErrRateLimited`. |
| Notion's per-DB share model | Adapter does NOT enumerate databases the integration cannot see — `ListDatabases` returns only the operator-shared set. This is desirable: prevents accidental cross-workspace discovery. |
| Block-tree complexity | Out of scope MVP. `CreatePage` only sets top-level title + simple string properties; rich block content (paragraphs, headings, lists) defers. |
| Webhooks (push) | Out of scope MVP. Pull only. |

## 8. Test plan (all hermetic)

- `TestListDatabases_HappyPath` — fake transport returns canned DBs;
  assert IDs + titles come through unchanged.
- `TestListDatabases_AttestationDenied_NoTokenLoad` — Attestor denies;
  `TokenSource.Token` MUST NOT be called.
- `TestQueryDatabase_DBIDEmpty_NoAttest` — `dbID == ""`; returns
  `ErrInvalidPage` BEFORE attestation runs.
- `TestQueryDatabase_HappyPath` — filter applied; pages returned;
  audit row has `db_id`, NO property values.
- `TestGetPage_NotFound` — transport returns `ErrNotFound`; sentinel.
- `TestCreatePage_TitleEmpty_NoAttest` — `p.Title == ""`; returns
  `ErrInvalidPage` BEFORE attestation.
- `TestCreatePage_AuditRowExcludesContent` — audit row JSON has
  `page_id` (returned from create) and `db_id`; omits `title`,
  `fields`.
- `TestCreatePage_RateLimit_RejectsBurst` — 3 creates in 1s succeed;
  4th returns `ErrRateLimited`; advances `Now`, resume.
- `TestNewRejectsMissingDeps` — nil deps each fail construction.

## 9. Trade-offs

- **UX**: per-DB share is Notion-native and matches operator
  expectations; no surprising blanket-workspace access.
- **Performance**: direct HTTP saves the modeling tax of `notionapi`
  for block types we don't touch. If we later need block trees, swap
  `Transport` to `notionapi`.
- **Long-term**: not scaffolding block-tree mutations keeps the
  surface small. Three similar lines beat a premature abstraction.

## 10. Out of scope (MVP)

- Page edits (only create).
- Block-tree content beyond title + simple string properties.
- Comments on pages.
- Page archival / restore.
- File uploads.
- Webhooks / push subscriptions.
- OAuth credential mode.
- Multi-workspace fan-out.
- Rich-text formatting in created pages.
- Operator-configurable rate limit.
- Full-content audit capture.

> All internal paths in this doc reflect the pre-2026-07-09 layout; current tree per `git ls-tree -d --name-only HEAD:internal/`.
