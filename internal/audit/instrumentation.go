package audit

import (
	"context"
	"os"

	"github.com/trilam/leah/internal/obs"
)

func Bind(l *Logger, registry *obs.Registry) {
	if registry == nil || l == nil {
		return
	}
	appendC := registry.Counter("leah_audit_append_total")
	errC := registry.Counter("leah_audit_write_errors_total")
	l.OnAppend = func(err error) {
		appendC.Inc(nil)
		if err != nil {
			errC.Inc(nil)
		}
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
