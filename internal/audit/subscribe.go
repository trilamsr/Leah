package audit

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

type subscription struct {
	name    string
	ch      chan Entry
	done    chan struct{}
	dropped uint64
}

// Subscribe returns a bounded push channel and unsubscribe func. The channel is never closed; cancel closes a done sentinel that fanout selects on, so a concurrent Append cannot panic sending into a cancelled sub's channel.
func (l *Logger) Subscribe(buf int) (<-chan Entry, func()) {
	if buf < 1 {
		buf = 1
	}
	sub := &subscription{
		ch:   make(chan Entry, buf),
		done: make(chan struct{}),
	}
	sub.name = fmt.Sprintf("sub-%02x", uint8(uintptr(unsafe.Pointer(sub))>>4))

	l.subsMu.Lock()
	l.subs = append(l.subs, sub)
	l.subsMu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			close(sub.done)
			l.subsMu.Lock()
			defer l.subsMu.Unlock()
			for i, s := range l.subs {
				if s == sub {
					l.subs = append(l.subs[:i], l.subs[i+1:]...)
					return
				}
			}
		})
	}
	return sub.ch, cancel
}

// Dropped sums drop counts across LIVE subscribers; it flickers down on cancel. Use TotalDropped for a process-lifetime counter.
func (l *Logger) Dropped() uint64 {
	l.subsMu.Lock()
	defer l.subsMu.Unlock()
	var total uint64
	for _, s := range l.subs {
		total += atomic.LoadUint64(&s.dropped)
	}
	return total
}

// TotalDropped is monotonic across the logger's lifetime, retaining drops from cancelled subscribers.
func (l *Logger) TotalDropped() uint64 {
	return atomic.LoadUint64(&l.totalDropped)
}

func (l *Logger) fanout(e Entry) {
	l.subsMu.Lock()
	subs := append([]*subscription(nil), l.subs...)
	l.subsMu.Unlock()
	for _, s := range subs {
		select {
		case <-s.done:
			continue
		case s.ch <- e:
		default:
			select {
			case <-s.done:
				continue
			case <-s.ch:
			default:
			}
			select {
			case <-s.done:
				continue
			case s.ch <- e:
			default:
			}
			atomic.AddUint64(&s.dropped, 1)
			atomic.AddUint64(&l.totalDropped, 1)
			if l.OnSubscriberDrop != nil {
				l.OnSubscriberDrop(s.name, 1)
			}
		}
	}
}
