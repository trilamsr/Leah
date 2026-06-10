package maps

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// CorridorOpts shapes a POIAlongRoute query. Zero-valued fields adopt the
// defaults documented per field; callers only set what they care about.
type CorridorOpts struct {
	MaxDetourMinutes int       // default 10
	Categories       []string  // e.g. ["restaurant", "gas_station"]
	OpenAt           time.Time // filter to places open at this time; zero = no filter
	SampleEveryM     int       // default 1000 (1km anchor spacing)
	TopK             int       // default 10
	RadiusM          int       // default min(SampleEveryM, 1500)
}

const (
	defaultMaxDetourMin = 10
	defaultSampleM      = 1000
	defaultTopK         = 10
	maxNearbyRadiusM    = 1500
	// drivingSpeedMPS approximates urban driving for detour-minute budget
	// math. The spec calls for a real Distance Matrix call (W57+); until that
	// surface lands we use a flat speed so the algorithm + filter still gate
	// on operator-meaningful minutes, not raw meters.
	drivingSpeedMPS = 13.9 // ~50 km/h
)

// POIAlongRoute returns operator-meaningful "on the way" places, ranked by
// rating × inverse detour. Spec §4: decode polyline → sample anchors → nearby
// per anchor (dedup) → score by detour cost vs straight-through → top-K.
func (a *Adapter) POIAlongRoute(ctx context.Context, route Route, opts CorridorOpts) ([]Place, error) {
	if err := a.gate(ctx, ScopePOIAlongRoute); err != nil {
		return nil, err
	}
	if route.Polyline == "" {
		return nil, ErrNoRoute
	}
	opts = withCorridorDefaults(opts)

	pts, err := decodePolyline(route.Polyline)
	if err != nil {
		return nil, fmt.Errorf("maps: decode route polyline: %w", err)
	}
	if len(pts) < 2 {
		return nil, ErrNoRoute
	}
	anchors := samplePolyline(pts, opts.SampleEveryM)
	if len(anchors) < 2 {
		return nil, ErrNoRoute
	}

	type scored struct {
		place    Place
		detourM  float64
		score    float64
		anchorIx int
	}
	// Dedup by place_id; keep the entry with smallest detour across anchors.
	best := map[string]scored{}

	for i := 0; i < len(anchors)-1; i++ {
		center := Place{Lat: anchors[i].lat, Lng: anchors[i].lng}
		for _, cat := range opts.Categories {
			// POINearby re-attests on the underlying maps:poi scope; that's
			// intentional — the corridor query is composed of N nearby
			// queries, and the gate-per-RPC contract treats each as its own
			// audited action. Operator UX folds these via the attestation
			// pool (PR #67) so the human sees one prompt, not N.
			cands, err := a.POINearby(ctx, center, opts.RadiusM, cat)
			if err != nil {
				if errors.Is(err, ErrAttestationDenied) {
					return nil, err
				}
				continue
			}
			for _, p := range cands {
				if !openAt(p, opts.OpenAt) {
					continue
				}
				det := detourMeters(anchors[i], latLng{p.Lat, p.Lng}, anchors[i+1])
				if det/drivingSpeedMPS > float64(opts.MaxDetourMinutes)*60 {
					continue
				}
				prior, seen := best[p.ID]
				if !seen || det < prior.detourM {
					best[p.ID] = scored{place: p, detourM: det, anchorIx: i}
				}
			}
		}
	}

	out := make([]scored, 0, len(best))
	for _, s := range best {
		detourMin := s.detourM / drivingSpeedMPS / 60
		s.score = float64(s.place.Rating) * (1.0 / (1.0 + detourMin))
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].place.ID < out[j].place.ID
	})

	k := opts.TopK
	if k > len(out) {
		k = len(out)
	}
	ranked := make([]Place, 0, k)
	for i := 0; i < k; i++ {
		ranked = append(ranked, out[i].place)
	}
	return ranked, nil
}

func withCorridorDefaults(o CorridorOpts) CorridorOpts {
	if o.MaxDetourMinutes <= 0 {
		o.MaxDetourMinutes = defaultMaxDetourMin
	}
	if o.SampleEveryM <= 0 {
		o.SampleEveryM = defaultSampleM
	}
	if o.TopK <= 0 {
		o.TopK = defaultTopK
	}
	if o.RadiusM <= 0 {
		o.RadiusM = o.SampleEveryM
		if o.RadiusM > maxNearbyRadiusM {
			o.RadiusM = maxNearbyRadiusM
		}
	}
	return o
}

// openAt returns true when the place is open at t. Zero t skips the filter.
// MVP OpenHours only carries OpenNow, so until PlaceDetails lands we treat
// OpenNow=true as "open" and OpenNow=false as "closed". A zero-valued
// OpenHours (no signal) is treated as open — better to surface a candidate
// the operator can verify than to silently drop it.
func openAt(p Place, t time.Time) bool {
	if t.IsZero() {
		return true
	}
	if (p.Hours == OpenHours{}) {
		return true
	}
	return p.Hours.OpenNow
}

// latLng is the internal geometry tuple; Place carries the operator-facing
// fields, this carries only the math the corridor algorithm needs.
type latLng struct {
	lat, lng float64
}

// decodePolyline implements Google's encoded-polyline algorithm. Inline (~30
// LOC) — no SDK dependency for an algorithm this small and stable.
func decodePolyline(s string) ([]latLng, error) {
	pts := make([]latLng, 0, len(s)/4)
	var lat, lng int32
	i := 0
	for i < len(s) {
		dlat, n, err := decodeSigned(s, i)
		if err != nil {
			return nil, err
		}
		i += n
		dlng, n, err := decodeSigned(s, i)
		if err != nil {
			return nil, err
		}
		i += n
		lat += dlat
		lng += dlng
		pts = append(pts, latLng{float64(lat) / 1e5, float64(lng) / 1e5})
	}
	return pts, nil
}

func decodeSigned(s string, start int) (int32, int, error) {
	var result uint32
	var shift uint
	i := start
	for i < len(s) {
		if i-start > 6 {
			return 0, 0, fmt.Errorf("maps: polyline chunk too long at %d", start)
		}
		b := s[i] - 63
		i++
		result |= uint32(b&0x1f) << shift
		if b < 0x20 {
			break
		}
		shift += 5
	}
	if i == start {
		return 0, 0, fmt.Errorf("maps: polyline truncated at %d", start)
	}
	v := int32(result >> 1)
	if result&1 != 0 {
		v = ^v
	}
	return v, i - start, nil
}

// samplePolyline walks the polyline emitting one anchor every spacingM
// meters. The trailing residual (< spacingM) is dropped so every adjacent
// anchor pair has a consistent ~spacingM gap.
func samplePolyline(pts []latLng, spacingM int) []latLng {
	if len(pts) == 0 {
		return nil
	}
	if len(pts) == 1 || spacingM <= 0 {
		return []latLng{pts[0]}
	}
	step := float64(spacingM)
	out := []latLng{pts[0]}
	carry := 0.0
	// Operate on a local copy so callers never see their slice mutated.
	buf := append([]latLng(nil), pts...)
	for i := 1; i < len(buf); i++ {
		segLen := haversineM(buf[i-1], buf[i])
		remaining := segLen
		for carry+remaining >= step {
			advance := step - carry
			t := advance / segLen
			lat := buf[i-1].lat + t*(buf[i].lat-buf[i-1].lat)
			lng := buf[i-1].lng + t*(buf[i].lng-buf[i-1].lng)
			out = append(out, latLng{lat, lng})
			carry = 0
			remaining -= advance
			buf[i-1] = latLng{lat, lng}
			segLen = haversineM(buf[i-1], buf[i])
		}
		carry += remaining
	}
	// The trailing residual (< spacingM) is deliberately dropped — emitting
	// it would create a short last segment that violates the "anchors are
	// ~spacingM apart" contract callers rely on for detour math.
	return out
}

// haversineM returns great-circle distance in meters. Adequate for the
// kilometer-scale geometry the corridor algorithm operates on.
func haversineM(a, b latLng) float64 {
	const R = 6371000.0
	rad := math.Pi / 180
	dlat := (b.lat - a.lat) * rad
	dlng := (b.lng - a.lng) * rad
	la1 := a.lat * rad
	la2 := b.lat * rad
	h := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(la1)*math.Cos(la2)*math.Sin(dlng/2)*math.Sin(dlng/2)
	return 2 * R * math.Asin(math.Sqrt(h))
}

// detourMeters returns the extra distance incurred by routing prev → poi → next
// versus the straight-through prev → next. Real driving detour requires a
// Distance Matrix call (deferred to a later wave); geometric detour is the
// operator-meaningful approximation used to gate budget here.
func detourMeters(prev, poi, next latLng) float64 {
	via := haversineM(prev, poi) + haversineM(poi, next)
	direct := haversineM(prev, next)
	d := via - direct
	if d < 0 {
		return 0
	}
	return d
}
