---
slug: linear-adapter
status: draft
phase: self-host
owner: leah
---

# Linear adapter (MVP)

## 1. Goal

Give Leah a narrow, auditable Linear interface so the morning-brief
task can surface issues in the current sprint and the operator can
create an issue or comment from the dispatcher without leaving the
terminal. MVP exposes four RPCs (`ListMyIssues`, `GetIssue`,
`CreateIssue`, `Comment`) — projects, cycles, roadmaps, labels, and
the GraphQL subscription push surface defer to follow-up waves once
the operator-attestation flow is observed in real use. Shape mirrors
`internal/adapters/gmail` (Attestor + per-RPC scope + `gateAndToken`
ordering) so wiring code reuses one consent path across the work-tools
pack.

## 2. Dependency (adopt-over-build)

Linear is GraphQL-only — no REST surface. Two candidates evaluated:

- Raw HTTP + hand-rolled query strings.
- `github.com/Khan/genqlient` — code-gen typed GraphQL client.

MVP picks **`genqlient`**. Rationale: GraphQL without typed
generation is a foot-gun (typo'd field names compile fine and crash at
runtime); `genqlient` reads a `.graphql` schema + queries and emits Go
types at generate time. Adopt-over-build wins on type safety; the
generator is run via `go generate`, not at build time, so the runtime
dep is just `genqlient`'s small `graphql.Client`.

- Module: `github.com/Khan/genqlient`
- Pinned tag: `v0.7.0` (latest active release as of 2026-06-10)
- License: MIT (`https://github.com/Khan/genqlient/blob/v0.7.0/LICENSE`)
- Commit sha verification: `git ls-remote https://github.com/Khan/genqlient refs/tags/v0.7.0`
- Companion runtime: `golang.org/x/oauth2` (BSD-3-Clause) only if
  OAuth mode lands later; bypassed for API-key mode.

MVP defers the `go.mod` edit to the W50 combined tidy.

Generated artifacts (`zz_generated.go` style) land alongside the
hand-written adapter; the schema file `linear.graphql` (Linear's
published SDL) is checked in. `go generate ./internal/adapters/linear`
regenerates after schema bumps.

## 3. Surface

```go
package linear

const (
    ScopeListMy   = "linear:list_my"
    ScopeGetIssue = "linear:get_issue"
    ScopeCreate   = "linear:create_issue"
    ScopeComment  = "linear:comment"
)

var (
    ErrAttestationDenied = errors.New("linear: attestation denied")
    ErrAuthFailed        = errors.New("linear: auth failed")
    ErrNotFound          = errors.New("linear: not found")
    ErrRateLimited       = errors.New("linear: rate limited")
    ErrInvalidIssue      = errors.New("linear: invalid issue payload")
)

type Issue struct {
    ID         string
    Identifier string // "ENG-42" style
    Title      string
    State      string
    TeamKey    string
    URL        string
    Updated    time.Time
}

type IssueReq struct {
    TeamID      string
    Title       string
    Description string
    Priority    int // 0..4; 0 = no priority
}

type Attestor interface {
    Attest(ctx context.Context, scope string) error
}

type TokenSource interface {
    Token(ctx context.Context) (string, error)
}

type Transport interface {
    ListMyIssues(ctx context.Context, bearer string) ([]Issue, error)
    GetIssue(ctx context.Context, bearer, id string) (Issue, error)
    CreateIssue(ctx context.Context, bearer string, req IssueReq) (Issue, error)
    Comment(ctx context.Context, bearer, id, body string) error
}

type Config struct {
    Attestor    Attestor
    TokenSource TokenSource
    Transport   Transport
    Now         func() time.Time
}

type Client struct { /* unexported */ }

func New(cfg Config) (*Client, error)
func (*Client) ListMyIssues(ctx context.Context) ([]Issue, error)
func (*Client) GetIssue(ctx context.Context, id string) (Issue, error)
func (*Client) CreateIssue(ctx context.Context, req IssueReq) (Issue, error)
func (*Client) Comment(ctx context.Context, id, body string) error
```

`New` fail-closes on nil deps. `Now` defaults to `time.Now` when nil.

## 4. Operator-attestation flow

Mirror of gmail's `gateAndToken`. Per-call gate ordering is
load-bearing:

1. `CreateIssue` validates `req.TeamID != ""` and `req.Title != ""`.
   `Comment` validates `id != ""`, `body != ""`. `GetIssue` validates
   `id != ""`. A typo MUST NOT consume an attestation prompt.
2. Rate-limit check (Section 7) — burst-rejected calls MUST NOT
   consume an attestation prompt either.
3. `Attestor.Attest(ctx, scope)` runs. A non-nil return aborts with
   `ErrAttestationDenied` (wrapped). **No bearer token is loaded.**
4. Only on consent does `TokenSource.Token` execute and the transport
   issue the GraphQL call.
5. Each RPC writes an audit row `Kind: "linear_<op>"` with `success
   bool` and `issue_identifier` (the public "ENG-42" style identifier
   — non-sensitive by design). Title / description / comment body are
   NEVER recorded.

Attestation pool reuses `internal/attestation/pool.go` (PR #67).

## 5. Token storage

Credential lives at `$HOME/.leah-state/secrets/linear-token.json`
(path-only). File mode MUST be `0600`.

Two credential modes; MVP picks API key:

- **Personal API key** (MVP default): operator generates at
  `https://linear.app/<workspace>/settings/api`. Token file shape:
  `{"api_key": "lin_api_...", "workspace_id": "..."}`. Bearer header:
  `Authorization: <api_key>` (Linear is unusual — NO `Bearer ` prefix).
- **OAuth 2.0**: deferred. Linear's OAuth requires a public app
  registration which is overkill for the self-host MVP.

API keys do not expire; refresh logic is N/A.

## 6. Future wiring (NOT in this MVP)

- `cmd/leah/linear.go` — `leah connect linear` subcommand; walks
  operator through API key generation + paste.
- `cmd/leah-daemon/brief.go` — morning-brief task: "issues in your
  current cycle, blockers and high-priority items".
- `internal/dispatcher/linear_post.go` — `leah ship --linear` for
  operator-authored issue creation; `leah comment --linear <ID>`.
- Recommendation engine: `Recommendation.Source = "linear"`.

## 7. Threat model

| Surface | Mitigation |
| --- | --- |
| Token in panic traces | `gateAndToken` returns the bearer only after attestation passes; never logs the token. Linear's missing `Bearer ` prefix means the raw key is the header value — extra care: NEVER format the bearer into an error string. |
| Token-on-disk | `0600` mode required at write time. |
| Scope creep | 4 RPCs MVP. New RPC = new `Scope*` constant + PR. |
| Attestor-bypass | `New` refuses construction without an `Attestor`. |
| Cross-workspace leakage | `workspace_id` recorded in token file and audit rows. |
| Issue-body leak in audit | Audit row stores `issue_identifier` (e.g. "ENG-42") and `success bool`; NEVER `title`, `description`, `comment_body`. |
| GraphQL injection | Genqlient generates parameterized queries; raw-string interpolation into query bodies is impossible by construction. Reviewer flags any hand-rolled query that bypasses the generator. |
| Spam vector | Conservative MVP rate limit: max **10 writes per rolling 60s** (`linear:create_issue` and `linear:comment` share the budget). Linear's published quota is ~1500/hr; we run well under. |
| GraphQL subscription push | Out of scope MVP. Pull (query/mutation) only. |
| Over-fetching | `genqlient` queries request only the fields the spec needs; new fields require explicit query edits, reviewed at PR time. |

## 8. Test plan (all hermetic)

- `TestListMyIssues_HappyPath` — fake transport returns canned issues;
  assert identifiers + titles come through unchanged.
- `TestListMyIssues_AttestationDenied_NoTokenLoad` — Attestor denies;
  `TokenSource.Token` MUST NOT be called.
- `TestGetIssue_IDEmpty_NoAttest` — `id == ""`; returns
  `ErrInvalidIssue` BEFORE attestation runs.
- `TestGetIssue_NotFound` — transport returns `ErrNotFound`; sentinel.
- `TestCreateIssue_TeamIDEmpty_NoAttest` — `req.TeamID == ""`; returns
  `ErrInvalidIssue` BEFORE attestation.
- `TestCreateIssue_AuditRowExcludesBody` — audit row JSON has
  `issue_identifier`, omits `title`, `description`.
- `TestComment_BodyEmpty_NoAttest` — empty body; returns
  `ErrInvalidIssue` BEFORE attestation.
- `TestCreate_RateLimit_RejectsBurst` — 10 creates+comments in 30s
  succeed; 11th returns `ErrRateLimited`; advance `Now`, resume.
- `TestBearer_NoPrefix_InHeader` — fake transport asserts the
  Authorization header value equals the raw key (NO `Bearer ` prefix).
- `TestNewRejectsMissingDeps` — nil deps each fail construction.

## 9. Trade-offs

- **UX**: API-key paste is one step; matches the dev-tool ergonomic
  Linear's own users expect.
- **Performance**: `genqlient` type-safety pays a generate-step cost
  but eliminates a runtime-panic class. Adopt-over-build wins.
- **Long-term**: not scaffolding cycles / projects / roadmaps keeps
  the consent surface narrow. Each future RPC requires a new query
  file + scope constant.

## 10. Out of scope (MVP)

- Cycles / projects / roadmaps.
- Labels, priorities beyond a single `int` knob.
- Issue state transitions.
- GraphQL subscriptions / push.
- OAuth credential mode.
- Multi-workspace fan-out.
- Attachments.
- Rich Markdown in description (plain string only).
- Operator-configurable rate limit.
- Full-body audit capture.

> All internal paths in this doc reflect the pre-2026-07-09 layout; current tree per `git ls-tree -d --name-only HEAD:internal/`.
