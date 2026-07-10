package commsout

import (
	"context"
	"errors"

	"github.com/trilam/leah/internal/platform/contracts"
)

// Fanout dispatches one Notify call to every wrapped Notifier and joins
// their errors via errors.Join. A nil or partial failure DOES NOT short-
// circuit — every notifier still receives the call. Empty slice returns
// nil so composition roots can conditionally append.
type Fanout struct {
	Notifiers []contracts.Notifier
}

// Notify invokes each wrapped Notifier in order and joins any errors.
// Order is preserved so callers can rely on (cheap-first, expensive-last)
// scheduling — desktop banner before voice TTS, for instance.
func (f *Fanout) Notify(ctx context.Context, title, body string) error {
	var errs []error
	for _, n := range f.Notifiers {
		if n == nil {
			continue // defensive: composition roots may conditionally nil-out entries
		}
		if err := n.Notify(ctx, title, body); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
