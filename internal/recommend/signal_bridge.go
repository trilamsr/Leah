package recommend

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// Push-source → SignalEngine.OnSignal bridge (MAY-V8). Replaces the 60s
// daemon-tick poll: each whitelisted obs.Event becomes one OnSignal call so
// recommendation latency tracks the underlying push (NSWorkspace, EventKit,
// FSEvents) rather than the next tick boundary.

// EventSubscriber is the slice of obs.Broadcaster the bridge needs. Narrow
// surface keeps tests from constructing an SSE handler stack.
type EventSubscriber interface {
	Subscribe(ctx context.Context, kinds []string) (obs.SSESubscriber, error)
}

// SignalBridgeOptions configures StartSignalBridge. Kinds is required — an
// empty allowlist would subscribe to every event and turn the bridge into a
// firehose; explicit is safer than a silent default.
type SignalBridgeOptions struct {
	Kinds []string

	// OnError observes engine errors (test seam). Production wiring leaves it
	// nil — errors are dropped because the bridge cannot meaningfully recover.
	OnError func(error)
}

// SignalBridge is the running goroutine handle. Stop blocks until exit.
type SignalBridge struct {
	sub    obs.SSESubscriber
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

var errBridgeNoKinds = errors.New("recommend: SignalBridgeOptions.Kinds required")

// StartSignalBridge subscribes to bus, converts each event into a Signal, and
// invokes engine.OnSignal. Returns once the subscription is wired so the
// caller can rely on first-event delivery without a sleep.
func StartSignalBridge(ctx context.Context, bus EventSubscriber, engine *MemoryEngine, opts SignalBridgeOptions) (*SignalBridge, error) {
	if len(opts.Kinds) == 0 {
		return nil, errBridgeNoKinds
	}
	subCtx, cancel := context.WithCancel(ctx)
	sub, err := bus.Subscribe(subCtx, opts.Kinds)
	if err != nil {
		cancel()
		return nil, err
	}
	br := &SignalBridge{
		sub:    sub,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go br.run(subCtx, engine, opts.OnError)
	return br, nil
}

func (b *SignalBridge) run(ctx context.Context, engine *MemoryEngine, onErr func(error)) {
	defer close(b.done)
	events := b.sub.Events()
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
			if sig.At.IsZero() {
				sig.At = time.Now().UTC()
			}
			if _, err := engine.OnSignal(ctx, sig); err != nil && onErr != nil {
				onErr(err)
			}
		}
	}
}

// Stop cancels the bridge and waits for the goroutine to exit. Safe to call
// multiple times; redundant calls collapse into the first.
func (b *SignalBridge) Stop() {
	b.once.Do(func() {
		b.cancel()
		_ = b.sub.Close()
	})
	<-b.done
}

// Wait blocks until the bridge exits or timeout elapses. Used by tests that
// trigger exit via the parent context rather than Stop().
func (b *SignalBridge) Wait(timeout time.Duration) error {
	select {
	case <-b.done:
		return nil
	case <-time.After(timeout):
		return errors.New("recommend: SignalBridge.Wait timeout")
	}
}
