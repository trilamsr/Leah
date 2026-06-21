package selflearn

import "sync"

// OutcomeSink fans verdicts to an observer, dropping on overflow so Emit
// never blocks the resolver.
type OutcomeSink struct {
	ch   chan Outcome
	done chan struct{}
	wg   sync.WaitGroup
	once sync.Once
}

func NewOutcomeSink(buffer int, sink func(Outcome)) *OutcomeSink {
	if buffer < 1 {
		buffer = 1
	}
	s := &OutcomeSink{ch: make(chan Outcome, buffer), done: make(chan struct{})}
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
	select {
	case <-s.done:
	case s.ch <- o:
	default:
	}
}

func (s *OutcomeSink) Close() {
	s.once.Do(func() {
		close(s.done)
		close(s.ch)
	})
	s.wg.Wait()
}
