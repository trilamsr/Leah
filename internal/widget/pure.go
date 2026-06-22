package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// PureAdapter serves widget kinds where the LLM emits the payload directly
// (stat/table/list/code/diff/citation/image/chart-pure). No external fetch,
// so StaleAfter is zero and Refresh echoes prev when present.
type PureAdapter struct{ kind string }

// NewPureAdapter binds a pure adapter to a widget kind.
func NewPureAdapter(kind string) PureAdapter { return PureAdapter{kind: kind} }

func (p PureAdapter) Type() string { return p.kind }

// Validate accepts any non-empty JSON ≤ MaxEnvelopeBytes (256 KB cap, spec §10.7).
func (p PureAdapter) Validate(props json.RawMessage) error {
	if len(props) == 0 {
		return fmt.Errorf("widget %q: props empty", p.kind)
	}
	if len(props) > MaxEnvelopeBytes {
		return fmt.Errorf("widget %q: props %d bytes exceeds 256 KB cap", p.kind, len(props))
	}
	return nil
}

// Fetch returns props verbatim — LLM already authored the payload.
func (p PureAdapter) Fetch(ctx context.Context, props json.RawMessage) (Payload, error) {
	if err := ctx.Err(); err != nil {
		return Payload{}, err
	}
	return Payload{Data: props, FetchedAt: time.Now(), Source: "pure", StaleAfter: 0}, nil
}

// Refresh echoes prev (pure widgets never re-fetch). Falls back to Fetch if prev nil.
func (p PureAdapter) Refresh(ctx context.Context, _ string, props json.RawMessage, prev *Payload) (Payload, error) {
	if err := ctx.Err(); err != nil {
		return Payload{}, err
	}
	if prev != nil {
		return *prev, nil
	}
	return p.Fetch(ctx, props)
}
