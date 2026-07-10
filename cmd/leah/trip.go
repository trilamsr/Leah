package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/actions/adapters/flights"
	"github.com/trilam/leah/internal/actions/adapters/maps"
	"github.com/trilam/leah/internal/platform/contracts"
)

// runTrip surfaces the maps+flights infra as one CLI verb.
// Geocodes origin+dest via OSM, asks OSRM for a driving route, and (when
// the operator has connected flights) appends the top-3 cheapest offers.
//
// Honest-degrade contract: flights silently dropped when no token; OSM
// is keyless so the drive option is always available — UX > strict.
func runTrip(parent context.Context, args []string, w io.Writer) int {
	if shouldShowHelp(args) {
		printTripHelp(w)
		return 0
	}
	origin, dest, err := parseTripArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah trip: %v\n", err)
		printTripHelp(os.Stderr)
		return 2
	}

	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()

	osm, err := maps.NewOSM(maps.OSMConfig{
		Attestor:   newMapsAttestor(),
		HTTPClient: &http.Client{Timeout: 8 * time.Second},
		BaseURL:    os.Getenv("LEAH_TRIP_OSM_BASE"),
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah trip: maps: %v\n", err)
		return 1
	}

	originPlace, err := geocodeOne(ctx, osm, origin)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah trip: origin %q: %v\n", origin, err)
		return 1
	}
	destPlace, err := geocodeOne(ctx, osm, dest)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leah trip: destination %q: %v\n", dest, err)
		return 1
	}

	_, _ = fmt.Fprintf(w, "%s -> %s\n", originPlace.Name, destPlace.Name)

	// OSRM takes "lng,lat" pairs, not freeform; geocode-then-route is the
	// composition the spec calls "use existing constructors".
	route, rerr := osm.Route(ctx,
		lngLat(originPlace),
		lngLat(destPlace),
		maps.ModeDriving)
	if rerr != nil {
		_, _ = fmt.Fprintf(w, "  drive: unavailable (%v)\n", rerr)
	} else {
		_, _ = fmt.Fprintf(w, "  drive: %s, %s\n", formatKm(route.DistanceM), formatDuration(route.DurationS))
	}

	// Flights are best-effort: an unconfigured operator should see route
	// info, not a hard fail. SearchOffers errors degrade the same way.
	if offers := tryFlights(ctx, originPlace, destPlace); len(offers) > 0 {
		sort.SliceStable(offers, func(i, j int) bool { return offers[i].Price.Amount < offers[j].Price.Amount })
		top := offers
		if len(top) > 3 {
			top = top[:3]
		}
		for i, o := range top {
			_, _ = fmt.Fprintf(w, "  flight %d: %s %.2f (offer %s)\n",
				i+1, o.Price.Currency, float64(o.Price.Amount)/100, o.ID)
		}
	}
	return 0
}

// parseTripArgs accepts either `<dest>` (origin from $LEAH_TRIP_DEFAULT_ORIGIN)
// or `<origin> -> <dest>` (explicit). The arrow is a single token so the shell
// quotes naturally: `leah trip London "->" Paris`.
func parseTripArgs(args []string) (origin, dest string, err error) {
	if len(args) == 0 {
		return "", "", errors.New("destination required")
	}
	// Arrow form: <origin> -> <dest> (with arbitrary whitespace inside each side).
	for i, a := range args {
		if a == "->" {
			if i == 0 || i == len(args)-1 {
				return "", "", errors.New("arrow form requires origin and destination")
			}
			return joinWords(args[:i]), joinWords(args[i+1:]), nil
		}
	}
	dest = joinWords(args)
	origin = os.Getenv("LEAH_TRIP_DEFAULT_ORIGIN")
	if origin == "" {
		return "", "", errors.New("no origin: pass `<origin> -> <dest>` or set LEAH_TRIP_DEFAULT_ORIGIN")
	}
	return origin, dest, nil
}

func joinWords(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

// geocodeOne hides the "list might be empty" branch every caller needs;
// freeform geocode of an unknown name is a UX failure, not a route failure.
func geocodeOne(ctx context.Context, osm *maps.OSM, q string) (maps.Place, error) {
	hits, err := osm.Geocode(ctx, q)
	if err != nil {
		return maps.Place{}, err
	}
	if len(hits) == 0 {
		return maps.Place{}, fmt.Errorf("no geocode results")
	}
	return hits[0], nil
}

func lngLat(p maps.Place) string {
	return strconv.FormatFloat(p.Lng, 'f', -1, 64) + "," + strconv.FormatFloat(p.Lat, 'f', -1, 64)
}

func formatKm(distanceM int) string {
	return fmt.Sprintf("%.0f km", float64(distanceM)/1000)
}

func formatDuration(seconds int) string {
	d := time.Duration(seconds) * time.Second
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", h, m)
}

// tryFlights returns offers or nil — silent-degrade is the contract.
// Returns nil when the operator hasn't connected Amadeus (no token file)
// so the dest-only invocation still yields a usable drive line.
func tryFlights(ctx context.Context, origin, dest maps.Place) []flights.FlightOffer {
	ts := staticTokenForName("flights")
	if ts == "" {
		return nil
	}
	ad, err := flights.New(flights.Config{
		Attestor:    flightsAutoAttestor{},
		TokenSource: ts,
		HTTPClient:  &http.Client{Timeout: 8 * time.Second},
		BaseURL:     os.Getenv("LEAH_TRIP_FLIGHTS_BASE"),
	})
	if err != nil {
		return nil
	}
	// Two-week lead time gives realistic offer windows without staleness.
	depart := time.Now().AddDate(0, 0, 14)
	ret := depart.AddDate(0, 0, 1)
	offers, err := ad.SearchOffers(ctx, flights.SearchReq{
		Origin:   origin.Name,
		Dest:     dest.Name,
		DepartOn: depart,
		ReturnOn: ret,
		Pax:      1,
	})
	if err != nil {
		return nil
	}
	return offers
}

// mapsAttestor mirrors feedsAttestor — env-gated auto-attest keeps the
// brief daemon and tests non-interactive; otherwise prompt the operator.
type mapsAttestor struct{}

func newMapsAttestor() contracts.Attestor { return mapsAttestor{} }

func (mapsAttestor) Attest(_ context.Context, scope string) error {
	if os.Getenv("LEAH_MAPS_AUTO_ATTEST") == "1" {
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "attest %s? [Y/n] ", scope)
	var resp string
	_, _ = fmt.Fscanln(os.Stdin, &resp)
	switch resp {
	case "", "y", "Y", "yes", "YES":
		return nil
	}
	return errors.New("denied")
}

// flightsAutoAttestor is the trip-verb's flights gate — when the operator
// got far enough to connect Amadeus, a per-search re-prompt is friction
// without value. Same shape as purgeAutoAttestor.
type flightsAutoAttestor struct{}

func (flightsAutoAttestor) Attest(_ context.Context, _ string) error { return nil }

func printTripHelp(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: leah trip <destination>")
	_, _ = fmt.Fprintln(w, "       leah trip <origin> -> <destination>")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Compose a destination plan from the OSM + flights adapters.")
	_, _ = fmt.Fprintln(w, "Origin defaults to $LEAH_TRIP_DEFAULT_ORIGIN when omitted.")
	_, _ = fmt.Fprintln(w, "Flight offers appear only after `leah connect flights`.")
}
