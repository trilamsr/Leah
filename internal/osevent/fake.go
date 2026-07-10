package osevent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// Fake is a hermetic in-process Source for tests. Inject delivers an event
// to every matching subscriber synchronously; tests need no sleep.
type Fake struct {
	mu        sync.Mutex
	closed    bool
	blocklist []string
	subs      []*fakeSub
}

type fakeSub struct {
	kind   Kind
	ch     chan Event
	ctx    context.Context
	stop   func()
	closed bool // guarded by Fake.mu — single-shot channel-close
}

// NewFake returns a hermetic Source carrying cfg.Blocklist. Tests use
// Inject() to drive events.
func NewFake(cfg Config) *Fake {
	return &Fake{blocklist: append([]string(nil), cfg.Blocklist...)}
}

// Subscribe returns a buffered channel (16) for events matching kind. The
// channel closes on ctx.Done() or Close(). Spec §5 buffer policy: drop on
// full so a slow consumer cannot wedge the pump.
func (f *Fake) Subscribe(ctx context.Context, kind Kind) (<-chan Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, errors.New("osevent: source closed")
	}
	sctx, cancel := context.WithCancel(ctx)
	s := &fakeSub{kind: kind, ch: make(chan Event, 16), ctx: sctx, stop: cancel}
	f.subs = append(f.subs, s)
	go func() {
		<-sctx.Done()
		f.removeSub(s)
	}()
	return s.ch, nil
}

// Inject delivers e to every subscriber matching e.Kind. Active-app events
// for blocklisted bundles are dropped at the source.
func (f *Fake) Inject(e Event) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if f.suppressed(e) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.subs {
		if s.kind != e.Kind || s.closed {
			continue
		}
		select {
		case s.ch <- e:
		default: // drop-on-full mirrors prod push pump policy
		}
	}
}

func (f *Fake) suppressed(e Event) bool {
	if e.Kind != WorkspaceActiveAppChanged {
		return false
	}
	bid, _ := e.Detail["bundle_id"].(string)
	for _, p := range f.blocklist {
		if p != "" && strings.HasPrefix(bid, p) {
			return true
		}
	}
	return false
}

func (f *Fake) removeSub(s *fakeSub) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
	for i, x := range f.subs {
		if x == s {
			f.subs = append(f.subs[:i], f.subs[i+1:]...)
			return
		}
	}
}

// Close shuts down every subscription. Idempotent.
func (f *Fake) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	subs := f.subs
	f.subs = nil
	for _, s := range subs {
		if !s.closed {
			s.closed = true
			close(s.ch)
		}
	}
	f.mu.Unlock()
	for _, s := range subs {
		s.stop()
	}
	return nil
}
