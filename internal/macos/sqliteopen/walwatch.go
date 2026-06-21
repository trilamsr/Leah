package sqliteopen

import (
	"context"
	"sync"
	"time"
)

// WALWatcher delivers a tick per filesystem event observed on a single SQLite
// WAL path. Used by messages/mail/notes push sources — those packages cannot
// observe row-level changes without opening the DB, so they coalesce off the
// WAL's mtime/size mutation as a proxy for "store changed". Subscribe returns
// a channel that closes on Close() or ctx.Done(); drop-on-full per the
// existing osevent buffer policy.
type WALWatcher interface {
	Subscribe(ctx context.Context) (<-chan WALEvent, error)
	Close() error
}

// WALEvent carries only the timestamp — the path is implicit in the watcher
// instance, and consumers re-stat the DB themselves rather than trusting an
// event payload they did not produce.
type WALEvent struct {
	At time.Time
}

// FakeWALWatcher is a hermetic in-process WALWatcher for tests. Inject pushes
// to every active subscriber synchronously; drop-on-full mirrors prod policy.
type FakeWALWatcher struct {
	mu     sync.Mutex
	closed bool
	subs   []*fakeWALSub
}

type fakeWALSub struct {
	ch     chan WALEvent
	stop   func()
	closed bool
}

func NewFakeWALWatcher() *FakeWALWatcher { return &FakeWALWatcher{} }

func (f *FakeWALWatcher) Subscribe(ctx context.Context) (<-chan WALEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, errClosed
	}
	sctx, cancel := context.WithCancel(ctx)
	s := &fakeWALSub{ch: make(chan WALEvent, 16), stop: cancel}
	f.subs = append(f.subs, s)
	go func() {
		<-sctx.Done()
		f.remove(s)
	}()
	return s.ch, nil
}

// Inject delivers ev to every active subscriber. Drop-on-full to mirror the
// prod FSEvents push pump's policy — a stalled consumer cannot wedge the pump.
func (f *FakeWALWatcher) Inject(ev WALEvent) {
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.subs {
		if s.closed {
			continue
		}
		select {
		case s.ch <- ev:
		default:
		}
	}
}

func (f *FakeWALWatcher) remove(s *fakeWALSub) {
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

func (f *FakeWALWatcher) Close() error {
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
