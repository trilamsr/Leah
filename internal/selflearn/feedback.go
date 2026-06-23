package selflearn

import "sync"

// OutcomeSink fans verdicts to an observer, dropping on overflow so Emit
// never blocks the resolver.
type OutcomeSink struct {
	ch     chan Outcome
	mu     sync.RWMutex // RLock guards in-flight sends; Lock gates close
	closed bool
	wg     sync.WaitGroup
	once   sync.Once
}

func NewOutcomeSink(buffer int, sink func(Outcome)) *OutcomeSink {
	if buffer < 1 {
		buffer = 1
	}
	s := &OutcomeSink{ch: make(chan Outcome, buffer)}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for o := range s.ch {
			sink(o)
		}
	}()
	return s
}

func (s *OutcomeSink) Emit(o Outcome) {
	// RLock lets multiple emitters proceed in parallel; Close's Lock
	// waits for all in-flight sends before close(s.ch), which makes
	// send-to-closed-channel impossible.
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- o:
	default:
	}
}

func (s *OutcomeSink) Close() {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		close(s.ch)
		s.mu.Unlock()
	})
	s.wg.Wait()
}
