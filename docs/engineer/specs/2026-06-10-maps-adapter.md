---
slug: maps-adapter
status: draft
phase: trip-planning
owner: leah
---

# Maps adapter — places + routing + POI corridor (MVP)

## 1. Goal

Give Leah an operator-attested seam over maps + places + routing so the
trip-planning composition layer (`internal/tripplanner/`, W61) can answer
concrete operator asks like "what's on the way from Seattle to Bellevue",
"nearest McDonald's on my route", and "coffee shop within walking distance
of my 2pm meeting". The differentiator is **POIAlongRoute** — a true
corridor search with detour-cost ranking, not the radius-from-midpoint
shortcut most map APIs surface.

MVP is the seven RPCs in §3 — turn-by-turn navigation, tile rendering,
re-routing, multi-modal trips, bike/scooter routing, and vehicle-specific
constraints all defer until a concrete caller files an issue.

## 2. Dependency (adopt-over-build)

**Primary: Google Maps Platform** via the official Go client.

- Module: `googlemaps.github.io/maps`
- Pinned version: `v1.7.0` (release 2026-02-12)
- License: Apache-2.0 — `https://github.com/googlemaps/google-maps-services-go/blob/v1.7.0/LICENSE`
- APIs consumed: Places (Nearby + Details + Text Search), Directions,
  Geocoding, Distance Matrix, Routes (`computeRoutes` v2 for traffic ETA).
- Why this provider: best global POI density + best transit data + stable
  since 2005. UX wins per `README.md` § House rules priority (UX > performance > long-term)
  despite higher per-call cost than alternatives.

`go.mod` edit deferred to the wiring wave (W56), mirroring the gmail /
gcal pattern. The adapter is structured so the SDK lands behind the
`Transport` interface — adding the import is additive and does not change
the public surface.

Require line to add in the wiring wave:

```
require googlemaps.github.io/maps v1.7.0
```

**Documented alternatives (not picked for MVP):**

- **Mapbox** (W57 alternative). Cheaper per call, `tilequery` corridor
  primitive matches POIAlongRoute well. Worse POI coverage outside the US.
  Behind the same `Adapter` interface — swap at config-time.
- **OpenStreetMap + Nominatim + OSRM** (W60 sovereign fallback). Free,
  self-hostable, no key. POI sparser; operator self-hosts. Behind the
  same `Adapter` interface, used for offline mode + sovereign-data posture.
- **Apple MapKit JS**. Best on macOS native, but bundle-ID-bound tokens
  awkward from non-app contexts. Rejected for MVP; revisit if HUD (W34+)
  needs in-process tile rendering.

## 3. Surface

```go
// internal/adapters/maps
type Place struct {
    ID         string
    Name       string
    Lat, Lng   float64
    Categories []string
    Hours      OpenHours
    Rating     float32
    PriceLevel int
}

type Route struct {
    Polyline  string  // encoded polyline (Google format)
    DistanceM int
    DurationS int
    Steps     []Step
    Tolls     []Toll
}

type CorridorOpts struct {
    MaxDetourMinutes int       // default 10
    Categories       []string  // e.g. ["restaurant", "mcdonalds"]
    OpenAt           time.Time // filter to places open at this time
    SampleEveryM     int       // default 1000 (1 km)
    TopK             int       // default 10
}

type Attestor    interface { Attest(ctx context.Context, scope string) error }
type KeySource   interface { Key(ctx context.Context) (string, error) }
type Transport   interface { /* internal Google Maps service calls */ }
type Config      struct { Attestor; KeySource; Transport; Cache; Budget }
type Client      struct { /* unexported */ }

type Adapter interface {
    Geocode(ctx context.Context, address string) ([]Place, error)
    ReverseGeocode(ctx context.Context, lat, lng float64) ([]Place, error)
    Route(ctx context.Context, origin, dest string, mode TransportMode) (Route, error)
    POINearby(ctx context.Context, center Place, radiusM int, category string) ([]Place, error)
    POIAlongRoute(ctx context.Context, route Route, opts CorridorOpts) ([]Place, error)
    PlaceDetails(ctx context.Context, placeID string) (PlaceDetail, error)
    DistanceMatrix(ctx context.Context, origins, dests []string, mode TransportMode) (Matrix, error)
    TrafficETA(ctx context.Context, route Route, departAt time.Time) (ETA, error)
}

func New(Config) (*Client, error)
```

Sentinel errors: `ErrAttestationDenied`, `ErrAuthRequired`,
`ErrBudgetExceeded`, `ErrNotFound`, `ErrRateLimited`, `ErrNoRoute`,
`ErrInvalidQuery`.

Per-RPC scope constants drive the audit grain:

- `ScopeGeocode = "maps:geocode"`
- `ScopeRoute   = "maps:route"`
- `ScopePOI     = "maps:poi"`
- `ScopeTraffic = "maps:traffic"`
- `ScopeDetails = "maps:place_details"`

`TransportMode` enum: `Driving`, `Walking`, `Transit`, `Bicycling`
(returned from API verbatim — Leah only emits Driving/Walking/Transit in
MVP callers).

## 4. POIAlongRoute design (load-bearing)

This is THE differentiating capability — most map SDKs expose
radius-from-point or radius-from-route-midpoint, not a true corridor
with operator-meaningful detour cost.

Algorithm:

1. Decode `Route.Polyline` (Google encoded-polyline format) into N points.
2. Resample to one anchor every `CorridorOpts.SampleEveryM` meters (default
   1 km). For a 50 km route at default settings: 50 anchors.
3. For each anchor, issue a Places Nearby Search with the operator-supplied
   category filter (`restaurant`, `mcdonalds`, `gas_station`,
   `tourist_attraction`, …). Radius = `min(SampleEveryM, 1500)`. Dedup by
   `place_id` across anchors.
4. For each candidate POI, compute REAL detour cost via Distance Matrix:
   `detour_s = directions(prev_anchor → poi → next_anchor).durationS
              - directions(prev_anchor → next_anchor).durationS`
   This is the corridor's killer feature — most APIs skip it.
5. Filter candidates where `detour_s > MaxDetourMinutes * 60`.
6. Filter where `place.Hours.OpenAt(opts.OpenAt) == false`.
7. Rank by `score = rating × (1 / (1 + detour_s / 60))`. Configurable
   weighting deferred to W57.
8. Return top `CorridorOpts.TopK` (default 10).

Anchor sampling, dedup, and the matrix call are the only places network
cost compounds — the budget guard (§7) caps total POIAlongRoute spend at
operator-configurable $/day.

Cache key for the matrix step is the anchor-pair tuple, not the POI list —
detour costs along common corridors (operator's daily commute) become
near-free after the first run.

## 5. Operator-attestation

First `maps:*` call per session is attested via `internal/attestation`
(per PR #67 attestation pool). Subsequent same-scope calls in the same
session reuse the prior attestation row, mirroring gmail/gcal.

API key storage:

- Path: `$HOME/.leah-state/secrets/google-maps-key.json`
- Mode: `0600` (enforced by `leah connect maps`, not by the adapter)
- Shape: `{ "api_key": "...", "added_at": "..." }`
- Never logged; redacted in any debug-dump command.

`leah connect maps` first-launch flow (wiring wave):

1. Prompts an attestation question, audits the answer.
2. Asks operator to paste the API key.
3. Validates with one `Geocode("1600 Amphitheatre Parkway")` test call.
4. On success: writes the key 0600 + records connect audit row.
5. On failure: does NOT persist the key — operator retries.

`leah disconnect maps` (PR #66 pattern): removes the key file +
attestation row + maps cache rows.

Per-RPC ordering inside `gateAndKey`: **attestation runs before key
load**. A denied action must not materialize the key — same load-bearing
ordering as the gmail/gcal `gateAndToken` flow. `failingKeySource` test
helper asserts `Key()` is never called when `Attest` returns non-nil.

## 6. Caching + freshness

Storage: `$HOME/.leah-state/maps-cache.db` SQLite, mode 0600. One table
per surface; `(scope, key_hash, expires_at, value_blob)` schema. Mirrors
`feeds-cache.db` (info-feeds §4) so the operator has one cache-management
mental model.

| Surface       | TTL    | Rationale |
| ---           | ---    | --- |
| Geocode       | 30d    | Addresses don't move. |
| ReverseGeocode| 30d    | Lat/lng → place is stable. |
| Route         | 5min   | Traffic shifts at this cadence. |
| POINearby     | 6h     | Opening hours change more often than ratings. |
| POIAlongRoute | 6h     | Same as POINearby; key includes route + opts hash. |
| PlaceDetails  | 24h    | Phone / website / hours rarely change. |
| DistanceMatrix| 5min   | Same as Route — traffic-sensitive. |
| TrafficETA    | 60s    | Operator wants real-time on this surface. |

Stale-while-revalidate is **off** for maps. Routes and traffic ETAs are
latency-critical AND staleness-toxic: a 6-minute-old route can route the
operator into a closed lane. Either fresh-from-cache or fresh-from-API
or `ErrCacheStale` — no silent stale.

LRU eviction: 50k rows / 100MB disk, operator-configurable.

## 7. Threat model

| Threat | Mitigation |
| --- | --- |
| API key leak via log | Key stored 0600; never logged; redacted in `log.Print` patterns; never appears in panic stack traces. |
| Cost blow-up on compromised key | **Hard $/day cap** operator-configurable (default $5/day). `Budget.Charge(estCost)` runs BEFORE every API call; over-budget returns `ErrBudgetExceeded` and the daemon kills further `maps:*` calls until the next UTC midnight. Audit row per call records estimated cost. |
| Location-history leak to reasoner | Query and result stay local. Reasoner receives derived scalars only — "operator queried a route" not "operator at GPS 47.6,-122.3". Test fixture asserts no lat/lng in reasoner-bound payloads. |
| IP-geolocation tracking | **Never.** Operator MUST set home location explicitly via `leah set home <address>` (PR #66 pattern). Adapter returns `ErrNoHome` if a relative-to-home call lands without an explicit home. No silent fallback to client-IP geocoding. |
| Cross-tenant query correlation | Audit log stores SHA-256 prefix of route digest, never plaintext origin/dest. Operator can opt into plaintext via `leah set audit.maps.plaintext=true` (default false). |
| Attestor-bypass | `New` refuses to construct without `Attestor`, `KeySource`, `Transport`. No nil fallback. Test: `TestNewRejectsMissingDeps`. |
| Place-injection (operator types attacker-controlled address) | `Geocode` validates inputs ≤ 256 chars, non-empty, no control bytes. Place IDs returned to callers are opaque tokens; never echoed into shell commands. |
| Rate-limit ban | Per-API exponential backoff (1s → 2s → 4s → 16s max). Circuit breaker opens after 3 consecutive 429s; half-open after 5min. Mirrors info-feeds breaker pattern. |
| Stale-route-into-closed-lane | Caching policy §6 — no stale-while-revalidate for Route / Traffic / DistanceMatrix. Either fresh or error. |

## 8. Test plan

Failing-test-first per `README.md` § House rules. Tests live next to source.

- **Per-RPC adapter tests** (`maps_test.go`): `httptest.Server` stands in
  for the Google Maps API; canned responses cover happy path, 429, 5xx,
  malformed JSON, empty result, timeout.
- **POIAlongRoute golden fixture** (`poi_along_route_test.go`): a real
  Seattle → Bellevue route polyline + canned Places responses → expected
  ranked POI list. `-update` flag regenerates; PR review surfaces drift.
- **Detour-correctness test**: synthetic route + synthetic POI → asserts
  computed detour minutes within ±10% of analytical truth.
- **Cache TTL test**: second call within TTL → no transport invocation;
  past TTL → transport invocation + cache refresh; stale Route → error,
  no stale-serve.
- **Budget-cap test** (`budget_test.go`): pre-charge to one cent under
  cap; next call returns `ErrBudgetExceeded`; no Transport invocation.
- **Attestation gate** (`attest_test.go`): `failingKeySource.Key()`
  assertion fires the test if denial-path materializes the key.
- **IP-geolocation refusal** (`home_test.go`): relative-to-home call
  with no home configured returns `ErrNoHome`. Adapter does not consult
  network IP under any code path.
- **Plaintext-audit-default-off** (`audit_test.go`): audit row contains
  only the SHA-256 prefix when default config; full plaintext only after
  explicit opt-in.

All tests hermetic — no real Google Maps calls, no disk outside `t.TempDir()`,
no network.

## 9. Future wiring (W56 + downstream)

- `cmd/leah/maps.go` — `leah connect maps` / `leah disconnect maps` /
  `leah set home <address>` subcommands.
- `cmd/leahd/build_maps.go` — daemon-boot constructor passing the
  shared `Attestor` (same instance gmail / gcal / feeds receive) plus a
  `fileKeySource` and the SQLite `Cache`.
- `internal/tripplanner/` (W61) — composition layer using `Adapter`
  directly (no extra abstraction layer).
- `internal/voice/maps_handler.go` (W62) — voice intents bind to
  `OnTheWay` and `MeetInMiddle`.
- `internal/hud/maps_panel.go` (W63) — HUD map preview panel subscribes
  to the daemon's `Route` cache.
- `internal/feeds/` does NOT consume maps directly — keep adapters
  independent; trip-planner composes them.

## 10. Out of scope (MVP)

- Live turn-by-turn navigation (deferred to voice-comm W11+ integration).
- Map-tile rendering in HUD (deferred to HUD W34+).
- Off-route detection + re-routing.
- Multi-modal trips (transit + walking + ride-share combined).
- Bicycle routing surface in callers (the SDK supports it; no MVP caller).
- Bike-share / scooter / micro-mobility.
- Vehicle-specific routing (truck, EV-charging-aware).
- Historical traffic queries ("what was traffic like last Tuesday at 5pm").
- Geofencing / arrival notifications (defer to daemon push framework).

Each reopens when a real caller files an issue citing the gap.

## 11. Risks + open questions

- **Google Maps pricing pressure.** $200/mo free credit is generous for
  one operator; if Leah ships to many operators each with their own key,
  no shared blast radius. Per-operator budget cap (§7) prevents single-
  operator surprise bills.
- **Polyline-decoding drift.** Google encoded-polyline format is stable
  but historically had a v5 → v6 transition. Pin decode logic in
  `internal/adapters/maps/polyline.go` with a fixture; do not import
  alternative decoders.
- **Place ID stability.** Google warns place_ids can change. Cache the
  ID + the lookup-key shape; refresh on `NOT_FOUND` instead of failing.
- **Transit data coverage.** Google has solid US/EU coverage; sparse in
  some Asia + Africa markets. Operator-visible via `Route.DurationS == 0`
  on a transit query. Document; do not paper over.
- **POIAlongRoute cost on long routes.** A 500 km route at 1 km sampling
  is 500 anchors × 1 Places call × N matrix calls — easily $5. Mitigation:
  `SampleEveryM` defaults grow with route length (1 km < 50 km, 5 km <
  500 km, 25 km ≥ 500 km). Operator can override.
