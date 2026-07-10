# Leah Social-Media Strategist — MVP Design

## Goal

Leah generates social-media posts (text + image + 5–10s short clip) for LinkedIn / Instagram / Facebook / TikTok from the operator's git commits, voice-text dumps, and news-take inputs. The operator posts manually — leah never authenticates to a social platform. The subsystem ships behind `leah strategist <sub>` with a maildir-style queue/inbox the operator inspects and curates with plain `ls`/`cat`/`vim`.

## Locked decisions (operator-approved upstream)

- Channels: LinkedIn, Instagram, Facebook, TikTok. Personal use. No platform OAuth.
- Sources: git commits, voice-text dumps, news+take. Strategist routes per slot.
- Media stack: Higgsfield (Sora 2 / Veo 3 / Seedance / DoP / Soul image / Flux Kontext via single key) + OpenAI TTS + local `ffmpeg` + `ImageMagick`. `leah doctor` shims check `brew install ffmpeg imagemagick`.
- Persona: operator-defined `~/.config/leah/strategist/persona.md`. Single-voice fallback when file absent. (PR 338.)
- Cadence: hybrid pre-generated 7-day queue PLUS 3–5 inbox candidates per `queue refill` invocation; `leah strategist next` pops from queue; `leah strategist inbox` lists candidates. "Per day" is operator-driven — they run `queue refill` when they want depth restored; no cron in v1.

## In-spec decisions

### A — Video scope: text + image + 5–10s clip, no voiceover (A2)

- A1 (text+image only) ships nothing motion-bearing — defeats the wedge against generic blog-style assistants on day one.
- A3 (clip + TTS voiceover via ffmpeg compose) doubles the adapter surface (openai_tts) and adds filter-graph sync + dual content-policy refusal handling — ship-in-session rules it out.
- A2 reuses the single Higgsfield key already required for image (Veo/Sora share auth). One adapter, one failure mode, voiceover deferred to v2 behind a `--voice` flag.

### B — Storage shape: maildir-style markdown dirs (B3)

- Operator inspects with `ls`/`cat`/`vim`; B2's SQLite needs `sqlite3 + schema knowledge` to debug.
- Filesystem `rename(2)` between `queue/ → sent/` (and `inbox/ → queue/`) is atomic — state transitions ARE the audit log; no second store.
- B1's single `queue.yml` is a write-contention surface as soon as `leah strategist next` and `leah strategist inbox approve` race; one-file-per-item sidesteps.

### C — Agent shape: stateless per-invocation (C1)

- C2 daemon is premature — operator demand pattern is unobserved; queue-watcher state isn't worth corrupting before we know the cadence.
- C3 cron coupling MVP success to launchd plist correctness blocks ship; add via `leah init` plist extension after 5+ days of manual use.
- C1 keeps each invocation deletable — `leah strategist next` is a pure function of `persona.md + queue/*.md + sources`; no daemon to migrate.

## Subsystem decomposition

```
cmd/leah/strategist.go                 CLI dispatch (post, next, inbox, queue, doctor)
  │
  ▼
internal/strategist/                   orchestration package
  ├── persona/   strategist persona.md load + fallback voice (concrete; distinct from internal/persona/ workspace store)
  ├── store/     maildir state machine (concrete struct MailDir)
  ├── generator/ Compose(): slot→source routing + prompt assembly + LLM call + asset orchestration (concrete func)
  └── source/
       ├── git/   git-log → candidate facts   (implements Source)
       ├── voice/ voice-text dump → candidate facts   (implements Source)
       └── news/  news+take → candidate facts   (implements Source)
  │
  ▼
internal/adapters/higgsfield/          image + clip transport + adapter
internal/connect/registry.go           token registration (`higgsfield`)
```

### Types + seams

Interface seams ONLY where a fake-or-real swap earns its keep: the LLM call site and the Higgsfield HTTP transport. Everything else is a concrete struct — collapses cleanly, no premature `interface{}` ceremony.

```go
// internal/strategist/store — CONCRETE struct, no interface (one impl, no second
// planned). v2 can extract an interface if (and only if) a second backend lands.
type Item struct {
    ID       string    // ULID
    Channel  string    // linkedin|instagram|facebook|tiktok
    Slot     string    // commit|voice|news
    Text     string
    ImageRef string    // absolute path; lives under <stateDir>/strategist/assets/
    ClipRef  string    // absolute path; empty when clip-less
    Created  time.Time
}
type MailDir struct{ root string }            // root = <configDir>/strategist/
func Open(root string) (*MailDir, error)
func (m *MailDir) Enqueue(it Item) error      // write to queue/<id>.md; idempotent: existing-id → error
func (m *MailDir) Inbox(it Item) error        // write to inbox/<id>.md; idempotent: existing-id → error
func (m *MailDir) Next() (Item, error)        // read oldest queue/*.md; return; caller MUST then call Sent
func (m *MailDir) Sent(id string) error       // atomic rename queue/<id>.md → sent/<id>.md
func (m *MailDir) Killed(id string) error     // atomic rename <any>/<id>.md → killed/<id>.md
func (m *MailDir) ListQueue() ([]Item, error)
func (m *MailDir) ListInbox() ([]Item, error)

// internal/strategist/source — interface because there ARE three implementers
// (git, voice, news) and the CLI dispatches across them as a set.
type Source interface {
    Name() string                                   // "git" | "voice" | "news"
    Candidates(ctx context.Context, since time.Time) ([]Fact, error)
}
type Fact struct {
    Slot    string  // commit|voice|news
    Summary string  // short input the generator riffs on
    Origin  string  // path or URL — kept in front-matter for traceability
}

// internal/strategist/generator — CONCRETE Compose function; the LLM seam is the
// LLMClient interface below (fakeable for golden-prompt tests).
type LLMClient interface {
    Complete(ctx context.Context, system, user string) (string, error)
}

// Outcome discriminates routing so the CLI has a single switch — known failures
// (rate-limit, partial asset, policy refusal) are values, not errors.
// Compose's error return is reserved for unexpected failures only (panic-shaped).
type Outcome int
const (
    OutcomeQueued  Outcome = iota  // healthy → queue/
    OutcomeInboxed                 // soft-fail (rate-limit, clip timeout) → inbox/, retryable
    OutcomeKilled                  // hard-fail (policy refusal, auth) → killed/, do not retry
)
type Result struct {
    Item    store.Item
    Outcome Outcome
    Reason  string  // populated when Outcome != OutcomeQueued; written to front-matter
}
func Compose(ctx context.Context, llm LLMClient, hf *higgsfield.Adapter,
    channel string, facts []Fact, persona string) (Result, error)

// internal/adapters/higgsfield — mirrors slack adapter shape (proven seam:
// Transport is faked in every adapter test in this repo; pattern is load-bearing).
type Transport interface {
    GenerateImage(ctx context.Context, key, prompt string) (bytes []byte, mime string, err error)
    SubmitClip(ctx context.Context, key, prompt string, seconds int) (jobID string, err error)
    PollClip(ctx context.Context, key, jobID string) (status string, mp4 []byte, err error)
}

// Attestor and TokenSource follow the SAME shape as slack.Attestor /
// slack.TokenSource (internal/adapters/slack/slack.go:61,68). The base
// connect.Attestor interface lives in internal/connect/connect.go:28; each
// adapter declares its own narrow local interface so test fakes don't drag in
// the full connect surface. Defined inline here to keep the file self-contained,
// satisfied by the existing connect.Attestor / connect.TokenSource impls at
// wire-up time. Verified against repo: `grep -n "type Attestor" internal/`.
type Attestor interface {
    Attest(ctx context.Context, scope string) error  // returns ErrAttestationDenied on operator decline
}
type TokenSource interface {
    Token(ctx context.Context) (string, error)       // reads <stateDir>/connect/higgsfield.token
}
type Adapter struct {
    att   Attestor
    ts    TokenSource
    tr    Transport
    audit AuditSink
    now   func() time.Time
    m     *connectadapter.Metrics
}
```

**Token write/read flow:** `leah connect higgsfield` (registered via the one-line addition in `internal/connect/registry.go`) uses the existing `newTokenPaste(tokenPasteSpec{name, fields, assemble: rawToken}, ...)` helper (`internal/connect/tokenpaste.go:24`, type signature `assemble func([]string) string` confirmed against current code), which prompts for the API key on stdin and writes it to `<stateDir>/connect/higgsfield.token` with mode `0600`. `TokenSource.Token` reads that same path. No new transport wiring — both ends already exist; the registry-line addition is sufficient.

**LLM provider:** `LLMClient` is satisfied by the existing `internal/reasoner` package (Claude via the same path `ask`/`ship` use). No new provider. Cost flows through the existing budget gate.

**Idempotency / collision:** `Enqueue` and `Inbox` use `O_CREATE|O_EXCL` — duplicate ID is an error, not a silent overwrite. IDs are ULIDs (lexicographic + monotonic), so collisions are vanishingly rare; an error is correct and tells the caller a logic bug exists.

**Persona load + fallback (CLI-side, before Compose):** the CLI does `personaText, err := strategistpersona.Load(configDir)`; on `os.ErrNotExist` it substitutes the hard-coded default voice constant (`strategistpersona.DefaultVoice`) and prints one stderr WARN line. By the time `Compose` is called, the `persona string` parameter holds the **full persona-markdown body** (not a voice ID or file path) — Compose splices it verbatim into the LLM system prompt. Empty-string is unreachable.

**Why a new `internal/strategist/persona/` and not the existing `internal/persona/`:** the existing `internal/persona` package is a SQLite-backed *workspace* persona store (`Open(path) → *Store`, `Load(workspace) → Persona`, used by `reasoner.Ask`'s system-prompt prefix) — different lifecycle, different storage, different consumer. The strategist persona is a single markdown file on disk under `<configDir>`, edited by hand, never written by leah. Folding it into `internal/persona` would force the workspace store to learn about file-backed personas — net surface increase, not reuse. Kept separate.

## CLI surface — `cmd/leah/strategist.go`

```
leah strategist post           # generate ONE post NOW for prompted channel + slot, write to queue/
leah strategist next [--channel=linkedin]  # pop next queued; print body + asset paths; rename → sent/
leah strategist inbox          # list candidates: ID | channel | slot | summary
leah strategist inbox approve <id>     # mv inbox/<id>.md queue/<id>.md
leah strategist inbox kill <id>        # mv inbox/<id>.md killed/<id>.md
leah strategist queue          # list queued items
leah strategist queue refill   # ensure ~7-day queue depth: generate up to (7 - len(queue)) items
leah strategist doctor         # check higgsfield key present + ffmpeg + ImageMagick installed
```

Dispatch wires into `cmd/leah/main.go` `runCommand` switch under `case "strategist":` — single-owner edit per README.md.

### Per-subcommand contract

Exit code precedence is fixed left-to-right: usage check (exit 2) → config/preflight check (exit 2: missing token, missing required tool) → runtime (exit 3). If higgsfield.token is missing AND no candidate facts exist, `strategist post` returns exit 2 (token check fires first; no LLM call is charged).


| Subcommand          | Exit 0                                       | Exit 2 (usage / config)            | Exit 3 (runtime)                          |
|---------------------|----------------------------------------------|-------------------------------------|-------------------------------------------|
| `post`              | item written to queue/ OR inbox/ OR killed/, ID + outcome printed | bad flag, missing higgsfield token  | no candidate facts since since-window     |
| `next`              | body + asset paths printed, item renamed to sent/ | bad flag                           | empty queue                               |
| `inbox`             | table of pending candidates printed          | bad flag                           | —                                         |
| `inbox approve <id>`| `mv inbox/<id>.md queue/<id>.md`             | bad flag, missing id arg           | id not in inbox                           |
| `inbox kill <id>`   | `mv <any>/<id>.md killed/<id>.md`            | bad flag, missing id arg           | id not found                              |
| `queue`             | table of queued items printed                | bad flag                           | —                                         |
| `queue refill`      | "generated N items" line, N ≥ 0              | bad flag                           | higgsfield error during fill (partial-fill commits items already written) |
| `doctor`            | "all tools present" line                     | bad flag                           | one or more of {higgsfield-token, ffmpeg, ImageMagick} missing — each printed on its own line |

## Connect registry additions

`internal/connect/registry.go`: append one entry —

```go
newTokenPaste(tokenPasteSpec{name: "higgsfield", fields: []string{"API key"}, assemble: rawToken}, os.Stdin, os.Stdout),
```

`openai` is NOT registered for v1 (TTS deferred). When v2 adds voiceover, add a second token-paste entry.

## Data flow

```
operator: `leah strategist post`
   │
   ▼
strategist.CLI ── persona.Load(configDir)
   │              └─ on ErrNotExist → persona.DefaultVoice + stderr WARN
   │            ── for each Source in {git, voice, news}: source.Candidates(since=24h)
   │              └─ on err → log, skip source; if ALL sources fail → exit 3
   │            ── generator.Compose(llm, hf, channel, facts, persona) → Result
   │                    │
   │                    ▼
   │            reasoner.Complete(text body) ── higgsfield.GenerateImage(prompt)
   │                                         ── higgsfield.SubmitClip(prompt, 8s)
   │                                         ── higgsfield.PollClip(jobID)  ── poll 10s, deadline 5 min
   │            ── assets written to <stateDir>/strategist/assets/<id>/
   │            ── switch Result.Outcome:
   │                  Queued  → store.Enqueue → <configDir>/strategist/queue/<id>.md
   │                  Inboxed → store.Inbox   → <configDir>/strategist/inbox/<id>.md
   │                  Killed  → write directly to <configDir>/strategist/killed/<id>.md
   ▼
operator: `leah strategist next` → prints body + asset paths → store.Sent → sent/
```

### Queue item markdown shape

```markdown
---
schema: 1
id: 01J8ABCDEF
channel: linkedin
slot: commit
origin: 730bcd7
image: /Users/x/.leah-state/strategist/assets/01J8ABCDEF/image.png
clip:  /Users/x/.leah-state/strategist/assets/01J8ABCDEF/clip.mp4
created: 2026-06-21T21:39:00-07:00
---

<post body in operator persona voice>
```

Front-matter is the single source of truth for asset paths — the renderer never re-derives them, so the operator can rename or move assets without breaking the queue.

**Schema versioning:** `schema: 1` is mandatory in front-matter. The loader rejects unknown schema values with a clear error rather than guessing. v2 fields (e.g. multi-persona) bump to `schema: 2` and the loader handles both — old items keep working without migration.

**Directory split rationale:** queue/inbox/sent/killed markdown lives under `<configDir>/strategist/` because it is operator-curated (`vim`, `git`, `dotfiles`-managed). Binary assets (PNG, MP4, up to MB-scale) live under `<stateDir>/strategist/assets/<id>/` because they are regeneration-cheap and don't belong in a dotfiles backup. `<configDir>` resolves to `~/.config/leah/` (XDG) and `<stateDir>` to `~/.leah-state/` per the existing `stateDir()` helper in `cmd/leah/main.go`.

## Error handling

| Failure                                | Behavior                                                              |
|----------------------------------------|-----------------------------------------------------------------------|
| `higgsfield.token` missing             | `strategist post` returns exit 2 with "run `leah connect higgsfield`" |
| `ffmpeg` or `ImageMagick` missing      | `strategist doctor` lists missing; `post` falls back to text+image (clip skipped) and prints WARN to stderr |
| Higgsfield rate-limit (HTTP 429)       | Compose returns `OutcomeInboxed`, reason=`rate_limited`; CLI writes to `inbox/` |
| Higgsfield content-policy refusal      | Compose returns `OutcomeKilled`, reason=`policy_refused`; CLI writes to `killed/`; no retry loop |
| LLM refusal / safety filter            | Compose returns `OutcomeKilled`, reason=`llm_refused`; sources untouched |
| Clip poll deadline reached (5 min wall) | Compose returns `OutcomeQueued` with `clip:""` and reason=`clip_unavailable`. Poll interval = 10s, deadline = 5 min hard cap. |
| One source fails (e.g., git unreadable, news feed timeout) | The failing source is skipped (logged to stderr WARN); other sources continue. If ALL sources fail, `strategist post` returns exit 3 ("no candidate facts"). Sources are independent — one bad feed never blocks the rest. |
| All sources return zero facts          | `strategist post` returns exit 3 with "no candidate facts since <RFC3339>"; no LLM call charged |

## Test strategy

- Fake `Transport` in `internal/adapters/higgsfield/transport_test.go` — fixture-driven; verifies error mapping (429 → `ErrRateLimited`, 403 → `ErrAuthFailed`, content-policy code → `ErrPolicyRefused`).
- Golden-file assertions for generator prompts: `internal/strategist/generator/testdata/golden/<slot>.txt` — captures `(channel, facts, persona) → prompt` so a persona drift or slot routing change shows in diff.
- Maildir store: temp-dir-per-test; covers concurrent `Next` + `inbox approve` (filesystem rename is atomic; assertion = exactly-one-success).
- CLI table-test: `cmd/leah/strategist_test.go` table over subcommands, asserts exit code + stdout shape using a real `MailDir` (temp dir) + fake `LLMClient` + fake `higgsfield.Transport`. (Store and Compose are concrete, so the seam for tests is at the LLM and HTTP boundaries.)
- Schema-version test: hand-crafted `schema: 2` fixture in `store/testdata/` asserts loader returns "unknown schema" error rather than silent misparse.
- No live Higgsfield calls in any test — transport is faked everywhere except the manual smoke runbook.

## Out of scope (deferred to v2)

- Full video (>10s, voiceover via openai_tts + ffmpeg compose).
- Multi-persona (e.g. founder voice vs engineer voice).
- Calendar-aware scheduling (post-at-best-time per platform).
- Platform API auth (autoposting). Operator posts manually in v1.
- Analytics ingestion (engagement-driven persona tuning).

## Deletion-default justification (additive surface accounting)

| Surface added                          | Why not reuse existing?                                                            |
|----------------------------------------|------------------------------------------------------------------------------------|
| `internal/adapters/higgsfield/`        | No existing image/video adapter. Net-new third-party API.                          |
| `internal/strategist/persona/`         | Existing `internal/persona/` is a SQLite *workspace* store; strategist persona is a hand-edited file under `<configDir>`. Folding would grow the workspace store, not shrink anything. ~40 LOC. |
| `internal/strategist/store/`           | Maildir state machine has no analog in repo. ~80 LOC; pure FS ops.                 |
| `internal/strategist/source/`          | git/voice/news ingestion exists in fragments (news.go, listen.go), but each returns its own typed value; unifying via the `Source` interface here is cheaper than threading 3 typed Gather calls through `Compose`. ~120 LOC across 3 files. |
| `internal/strategist/generator/`       | LLM prompt assembly + asset orchestration is strategist-specific. ~100 LOC.        |
| `cmd/leah/strategist.go`               | Single new subcommand entry, mirrors `brief.go` shape. ~150 LOC.                   |

No existing surface gets larger except `cmd/leah/main.go` (+1 switch case) and `internal/connect/registry.go` (+1 token-paste line). No package is deleted by this MVP; v2's voiceover work will revisit whether the `Generator` Compose can swallow openai_tts without growing the package.

## Spec self-review checklist

- [x] No `TBD` / `TODO` strings.
- [x] Decisions A/B/C each have 3-bullet justification.
- [x] Concrete structs default; interfaces only at fakeable seams (Source × 3 impls, LLMClient for golden tests, Transport for HTTP fakes). Store and Compose are concrete.
- [x] Every interface in "Types + seams" has either ≥3 implementers OR ≥1 fake-in-test consumer.
- [x] Connect registry edit is a one-line append — single-owner rule respected.
- [x] No scope creep: voiceover, multi-persona, autoposting all in "Out of scope".
- [x] State-vs-config directory split is explicit with rationale.
- [x] Idempotency (`O_CREATE|O_EXCL` on Enqueue/Inbox) is spelled out.
- [x] LLM provider is named (`internal/reasoner`); no "?" left in the data flow.
- [x] Clip poll deadline is concrete (10s interval, 5 min hard cap).
- [x] Compose return is `(Result, error)` with discriminated `Outcome` so CLI has one routing switch.
- [x] Source failure isolation spelled out (skip-one vs. fail-all-three) with exit-3 contract.
- [x] Attestor / TokenSource explicitly defined and traced to slack/gmail reuse.
- [x] Token write/read flow named end-to-end (`leah connect higgsfield` → tokenPaste → `<stateDir>/connect/higgsfield.token` → `TokenSource.Token`).
- [x] Persona fallback location pinned: CLI layer, before Compose; Compose never sees empty persona.
- [x] Front-matter `schema: 1` is mandatory; loader rejects unknown — v2 forward-compat without migration.
- [x] Per-subcommand exit-code contract spelled out.
- [x] Failure modes are concrete (exit code OR queue-state-transition), not narrative.
- [x] Deletion-default audit table accounts for every new surface.

> All internal paths in this doc reflect the pre-2026-07-09 layout; current tree per `git ls-tree -d --name-only HEAD:internal/`.
