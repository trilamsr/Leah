// Package recommend's SignalDispatcher routes obs.Broadcaster events to
// SignalEngine.OnSignal. Bus uses dotted KnownEventKinds; matcher classes
// use underscore form and never appear on the bus. Engine's lastFiredAt
// debounce state is in-memory only — restart resets the 30s window.
package recommend

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/trilam/leah/internal/platform/telemetry"
)

type ContextProvider func() string

type SignalDispatcher struct {
	engine SignalEngine
	bus    *telemetry.Broadcaster
	ctxFn  ContextProvider
	kinds  []string
	reg    *telemetry.Registry
	tick   time.Duration

	mu       sync.Mutex
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	lastDrop uint64
}

// DefaultSignalKinds — every entry must exist in obs.KnownEventKinds.
// recommendation.propose excluded: Seed publishes it; admitting it loops.
var DefaultSignalKinds = []string{
	"voice.speak",
	"audit.append",
	"dispatch.ship",
	"hud.state",
	"memory.upsert",
	"workspace.active_app_changed",
}

const DropMonitorInterval = 10 * time.Second

var ErrFeedbackLoopKind = errors.New("recommend: kind would feedback-loop into OnSignal")

func NewSignalDispatcher(engine SignalEngine, bus *telemetry.Broadcaster, ctxFn ContextProvider) *SignalDispatcher {
	return &SignalDispatcher{
		engine: engine,
		bus:    bus,
		ctxFn:  ctxFn,
		kinds:  DefaultSignalKinds,
	}
}

// WithKinds rejects recommendation.propose to avoid Seed→OnSignal feedback.
func (d *SignalDispatcher) WithKinds(kinds []string) (*SignalDispatcher, error) {
	if len(kinds) == 0 {
		d.kinds = DefaultSignalKinds
		return d, nil
	}
	for _, k := range kinds {
		if k == "recommendation.propose" {
			return nil, ErrFeedbackLoopKind
		}
	}
	d.kinds = kinds
	return d, nil
}

func (d *SignalDispatcher) WithRegistry(reg *telemetry.Registry) *SignalDispatcher {
	d.reg = reg
	return d
}

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
	telemetry.SafeGo(nil, d.reg, "recommend-pump", func() { d.pump(runCtx, sub) })
	if d.reg != nil {
		d.wg.Add(1)
		interval := d.tick
		if interval <= 0 {
			interval = DropMonitorInterval
		}
		telemetry.SafeGo(nil, d.reg, "recommend-drop-monitor", func() { d.monitorDrops(runCtx, interval) })
	}
	return nil
}

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

func (d *SignalDispatcher) pump(ctx context.Context, sub telemetry.SSESubscriber) {
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
			sig := Signal{Kind: e.Kind, At: e.TS, Detail: e.Detail}
			if d.ctxFn != nil {
				sig.Context = d.ctxFn()
			}
			_, _ = d.engine.OnSignal(ctx, sig)
		}
	}
}

func (d *SignalDispatcher) monitorDrops(ctx context.Context, interval time.Duration) {
	defer d.wg.Done()
	c := d.reg.Counter("leah_signal_dispatcher_dropped_total")
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cur := d.bus.Dropped()
			d.mu.Lock()
			delta := cur - d.lastDrop
			d.lastDrop = cur
			d.mu.Unlock()
			if delta > 0 {
				c.Add(nil, int64(delta))
			}
		}
	}
}
