package tripplanner

import (
	"strings"
	"testing"
)

// Locks the v1 contract: panel HTML carries the suggestion name, lat/lng,
// and a deterministic placeholder map-tile URL keyed by lat/lng — real tile
// rendering is a follow-up; HUD just needs a data-bound surface.
func TestRenderMapPreviewPanel_HappyPath(t *testing.T) {
	p := Place{
		ID:   "sushi-jiro",
		Name: "Sushi Jiro",
		Lat:  35.6717,
		Lng:  139.7639,
	}
	html, err := RenderMapPreviewPanel(p)
	if err != nil {
		t.Fatalf("RenderMapPreviewPanel: %v", err)
	}
	for _, want := range []string{
		`class="widget trip-mappreview"`,
		`Sushi Jiro`,
		`data-lat="35.6717"`,
		`data-lng="139.7639"`,
		`/api/widgets/trip-mappreview/tile?lat=35.6717&amp;lng=139.7639`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("panel missing %q\n--- panel ---\n%s", want, html)
		}
	}
}

// Empty Place still produces an explicit error tile, not silent em-dash —
// matches the existing widget convention.
func TestRenderMapPreviewPanel_EmptyPlaceIsError(t *testing.T) {
	html, err := RenderMapPreviewPanel(Place{})
	if err == nil {
		t.Fatalf("expected error for empty Place, got nil")
	}
	if html != "" {
		t.Errorf("expected empty html on error, got %q", html)
	}
}

// XSS sanity: HTML-special chars in Name must be escaped — the operator
// can name a place arbitrarily; the template runs through html/template.
func TestRenderMapPreviewPanel_NameIsEscaped(t *testing.T) {
	p := Place{Name: `<script>alert(1)</script>`, Lat: 1, Lng: 2}
	html, err := RenderMapPreviewPanel(p)
	if err != nil {
		t.Fatalf("RenderMapPreviewPanel: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("Name not escaped:\n%s", html)
	}
}
