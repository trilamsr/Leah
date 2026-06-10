---
slug: jira-adapter
status: draft
phase: self-host
owner: leah
---

# Jira adapter (MVP)

## 1. Goal

Give Leah a narrow, auditable Jira interface so the morning-brief task
can surface "issues assigned to you" and the operator can create an
issue or comment from the dispatcher without leaving the terminal. MVP
exposes four RPCs (`ListMyIssues`, `GetIssue`, `CreateIssue`,
`Comment`) — sprints, boards, JQL search, transitions, and attachments
defer to follow-up waves once the operator-attestation flow is observed
in real use. Shape mirrors `internal/adapters/gmail` (Attestor +
per-RPC scope + `gateAndToken` ordering) so wiring code reuses one
consent path across the work-tools pack.

## 2. Dependency (adopt-over-build)

Adopts the same `go-atlassian` SDK as the Confluence sibling adapter.
One module, two adapters — the W50 combined tidy lands the require
line once.

- Module: `github.com/ctreminiom/go-atlassian/v2`
- Pinned tag: `v2.6.1` (latest active release as of 2026-06-10)
- License: MIT
- Commit sha verification: same `git ls-remote` as Confluence spec.
- Companion runtime: `golang.org/x/oauth2` (BSD-3-Clause) for OAuth 3LO
  token refresh; bypassed when API-token mode is selected.

`go.mod` edit deferred to the W50 combined tidy — Confluence
(sibling) also requires this module; one combined tidy avoids a
rebase storm.

Fallback if the SDK ever stalls: direct REST against
`https://your-domain.atlassian.net/rest/api/3`. The `Transport`
interface makes the swap mechanical.

## 3. Surface

```go
package jira

const (
    ScopeListMy   = "jira:list_my"
    ScopeGetIssue = "jira:get_issue"
    ScopeCreate   = "jira:create_issue"
    ScopeComment  = "jira:comment"
)

var (
    ErrAttestationDenied = errors.New("jira: attestation denied")
    ErrAuthFailed        = errors.New("jira: auth failed")
    ErrNotFound          = errors.New("jira: not found")
    ErrRateLimited       = errors.New("jira: rate limited")
    ErrInvalidIssue      = errors.New("jira: invalid issue payload")
)

type Issue struct {
    Key        string // e.g. "ENG-42"
    Summary    string
    Status     string
    Assignee   string
    ProjectKey string
    URL        string
    Updated    time.Time
}

type IssueReq struct {
    ProjectKey string
    Summary    string
    Description string
    IssueType  string // "Task", "Bug", "Story"
}

type Attestor interface {
    Attest(ctx context.Context, scope string) error
}

type TokenSource interface {
    Token(ctx context.Context) (string, error)
}

type Transport interface {
    ListMyIssues(ctx context.Context, bearer string) ([]Issue, error)
    GetIssue(ctx context.Context, bearer, key string) (Issue, error)
    CreateIssue(ctx context.Context, bearer string, req IssueReq) (Issue, error)
    Comment(ctx context.Context, bearer, key, body string) error
}

type Config struct {
    Attestor    Attestor
    TokenSource TokenSource
    Transport   Transport
    BaseURL     string // e.g. https://acme.atlassian.net
}

type Client struct { /* unexported */ }

func New(cfg Config) (*Client, error)
func (*Client) ListMyIssues(ctx context.Context) ([]Issue, error)
func (*Client) GetIssue(ctx context.Context, key string) (Issue, error)
func (*Client) CreateIssue(ctx context.Context, req IssueReq) (Issue, error)
func (*Client) Comment(ctx context.Context, key, body string) error
```

`New` fail-closes on nil `Attestor`, nil `TokenSource`, nil `Transport`,
or empty `BaseURL`. There is no silent default.

## 4. Operator-attestation flow

Mirror of gmail's `gateAndToken`. Per-call gate ordering is
load-bearing:

1. `CreateIssue` validates `req.ProjectKey != ""`, `req.Summary != ""`,
   `req.IssueType != ""`. `Comment` validates `key != ""`, `body !=
   ""`. A typo MUST NOT consume an attestation prompt.
2. `Attestor.Attest(ctx, scope)` runs. A non-nil return aborts with
   `ErrAttestationDenied` (wrapped). **No bearer token is loaded.**
3. Only on consent does `TokenSource.Token` execute and the transport
   issue the call.
4. Each RPC writes an audit row `Kind: "jira_<op>"` with `success bool`
   and `issue_key` (the public Jira key — non-sensitive by design).

Scopes per action so the audit log distinguishes read (`jira:list_my`,
`jira:get_issue`) from write (`jira:create_issue`, `jira:comment`).
Attestation pool reuses `internal/attestation/pool.go` (PR #67).

## 5. Token storage

Credential lives at `$HOME/.leah-state/secrets/jira-token.json`
(path-only; adapter does NOT create or rotate it — W51 CLI owns that).
File mode MUST be `0600`.

Two credential modes; MVP picks API-token for simplicity (same
rationale as Confluence):

- **API token** (MVP default): operator generates at
  `https://id.atlassian.com/manage-profile/security/api-tokens`, paired
  with their Atlassian account email. Token file shape:
  `{"email": "...", "api_token": "...", "workspace_id": "..."}`.
- **OAuth 3LO**: deferred.

If the operator already pasted a Confluence token for the same
workspace, W51 detects the duplicate and offers to share — one token,
two adapters. Implementation note for the wiring wave: the adapter does
NOT make this decision; `TokenSource` is opaque from the adapter's
perspective.

## 6. Future wiring (NOT in this MVP)

- `cmd/leah/jira.go` — `leah connect jira` subcommand; registers in
  `internal/connect/registry.go`.
- `cmd/leah-daemon/brief.go` — morning-brief task: "issues assigned to
  you, status changed since yesterday".
- `internal/dispatcher/jira_post.go` — `leah ship --jira` for
  operator-authored issue creation; `leah comment --jira <KEY>`.
- Recommendation engine: `Recommendation.Source = "jira"` for
  Jira-derived suggestions.

## 7. Threat model

| Surface | Mitigation |
| --- | --- |
| Token in panic traces | `gateAndToken` returns the bearer only after attestation passes; never logs the token. |
| Token-on-disk | `0600` mode required at write time (enforced by W51 CLI). |
| Scope creep | 4 RPCs MVP. Adding a new RPC requires a new `Scope*` constant + a brief PR. |
| Attestor-bypass | `New` refuses construction without an `Attestor`. |
| Cross-tenant leakage | `workspace_id` recorded in token file and audit rows. |
| Issue-body leak in audit | Audit row stores `issue_key` (e.g. "ENG-42") and `success bool`; NEVER `summary`, `description`, or `comment_body`. `Detail` field excludes user-authored text. |
| Spam vector (create / comment loop) | Conservative MVP rate limit: max **10 writes per rolling 60s** (`jira:create_issue` and `jira:comment` share the budget). 11th write returns `ErrRateLimited`. Operator-configurable limit deferred. |
| JQL injection (search) | Search is NOT in the MVP surface. When added, JQL parameters must escape via the SDK helpers; raw concatenation banned. |
| Webhooks (push from Jira) | Out of scope MVP. Pull only. |

## 8. Test plan (all hermetic)

- `TestListMyIssues_HappyPath` — fake `Transport` returns canned
  issues; assert keys + statuses come through unchanged.
- `TestListMyIssues_AttestationDenied_NoTokenLoad` — `failingTokenSource`;
  Attestor denies; `TokenSource.Token` MUST NOT be called.
- `TestGetIssue_NotFound` — transport returns `ErrNotFound`; sentinel
  surfaces.
- `TestCreateIssue_HappyPath` — valid `IssueReq`; transport returns
  created `Issue`; audit row has `Kind: "jira_create_issue"`,
  `success: true`, `issue_key`, NO `summary` or `description`.
- `TestCreateIssue_MissingProject_NoAttest` — `req.ProjectKey == ""`;
  returns `ErrInvalidIssue` BEFORE attestation runs.
- `TestComment_BodyEmpty_NoAttest` — empty body; returns
  `ErrInvalidIssue` BEFORE attestation.
- `TestCreate_AuditRowExcludesBody` — audit row JSON contains
  `issue_key`, omits `summary` / `description`.
- `TestRateLimit_RejectsBurst` — 10 creates+comments in 30s succeed;
  11th returns `ErrRateLimited`; advances `Now` past window, resume.
- `TestNewRejectsMissingDeps` — nil deps + empty `BaseURL` each fail
  construction.

## 9. Trade-offs

- **UX**: 4 RPCs is the minimum for the morning-brief use case
  (`ListMyIssues`) and the operator's "I want to log this bug right
  now" use case (`CreateIssue` + `Comment`). Adding a 5th (transitions)
  is the most-requested gap — defer until a caller files it.
- **Performance**: `ListMyIssues` uses Jira's default page size (50);
  callers needing more pages reopen the gap.
- **Long-term**: not scaffolding transitions / sprints / boards keeps
  the consent surface auditable. Each future RPC adds a `Scope*`
  constant the reviewer can flag.

## 10. Out of scope (MVP)

- Issue transitions (open -> in-progress -> done).
- JQL search (free-form query).
- Sprints, boards, epics, links between issues.
- Attachments.
- Worklogs / time tracking.
- Webhooks.
- OAuth 3LO mode.
- Multi-workspace fan-out.
- Full-body audit capture (operator-toggle deferred).
