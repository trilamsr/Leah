package flights

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/trilam/leah/internal/widget"
)

var iata = regexp.MustCompile(`^[A-Z]{3}$`)

// Adapter implements widget.WidgetAdapter for kind "flights".
type Adapter struct{ p Provider }

func NewAdapter(p Provider) *Adapter { return &Adapter{p: p} }

func (a *Adapter) Type() string { return "flights" }

func (a *Adapter) Validate(props json.RawMessage) error {
	var q Query
	if err := json.Unmarshal(props, &q); err != nil {
		return fmt.Errorf("flights: invalid props: %w", err)
	}
	if !iata.MatchString(q.Origin) {
		return fmt.Errorf("flights: origin %q must match %s", q.Origin, iata)
	}
	if !iata.MatchString(q.Destination) {
		return fmt.Errorf("flights: destination %q must match %s", q.Destination, iata)
	}
	if q.Pax < 1 || q.Pax > 9 {
		return fmt.Errorf("flights: pax %d outside 1..9", q.Pax)
	}
	if q.MaxStops < 0 || q.MaxStops > 2 {
		return fmt.Errorf("flights: max_stops %d outside 0..2", q.MaxStops)
	}
	return nil
}

func (a *Adapter) Fetch(ctx context.Context, props json.RawMessage) (widget.Payload, error) {
	if err := ctx.Err(); err != nil {
		return widget.Payload{}, err
	}
	if err := a.Validate(props); err != nil {
		return widget.Payload{}, err
	}
	var q Query
	_ = json.Unmarshal(props, &q)
	m, err := a.p.Search(ctx, q)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("flights: search: %w", err)
	}
	tagGlobalMin(&m)
	if m.FetchedAt.IsZero() {
		m.FetchedAt = time.Now()
	}
	data, err := json.Marshal(m)
	if err != nil {
		return widget.Payload{}, fmt.Errorf("flights: marshal: %w", err)
	}
	return widget.Payload{
		Data:       data,
		FetchedAt:  m.FetchedAt,
		StaleAfter: 0,
		Source:     a.p.Source(),
	}, nil
}

// Refresh is manual-only — auto-refresh callers receive ErrManualOnly so the
// scheduler can skip without retrying.
func (a *Adapter) Refresh(_ context.Context, _ string, _ json.RawMessage, _ *widget.Payload) (widget.Payload, error) {
	return widget.Payload{}, ErrManualOnly
}

func tagGlobalMin(m *Matrix) {
	if len(m.Cells) == 0 {
		return
	}
	minIdx := 0
	for i := 1; i < len(m.Cells); i++ {
		if m.Cells[i].PriceCents < m.Cells[minIdx].PriceCents {
			minIdx = i
		}
	}
	for i := range m.Cells {
		m.Cells[i].GlobalMin = i == minIdx
	}
}

// FixtureProvider serves a canned matrix from disk for v1 + tests.
type FixtureProvider struct {
	matrix Matrix
}

func NewFixtureProvider(path string) (*FixtureProvider, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("flights fixture %s: %w", path, err)
	}
	var m Matrix
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("flights fixture %s: %w", path, err)
	}
	return &FixtureProvider{matrix: m}, nil
}

func (f *FixtureProvider) Search(_ context.Context, _ Query) (Matrix, error) {
	out := f.matrix
	out.Cells = append([]Cell(nil), f.matrix.Cells...)
	return out, nil
}

func (f *FixtureProvider) Source() string { return "flights-fixture" }

var (
	_ Provider             = (*FixtureProvider)(nil)
	_ widget.WidgetAdapter = (*Adapter)(nil)
)
