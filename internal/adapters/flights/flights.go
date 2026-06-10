package flights

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/trilam/leah/internal/contracts"
)

var (
	ErrAttestationDenied = errors.New("flights: attestation denied")
	ErrAttestorRequired  = errors.New("flights: Config.Attestor required (operator-attestation gate)")
	ErrTokenRequired     = errors.New("flights: Config.TokenSource required")
	ErrHTTPRequired      = errors.New("flights: Config.HTTPClient required")
	ErrNotFound          = errors.New("flights: not found")
	ErrInvalidQuery      = errors.New("flights: invalid query")
	ErrAPIStatus         = errors.New("flights: api non-OK status")
)

// Per-RPC scopes — the operator-attestation log attributes consent per action.
const (
	ScopeSearch  = "flights:search"
	ScopeStatus  = "flights:status"
	ScopeAirport = "flights:airport"
	ScopeWatch   = "flights:watch"
)

const defaultBaseURL = "https://test.api.amadeus.com"

type (
	Attestor    = contracts.Attestor
	TokenSource = contracts.TokenSource
	HTTPClient  = contracts.HTTPClient
)

type Money struct {
	Amount   int64
	Currency string
}

type Airport struct {
	IATA, ICAO, Name, City, Country, TZ string
}

type FlightOffer struct {
	ID        string
	Price     Money
	ExpiresAt time.Time
}

type SearchReq struct {
	Origin, Dest string
	DepartOn     time.Time
	ReturnOn     time.Time
	Pax          int
	Cabin        string
	MaxStops     int
	NonStop      bool
}

type Status struct {
	Carrier            string
	FlightNumber       string
	State              string
	ScheduledDeparture time.Time
	ScheduledArrival   time.Time
}

type Config struct {
	Attestor    Attestor
	TokenSource TokenSource
	HTTPClient  HTTPClient
	BaseURL     string
}

// Adapter is the flights adapter; lifecycle owned by the caller, no goroutines.
type Adapter struct {
	att     Attestor
	ts      TokenSource
	http    HTTPClient
	baseURL string

	mu      sync.Mutex
	watches map[string]watch
}

type watch struct {
	Req       SearchReq
	Threshold Money
	CreatedAt time.Time
}

func New(cfg Config) (*Adapter, error) {
	if cfg.Attestor == nil {
		return nil, ErrAttestorRequired
	}
	if cfg.TokenSource == nil {
		return nil, ErrTokenRequired
	}
	if cfg.HTTPClient == nil {
		return nil, ErrHTTPRequired
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	return &Adapter{
		att:     cfg.Attestor,
		ts:      cfg.TokenSource,
		http:    cfg.HTTPClient,
		baseURL: base,
		watches: make(map[string]watch),
	}, nil
}

// gateAndToken runs attestation BEFORE materializing the bearer token so a
// denial never leaves credentials in a panic trace or log.
func (a *Adapter) gateAndToken(ctx context.Context, scope string) (string, error) {
	if err := a.att.Attest(ctx, scope); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	tok, err := a.ts.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("flights: token load (%s): %w", scope, err)
	}
	return tok, nil
}

func (a *Adapter) do(ctx context.Context, scope, path string, q url.Values, out any) error {
	tok, err := a.gateAndToken(ctx, scope)
	if err != nil {
		return err
	}
	u := a.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("flights: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("flights: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%w: %d", ErrAPIStatus, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("flights: decode: %w", err)
	}
	return nil
}

type offersResp struct {
	Data []struct {
		ID    string `json:"id"`
		Price struct {
			Total    string `json:"total"`
			Currency string `json:"currency"`
		} `json:"price"`
		LastTicketingDate string `json:"lastTicketingDate"`
	} `json:"data"`
}

func (a *Adapter) SearchOffers(ctx context.Context, req SearchReq) ([]FlightOffer, error) {
	if req.Origin == "" || req.Dest == "" || req.DepartOn.IsZero() {
		return nil, ErrInvalidQuery
	}
	pax := req.Pax
	if pax <= 0 {
		pax = 1
	}
	q := url.Values{}
	q.Set("originLocationCode", req.Origin)
	q.Set("destinationLocationCode", req.Dest)
	q.Set("departureDate", req.DepartOn.Format("2006-01-02"))
	if !req.ReturnOn.IsZero() {
		q.Set("returnDate", req.ReturnOn.Format("2006-01-02"))
	}
	q.Set("adults", strconv.Itoa(pax))
	if req.Cabin != "" {
		q.Set("travelClass", req.Cabin)
	}
	if req.NonStop {
		q.Set("nonStop", "true")
	}
	if req.MaxStops > 0 {
		q.Set("maxNumberOfConnections", strconv.Itoa(req.MaxStops))
	}
	var body offersResp
	if err := a.do(ctx, ScopeSearch, "/v2/shopping/flight-offers", q, &body); err != nil {
		return nil, err
	}
	out := make([]FlightOffer, 0, len(body.Data))
	for _, d := range body.Data {
		amt, _ := parsePriceMinor(d.Price.Total)
		out = append(out, FlightOffer{
			ID:    d.ID,
			Price: Money{Amount: amt, Currency: d.Price.Currency},
		})
	}
	return out, nil
}

type statusResp struct {
	Data []struct {
		FlightDesignator struct {
			CarrierCode  string `json:"carrierCode"`
			FlightNumber string `json:"flightNumber"`
		} `json:"flightDesignator"`
		FlightStatus string `json:"flightStatus"`
		FlightPoints []struct {
			Departure struct {
				Timings []struct {
					Value string `json:"value"`
				} `json:"timings"`
			} `json:"departure"`
			Arrival struct {
				Timings []struct {
					Value string `json:"value"`
				} `json:"timings"`
			} `json:"arrival"`
		} `json:"flightPoints"`
	} `json:"data"`
}

// splitFlightNumber parses "UA837" into ("UA", "837"); a bare numeric stays as
// (carrier="", number=raw) so callers see ErrInvalidQuery upstream.
func splitFlightNumber(s string) (carrier, num string) {
	for i, r := range s {
		if r >= '0' && r <= '9' {
			return s[:i], s[i:]
		}
	}
	return "", s
}

func (a *Adapter) FlightStatus(ctx context.Context, flightNumber string, on time.Time) (Status, error) {
	flightNumber = strings.TrimSpace(flightNumber)
	carrier, num := splitFlightNumber(flightNumber)
	if carrier == "" || num == "" {
		return Status{}, fmt.Errorf("%w: flight number must be carrier+digits (e.g. UA837)", ErrInvalidQuery)
	}
	if on.IsZero() {
		return Status{}, ErrInvalidQuery
	}
	q := url.Values{}
	q.Set("carrierCode", carrier)
	q.Set("flightNumber", num)
	q.Set("scheduledDepartureDate", on.Format("2006-01-02"))
	var body statusResp
	if err := a.do(ctx, ScopeStatus, "/v2/schedule/flights", q, &body); err != nil {
		return Status{}, err
	}
	if len(body.Data) == 0 {
		return Status{}, ErrNotFound
	}
	d := body.Data[0]
	st := Status{
		Carrier:      d.FlightDesignator.CarrierCode,
		FlightNumber: d.FlightDesignator.FlightNumber,
		State:        d.FlightStatus,
	}
	for _, p := range d.FlightPoints {
		if len(p.Departure.Timings) > 0 {
			if t, err := time.Parse(time.RFC3339, p.Departure.Timings[0].Value); err == nil {
				st.ScheduledDeparture = t
			}
		}
		if len(p.Arrival.Timings) > 0 {
			if t, err := time.Parse(time.RFC3339, p.Arrival.Timings[0].Value); err == nil {
				st.ScheduledArrival = t
			}
		}
	}
	return st, nil
}

type locationResp struct {
	Data []struct {
		IATACode string `json:"iataCode"`
		Name     string `json:"name"`
		Address  struct {
			CityName    string `json:"cityName"`
			CountryName string `json:"countryName"`
		} `json:"address"`
		TimeZoneOffset string `json:"timeZoneOffset"`
	} `json:"data"`
}

func (a *Adapter) AirportInfo(ctx context.Context, iata string) (Airport, error) {
	iata = strings.ToUpper(strings.TrimSpace(iata))
	if len(iata) != 3 {
		return Airport{}, fmt.Errorf("%w: iata must be 3 letters", ErrInvalidQuery)
	}
	q := url.Values{}
	q.Set("subType", "AIRPORT")
	q.Set("keyword", iata)
	var body locationResp
	if err := a.do(ctx, ScopeAirport, "/v1/reference-data/locations", q, &body); err != nil {
		return Airport{}, err
	}
	if len(body.Data) == 0 {
		return Airport{}, ErrNotFound
	}
	d := body.Data[0]
	return Airport{
		IATA:    d.IATACode,
		Name:    d.Name,
		City:    d.Address.CityName,
		Country: d.Address.CountryName,
		TZ:      d.TimeZoneOffset,
	}, nil
}

// WatchPrice records an in-memory watch and returns its opaque id. Persistent
// storage (SQLite per spec §5) and the daemon tick loop land in the wiring wave.
func (a *Adapter) WatchPrice(ctx context.Context, req SearchReq, threshold Money) (string, error) {
	if err := a.att.Attest(ctx, ScopeWatch); err != nil {
		return "", fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if req.Origin == "" || req.Dest == "" || req.DepartOn.IsZero() {
		return "", ErrInvalidQuery
	}
	if threshold.Amount <= 0 || threshold.Currency == "" {
		return "", fmt.Errorf("%w: threshold must be positive with currency", ErrInvalidQuery)
	}
	id, err := newWatchID()
	if err != nil {
		return "", fmt.Errorf("flights: gen watch id: %w", err)
	}
	a.mu.Lock()
	a.watches[id] = watch{Req: req, Threshold: threshold, CreatedAt: time.Now()}
	a.mu.Unlock()
	return id, nil
}

func newWatchID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "fw_" + hex.EncodeToString(b[:]), nil
}

// parsePriceMinor converts "123.45" → 12345 minor units. Amadeus returns price
// totals as decimal strings; everything downstream uses int64 cents to dodge
// float drift on threshold comparisons.
func parsePriceMinor(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, err
		}
		return n * 100, nil
	}
	whole, err := strconv.ParseInt(s[:dot], 10, 64)
	if err != nil {
		return 0, err
	}
	frac := s[dot+1:]
	switch {
	case len(frac) == 0:
		return whole * 100, nil
	case len(frac) == 1:
		f, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, err
		}
		return whole*100 + f*10, nil
	default:
		f, err := strconv.ParseInt(frac[:2], 10, 64)
		if err != nil {
			return 0, err
		}
		return whole*100 + f, nil
	}
}
