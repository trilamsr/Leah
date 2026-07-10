package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/obs"
	"github.com/trilam/leah/internal/regattaclient"
)

// ErrRegattaNotConnected: Detect resolved a Mode but no transport is bound —
// writes must refuse so a broken wiring never silently no-ops Ship.
var ErrRegattaNotConnected = errors.New("regatta: transport not implemented for mode")

type bootRegattaOpts struct {
	Detect   func(ctx context.Context, opts regattaclient.DetectOpts) (regattaclient.Mode, regattaclient.Config, error)
	Attestor contracts.Attestor
	Audit    regattaclient.AuditSink
	Logger   io.Writer
	Registry *telemetry.Registry
}

// bootRegatta: Detect → on ErrNoMode gauge=1 + (nil,nil) graceful skip
// (not configured is normal for personal-use; no log to avoid polluting
// diag.state.last_error); on other error log-once + gauge=1 + (nil,err);
// on success wrap inner in NewGated + gauge=0 with mode label. Nil gated
// means "no regatta".
func bootRegatta(ctx context.Context, o bootRegattaOpts) (*regattaclient.GatedClient, error) {
	if o.Detect == nil {
		o.Detect = regattaclient.Detect
	}
	mode, cfg, err := o.Detect(ctx, regattaclient.DetectOpts{})
	gauge := o.Registry.Gauge("leah_regatta_unavailable")
	if errors.Is(err, regattaclient.ErrNoMode) {
		gauge.Set(nil, 1)
		return nil, nil
	}
	if err != nil {
		logRegattaUnavailableOnce(o.Logger, "leah-daemon: regatta detect failed: %v\n", err)
		gauge.Set(nil, 1)
		return nil, err
	}
	inner := newDaemonRegattaInner(mode, cfg)
	gated := regattaclient.NewGated(inner, o.Attestor, o.Audit)
	gauge.Set(map[string]string{"mode": string(mode)}, 0)
	return gated, nil
}

// daemonRegattaInner: boot placeholder ClientInterface — Status reports the
// resolved Mode; Ship/Review fail closed until a transport wave binds Config.
type daemonRegattaInner struct {
	mode regattaclient.Mode
	cfg  regattaclient.Config
}

func newDaemonRegattaInner(mode regattaclient.Mode, cfg regattaclient.Config) *daemonRegattaInner {
	return &daemonRegattaInner{mode: mode, cfg: cfg}
}

func (d *daemonRegattaInner) Ship(context.Context, regattaclient.ShipRequest) (regattaclient.ShipResponse, error) {
	return regattaclient.ShipResponse{}, fmt.Errorf("%w: %s", ErrRegattaNotConnected, d.mode)
}

func (d *daemonRegattaInner) Review(context.Context, regattaclient.ReviewRequest) (regattaclient.ReviewResponse, error) {
	return regattaclient.ReviewResponse{}, fmt.Errorf("%w: %s", ErrRegattaNotConnected, d.mode)
}

func (d *daemonRegattaInner) Status(context.Context) (regattaclient.Status, error) {
	return regattaclient.Status{Mode: string(d.mode), Healthy: true}, nil
}

type auditLoggerSink struct {
	l *audit.Logger
}

func newAuditLoggerSink(l *audit.Logger) regattaclient.AuditSink {
	return &auditLoggerSink{l: l}
}

func (s *auditLoggerSink) Record(row regattaclient.AuditRow) {
	outcome := "success"
	if !row.Success {
		outcome = "failure"
	}
	_ = s.l.Append(audit.Entry{Kind: row.Kind, Outcome: outcome})
}

// noopAttestor: daemon has no interactive operator at boot. A future IPC
// attestor will replace this; until then Ship/Review pass attestation
// unconditionally — daemon-initiated regatta calls are not yet a feature.
type noopAttestor struct{}

func (noopAttestor) Attest(context.Context, string) error { return nil }

// regattaLogOnce: var not const so tests can reset between cases.
var regattaLogOnce = &sync.Once{}

func logRegattaUnavailableOnce(w io.Writer, format string, args ...any) {
	regattaLogOnce.Do(func() {
		_, _ = fmt.Fprintf(w, format, args...)
	})
}
