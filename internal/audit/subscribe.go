package audit

import (
	"strconv"
	"sync"
	"sync/atomic"
)

type subscription struct {
	name    string
	ch      chan Entry
	dropped uint64
}

// Subscribe returns a bounded push channel and unsubscribe func. On full,
// the oldest queued entry is dropped so newest delivery is preserved and
// the audit-write hot path never blocks on a slow consumer.
func (l *Logger) Subscribe(buf int) (<-chan Entry, func()) {
	if buf < 1 {
		buf = 1
	}
	l.subsMu.Lock()
	l.subSeq++
	sub := &subscription{
		name: "sub-" + strconv.FormatUint(l.subSeq, 10),
		ch:   make(chan Entry, buf),
	}
	l.subs = append(l.subs, sub)
	l.subsMu.Unlock()

	if l.OnSubscriberDrop != nil {
		l.OnSubscriberDrop(sub.name, 0)
	}

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			l.subsMu.Lock()
			defer l.subsMu.Unlock()
			for i, s := range l.subs {
				if s == sub {
					l.subs = append(l.subs[:i], l.subs[i+1:]...)
					close(s.ch)
					return
				}
			}
		})
	}
	return sub.ch, cancel
}

// Dropped is the cumulative drop count across live subscribers.
func (l *Logger) Dropped() uint64 {
	l.subsMu.Lock()
	defer l.subsMu.Unlock()
	var total uint64
	for _, s := range l.subs {
		total += atomic.LoadUint64(&s.dropped)
	}
	return total
}

func (l *Logger) fanout(e Entry) {
	l.subsMu.Lock()
	subs := l.subs
	l.subsMu.Unlock()
	for _, s := range subs {
		select {
		case s.ch <- e:
		default:
			select {
			case <-s.ch:
			default:
			}
			select {
			case s.ch <- e:
			default:
			}
			atomic.AddUint64(&s.dropped, 1)
			if l.OnSubscriberDrop != nil {
				l.OnSubscriberDrop(s.name, 1)
			}
		}
	}
}
