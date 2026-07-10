package audit

import (
	"context"
	"os"

	"github.com/trilam/leah/internal/platform/telemetry"
)

func Bind(l *Logger, registry *telemetry.Registry) {
	if registry == nil || l == nil {
		return
	}
	appendC := registry.Counter("leah_audit_append_total")
	errC := registry.Counter("leah_audit_write_errors_total")
	dropC := registry.Counter("leah_audit_subscriber_dropped_total")
	l.OnAppend = func(err error) {
		appendC.Inc(nil)
		if err != nil {
			errC.Inc(nil)
		}
	}
	l.OnSubscriberDrop = func(subscriber string, n int64) {
		dropC.Add(map[string]string{"subscriber": subscriber}, n)
	}
}

type SelfChecker struct{ Logger *Logger }

func (c *SelfChecker) SelfCheck(ctx context.Context) error {
	_ = ctx
	if c == nil || c.Logger == nil {
		return nil
	}
	if err := c.Logger.Append(Entry{Kind: "obs.selfcheck", Outcome: "probe"}); err != nil {
		return err
	}
	_, err := os.Stat(c.Logger.Path)
	return err
}
