# Leah — Widget Protocol v1 (data + lifecycle + registry)

Companion to:
- `2026-06-21-leah-macos-native-ui-design-v1.md` § 1.3 (Focus chamber), § 5.2 (chamber anatomy), § 9 (widget canvas visuals — concurrent author)
- Wizard / Settings surfaces: same doc § 1.5, § 1.6 (per-widget enable toggles live in Settings → Widgets)

**Scope:** the data protocol — JSON tool-call shape, daemon adapter contract, lifecycle, streaming frames, registry, security, extensibility, tests. **Not** visuals; the design doc owns layout, color, typography, motion.

**Protocol version:** `widget-protocol/1` (semver — bump major on breaking schema change).

---

## 1. Tool-call schema

The LLM invokes a single tool `render_widget`. Every payload conforms to a discriminated union keyed on `widget`. All schemas are JSON Schema draft-07.

### 1.0 Envelope (shared)

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "leah://widget/envelope/1",
  "type": "object",
  "required": ["widget", "id", "size"],
  "properties": {
    "widget":  { "type": "string", "enum": ["market","flights","calendar","weather","maps","table","chart","image","code","citation","stat","list","diff"] },
    "id":      { "type": "string", "pattern": "^[a-z0-9_-]{1,64}$", "description": "stable across refresh/pin; LLM may reuse id to update an existing tile" },
    "size":    { "type": "string", "enum": ["small","medium","large","hero"] },
    "refresh": { "type": ["integer","null"], "minimum": 5, "description": "seconds between auto-refresh; null = manual only" },
    "actions": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["label","callback"],
        "properties": {
          "label":    { "type": "string", "maxLength": 32 },
          "callback": { "type": "string", "pattern": "^leah://action/[a-z_]+(\\?.*)?$" },
          "icon":     { "type": "string" }
        },
        "additionalProperties": false
      },
      "maxItems": 4
    },
    "props":   { "type": "object" }
  },
  "additionalProperties": false
}
```

The `props` shape is constrained per-widget below.

### 1.1 `market`

Fetched (new `internal/markets/` adapter).

```json
{
  "$id": "leah://widget/market/1",
  "type": "object",
  "required": ["symbols","range"],
  "properties": {
    "symbols":    { "type": "array", "items": { "type": "string", "pattern": "^[A-Z0-9.\\-:]{1,12}$" }, "minItems": 1, "maxItems": 10 },
    "range":      { "type": "string", "enum": ["1D","5D","1M","3M","6M","1Y","5Y","MAX"] },
    "compare_to": { "type": ["string","null"], "description": "ISO 8601 timestamp baseline; e.g. yesterday-close" },
    "show":       { "type": "array", "items": { "enum": ["price","pct","volume","sparkline"] }, "default": ["price","pct","sparkline"] }
  },
  "additionalProperties": false
}
```

Example — *"market today vs yesterday"*:
```json
{ "widget":"market", "id":"mkt_spy_qqq", "size":"medium", "refresh":60,
  "props": { "symbols":["SPY","QQQ"], "range":"1D", "compare_to":"yesterday-close" } }
```

### 1.2 `flights`

Fetched (`internal/flights/`).

```json
{
  "$id": "leah://widget/flights/1",
  "type": "object",
  "required": ["origin","destination"],
  "properties": {
    "origin":      { "type": "string", "pattern": "^[A-Z]{3}$" },
    "destination": { "type": "string", "pattern": "^[A-Z]{3}$" },
    "depart":      { "type": "string", "format": "date" },
    "return":      { "type": ["string","null"], "format": "date" },
    "pax":         { "type": "integer", "minimum": 1, "maximum": 9, "default": 1 },
    "cabin":       { "type": "string", "enum": ["economy","premium","business","first"], "default": "economy" },
    "max_stops":   { "type": "integer", "minimum": 0, "maximum": 2 }
  },
  "additionalProperties": false
}
```

Example: `{ "widget":"flights","id":"fl_sfo_jfk_jul14","size":"large","props":{"origin":"SFO","destination":"JFK","depart":"2026-07-14","return":"2026-07-21","cabin":"business"} }`

### 1.3 `calendar`

Fetched (`internal/macos/calendar/`).

```json
{
  "$id": "leah://widget/calendar/1",
  "type": "object",
  "required": ["range"],
  "properties": {
    "range":     { "type": "string", "enum": ["today","tomorrow","this_week","next_7d","custom"] },
    "from":      { "type": "string", "format": "date-time" },
    "to":        { "type": "string", "format": "date-time" },
    "calendars": { "type": "array", "items": { "type": "string" }, "description": "calendar IDs; empty = all enabled" },
    "show_declined": { "type": "boolean", "default": false }
  },
  "additionalProperties": false
}
```

Example: `{ "widget":"calendar","id":"cal_today","size":"medium","refresh":300,"props":{"range":"today"} }`

### 1.4 `weather`

Fetched (`internal/weather/`).

```json
{
  "$id": "leah://widget/weather/1",
  "type": "object",
  "required": ["location"],
  "properties": {
    "location": { "type": "string", "description": "place name or 'lat,lon'" },
    "horizon":  { "type": "string", "enum": ["now","hourly_24h","daily_7d","daily_14d"], "default": "daily_7d" },
    "units":    { "type": "string", "enum": ["imperial","metric"], "default": "imperial" }
  },
  "additionalProperties": false
}
```

Example: `{ "widget":"weather","id":"wx_sf","size":"small","refresh":1800,"props":{"location":"San Francisco","horizon":"daily_7d"} }`

### 1.5 `maps`

Fetched (`internal/maps/`).

```json
{
  "$id": "leah://widget/maps/1",
  "type": "object",
  "oneOf": [
    { "required": ["center"], "properties": { "mode": { "const": "view" } } },
    { "required": ["route"],  "properties": { "mode": { "const": "route" } } }
  ],
  "properties": {
    "mode":   { "type": "string", "enum": ["view","route"], "default": "view" },
    "center": { "type": "object", "required": ["lat","lon"], "properties": { "lat": { "type": "number" }, "lon": { "type": "number" } } },
    "zoom":   { "type": "integer", "minimum": 1, "maximum": 20 },
    "pins":   { "type": "array", "items": { "type": "object", "required": ["lat","lon"], "properties": { "lat":{"type":"number"}, "lon":{"type":"number"}, "label":{"type":"string"} } } },
    "route":  { "type": "object", "required": ["from","to"], "properties": { "from": { "type": "string" }, "to": { "type": "string" }, "mode": { "enum": ["drive","walk","bike","transit"] } } }
  }
}
```

Example: `{ "widget":"maps","id":"map_route_office","size":"large","props":{"mode":"route","route":{"from":"home","to":"office","mode":"drive"}} }`

### 1.6 `table`

**Pure LLM output** (no fetch) — the LLM emits rows directly.

```json
{
  "$id": "leah://widget/table/1",
  "type": "object",
  "required": ["columns","rows"],
  "properties": {
    "columns": { "type": "array", "items": { "type": "object", "required": ["key","label"], "properties": { "key":{"type":"string"}, "label":{"type":"string"}, "align":{"enum":["left","right","center"]}, "format":{"enum":["text","number","pct","currency","date","relative_time"]} } }, "minItems": 1, "maxItems": 8 },
    "rows":    { "type": "array", "items": { "type": "object" }, "maxItems": 200 },
    "sort":    { "type": "object", "properties": { "column":{"type":"string"}, "dir":{"enum":["asc","desc"]} } }
  },
  "additionalProperties": false
}
```

Example: `{ "widget":"table","id":"tbl_prs","size":"large","props":{"columns":[{"key":"num","label":"#","format":"number"},{"key":"title","label":"Title"}],"rows":[{"num":321,"title":"server-pushed widget refresh"}]} }`

### 1.7 `chart`

**Pure LLM output** OR fetched (if `source.adapter` named). Discriminator on `source`.

```json
{
  "$id": "leah://widget/chart/1",
  "type": "object",
  "required": ["kind","series"],
  "properties": {
    "kind":   { "type": "string", "enum": ["line","bar","area","scatter","sparkline"] },
    "x_axis": { "type": "object", "properties": { "label":{"type":"string"}, "type":{"enum":["time","numeric","category"]} } },
    "y_axis": { "type": "object", "properties": { "label":{"type":"string"}, "min":{"type":"number"}, "max":{"type":"number"} } },
    "series": { "type": "array", "items": { "type": "object", "required": ["name","points"], "properties": { "name":{"type":"string"}, "points":{"type":"array","items":{"type":"object","required":["x","y"],"properties":{"x":{},"y":{"type":"number"}}}} } } },
    "source": { "type": "object", "properties": { "adapter":{"enum":["market","weather"]}, "ref":{"type":"string"} } }
  }
}
```

### 1.8 `image`

Fetched (daemon-side; URL never resolved by LLM).

```json
{
  "$id": "leah://widget/image/1",
  "type": "object",
  "required": ["url"],
  "properties": {
    "url":     { "type": "string", "format": "uri", "pattern": "^https://" },
    "alt":     { "type": "string" },
    "caption": { "type": "string" }
  },
  "additionalProperties": false
}
```

### 1.9 `code`

**Pure LLM output.**

```json
{
  "$id": "leah://widget/code/1",
  "type": "object",
  "required": ["language","source"],
  "properties": {
    "language":     { "type": "string", "pattern": "^[a-z0-9_+\\-]{1,24}$" },
    "source":       { "type": "string", "maxLength": 16384 },
    "filename":     { "type": "string" },
    "highlight":    { "type": "array", "items": { "type": "integer", "minimum": 1 } },
    "runnable":     { "type": "boolean", "default": false }
  },
  "additionalProperties": false
}
```

### 1.10 `citation`

Fetched (daemon resolves URL via `internal/web/` if present, else metadata-only).

```json
{
  "$id": "leah://widget/citation/1",
  "type": "object",
  "required": ["url"],
  "properties": {
    "url":       { "type": "string", "format": "uri" },
    "title":     { "type": "string" },
    "author":    { "type": "string" },
    "published": { "type": "string", "format": "date" },
    "snippet":   { "type": "string", "maxLength": 400 },
    "kind":      { "type": "string", "enum": ["paper","article","docs","gh_release","gh_issue","tweet","video","other"] }
  },
  "additionalProperties": false
}
```

### 1.11 `stat`

**Pure LLM output.** Single big number.

```json
{
  "$id": "leah://widget/stat/1",
  "type": "object",
  "required": ["label","value"],
  "properties": {
    "label":      { "type": "string", "maxLength": 60 },
    "value":      { "type": ["string","number"] },
    "unit":       { "type": "string" },
    "delta":      { "type": "number" },
    "delta_unit": { "type": "string", "enum": ["abs","pct"] },
    "trend":      { "type": "string", "enum": ["up","down","flat"] }
  },
  "additionalProperties": false
}
```

### 1.12 `list`

**Pure LLM output.**

```json
{
  "$id": "leah://widget/list/1",
  "type": "object",
  "required": ["items"],
  "properties": {
    "items": { "type": "array", "maxItems": 50, "items": { "type": "object", "required": ["text"], "properties": { "text":{"type":"string"}, "meta":{"type":"string"}, "icon":{"type":"string"}, "callback":{"type":"string","pattern":"^leah://action/"} } } },
    "ordered": { "type": "boolean", "default": false }
  },
  "additionalProperties": false
}
```

### 1.13 `diff`

**Pure LLM output.**

```json
{
  "$id": "leah://widget/diff/1",
  "type": "object",
  "required": ["hunks"],
  "properties": {
    "language": { "type": "string" },
    "filename": { "type": "string" },
    "hunks": { "type": "array", "items": { "type": "object", "required": ["old","new"], "properties": { "old":{"type":"string"}, "new":{"type":"string"}, "context_before":{"type":"string"}, "context_after":{"type":"string"}, "old_start":{"type":"integer"}, "new_start":{"type":"integer"} } } }
  },
  "additionalProperties": false
}
```

---

## 2. Data adapter contract

```go
// internal/widget/adapter.go
package widget

import (
    "context"
    "encoding/json"
    "time"
)

type Payload struct {
    Data        json.RawMessage `json:"data"`         // adapter-shaped result (matches widget's render contract)
    FetchedAt   time.Time       `json:"fetched_at"`
    StaleAfter  time.Duration   `json:"stale_after"`  // hint for refresh scheduler
    Source      string          `json:"source"`       // adapter id, for telemetry
    Etag        string          `json:"etag,omitempty"` // for cheap refresh diffing
}

type WidgetAdapter interface {
    Type() string                                                        // "market","flights",...
    Validate(props json.RawMessage) error                                 // JSON Schema + semantic checks
    Fetch(ctx context.Context, props json.RawMessage) (Payload, error)
    Refresh(ctx context.Context, id string, props json.RawMessage, prev *Payload) (Payload, error)
}

// Pure-LLM widgets implement a no-op adapter that echoes props as data:
type PureAdapter struct{ kind string }
func (p PureAdapter) Type() string { return p.kind }
func (p PureAdapter) Validate(props json.RawMessage) error { return validateSchema(p.kind, props) }
func (p PureAdapter) Fetch(_ context.Context, props json.RawMessage) (Payload, error) {
    return Payload{Data: props, FetchedAt: time.Now(), Source: "llm"}, nil
}
func (p PureAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, _ *Payload) (Payload, error) {
    return p.Fetch(ctx, props)
}
```

### 2.1 Widget → adapter map

| Widget    | Adapter      | Status / source                                          |
|-----------|--------------|----------------------------------------------------------|
| market    | `markets`    | **NEW** — add `internal/markets/` (price quotes + history; suggested provider: Alpha Vantage or Yahoo Finance scrape — feeds-style polite poller; watchlist already stores symbols) |
| flights   | `flights`    | Reuse `internal/flights/`                                |
| calendar  | `calendar`   | Reuse `internal/macos/calendar/`                         |
| weather   | `weather`    | Reuse `internal/weather/`                                |
| maps      | `maps`       | Reuse `internal/maps/`                                   |
| citation  | `web`        | Reuse `internal/web/` for URL meta; `internal/papers/` and `internal/feeds/` (arxiv, releases) for typed `kind` enrichment |
| image     | `web`        | Daemon fetches + caches; never exposes URL to LLM round-trip |
| chart     | `markets` \| `weather` \| pure | Discriminator on `props.source.adapter`; absent → pure |
| table     | pure         | LLM emits rows                                           |
| code      | pure         | LLM emits source                                         |
| stat      | pure         | LLM emits value/delta                                    |
| list      | pure         | LLM emits items                                          |
| diff      | pure         | LLM emits hunks                                          |

Registration:
```go
// internal/widget/registry.go
var builtins = []WidgetAdapter{
    markets.New(...), flights.New(...), calendar.New(...), weather.New(...), maps.New(...),
    web.NewCitation(...), web.NewImage(...),
    PureAdapter{"table"}, PureAdapter{"code"}, PureAdapter{"stat"}, PureAdapter{"list"}, PureAdapter{"diff"},
    chart.New(markets, weather), // delegates by source
}
```

---

## 3. Lifecycle

State machine per tile: `spawning → live → refreshing → live | stale | error → dismissed`. Pinned tiles persist across chamber dismiss.

| Event | Trigger | Daemon | UI |
|---|---|---|---|
| **Spawn** | LLM emits `render_widget` tool-call mid-stream | `Validate(props)` → `Fetch()` → emit `widget.mount{id,data}` frame | Mounts tile at end of current prose block; "spawning" shimmer until first frame |
| **Refresh (auto)** | `refresh` timer expires | `Refresh(ctx,id,props,prev)` → emit `widget.update{id,data,etag}`; skip if `etag` unchanged | Animates value delta (see design doc § 3.2) |
| **Refresh (manual)** | Operator clicks refresh chip OR daemon push (MAY-19 B1) | Same as auto; bypasses timer | Same |
| **Pin** | Operator clicks pin | Append `{id, type, props, refresh}` to `~/.leah-state/pinned-widgets.json`; ambient HUD watcher (`internal/hud/`) re-reads file via fsnotify | Tile gains pin-state badge; ambient slot renders `small` variant |
| **Unpin** | Operator clicks pin again OR Settings → Pinned widgets → remove | Remove entry; HUD re-reads; ambient slot frees | Tile loses badge |
| **Dismiss** | Operator clicks × OR chamber close on unpinned tile | No state mutation; cancel refresh timer | Tile collapses (design doc § 3.2 dismiss curve) |
| **Error** | `Fetch`/`Refresh` returns non-nil err | If `prev` payload exists, emit `widget.stale{id,data:prev,reason}`; else `widget.error{id,reason,retry_in}` | Stale: oxblood underline + cached value + relative-age caption. Error: oxblood frame + retry chip |

**Cached-last-good cache:** `~/.leah-state/widget-cache/<adapter>/<sha256(props)>.json`. TTL = `Payload.StaleAfter` × 4 (cap 24h). Cache survives daemon restart; pin-driven refresh on daemon start re-warms.

**ID stability:** the LLM is instructed to reuse the same `id` when the user iterates (`"add MSFT"` → same `mkt_*` id). Re-emit with same `id` = update-in-place, not new tile. New `id` = new tile.

---

## 4. Widget registry

`~/.leah-state/widget-registry.json` — single source of truth for "what widget types this daemon understands and renders." Persisted (so Settings UI can flip per-widget enable without restart).

```json
{
  "protocol_version": "1",
  "updated_at": "2026-06-21T08:00:00Z",
  "widgets": [
    { "type": "market",   "version": "1", "source": "builtin", "enabled": true,
      "schema_uri": "leah://widget/market/1", "adapter": "markets",
      "default_size": "medium", "default_refresh": 60,
      "actions": ["pin","refresh","dismiss","change_range"] },
    { "type": "flights",  "version": "1", "source": "builtin", "enabled": true,
      "schema_uri": "leah://widget/flights/1", "adapter": "flights",
      "default_size": "large", "default_refresh": null,
      "actions": ["pin","refresh","dismiss","book"] }
    // ... one entry per widget type
  ]
}
```

**Versioning:** every entry pins a major version. The registry rejects tool-calls whose `widget` references an entry whose `enabled=false`, or where the daemon binary's compiled-in protocol version differs from the file's `protocol_version` (operator gets a settings prompt to upgrade — daemon writes `<file>.bak` and rewrites).

**Settings UI cross-ref** (design doc § 1.6): Settings → Widgets surface lists every entry with a toggle and a "configure defaults" disclosure.

---

## 5. Streaming + partial renders

### 5.1 Transport

Daemon → UI is one persistent local IPC channel (Unix socket at `~/.leah-state/leah.sock`; framing: length-prefixed JSON per frame). Same channel carries prose deltas and widget events — interleaving is by emission order.

### 5.2 Frame types

```ts
type Frame =
  | { kind: "prose.delta",  turn_id: string, seq: number, text: string }
  | { kind: "prose.end",    turn_id: string, seq: number }
  | { kind: "widget.mount", turn_id: string, seq: number, id: string, widget: string, size: string, props: object, data: object, refresh: number|null, actions?: Action[] }
  | { kind: "widget.update",turn_id: string, seq: number, id: string, data: object, etag?: string }
  | { kind: "widget.stale", turn_id: string, seq: number, id: string, data: object, reason: string, fetched_at: string }
  | { kind: "widget.error", turn_id: string, seq: number, id: string, reason: string, retry_in?: number }
  | { kind: "widget.unmount", turn_id: string, seq: number, id: string }
  | { kind: "turn.end",     turn_id: string, seq: number, cost: object }
```

`seq` is monotonic per `turn_id`; the UI uses it to reorder if the OS coalesces writes. `turn_id` lets a future operator-interrupt cancel an in-flight turn cleanly (cancel timers + remove unpinned tiles spawned by that turn).

### 5.3 Interleaving rule

When the LLM mid-stream emits a complete `render_widget` tool-call (the daemon JSON-decoder watches for a balanced `{...}` boundary while accumulating chunks), the daemon:
1. flushes any pending `prose.delta`,
2. validates + fetches (concurrent — does not block subsequent prose deltas),
3. on fetch return, emits `widget.mount` at the current `seq` position.

UI renders chunks in `seq` order: prose appends to current paragraph; widget mounts insert a tile *between* paragraphs (never mid-sentence). If a widget's fetch is still in-flight when the next prose chunk arrives, the daemon may emit a `widget.mount` with `data:{}` (placeholder; UI shows "spawning" shimmer) followed by a later `widget.update` once data lands — this preserves visual order.

---

## 6. Security

| Surface | Control |
|---|---|
| **Schema validation** | Every `render_widget` invocation runs `Validate(props)` before any I/O. Failure → no tile, single `widget.error` with reason; LLM gets a `tool_error` it can recover from. |
| **Type allowlist** | Only types present in `widget-registry.json` with `enabled=true` execute. Unknown type → reject. |
| **URL handling** | `image` and `citation` URLs are dereferenced by the daemon (`internal/web/`), never opened by the LLM directly. Daemon enforces: `https://` only, max body 10 MB, MIME allowlist for `image` (`image/png`, `image/jpeg`, `image/webp`, `image/gif`), 5s connect timeout, follow ≤3 redirects, no private IP ranges (RFC1918, link-local, loopback). Cached under `~/.leah-state/widget-cache/web/`. |
| **Action callbacks** | `actions[].callback` must match `^leah://action/[a-z_]+`. The daemon maintains an allowlist of action verbs (`pin`, `refresh`, `dismiss`, `book`, `change_range`, `open_url`, ...); unknown verbs are dropped at registry-load time. `open_url` action requires the URL be in a tile already rendered (i.e., previously vetted). |
| **Per-widget disable** | Settings → Widgets → toggle off flips `enabled=false` in registry; daemon hot-reloads via fsnotify. Disabled type → LLM call rejected with explanatory `tool_error` so the LLM can fall back to prose. |
| **Adapter sandboxing** | Each adapter runs inside the daemon process but with its own `context.WithTimeout` (default 5s fetch, 3s refresh). No adapter shells out. Network adapters use the shared `internal/web/` httpclient (TLS verify on, no proxies unless `LEAH_HTTP_PROXY` set). |
| **Pure-LLM widgets** | `table`, `code`, `stat`, `list`, `diff` still pass JSON-Schema validation; `code.source` capped at 16 KB; `table.rows` capped at 200; `list.items` capped at 50 to bound render cost. |
| **Telemetry** | Every validate/fetch/error emits to `internal/obs/` (`widget.validate`, `widget.fetch.duration_ms`, `widget.fetch.error`, `widget.cache.hit`). |

---

## 7. Extensibility

Adding a new widget type `foo` is three additive PRs (file-disjoint, parallelizable):

1. **Schema** — `internal/widget/schemas/foo.json` (draft-07); register `leah://widget/foo/1`.
2. **Adapter** — `internal/foo/adapter.go` implementing `WidgetAdapter`; wire into `widget.builtins`. If pure-LLM, use `PureAdapter{"foo"}` and skip the adapter package.
3. **UI component** — Swift/SwiftUI `FooTile` conforming to chamber tile protocol (see design doc § 9 once authored); registered in UI-side type map.

**v1 has no plugin system.** Third-party widgets are out of scope — the registry is closed-set and shipped in the daemon binary. Operator-toggleable, not operator-extensible. Reasoning: review surface + security review per widget; one operator does not need plugin breadth.

To deprecate a widget: set `enabled=false` in default registry, ship a migration that drops it from operator's registry on next launch.

Breaking schema change: bump `protocol_version` to `2`; ship a migrator that translates v1 pinned widgets → v2 (or drops with operator notification). The daemon refuses to start against a mismatched registry version without operator confirmation.

---

## 8. Test plan

### 8.1 Schema validation tests
`internal/widget/schema_test.go`

| Case | Expected |
|---|---|
| Each widget type — minimal valid payload | accepted |
| Each widget type — fully-populated valid payload | accepted |
| Each widget type — missing required field | rejected with `missing_required` |
| Each widget type — extra field | rejected (`additionalProperties:false`) |
| `id` over 64 chars / illegal chars | rejected |
| Unknown `widget` discriminator | rejected by registry, not schema |
| `code.source` > 16 KB | rejected |
| `table.rows` > 200 | rejected |
| `actions[].callback` not matching `leah://action/...` | rejected |
| `image.url` non-https | rejected |
| `image.url` pointing at 192.168.x.x | rejected by adapter (not schema) |

### 8.2 Adapter contract tests
For each non-pure adapter (`internal/{markets,flights,calendar,weather,maps,web}/adapter_test.go`):

- **Happy path** — mock upstream returns canned payload → `Fetch` returns Payload with non-zero `FetchedAt`, expected `Data` shape, expected `StaleAfter`.
- **Upstream 5xx** → returns error; cache untouched.
- **Upstream timeout** (`httptest` server delays past `context.WithTimeout`) → context-cancelled error.
- **Etag short-circuit** — `Refresh` with matching prev Etag → returns prev `Payload` with `FetchedAt` unchanged, no upstream call (verified via mock counter).
- **Validate** — golden good/bad props table.

Pure adapters: round-trip identity test — Fetch echoes props as data; Validate rejects malformed props per schema.

### 8.3 Lifecycle integration test
`internal/widget/lifecycle_test.go` — single fake adapter + in-memory state + in-memory IPC channel:

```
1. mount(id=m1, refresh=60s)        → assert: live, widget.mount frame
2. tick(61s)                         → assert: widget.update frame, value delta
3. pin(m1)                           → assert: pinned-widgets.json contains m1
4. simulate chamber dismiss          → assert: tile still tracked (pinned), refresh timer alive
5. simulate fetch error              → assert: widget.stale frame (cache hit) — NOT widget.error
6. drop cache, fetch error           → assert: widget.error frame
7. unpin(m1)                         → assert: pinned-widgets.json empty, ambient HUD watcher fires
8. dismiss(m1)                       → assert: widget.unmount, timer cancelled, no state mutation
```

### 8.4 Streaming test
`internal/daemonloop/stream_test.go`:

- Driver injects a synthetic LLM stream: `"hello "` → `"world."` → tool-call → `" continuing."` → end.
- Assert frame order on socket: `prose.delta("hello ")`, `prose.delta("world.")`, `widget.mount(...)`, `prose.delta(" continuing.")`, `turn.end`.
- Assert: `widget.mount` `seq` is between the two prose deltas; never split mid-prose-delta.
- Concurrency: simulate slow fetch (200ms) — assert `widget.mount` with `data:{}` placeholder emitted promptly, `widget.update` lands later; subsequent prose deltas are NOT blocked.

### 8.5 Security tests
- `image` URL = `http://...` → reject.
- `image` URL = `https://10.0.0.1/x.png` → adapter rejects (private IP).
- `render_widget` for a `widget` whose registry entry has `enabled=false` → reject with `tool_error`.
- Action callback `leah://action/rm_rf` → not registered, dropped before render.
- Registry hot-reload: write `enabled=false` to disk → next `render_widget` for that type fails within 1s.

### 8.6 Cross-doc parity check
A `make check` rule asserts:
- Every `widget` enum value in § 1.0 envelope has (a) a per-type schema file, (b) a registry entry, (c) an adapter (or `PureAdapter`), (d) at least one test case in § 8.1.
- Visual design doc § 9 widget list ⊇ this protocol's widget list (visuals may stub future widgets, but every protocol widget must have a tile design).

---

## Appendix A — Why these choices

- **Discriminated union on `widget`** beats per-tool-name proliferation: one tool definition the LLM learns once; new widgets don't grow the tool list. (Vercel AI SDK `generative-ui` pattern.)
- **`id` for stable updates** mirrors Slack Block Kit `block_id` and React keys; lets the LLM iterate ("add MSFT") without us inferring intent.
- **Pure-LLM widgets** (table/code/stat/list/diff/most chart) cut fetch latency to zero and avoid an adapter-per-shape sprawl. Adapters exist only where an external data source is the source of truth.
- **One IPC channel with seq numbers** beats two channels (prose + widget) — interleaving is the product requirement, and ordering across channels is a known foot-gun.
- **Closed registry, no plugins, v1** — review surface is bounded; one operator does not need third-party extensibility; protocol can evolve without semver-locking plugins.
- **Cached-last-good with oxblood frame** matches design doc § 5.3 (states): degradation is visible, not silent.
