package widget

import (
	"context"
	"encoding/json"
	"time"
)

// Payload is one adapter fetch result — spec §10.6.
type Payload struct {
	Data       json.RawMessage `json:"data"`
	FetchedAt  time.Time       `json:"fetched_at"`
	StaleAfter time.Duration   `json:"stale_after"`
	Source     string          `json:"source"`
	Etag       string          `json:"etag,omitempty"`
}

// WidgetAdapter binds a widget kind to its data source. Spec §10.6.
type WidgetAdapter interface {
	Type() string
	Validate(props json.RawMessage) error
	Fetch(ctx context.Context, props json.RawMessage) (Payload, error)
	Refresh(ctx context.Context, id string, props json.RawMessage, prev *Payload) (Payload, error)
}
