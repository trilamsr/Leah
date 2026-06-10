# Plan — trip-planning composition + W56-W64

Companion to:
- `docs/engineer/specs/2026-06-10-maps-adapter.md`
- `docs/engineer/specs/2026-06-10-flights-adapter.md`

Nine waves, each PR-sized (~500 LOC), each lands behind a feature flag
where the surface is operator-visible. Order respects dependencies; no
wave assumes a future wave shipped.

## Architecture

`internal/tripplanner/` is a thin composition layer over the existing
adapter surface — no new third-party deps, no new persistent state
beyond the per-adapter caches each spec defines.

```go
// internal/tripplanner
type Profile interface {
    LocationHome(ctx context.Context) (string, error)
    Preferences(ctx context.Context) (Prefs, error)  // cuisine, walking-tolerance, ...
}

type Planner interface {
    OnTheWay(ctx context.Context, origin, dest string, opts maps.CorridorOpts) ([]maps.Place, error)
    SuggestTrip(ctx context.Context, dest string, durationDays int, profile Profile) (Trip, error)
    DailyItinerary(ctx context.Context, city string, on date.Date, profile Profile) (Itinerary, error)
    MeetInMiddle(ctx context.Context, partyA, partyB string, category string) ([]maps.Place, error)
}
```

Implementations are deliberately short — `OnTheWay` is 1 `maps.Route`
call + 1 `maps.POIAlongRoute` + 1 `recommendation.Rank`. `SuggestTrip`
is `flights.SearchOffers` + `maps.PlaceDetails` (for destination
metadata) + a small day-allocation heuristic. Three similar lines beat
a premature abstraction (per `CLAUDE.md`).

### Reuse table

| Capability                  | Source                                       |
| ---                         | ---                                          |
| Route + POI corridor        | `internal/adapters/maps`                     |
| Flight search + watch       | `internal/adapters/flights`                  |
| Operator attestation        | `internal/attestation` (PR #67)              |
| Audit                       | `internal/audit`                             |
| Calendar context (2pm mtg)  | `internal/adapters/gcal`                     |
| Recommendation ranking      | `internal/recommend` (W15+)                  |
| Brief surface               | `internal/brief`                             |
| Voice surface (post W11)    | `internal/voice`                             |
| HUD surface (post W34)      | `internal/hud`                               |
| Knowledge-graph location entities | `internal/kg` (PR #72)                |

No new persistence; no new daemon goroutine; no new gRPC method. The
trip-planner is a callable, not a service.

## Wave 56 — Maps adapter skeleton + connect

**Goal.** New `internal/adapters/maps/` compiles. `Adapter` interface +
`Client` constructor + three of seven RPCs (`Geocode`,
`ReverseGeocode`, `Route`) wired through Google Maps Go SDK. `leah
connect maps` first-launch flow lands. No POI yet, no corridor yet.

**Files touched.**
- `internal/adapters/maps/maps.go` — new, `Adapter` interface + `Client`.
- `internal/adapters/maps/geocode.go` — new, Geocode + ReverseGeocode.
- `internal/adapters/maps/route.go` — new, Route + polyline decode.
- `internal/adapters/maps/cache.go` — new, SQLite cache wrapper.
- `internal/adapters/maps/budget.go` — new, $/day cap.
- `internal/adapters/maps/maps_test.go` — httptest hermetic.
- `cmd/leah/maps.go` — new, `connect` / `disconnect` / `set home`.
- `go.mod` — add `googlemaps.github.io/maps v1.7.0`.

**Risk.** `go.mod` edit collides with parallel waves. Mitigation:
follow the gmail/gcal pattern — adapter ships behind `Transport`
interface; `go.mod` edit lands first, single rebase.

**Size.** ~500 LOC (300 adapter + 100 cache + 60 connect + 40 tests).

**Test plan.**
- `TestGeocode_Happy` + `TestGeocode_Cache_Hit` (golden hermetic).
- `TestRoute_Happy` + `TestRoute_NoStaleServe` (TTL contract).
- `TestNewRejectsMissingDeps` — fail-closed wiring.
- `TestConnect_BadKey_DoesNotPersist`.

**Unblocks.** W57 (POIAlongRoute uses Route + cache), W58 (TrafficETA
uses Route), W61 (tripplanner needs Route).

## Wave 57 — POIAlongRoute corridor + ranking

**Goal.** Implement the differentiating capability — POIAlongRoute with
detour-cost ranking per spec §4. Add `POINearby`, `PlaceDetails`,
`DistanceMatrix`. Golden fixture for Seattle → Bellevue.

**Files touched.**
- `internal/adapters/maps/poi.go` — new, POINearby + PlaceDetails.
- `internal/adapters/maps/corridor.go` — new, POIAlongRoute algorithm.
- `internal/adapters/maps/matrix.go` — new, DistanceMatrix.
- `internal/adapters/maps/polyline.go` — new, encoded-polyline decode.
- `internal/adapters/maps/corridor_test.go` — Seattle → Bellevue
  golden fixture; detour-correctness synthetic.

**Risk.** Cost on long routes. Mitigation: anchor-sampling defaults
scale with route length (spec §11); operator-visible budget cap kills
runaway queries.

**Size.** ~450 LOC.

**Test plan.**
- `TestPOIAlongRoute_Seattle_Bellevue` — golden file.
- `TestDetour_Within10pct` — synthetic analytical truth.
- `TestPOIAlongRoute_Cost_Capped` — synthetic 1000 km route stops at
  budget, returns partial + `ErrBudgetExceeded`.
- `TestPOIAlongRoute_OpenAt_Filters` — closed-at-query-time POIs
  excluded.

**Unblocks.** W61 (`OnTheWay` is a thin wrapper over this).

## Wave 58 — TrafficETA + cache hardening

**Goal.** Real-time + predicted ETA via Routes v2 `computeRoutes`. No
stale-serve for traffic. Audit row per call records est cost.

**Files touched.**
- `internal/adapters/maps/traffic.go` — new, TrafficETA.
- `internal/adapters/maps/cache.go` — extend with LRU eviction + size
  cap.
- `internal/adapters/maps/audit.go` — new, per-call audit emitter.
- `internal/adapters/maps/traffic_test.go`.

**Risk.** Costliest API. Mitigation: budget cap (spec §7); operator-
visible "current spend today" via `leah maps stats`.

**Size.** ~300 LOC.

**Test plan.**
- `TestTrafficETA_FreshOrError` — past-TTL never serves stale.
- `TestCache_LRU_AtBound` — eviction at row + byte bounds.
- `TestAudit_HashOnly_Default` — audit row contains route-digest
  SHA prefix, no plaintext.
- `TestAudit_Plaintext_OptIn` — `audit.maps.plaintext=true` flips.

**Unblocks.** W61 (trip-planner wants ETA on operator-saved routes).

## Wave 59 — Flights adapter skeleton + watch loop

**Goal.** New `internal/adapters/flights/` compiles. Four RPCs
(`SearchOffers`, `FlightStatus`, `AirportInfo`, `WatchPrice`) +
`Unwatch` + `ListWatches`. `leah connect flights` first-launch flow.
Daemon tick polls watches.

**Files touched.**
- `internal/adapters/flights/flights.go` — new, `Adapter` + `Client`.
- `internal/adapters/flights/search.go` — new, SearchOffers.
- `internal/adapters/flights/status.go` — new, FlightStatus.
- `internal/adapters/flights/watch.go` — new, watch store + poll.
- `internal/adapters/flights/cache.go` — new, SQLite.
- `internal/adapters/flights/flights_test.go`.
- `cmd/leah/flights.go` — new, `connect` / `disconnect` / `search` /
  `watch` / `unwatch` / `status`.
- `cmd/leahd/build_flights.go` — daemon-boot wiring.

**Risk.** Accidental booking surface drift. Mitigation: forbidden-token
lint per spec §7 (`Book`, `Pay`, `Purchase`, `Confirm`).

**Size.** ~500 LOC.

**Test plan.**
- `TestSearchOffers_Happy`.
- `TestWatchPrice_RoundTrip` — create → tick → notify → re-tick → no
  re-notify within 24h → unwatch.
- `TestForbidden_Booking_Tokens` — static lint.
- `TestReasoner_NoIATA_Leak` — fixture asserts no IATA / dates in
  reasoner-bound payloads.

**Unblocks.** W61 (SuggestTrip uses SearchOffers).

## Wave 60 — Sovereign-fallback: OSM + OSRM adapter

**Goal.** Behind the same `maps.Adapter` interface, implement an
OpenStreetMap (Nominatim) + OSRM-backed adapter. Operator picks
provider in `~/.leah-state/maps.json`. Default stays Google; OSM is
the offline / sovereign-data option.

**Files touched.**
- `internal/adapters/maps/osm/osm.go` — new, alternative `Adapter` impl.
- `internal/adapters/maps/osm/osrm.go` — new, route via local OSRM.
- `internal/adapters/maps/osm/nominatim.go` — new, geocode via
  Nominatim (self-hostable).
- `internal/adapters/maps/osm/osm_test.go`.
- `cmd/leah/maps.go` — extend `connect maps --provider=osm`.

**Risk.** POI sparser; operator-visible quality drop. Mitigation:
default stays Google; OSM is opt-in; brief documents the tradeoff.

**Size.** ~450 LOC.

**Test plan.**
- `TestOSM_Geocode_Happy` — fixture against canned Nominatim response.
- `TestOSM_Route_Happy` — canned OSRM response.
- `TestOSM_POINearby_SparseResult` — asserts adapter returns "no POI"
  cleanly instead of erroring when Nominatim is thin.

**Unblocks.** Privacy posture story — operator can self-host the
whole maps stack.

## Wave 61 — tripplanner composition: OnTheWay + MeetInMiddle

**Goal.** New `internal/tripplanner/` package. `OnTheWay` and
`MeetInMiddle` land — the two highest-value composition surfaces. No
voice / HUD yet.

**Depends on.** W57 (POIAlongRoute) and W58 (ETA).

**Files touched.**
- `internal/tripplanner/planner.go` — new, `Planner` interface +
  default impl.
- `internal/tripplanner/on_the_way.go` — new.
- `internal/tripplanner/meet_in_middle.go` — new (geographic midpoint
  via geodesic; POINearby at midpoint with category filter).
- `internal/tripplanner/planner_test.go`.
- `cmd/leah/onway.go` — new, `leah on-the-way <origin> <dest>
  [--category restaurant]`.
- `cmd/leah/meet.go` — new, `leah meet <partyA> <partyB> [--category
  coffee]`.

**Risk.** Composition layer drifts into business logic that belongs
in the adapters. Mitigation: package has no SQLite, no HTTP client, no
goroutines — strict composition only. PR review enforces.

**Size.** ~350 LOC (composition is thin by design).

**Test plan.**
- `TestOnTheWay_SeattleBellevue` — golden file, hermetic adapters.
- `TestMeetInMiddle_Midpoint_Correct` — analytical-truth midpoint.
- `TestPlanner_Reuses_AdapterErrors` — `ErrBudgetExceeded` propagates
  unwrapped.

**Unblocks.** W62 (voice), W63 (HUD), W64 (recommendation tie-in).

## Wave 62 — Voice integration

**Goal.** Voice intents bind to `OnTheWay` and `MeetInMiddle`. "What's
nearby on my drive to Bellevue", "find coffee near my 2pm meeting"
(calendar-aware via `gcal.ListToday`).

**Depends on.** W11–W14 (voice-comm pipeline) AND W61 (planner).

**Files touched.**
- `internal/voice/maps_handler.go` — new, intent matcher.
- `internal/voice/calendar_aware.go` — new, "near my 2pm meeting"
  resolves via `gcal.ListToday`.
- `internal/voice/maps_handler_test.go`.

**Risk.** Voice utters lat/lng instead of place names. Mitigation:
handler always speaks the rendered `Place.Name`, never coordinates;
asserted by golden fixture.

**Size.** ~300 LOC.

**Test plan.**
- `TestVoice_OnTheWay_Intent` — utterance → planner call → spoken
  result.
- `TestVoice_Calendar_2pm` — gcal fixture has 2pm meeting at address;
  voice resolves "near my 2pm" correctly.
- `TestVoice_No_LatLng_Spoken` — output never contains a decimal
  coordinate.

**Unblocks.** Hands-free trip-planning ask (operator use case #1 + #3).

## Wave 63 — HUD integration

**Goal.** HUD map-preview panel renders the result of the most-recent
`OnTheWay` / `Route` call. Static preview (no panning, no zoom); map
tile served from Google Maps Static API.

**Depends on.** W34+ (HUD framework) AND W61 (planner).

**Files touched.**
- `internal/hud/maps_panel.go` — new, panel subscribes to telemetry
  stream emitting the most-recent route polyline + POI list.
- `internal/hud/maps_panel_test.go`.
- `internal/adapters/maps/static.go` — new, `StaticMap(polyline,
  pois) (pngBytes, error)`.

**Risk.** Static-map API has separate per-call pricing. Mitigation:
cache rendered PNG by polyline+POI digest for 1h; budget guard
inherits.

**Size.** ~350 LOC.

**Test plan.**
- `TestHUDPanel_Renders_LastRoute` — fixture telemetry → panel state.
- `TestStaticMap_Cache_Hit` — same digest, no API call.

**Unblocks.** Visual surface for operator use case #5 (drive-home
preview).

## Wave 64 — Recommendation engine tie-in

**Goal.** `OnTheWay` ranking consults `recommend.Engine` (W15+):
operator profile signals (cuisine preferences, "avoided last time")
adjust POI rank.

**Depends on.** W15–W19 (recommend engine + feedback loop) AND W61.

**Files touched.**
- `internal/tripplanner/personalize.go` — new, profile-aware rank
  adjustment over `maps.POIAlongRoute` output.
- `internal/recommend/patterns/cuisine_preference.go` — new pattern.
- `internal/recommend/patterns/place_avoided.go` — new pattern.
- `internal/tripplanner/personalize_test.go`.

**Risk.** Personalization over-fits — operator gets shown only their
last favorite place forever. Mitigation: rank blend is
`base × 0.7 + profile × 0.3`; cap profile boost at 1.5x; golden
regression holds top-K diversity ≥ 3 distinct categories across 100
queries.

**Size.** ~400 LOC.

**Test plan.**
- `TestPersonalize_FavoriteCuisine_Boosts` — operator likes Thai →
  Thai places rank higher.
- `TestPersonalize_Avoided_Demotes` — operator marked avoided →
  demoted, not removed.
- `TestPersonalize_Diversity_Maintained` — top-K across 100 random
  routes covers ≥ 3 categories.
- `TestForget_Cuisine_Pattern` — `leah forget cuisine_preference`
  resets boost to 1.0.

**Unblocks.** Closes the W56-W64 arc. Operator gets the "Leah remembers
what I like" experience without surrendering their location history
off-device.

## Cross-wave invariants

- Every wave adds **no new dependency** outside the standard library,
  `modernc.org/sqlite` (in tree), `oklog/ulid/v2` (in tree),
  `googlemaps.github.io/maps` (W56 only), and direct REST for Amadeus
  (no SDK).
- Every wave's PR body cites the spec section it implements.
- No wave self-approves; reviewer subagent spawned per `CLAUDE.md`.
- No wave merges before tests are golden-stable across 3 runs (network
  fixtures + time-of-day filters are timing-sensitive — `Now` and
  `Rand` are always injected).
- Schema migrations additive-only across all waves; no drops.
- No wave introduces a goroutine that runs outside the daemon's
  existing tick framework (PR #45). The flight-watch poll is a
  registered tick handler, not a self-scheduled goroutine.

## Sequencing summary

```
W56 (maps skeleton)
  ├── W57 (POI corridor)
  ├── W58 (ETA + cache)
  ├── W59 (flights skeleton + watch) ── parallel-safe with W57/W58
  └── W60 (OSM fallback) ── parallel-safe with W57/W58/W59

W61 (tripplanner) ── needs W57 + W58 + W59
  ├── W62 (voice) ── also needs W11-W14
  ├── W63 (HUD) ── also needs W34
  └── W64 (recommend tie-in) ── also needs W15-W19
```

W56 is the only true serial bottleneck. W57, W58, W59, W60 can ship in
parallel once W56 lands (coordinate one `go.mod` rebase per wave).
W61 is the next bottleneck. W62/63/64 fan out from W61 with their
respective cross-arc deps.

## Decision-priority audit

Per `CLAUDE.md`:

- **UX > performance > long-term.** Google Maps over Mapbox despite
  higher per-call cost — POI density UX wins. Stale-while-revalidate
  off for Route + Traffic — a 5-min-old route is a UX bug.
- **Default simpler.** Single-provider for MVP; multi-provider lives
  behind the same interface (W60). No abstraction layer above the
  adapter — `Planner` IS the abstraction.
- **Three similar lines beat premature abstraction.** Each Planner
  method is < 30 lines and calls 2-3 adapter methods directly. No
  `Strategy` / `Visitor` / `Pipeline` wrappers.
- **Adopt-over-build.** Official Google Maps Go SDK; direct REST for
  Amadeus (its Go SDK is stale). Reuse `internal/attestation`,
  `internal/audit`, `internal/recommend`, `internal/brief`,
  `internal/voice`, `internal/hud`, `internal/kg` — no new
  infrastructure.
- **Deletion default.** Every wave's PR answers "what got smaller?"
  W56: replaces zero ad-hoc maps code (none exists yet) — `what got
  smaller` is "the count of operator-facing questions Leah can't
  answer." W61: replaces zero composition glue (none exists yet) —
  `what got smaller` is "the number of places trip-planning logic
  could live" — one canonical home.
- **Drop ceremony.** No decorative sections. No status badges. No
  "Background" / "Motivation" preamble.

## Forbidden-reference posture

Per `CLAUDE.md`, Leah is Leah — no franchise mascot names, no movie-AI
aliases. PR review enforces via the canonical identity grep (see
`CLAUDE.md`). This document was authored under that constraint.
