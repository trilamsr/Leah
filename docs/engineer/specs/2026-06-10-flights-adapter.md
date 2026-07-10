---
slug: flights-adapter
status: draft
phase: trip-planning
owner: leah
---

# Flights adapter — search + status + watch (MVP)

## 1. Goal

Give Leah a typed seam over flight search + status + price-tracking so
trip planning (`internal/tripplanner/`, W61) and the morning brief
(PR #65) can answer "what flights SFO → NRT next week", "is United 837
on time", and "tell me when SFO → NRT drops under $700".

**Booking is explicitly out of scope.** No seat selection, no payment,
no PNR creation. Booking lives behind a paid-action gate (W64) and a
separate adapter; the price-watch surface stops at notification.

## 2. Dependency (adopt-over-build)

**Primary: Amadeus Self-Service API**, REST direct (no SDK).

- Endpoint base: `https://test.api.amadeus.com` (dev), `https://api.amadeus.com` (prod).
- Auth: OAuth2 client-credentials (`/v1/security/oauth2/token`).
- Why this provider: free tier (~2000 calls/mo), comprehensive (offers,
  hotels, transfer, POI, flight-inspiration), industry-standard schema
  (IATA codes, ICAO carrier codes). Single source covers the four MVP
  RPCs.
- Why direct REST (no SDK): `github.com/amadeus4dev/amadeus-go` last
  commit > 18 months stale as of 2026-06-10; vendoring the surface area
  Leah uses is < 200 LOC of HTTP plumbing and avoids a soft-abandoned
  dependency. Mirrors the `feeds/` direct-REST pattern over the
  Hacker-News / RSS sources.
- License posture: no library import; only network use of Amadeus per
  their developer terms — operator accepts terms during `leah connect
  flights`.

**Alternatives documented:**

- **Duffel API** (Pro-tier upgrade path). Modern REST, easier auth, no
  free tier. Behind the same `Adapter` interface — swap at config-time.
- **Skyscanner RapidAPI**. Cheaper, but RapidAPI proxy adds a third party
  to the request chain. Rejected for MVP on privacy grounds (operator
  flight queries are sensitive — minimize hops).
- **Google Flights / ITA**. No public API.

No `go.mod` edit required for MVP — Amadeus is REST + std-lib JSON.

## 3. Surface

```go
// internal/adapters/flights
type Money struct { Amount int64; Currency string }  // minor units (cents)

type Airport struct { IATA, ICAO, Name, City, Country string; TZ string }

type Segment struct {
    Departure, Arrival   Airport
    DepartAt, ArriveAt   time.Time
    Carrier              string  // IATA airline code, e.g. "UA"
    FlightNumber         string  // e.g. "837"
    Aircraft             string  // e.g. "789"
    DurationS            int
    OperatedBy           string  // codeshare resolution
}

type Itinerary struct {
    Segments []Segment
    Duration time.Duration
}

type FlightOffer struct {
    ID                string  // opaque, valid until offer expiry
    Itineraries       []Itinerary
    Price             Money
    ValidatingAirline string
    Seats             int     // remaining inventory at this price
    ExpiresAt         time.Time
}

type SearchReq struct {
    Origin, Dest         string  // IATA codes
    DepartOn             date.Date
    ReturnOn             date.Date  // zero for one-way
    Pax                  int     // adults; MVP single-traveler
    Cabin                string  // "ECONOMY" | "PREMIUM_ECONOMY" | "BUSINESS" | "FIRST"
    MaxStops             int
    NonStop              bool
}

type Status struct {
    FlightNumber string
    Date         date.Date
    Carrier      string
    ScheduledDeparture, ActualDeparture time.Time
    ScheduledArrival, ActualArrival     time.Time
    State        string  // "SCHEDULED" | "BOARDING" | "DEPARTED" | "ARRIVED" | "CANCELLED" | "DIVERTED"
    Gate         string
    Terminal     string
}

type Attestor   interface { Attest(ctx context.Context, scope string) error }
type KeySource  interface { Credentials(ctx context.Context) (id, secret string, err error) }
type Transport  interface { /* internal Amadeus calls */ }
type Config     struct { Attestor; KeySource; Transport; Cache; Budget }
type Client     struct { /* unexported */ }

type Adapter interface {
    SearchOffers(ctx context.Context, req SearchReq) ([]FlightOffer, error)
    FlightStatus(ctx context.Context, carrier, flightNumber string, on date.Date) (Status, error)
    AirportInfo(ctx context.Context, iata string) (Airport, error)
    WatchPrice(ctx context.Context, req SearchReq, threshold Money) (watchID string, err error)
    Unwatch(ctx context.Context, watchID string) error
    ListWatches(ctx context.Context) ([]Watch, error)
}

func New(Config) (*Client, error)
```

Sentinel errors: `ErrAttestationDenied`, `ErrAuthRequired`,
`ErrBudgetExceeded`, `ErrNotFound`, `ErrRateLimited`, `ErrInvalidQuery`,
`ErrOfferExpired`, `ErrWatchNotFound`.

Per-RPC scope constants:

- `ScopeSearch  = "flights:search"`
- `ScopeStatus  = "flights:status"`
- `ScopeAirport = "flights:airport"`
- `ScopeWatch   = "flights:watch"`

## 4. Operator-attestation

First `flights:*` call per session is attested. Subsequent same-scope
calls reuse the attestation row per session, mirroring gmail / gcal /
maps.

Credential storage:

- Path: `$HOME/.leah-state/secrets/amadeus-creds.json`
- Mode: `0600` (enforced by `leah connect flights`, not by the adapter)
- Shape: `{ "client_id": "...", "client_secret": "...", "added_at": "..." }`
- Never logged; redacted in any debug-dump.

`leah connect flights` first-launch flow (wiring wave):

1. Prompts attestation question, audits the answer.
2. Asks operator to paste client_id + client_secret.
3. Validates with a single `oauth2/token` exchange + one
   `AirportInfo("SFO")` test call.
4. On success: writes creds 0600 + records connect audit row.
5. On failure: does NOT persist — operator retries.

`leah disconnect flights` (PR #66 pattern): removes creds + attestation
+ active watches + cache rows.

Per-RPC ordering inside `gateAndKey`: **attestation before key load**
(load-bearing — same rationale as gmail/gcal/maps).

## 5. Price-watch loop

Watches are operator-private, daemon-managed, never push notifications
the operator didn't opt into.

Storage: `$HOME/.leah-state/flight-watches.db` SQLite mode 0600. Schema:

```
CREATE TABLE flight_watches (
    id           TEXT PRIMARY KEY,    -- ulid
    req_json     TEXT NOT NULL,       -- serialized SearchReq
    threshold    INTEGER NOT NULL,    -- minor units
    currency     TEXT NOT NULL,
    last_price   INTEGER,             -- last observed best price
    last_checked INTEGER,             -- unix seconds
    notified_at  INTEGER,             -- unix seconds, 0 = never
    created_at   INTEGER NOT NULL
);
```

Daemon tick (60s — reuses `LastTick` from PR #45) selects watches whose
`last_checked` is older than the per-watch poll-interval (default 6h,
floor 1h, ceiling 24h — operator-configurable). For each:

1. Call `SearchOffers(ctx, req)`.
2. Compute `best = min(offer.Price for offer in offers)`.
3. If `best ≤ threshold` and `(notified_at == 0 || best < last_price * 0.97)`:
   - Append audit row `Kind="flight_price_alert"`.
   - Enqueue notification via voice + HUD + brief surfaces.
   - Set `notified_at = now`.
4. Update `last_price`, `last_checked`.

Notifications are descriptive only — "SFO → NRT dropped to $683 (-$117
since you set the watch at $800)". Never imperative — no "book now",
no "buy", no urgency push. Voice rate-limited to 1 alert per watch per
24h regardless of price re-drops.

`Unwatch` deletes the row + appends audit. Operator can `leah flights
unwatch <id>` or `leah flights unwatch all`.

## 6. Caching + freshness

Storage: `$HOME/.leah-state/flights-cache.db` SQLite mode 0600. Mirrors
the maps-cache + feeds-cache shape so the operator has one cache-
management mental model.

| Surface       | TTL    | Rationale |
| ---           | ---    | --- |
| SearchOffers  | 15min  | Inventory + price shift on this cadence. |
| FlightStatus  | 60s    | Day-of-travel needs near-real-time. |
| AirportInfo   | 30d    | IATA → name/city/TZ is effectively static. |

Stale-while-revalidate **off** for SearchOffers — a stale offer ID will
fail at booking; better to fail fast at search time. `FlightStatus`
also fresh-or-error (a stale "ON TIME" is worse than no answer).
`AirportInfo` can serve stale on background-refresh.

LRU eviction: 20k rows / 50MB disk, operator-configurable.

## 7. Threat model

| Threat | Mitigation |
| --- | --- |
| Credential leak via log | Creds 0600; never logged; redacted in `log.Print` sites; never in panic traces. |
| Cost blow-up on compromised key | **Hard $/day cap** (default $1/day — flights is search-heavy at $ per call). Budget guard runs before every API call; over-budget returns `ErrBudgetExceeded`; audit row per call records est cost. |
| Travel-plan leak to reasoner | Search queries stay local. Reasoner receives derived scalars only — "operator has a flight watch active" not "SFO → NRT 2026-12-15". Test asserts no IATA codes or dates in reasoner-bound payloads. |
| Accidental booking | **Booking is not in the surface.** No `Book` / `Pay` / `Purchase` method exists. Adding one requires a new spec + paid-action gate (W64). Lint rule in `flights_test.go` greps the package for forbidden tokens `Book`, `Pay`, `Purchase`, `Confirm` to keep MVP honest. |
| Upsell injection | The adapter returns Amadeus offers verbatim with no synthesis. Synthesis (top-3 ranking, "good deal" labels) lives in `internal/tripplanner/` — and the trip-planner ranks by `price × duration × stops`, never by carrier-paid promotion. |
| Attestor-bypass | `New` refuses construction without `Attestor`, `KeySource`, `Transport`. Test: `TestNewRejectsMissingDeps`. |
| Watch DoS (operator creates 1000 watches) | Hard cap: 20 active watches per operator (operator-configurable up to 100). `WatchPrice` returns `ErrWatchLimitExceeded` past the cap. |
| Notification spam | 24h rate-limit per watch (§5); a price oscillating around the threshold cannot flood voice/HUD. |
| Aggressive provider rate-limit on free tier | Exponential backoff (1s → 2s → 4s → 16s max); circuit breaker opens after 3 consecutive 429s; half-open after 5min. Mirrors info-feeds breaker. |

## 8. Test plan

Failing-test-first per `README.md` § House rules. Hermetic — no real Amadeus calls.

- **SearchOffers happy + error matrix** (`flights_test.go`): `httptest.Server`
  stands in for Amadeus; canned responses cover happy path, 401 (creds
  expired → re-auth), 429, 5xx, malformed JSON, empty result, timeout.
- **FlightStatus + AirportInfo**: same pattern.
- **WatchPrice round-trip** (`watch_test.go`): create → ListWatches →
  daemon-tick fixture drives one poll cycle → asserts notification when
  threshold crossed → asserts no re-notify within 24h → Unwatch removes.
- **Watch-cap test**: 21st watch returns `ErrWatchLimitExceeded`.
- **Booking-token lint** (`forbidden_test.go`): static scan asserts no
  `Book` / `Pay` / `Purchase` / `Confirm` exported identifier exists in
  the flights package.
- **Reasoner-leak fixture** (`leak_test.go`): asserts no IATA codes or
  dates in any reasoner-bound payload emitted during a Search cycle.
- **Budget-cap test**: pre-charge to $1 minus one cent; next call
  `ErrBudgetExceeded`.
- **Attestation gate**: `failingKeySource.Credentials()` assertion fires
  the test if denial-path materializes credentials.

## 9. Future wiring (W59 + downstream)

- `cmd/leah/flights.go` — `leah connect flights` / `leah disconnect
  flights` / `leah flights search` / `leah flights watch` / `leah
  flights unwatch` / `leah flights status`.
- `cmd/leahd/build_flights.go` — daemon-boot constructor + watch tick
  registration.
- `internal/brief/brief.go` — adds "Active flight watches" section
  when any watches exist + a price recently dropped.
- `internal/tripplanner/` (W61) — `SuggestTrip` composes flights +
  hotels (future) + maps for itinerary.
- `internal/voice/flights_handler.go` (post-voice-comm) — "is my flight
  on time", "any deals SFO to Tokyo".
- `internal/hud/flights_panel.go` (post-HUD) — active-watch list with
  current price + delta.

## 10. Out of scope (MVP)

- **Booking, seat selection, baggage purchase, payment.** Hard line —
  W64 paid-action gate is the earliest reopen point.
- Multi-traveler search (adults + kids + infants).
- Loyalty-program-aware ranking (mileage value, status benefits).
- Hotel + car + transfer bundles (Amadeus supports; no MVP caller).
- Award / miles redemption search.
- Refund / cancellation handling.
- Schedule-change push notifications from airline (DCS feeds).
- Carbon-emission scoring.

Each reopens when a real caller files an issue citing the gap.

## 11. Risks + open questions

- **Amadeus test-tier inventory is synthetic.** Dev base
  `test.api.amadeus.com` returns canned data; production base requires
  a billing relationship. Mitigation: `leah connect flights --env=test`
  for first-run smoke; operator graduates to prod env when comfortable.
- **Offer expiry race.** `FlightOffer.ExpiresAt` is real — a search
  result is bookable only for ~5 min. Booking is out of scope for MVP
  but the field is exposed so the future booking adapter (W64+) has
  the data without a re-search hop.
- **Free-tier 2000 calls/mo across watches.** 20 watches × 4 polls/day
  × 30 days = 2400 calls — over the free tier. Mitigation: per-watch
  poll-interval defaults 6h not 1h; operator can lower.
- **IATA/ICAO data drift.** Airport codes are stable but new airports
  open. AirportInfo cache 30 days; missing-airport returns `ErrNotFound`
  and operator can refresh.
- **Carrier code aliasing.** Codeshare (`UA837` operated by `LH`) is
  exposed via `Segment.OperatedBy` separate from `Carrier`. Voice +
  brief synthesis must use the operating carrier for status lookups,
  not the marketing one. Documented; pinned in synth test fixture.

> All internal paths in this doc reflect the pre-2026-07-09 layout; current tree per `git ls-tree -d --name-only HEAD:internal/`.
