package recommend

import (
	"context"
	"sync"

	"github.com/trilam/leah/internal/obs"
)

// ContextProvider returns the operatormodel.ctxmgr.Current() label at call
// time. Indirection avoids an internal/operatormodel import cycle (recommend
// is imported by operatormodel, not the other way round).
type ContextProvider func() string

// SignalDispatcher subscribes to an obs.Broadcaster and translates Events
// into recommend.Signals fed to engine.OnSignal. Wave-9 V8 replaces the
// 60s daemon-tick Propose poll with this push path.
type SignalDispatcher struct {
	engine  SignalEngine
	bus     *obs.Broadcaster
	ctxFn   ContextProvider
	kinds   []string

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// DefaultSignalKinds is the broadcaster filter used when caller passes none.
// Mirrors the kinds the W18 propose-loop already cares about: app focus,
// calendar imminence, voice wake, plus the audit-mirror class.
var DefaultSignalKinds = []string{
	"app.focus",
	"calendar.imminent",
	"voice.speak",
	"audit.append",
	"context.transition",
}

// NewSignalDispatcher wires engine to bus. ctxFn may be nil; Signal.Context
// is "" in that case. Kinds defaults to DefaultSignalKinds when nil.
func NewSignalDispatcher(engine SignalEngine, bus *obs.Broadcaster, ctxFn ContextProvider) *SignalDispatcher {
	return &SignalDispatcher{
		engine: engine,
		bus:    bus,
		ctxFn:  ctxFn,
		kinds:  DefaultSignalKinds,
	}
}

// WithKinds overrides the default event-kind allowlist; nil/empty restores defaults.
func (d *SignalDispatcher) WithKinds(kinds []string) *SignalDispatcher {
	if len(kinds) == 0 {
		d.kinds = DefaultSignalKinds
	} else {
		d.kinds = kinds
	}
	return d
}

// Start opens a Broadcaster subscription and pumps events into engine.OnSignal
// on a single goroutine. Idempotent — second Start is a no-op until Stop.
func (d *SignalDispatcher) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		return nil
	}
	sub, err := d.bus.Subscribe(ctx, d.kinds)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel
	d.wg.Add(1)
	go d.pump(runCtx, sub)
	return nil
}

// Stop tears down the subscription goroutine and blocks until it returns.
func (d *SignalDispatcher) Stop() {
	d.mu.Lock()
	cancel := d.cancel
	d.cancel = nil
	d.mu.Unlock()
	if cancel != nil {
		cancel()
		d.wg.Wait()
	}
}

func (d *SignalDispatcher) pump(ctx context.Context, sub obs.SSESubscriber) {
	defer d.wg.Done()
	defer func() { _ = sub.Close() }()
	events := sub.Events()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			sig := Signal{
				Kind:   e.Kind,
				At:     e.TS,
				Detail: e.Detail,
			}
			if d.ctxFn != nil {
				sig.Context = d.ctxFn()
			}
			_, _ = d.engine.OnSignal(ctx, sig)
		}
	}
}
