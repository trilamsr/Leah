---
slug: macos-ecosystem-integration
status: draft
phase: self-host
owner: leah
---

# macOS ecosystem integration (MVP)

## 1. Goal

The operator runs Leah on a Mac. To act as a personal assistant Leah must
read and understand the native macOS surface — Calendar, Contacts, Reminders,
Notes, Mail, Messages, Photos, Safari, Music, Screen Time, Focus, the active
app and window, Bluetooth, Wi-Fi, notifications.

Scope is **read-heavy + occasional one-shot write**. Leah is NOT a
bidirectional sync engine for Apple services — iCloud already does that.
What Leah adds is the cross-app composition layer (see the companion
knowledge-graph spec, `2026-06-10-knowledge-graph.md`) so a question like
"what's on for today?" or a morning-brief enrichment can fuse signals across
Calendar + Mail + Messages + Reminders without the operator opening five apps.

Single canonical doc — future macOS adapters (FaceTime, Health, Wallet read,
HomeKit read) reuse the inventory pattern and threat-model rows in §7.

## 2. Inventory matrix

| App / Signal | Data source | Access mode | TCC permission | Bytes/sec typical | Latency budget (cold query) | MVP / future |
|---|---|---|---|---|---|---|
| Calendar | `~/Library/Calendars/Calendar.sqlitedb` + EventKit | SQLite RO; AppleScript for write | Calendars | <1 KB/s | 50 ms | **MVP** |
| Contacts | `~/Library/Application Support/AddressBook/AddressBook-v22.abcddb` | SQLite RO | Contacts | <1 KB/s | 50 ms | **MVP** |
| Reminders | `~/Library/Reminders/Container_v1/Stores/Data-*.sqlite` | SQLite RO; AppleScript for write | Reminders | <1 KB/s | 100 ms | **MVP** |
| Notes | `~/Library/Group Containers/group.com.apple.notes/NoteStore.sqlite` | SQLite RO | Full Disk Access | 1–10 KB/s | 200 ms | **MVP** |
| Mail (Mail.app) | `~/Library/Mail/V*/MailData/Envelope Index` | SQLite RO | Full Disk Access | 1–10 KB/s | 300 ms | **MVP** (distinct from `internal/adapters/gmail/`) |
| Messages (iMessage/SMS) | `~/Library/Messages/chat.db` | SQLite RO | Full Disk Access | 1–50 KB/s | 200 ms | **MVP** (HIGH-sensitivity) |
| Photos | `~/Pictures/Photos Library.photoslibrary/database/Photos.sqlite` | SQLite RO (metadata only) | Photos | <1 KB/s | 500 ms | future (HIGH-sensitivity) |
| Safari (history + reading list) | `~/Library/Safari/History.db` + `Bookmarks.plist` | SQLite RO + plist | Full Disk Access | 1 KB/s | 100 ms | future |
| Music (Apple Music / play history) | `~/Music/Music/Music Library.musiclibrary/Library.musicdb` | SQLite RO | Full Disk Access | <1 KB/s | 200 ms | future |
| Screen Time | `~/Library/Application Support/Knowledge/knowledgeC.db` | SQLite RO | Full Disk Access | <1 KB/s | 200 ms | future |
| Focus mode | `defaults read com.apple.donotdisturb.db` + `~/Library/DoNotDisturb/DB/Assertions.json` | plist + JSON | none | <1 KB/s | 20 ms | **MVP** |
| Active app + frontmost window | `NSWorkspace` via AppleScript / `lsappinfo` | subprocess | Accessibility (for window title) | <1 KB/s | 20 ms | **MVP** |
| Bluetooth devices | `system_profiler SPBluetoothDataType` / IOBluetooth | subprocess | none | <1 KB/s | 1 s (cold) | future |
| Wi-Fi (current SSID, RSSI) | `/usr/sbin/networksetup` + `/usr/sbin/wdutil info` | subprocess | Location Services (for SSID on 13+) | <1 KB/s | 200 ms | **MVP** |
| Notifications | `~/Library/Group Containers/group.com.apple.usernoted/db2/db` | SQLite RO | Full Disk Access | 1 KB/s | 100 ms | future (OFF by default) |
| Shortcuts.app | `shortcuts run <name>` CLI | subprocess | none (per-shortcut TCC at first run) | n/a | per-shortcut | W29 |

"Cold query" = adapter `Query` against the local mirror DB after warm sync.
Direct Apple-store queries are 2–10× slower; the mirror exists to make Leah's
hot path stay under the latency budget.

## 3. Permissions + first-launch

macOS gates every cross-app read behind TCC (Transparency, Consent, Control).
Leah does **not** invent a backdoor; it requests grants the operator already
understands from any third-party productivity app.

- Each app's read adapter is registered with a TCC scope (see matrix).
- Grants are requested **only for adapters the operator opts into** at
  `leah connect <macapp>`. Connecting `messages` triggers Full Disk Access;
  connecting `calendar` triggers the Calendars TCC entry only.
- The `connect` flow mirrors the gmail/gcal pattern but the credential
  exchange is replaced by:
  1. CLI prints the required TCC entry and opens
     `x-apple.systempreferences:com.apple.preference.security?Privacy_<Scope>`.
  2. Operator toggles Leah on in System Settings.
  3. CLI verifies by attempting a 1-row read; success appends an
     `audit.jsonl` row with `Kind: "connect_macapp"` and the scope.
  4. The attestation question (drawn from
     `internal/attestation/pool.go` `Pick(scope)`) is asked and the answer
     recorded — operator consent is captured at the action grain matching
     gmail/gcal.
- A missing TCC grant returns the adapter sentinel `ErrPermissionDenied`,
  never panics, never retries — re-running `leah connect <macapp>` is the
  recovery path.

### First-launch checklist (operator-facing)

```
leah init
  ├── detect macOS version + hardware
  ├── recommend default set: calendar, contacts, reminders, focus, wifi, active-app
  ├── for each: open System Settings → Privacy & Security → <Scope>
  ├── verify read → append audit row → write 0600 mirror entry
  └── opt-in to optional: notes, mail, messages, notifications, photos
```

`leah init` is a wizard — not a daemon command. Out of scope for this MVP
spec body (lands W31, see plan); the spec defines the shape so adapters
can be written against it.

## 4. Read-pipeline architecture

```
+-------------------------+        +---------------------------+
|  Apple-owned databases  |  RO    |   Per-app reader.go       |
|  (~/Library/...)        +--------> internal/macos/<app>/     |
+-------------------------+        +-------------+-------------+
                                                 |
                                                 v
                                  +-------------------------------+
                                  |  internal/macos/mirror        |
                                  |  ~/.leah-state/macos-mirror.db|
                                  |  (SQLite, 0600, schema v1)    |
                                  +-------------+-----------------+
                                                |
                                                v
                                  +-------------------------------+
                                  |  Query surface (Item normal.) |
                                  |  feeds knowledge graph        |
                                  +-------------------------------+
```

- **Read-only with `immutable=1`.** Every Apple SQLite is opened
  `file:<path>?mode=ro&immutable=1&_journal_mode=OFF&_query_only=1`.
  `immutable=1` tells SQLite the file will not change under it; this avoids
  the WAL-recovery dance that fights iCloud-sync writes and produces the
  "database is locked" failure mode the matrix calls out. The cost — we
  must re-open on each sync tick to see new rows — is the right trade per
  README.md priority (UX > performance): correctness over hot-loop speed.

- **Per-app reader.go.** Each app lives in
  `internal/macos/<app>/reader.go` implementing the `MacApp` interface
  (see §6). A reader translates Apple's per-app schema into the
  cross-app normalized `Item` shape — the mirror knows nothing about Apple
  schemas, only `Item`.

- **Local mirror.** `~/.leah-state/macos-mirror.db` is a single SQLite
  database, schema versioned `int` (mirror PR #58 int-parse compare
  pattern). Reasons:
  1. **Single query surface** for the knowledge layer — one place to
     join across apps without re-opening Apple stores per query.
  2. **Hammer-protection** — Apple's stores get touched every 60 s by
     Leah, not every Reasoner call.
  3. **Crash-safe** — Apple-store WAL corruption from Leah is impossible
     because Leah never holds a write lock on them.

- **60 s sync tick.** Configurable via `~/.leah-state/config.toml`
  `[macos] sync_interval_sec = 60`. The daemonloop `LastTick` field
  (PR #45) is the timer source — no second timer wheel. Skew on resume
  is intentional: a laptop waking from sleep will catch up on one tick
  instead of replaying queued ticks.

- **Schema versioning.**
  `macos_mirror_schema_version` row in a `meta` table. On boot, if
  schema version on disk < adapter version, the mirror is truncated and
  re-synced from Apple stores (Apple stores are the source of truth).
  No migrations — truncate is simpler and the data is reproducible.

## 5. Write pipeline (one-shot)

MVP write scope: create a Calendar event, mark a Reminder complete. Two ops,
both AppleScript subprocess. Rationale per README.md `UX > performance >
long-term`:

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| EventKit via CGO Swift bridge | Native, lowest latency, type-safe | Requires Swift toolchain in build, slower CI, blocks Linux-host dev | Defer |
| `osascript` AppleScript subprocess | Pure Go caller, no toolchain, well-documented Apple shape | ~80 ms overhead per call, parses string output | **MVP** |
| Direct SQLite write | Fastest in theory | Would corrupt Apple's WAL guarantees; iCloud sync would clobber | Banned |

Subprocess shape:

```go
// internal/macos/calendar/writer.go
func (w *Writer) CreateEvent(ctx context.Context, e Event) (id string, err error) {
    // gateAndAttest(ctx, ScopeCreateEvent) first — matches gmail/gcal
    // then exec.CommandContext(ctx, "osascript", "-l", "AppleScript", "-e", script, args...)
}
```

Future migration: when the Swift toolchain lands in CI (out of scope for
this spec), the `Writer` is the only seam to swap. Public `MacApp` surface
does not change.

## 6. Per-app adapter interface

```go
// internal/macos/macapp.go
type MacApp interface {
    Name() string
    Available(ctx context.Context) bool
    Sync(ctx context.Context) error
    Query(ctx context.Context, q Query) ([]Item, error)
}

type Item struct {
    ID          string     // adapter-scoped, e.g. "calendar:EVT-..."
    Title       string
    Body        string
    Timestamp   time.Time
    Source      string     // "calendar" | "messages" | ...
    ContactRefs []string   // canonicalized email / phone refs
    Tags        []string
}

type Query struct {
    Since, Until time.Time
    ContactRef   string
    Text         string  // FTS5 against mirror
    Limit        int
}
```

- `Name` is the adapter handle used in `leah connect <name>` and in the
  mirror DB `source` column.
- `Available` is a fail-fast check (binary present, TCC granted, file
  exists). Returns `false` instead of erroring so `leah init` can probe
  without raising.
- `Sync` reads from the Apple store into the mirror. Idempotent. Must
  succeed in <2 s for a typical store (Mail is the worst case).
- `Query` reads ONLY from the mirror — never the Apple store. The mirror
  has FTS5 on `Title+Body` for the text path.

Sentinel errors (package-level):
`ErrPermissionDenied`, `ErrSourceUnavailable`, `ErrAttestationDenied`,
`ErrMirrorCorrupt`.

## 7. Threat model

This is the load-bearing section. macOS integration crosses a privacy
boundary the gmail/gcal adapters do not — Apple's stores hold the
operator's family group chats, doctor's-appointment notes, and Photos
metadata. Treat every adapter as a leak surface.

| Risk | Likelihood | Mitigation |
|---|---|---|
| Apple-DB read mid-iCloud-sync corrupts read | Medium (laptops sync often) | `mode=ro&immutable=1`, retry once on `SQLITE_BUSY` with 50 ms backoff, then surface `ErrSourceUnavailable`. Mirror retains last-good rows. |
| TCC scope creep — operator agrees once, Leah expands later | Low (CLI is explicit) | Adapter registers a fixed scope at compile time. Adding an adapter requires a new `connect` flow + new attestation row. No runtime scope escalation. |
| Audit-log explosion from high-frequency events (active app changes 100×/min) | High (if naive) | Debounce active-app changes to ≥5 s residency before logging; summarize Bluetooth/Wi-Fi state-change events at 60 s aggregation; per-app daily cap (default 10 000 rows) — overflow drops with a single "dropped N events" summary. |
| Mirror DB leak via filesystem read | Medium (any local malware) | `~/.leah-state/macos-mirror.db` `0600`. Parent dir `0700`. Matches gmail/gcal token convention. No content in temp files. |
| Cross-app correlation = standalone privacy concern | Medium (this is the feature) | `~/.leah-state/config.toml` `[macos] cross_app_join = true|false`. Default `true` but explicitly affirmed at `leah connect` time via attestation question. Operator can flip to `false` and the knowledge graph downgrades to per-source queries only. |
| Notification surface = ambient screen-reading | High (notifications often contain MFA codes) | Notifications adapter is OFF by default. Operator must `leah connect notifications --acknowledge-mfa-risk`. Per-category opt-in (e.g. allow Calendar notifs, block all banking). |
| Foreground-app signal leaks sensitive surface ("Leah knows when I open Banking app") | Medium | Operator-maintained blocklist `~/.leah-state/macos-foreground-blocklist.txt`. App bundle IDs match prefix; blocklisted app → foreground signal records `Source: "active-app"` with `Title: "(redacted)"`. Default blocklist ships with `com.apple.Keychain`, common banking/health bundle IDs. |
| Photos + Messages content = HIGHEST sensitivity | High (extreme privacy weight) | Both adapters require an EXTRA attestation question per `internal/attestation/pool.go` `Pick(scope)` using the **high-sensitivity scope set** (`macos:messages:read`, `macos:photos:read`). Pool MUST be loaded with these scopes registered or `connect` fails closed. Body content for Messages is **stored in the mirror only as a 256-char prefix** by default; full body requires a second toggle. |
| Reasoner exfiltration of mirror content | Medium (remote LLM is the threat model) | Per the knowledge-graph spec §5, remote reasoner gets **derived scalars only** — never raw mirror rows. The macos package never marshals an `Item.Body` into a Reasoner prompt directly; the knowledge graph projects to scalars first. |
| Crash dump contains mirror path or row | Low | `Item.String()` does not include `Body`; `%v` on the struct elides body via custom formatter. Errors never wrap an `Item`. |
| AppleScript subprocess injected by malicious mirror row | Low (mirror is local-write, local-read) | Writers never compose AppleScript from mirror rows; AppleScript args are passed via `-e` template with `osascript` argv positional substitution, never string interpolation. |

## 8. Test plan

- **Hermetic per-adapter tests.** Each reader gets a golden SQLite fixture
  at `internal/macos/<app>/testdata/<app>.sqlite` populated with 5–20
  representative rows. Tests assert `Sync` produces the expected mirror
  rows and `Query` returns the expected `Item` slice.
- **Mirror-sync loop integration test.** A `fakeMacApp` array driven by a
  test clock advances the daemonloop `LastTick` and asserts each
  adapter's `Sync` was called once per tick within a 5 s budget.
- **Permission-denied path test.** Stand-in `Available()` returning false
  must short-circuit `Sync` without panic, must not create mirror rows.
- **Schema-version bump test.** Mirror at version N-1 on disk; adapter at
  N; assert truncate-and-resync runs once.
- **Attestation-denied path test.** Writers (`Writer.CreateEvent`,
  `Writer.CompleteReminder`) reject when `Attestor.Attest` returns
  non-nil; the `osascript` subprocess MUST NOT be invoked
  (use a `failingExec` that fails the test if called).
- **NO real-data tests in CI.** A test that reads the developer's actual
  `~/Library/Calendars` is forbidden by CI lint — both for reproducibility
  and because CI machines are not the operator's machine.

## 9. Distribution

- **Homebrew formula** — `brew install trilamsr/leah/leah`. Recommended
  path. Formula installs the binary signed + notarized (so Gatekeeper
  does not block first run) and seeds the launchd plist for the daemon.
- **Signed + notarized standalone binary** at
  `https://leah.maydow.com/download/leah-darwin-arm64` for operators who
  refuse Homebrew. Same notarization profile as the brew binary.
- **NOT the Mac App Store.** App Store sandboxing blocks the
  cross-app SQLite read pattern — Full Disk Access cannot be granted to
  sandboxed apps. The App Store path would require rewriting every
  adapter against EventKit/Contacts/MessageKit native APIs, which is the
  CGO-Swift rabbit hole §5 already rejected.
- **First-launch:** `leah init` walks the operator through TCC grants
  per chosen integrations.

## 10. Out of scope (MVP)

- iCloud Drive content scanning (file-by-file indexing of `~/Library/Mobile Documents/`).
- Family Sharing — shared Calendar / Reminders / Photos.
- Apple Wallet, Apple Pay, transactional surfaces.
- Home.app / HomeKit — device state and automations.
- iOS-only apps surfaced via Continuity (Handoff, Universal Clipboard).
- Time Machine — backup metadata reads.
- Live notification streaming (NotificationCenter LaunchAgent) — MVP
  reads the SQLite snapshot only, on a 60 s tick.
- Mail attachment body extraction (MVP indexes Envelope Index headers
  only; body extraction defers until a caller files an issue).
- Photos image-content indexing (CoreML embedding pipeline) — MVP reads
  EXIF + face/album metadata only.

Each of the above reopens when a real caller files an issue citing the
gap, mirroring gmail/gcal MVP discipline.

> All internal paths in this doc reflect the pre-2026-07-09 layout; current tree per `git ls-tree -d --name-only HEAD:internal/`.
