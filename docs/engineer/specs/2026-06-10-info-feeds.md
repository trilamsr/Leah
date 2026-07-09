# Info-feeds — Leah news + weather + market + general info synthesis

Status: spec (W31–W33 design lock)
Owner: trilamsr
Surface: `internal/feeds/`, `internal/feeds/{weather,news,market,wiki}/`, `cmd/leah/{news,weather,quote}.go`

## 1. Goal

Leah looks up AND synthesizes (NOT just fetches) news, weather, market data,
and general info on operator demand or proactively in the morning brief and
HUD. Synthesis is the differentiator: the operator doesn't want a JSON dump
of the top-30 news articles, they want "three things that matter today, with
sources." The HUD ambient panel and the morning-brief surface (PR #65) both
consume the same synthesized output via a stable interface.

The feeds layer is also the first place Leah talks to the public internet on
the operator's behalf at a non-trivial cadence. Privacy posture (no IP-
geolocation, explicit operator location), threat model (rate-limit, key
leak, misinformation), and cost posture (RSS-before-paid-API) all matter
more than which provider we pick.

## 2. Sources — inventory

| Feed | Provider | Auth | Rate-limit | Latency | License | MVP / future |
| --- | --- | --- | --- | --- | --- | --- |
| Weather | OpenWeatherMap | API key (free) | 60/min | <500ms | free tier OK for personal | **MVP** |
| Weather (fallback) | Apple WeatherKit | App-Store cert | 500/day free | <500ms | free w/ developer cert | future (W36) |
| News | Hacker News API | none | gentle | <1s | public | **MVP** |
| News | Reddit r/news RSS | none | gentle | <1s | RSS, attribution required | **MVP** |
| News | Google News RSS | none | gentle | <1s | RSS, attribution required | **MVP** |
| News | NewsAPI.org | API key (paid) | 100/day free | <1s | paid for commercial | future |
| Markets | Yahoo Finance unofficial | none | aggressive, IP-banned | <1s | unofficial — fragile | **MVP** |
| Markets | Alpha Vantage | API key (free) | 25/day free | <1s | free tier OK | **MVP** (fallback) |
| Markets | Polygon.io | API key (paid) | per-tier | <500ms | paid | future |
| General info | Wikipedia API | none | 200/s | <500ms | CC-BY-SA, attribution required | **MVP** |
| General info | DuckDuckGo Instant Answer | none | gentle | <1s | free | **MVP** |
| Sports | ESPN unofficial | none | gentle | <1s | unofficial — fragile | future (W36) |

MVP rule: RSS-and-public-API before paid keys. Operator pays for nothing
unless they `leah connect newsapi` themselves.

Every source plugs into a structural-typed interface so adding a source
doesn't ripple through callers:

```go
// internal/feeds
type Feed interface {
    Name() string
    Fetch(ctx context.Context, query Query) ([]Item, error)
}

type Item struct {
    ID         string
    Title      string
    Summary    string
    URL        string
    Source     string     // human-readable attribution
    Published  time.Time
    Domain     Domain     // weather / news / market / wiki / ...
    Raw        any        // domain-specific payload (Quote, Forecast, Article)
}
```

The interface is intentionally narrow. Domain-specific payload lives in
`Item.Raw` — the synthesizer down-casts. Callers that don't synthesize
(e.g. `leah news` CLI tail) treat all feeds uniformly.

## 3. Synthesis layer

Synthesis is the value-add. Raw `[]Item` is fetched; synthesizers turn it
into operator-facing summaries.

```go
// internal/feeds/synth
type Synthesizer[T any] interface {
    Summarize(ctx context.Context, items []Item) (T, error)
}
```

Per-domain synthesizers:

| Synthesizer | Input | Output | Method |
| --- | --- | --- | --- |
| `news.Summarize` | `[]Article` (multi-source) | `DailyDigest` (top-3) | rank by `source_trust × recency × dedup_cluster_size` |
| `market.Summarize` | `[]Quote` for tracked symbols | `MarketPulse` | per-symbol % change + flagged anomalies (|Δ| > 3σ vs 30d) |
| `weather.Summarize` | `Forecast` | `Briefing` | today high/low + precip + "bring umbrella" hint if precip > 30% |
| `wiki.Summarize` | `[]Item` (search results) | `WikiAnswer` | first paragraph of best-match article + attribution |

Synthesis runs in two tiers:

- **Default: local rule-based** — fast, free, deterministic. No LLM call.
  All four synthesizers ship a rule-based path for MVP.
- **Explain depth: reasoner** — only when the operator asks "explain" or
  "why" against a synthesized result. Routes through the existing
  `internal/reasoner` interface. Local-first per `README.md` § House rules simpler-default.

Source attribution is non-negotiable. Every synthesized output carries an
`[]Source{name, url, fetched_at}` list. The HUD and CLI render these as
chips / footnotes; the operator can verify and disagree.

## 4. Caching + freshness

Each source has a cache TTL appropriate to its volatility:

| Source | TTL | Rationale |
| --- | --- | --- |
| Weather | 10min | observations update on this cadence |
| News | 15min | catches hourly news cycle without thrashing |
| Markets (open hours) | 60s | intraday glance needs near-real-time |
| Markets (closed) | 1h | post-close prices don't move |
| Wikipedia | 24h | articles change rarely |
| DDG instant answer | 1h | mid-volatility |

Storage: `~/.leah-state/feeds-cache.db` (SQLite, mode 0600). One table per
domain or a single table with domain column — implementation detail. The
cache schema is operator-private; never ships off-device.

Stale-while-revalidate semantics: if a cached entry is past TTL but still
present, the synth surface returns the stale value immediately AND triggers
a background refresh. Next read gets the fresh value. The morning brief
(W33) and HUD (W36) both benefit — they never block on network.

Cache eviction: LRU bounded by row count (default 10k rows) and disk size
(default 50MB). Operator-config overrides both (W37).

## 5. Surface integration

**Morning brief** (extends PR #65):

- Brief gains three new lines after the existing gmail + gcal summary:
  - Weather: "Today: 18°C / 9°C, light rain expected ~3pm. Bring umbrella."
  - Top headline: "Headline (Source) — one-line summary."
  - Markets: "AAPL +1.2%, MSFT -0.4%, ACWI +0.3%."
- Operator configures tracked market symbols + weather location in
  `~/.leah-state/feeds.json`.
- Brief continues to be the single composition surface — the feeds layer
  does NOT own brief delivery, only the synth output.

**HUD ambient panel** (W36):

- Weather widget (icon + temp + high/low)
- Market ticker (2-3 tracked symbols, % change)
- Headline rotation (every 20s, no animation in DND/Focus)
- Subscribes to the same `Telemetry.Subscribe` gRPC stream the HUD already
  uses (see hud-ui spec §4)

**Voice** (post-W14):

- "What's the news" → speaks `DailyDigest`
- "How's the market" → speaks `MarketPulse`
- "Weather today" / "Weather in <city>" → speaks `Briefing`
- "What's <topic>" → routes to wiki/DDG → speaks `WikiAnswer`

**CLI**:

- `leah news [--source <name>]` — print `DailyDigest`
- `leah weather [city]` — print `Briefing`; default city from operator config
- `leah quote <symbol> [<symbol>…]` — print `MarketPulse` for given symbols
- `leah info <query>` — wiki/DDG `WikiAnswer`

Each command is a thin adapter — fetch via feed → synth via synthesizer →
render. No business logic in `cmd/leah/`.

## 6. Operator-attestation

First fetch from each external source = operator-attested. Same pattern as
gmail / gcal / macos. Attestation records under
`~/.leah-state/attestation/feeds-<source>.json` mode 0600.

API keys (where used):

- Storage: `~/.leah-state/secrets/<service>-key.json` mode 0600
- Shape mirrors gmail token pattern: `{ "api_key": "…", "added_at": "…" }`
- Never logged. Redacted in any debug-dump command.
- `leah connect <feed>` interactive first-launch flow:
  - Asks operator to paste the key
  - Validates with a single test request before persisting
  - Failure does NOT persist the key — operator retries
- `leah disconnect <feed>` (per PR #66 pattern): removes key + attestation
  + cache rows for that feed

## 7. Threat model

| Threat | Mitigation |
| --- | --- |
| API key leak via log | Key stored 0600; redacted in `log.Println`-style sites; never appears in panic stack traces |
| Rate-limit ban (esp. Yahoo Finance) | Exponential backoff (1s → 2s → 4s → 16s max); circuit breaker per source opens after 3 consecutive 429s, half-open after 5min |
| Misinformation in news | Operator-configurable source allowlist (default: includes HN, r/news; excludes unknown blogs); source attribution required in every synthesized output |
| Pump-and-dump / signal manipulation | Market synthesizer NEVER says "buy" / "sell" / "should" / "recommend". Only descriptive: % change, anomaly flag, recent range. Static lint rule in `market_test.go` enforces. |
| Geographic spoofing | Operator sets location explicitly via `leah connect weather --location <city>`. No IP-geolocation fallback. If location missing, weather feed returns an error, not a guess. |
| Cache poisoning | Cache writes go only through verified-source paths; cache rows include `source` + `fetched_at` and the synthesizer is suspicious of source mismatches |
| Personal-data leak in queries | Wikipedia / DDG queries are operator-typed plain text. The feeds layer redacts the operator-name / common-PII before logging the query string. |

## 8. Test plan

Failing-test-first per `README.md` § House rules. Tests live next to source:

- **Per-source adapter tests** (`weather/weather_test.go`,
  `news/hn_test.go`, etc.): `httptest.Server` fixtures replay canned
  provider responses. Covers happy path, 429, 5xx, malformed JSON, empty
  result, timeout.
- **Synthesizer tests** (`synth/news_test.go`, …): golden-file fixtures.
  Input: deterministic `[]Item` slice. Output: golden `DailyDigest` /
  `MarketPulse` / `Briefing`. Updates via `-update` flag, reviewed in PR.
- **Cache tests** (`cache_test.go`): TTL respected, stale-while-revalidate
  serves stale + triggers refresh, LRU eviction at bound, 0600 mode on
  fresh DB file.
- **Circuit breaker tests** (`feeds_test.go`): 3 consecutive 429s open
  breaker; subsequent calls fast-fail with `ErrCircuitOpen`; breaker
  half-opens after 5min.
- **Key-redaction lint** (`secrets_test.go`): scans the feeds tree for
  `log.Print` / `fmt.Errorf` patterns that pass through a known key var.
  Static check, no runtime cost.
- **Market-no-advice lint** (`market_test.go`): scans synthesized
  `MarketPulse` output for forbidden tokens (buy, sell, should,
  recommend). Static fixture-based.

## 9. Out of scope (MVP)

- Live financial trading execution
- Real-time trading signals or recommendations (explicit non-goal)
- In-depth chart rendering (candles, indicators)
- Satellite imagery / radar overlays
- Full-text article rendering (we link out, we don't republish)
- Multi-lingual news synthesis (English-only MVP)
- Push notifications on breaking news (operator initiates; Leah doesn't
  interrupt)

## 10. Risks + open questions

- **Yahoo Finance unofficial API breaks.** Likely; mitigated by Alpha
  Vantage fallback in `market.Fetch`. Tracking issue for proper paid
  provider migration.
- **OpenWeatherMap free tier rate.** 60 req/min easily exceeded across HUD
  + brief + voice. Mitigation: aggressive cache TTL + single fetch shared
  across surfaces (the synth result, not the raw call, is what HUD
  consumes).
- **RSS source attribution semantics.** Google News / Reddit RSS have
  fuzzy "source" — sometimes it's the upstream publisher, sometimes
  Reddit itself. Synthesizer normalizes via best-effort URL-host parse.
  Imperfect; operator-visible. Acceptable for MVP.
- **Operator-config feed schema churn.** `feeds.json` schema additions
  in W37 (operator-config) may break MVP config. Mitigation: schema
  version field + read-side migration.
