package tripplanner

import (
	"errors"
	"fmt"
	"html/template"
	"strings"
)

// ErrEmptyPlace is returned by RenderMapPreviewPanel when the Place lacks
// the minimum data the HUD panel needs (name + non-zero lat/lng). The HUD
// renders an explicit error tile so the operator sees "couldn't load"
// instead of an indistinguishable em-dash, matching the widget convention.
var ErrEmptyPlace = errors.New("tripplanner: empty place (need name and lat/lng)")

// tripMapPreviewTmpl is the v1 panel: name + lat/lng + a deterministic
// placeholder tile URL. Real map-tile rendering is a follow-up (W63b);
// this scaffold is what HUD binds to so the rest of the surface — widget
// registration, refresh loop, error semantics — can ship now.
var tripMapPreviewTmpl = template.Must(template.New("trip-mappreview").Parse(
	`<div class="widget trip-mappreview" data-ttl-ms="60000" ` +
		`data-lat="{{.Lat}}" data-lng="{{.Lng}}">` +
		`<span class="name">{{.Name}}</span>` +
		`<img class="map-tile" alt="map preview of {{.Name}}" src="{{.TileURL}}">` +
		`</div>`,
))

// RenderMapPreviewPanel projects a Place into the HUD's map-preview
// payload. v1 = data binding only; the tile URL is a placeholder the HUD
// can swap for a real tile source without a tripplanner change.
func RenderMapPreviewPanel(p Place) (string, error) {
	// Either coord at zero is treated as missing — silently rendering a tile
	// URL with one missing coordinate would point at the wrong location.
	if p.Name == "" || p.Lat == 0 || p.Lng == 0 {
		return "", fmt.Errorf("%w: name=%q lat=%v lng=%v", ErrEmptyPlace, p.Name, p.Lat, p.Lng)
	}
	tile := fmt.Sprintf("/api/widgets/trip-mappreview/tile?lat=%g&lng=%g", p.Lat, p.Lng)
	var b strings.Builder
	if err := tripMapPreviewTmpl.Execute(&b, struct {
		Name    string
		Lat     float64
		Lng     float64
		TileURL string
	}{p.Name, p.Lat, p.Lng, tile}); err != nil {
		return "", fmt.Errorf("tripplanner: render map preview: %w", err)
	}
	return b.String(), nil
}
