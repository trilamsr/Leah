// Package supervisor implements Phase 4 design §9 — restart, circuit-breaker, RSS leak detect,
// and PSI eviction. Failed self attestation (internal/attest) blocks all restarts.
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/trilam/leah/internal/attest"
)

type ProcessHandle int64

type ProcessState string

const (
	Starting    ProcessState = "Starting"
	Running     ProcessState = "Running"
	Crashed     ProcessState = "Crashed"
	Restarting  ProcessState = "Restarting"
	Disabled    ProcessState = "Disabled"
	CircuitOpen ProcessState = "CircuitOpen"
)

type RestartPolicy int

const (
	CrashOnly RestartPolicy = iota
	Always
	Never
)

type HealthCheck struct {
	Probe  func(context.Context) error
	Period time.Duration
}

type ResourceLimits struct {
	RSSmb, FDs, CPUPct int
}

type ProcessSpec struct {
	Name         string
	Runner       Runner
	Args, Env    []string
	StartTimeout time.Duration
	StopTimeout  time.Duration
	Restart      RestartPolicy
	BackoffMin   time.Duration
	BackoffMax   time.Duration
	Circuit      CircuitPolicy
	Health       HealthCheck
	Limits       ResourceLimits
}

type ProcessStatus struct {
	Name            string
	State           ProcessState
	RSSmb           int
	Restarts24h     int
	LastCrashReason string
}

// SupervisorEvent feeds T18 Health card + supervisor_event; Kind ∈ §9.6 {start|stop|crash|restart|leak|circuit_open|circuit_close|disabled}.
type SupervisorEvent struct {
	At      time.Time
	Process string
	Kind    string
	PID     int
	Code    int
	Reason  string
}

// Runner is the contract every supervised subsystem implements; subprocess-fork plugs in behind it later.
type Runner interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	RSSmb() int
}

type Config struct {
	Clock    func() time.Time
	Verifier attest.Verifier
	Leak     leakConfig
}

type Supervisor struct {
	cfg Config

	mu      sync.Mutex
	nextID  int64
	procs   map[ProcessHandle]*procState
	subs    []chan SupervisorEvent
	leak    *leakDetector
	closed  bool
	pending []pendingRestart
}

type procState struct {
	spec      ProcessSpec
	runner    Runner
	state     ProcessState
	backoff   *backoff
	circuit   *circuit
	restarts  []time.Time
	lastCrash string
}

type pendingRestart struct {
	h    ProcessHandle
	when time.Time
}

func New(cfg Config) *Supervisor {
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Leak == (leakConfig{}) {
		cfg.Leak = leakConfig{
			WarnSlopeMBPerMin:  5,
			WarnSustain:        10 * time.Minute,
			EvictSlopeMBPerMin: 20,
			EvictSustain:       5 * time.Minute,
			SampleWindow:       10 * time.Minute,
		}
	}
	return &Supervisor{
		cfg:   cfg,
		procs: map[ProcessHandle]*procState{},
		leak:  newLeakDetector(cfg.Leak),
	}
}

func (s *Supervisor) Register(spec ProcessSpec) ProcessHandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	h := ProcessHandle(s.nextID)
	s.procs[h] = &procState{
		spec:    spec,
		runner:  spec.Runner,
		state:   Starting,
		backoff: newBackoff(orDefault(spec.BackoffMin, 200*time.Millisecond), orDefault(spec.BackoffMax, 30*time.Second)),
		circuit: newCircuit(spec.Circuit),
	}
	return h
}

func (s *Supervisor) Start(ctx context.Context, h ProcessHandle) error {
	s.mu.Lock()
	p, ok := s.procs[h]
	s.mu.Unlock()
	if !ok {
		return errors.New("supervisor: unknown handle")
	}
	if err := p.runner.Start(ctx); err != nil {
		s.emit(SupervisorEvent{Kind: "crash", Process: p.spec.Name, Reason: err.Error()})
		return err
	}
	s.mu.Lock()
	p.state = Running
	s.mu.Unlock()
	s.emit(SupervisorEvent{Kind: "start", Process: p.spec.Name})
	return nil
}

func (s *Supervisor) Stop(ctx context.Context, h ProcessHandle) error {
	s.mu.Lock()
	p, ok := s.procs[h]
	s.mu.Unlock()
	if !ok {
		return errors.New("supervisor: unknown handle")
	}
	if err := p.runner.Stop(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	p.state = Disabled
	s.mu.Unlock()
	s.emit(SupervisorEvent{Kind: "stop", Process: p.spec.Name})
	return nil
}

func (s *Supervisor) Restart(ctx context.Context, h ProcessHandle) error {
	if err := s.Stop(ctx, h); err != nil {
		return err
	}
	if err := s.Start(ctx, h); err != nil {
		return err
	}
	s.mu.Lock()
	if p, ok := s.procs[h]; ok {
		p.backoff.reset()
		p.circuit.reset()
	}
	s.mu.Unlock()
	return nil
}

func (s *Supervisor) Status() []ProcessStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ProcessStatus, 0, len(s.procs))
	now := s.cfg.Clock()
	cutoff := now.Add(-24 * time.Hour)
	for _, p := range s.procs {
		n := 0
		for _, t := range p.restarts {
			if t.After(cutoff) {
				n++
			}
		}
		out = append(out, ProcessStatus{
			Name:            p.spec.Name,
			State:           p.state,
			RSSmb:           p.runner.RSSmb(),
			Restarts24h:     n,
			LastCrashReason: p.lastCrash,
		})
	}
	return out
}

func (s *Supervisor) Subscribe() <-chan SupervisorEvent {
	ch := make(chan SupervisorEvent, 32)
	s.mu.Lock()
	s.subs = append(s.subs, ch)
	s.mu.Unlock()
	return ch
}

// Close stops new emissions; subscribers see no further events. Channels are not closed because
// emit() runs concurrently — closing here would race-panic; the channels age out via GC once
// callers stop referencing them.
func (s *Supervisor) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.subs = nil
	return nil
}

// notifyCrash schedules a restart per policy; becomes the SIGCHLD path when subprocess support lands.
func (s *Supervisor) notifyCrash(h ProcessHandle, cause error) {
	now := s.cfg.Clock()
	var events []SupervisorEvent
	s.mu.Lock()
	p, ok := s.procs[h]
	if !ok {
		s.mu.Unlock()
		return
	}
	reason := ""
	if cause != nil {
		reason = cause.Error()
	}
	p.state = Crashed
	p.lastCrash = reason
	p.circuit.recordCrash(now)
	events = append(events, SupervisorEvent{At: now, Kind: "crash", Process: p.spec.Name, Reason: reason})

	switch {
	case p.circuit.isOpen(now):
		p.state = CircuitOpen
		events = append(events, SupervisorEvent{At: now, Kind: "circuit_open", Process: p.spec.Name})
	case p.spec.Restart == Never:
		p.state = Disabled
		events = append(events, SupervisorEvent{At: now, Kind: "disabled", Process: p.spec.Name, Reason: "restart=Never"})
	default:
		delay := p.backoff.next()
		s.pending = append(s.pending, pendingRestart{h: h, when: now.Add(delay)})
		p.state = Restarting
	}
	s.mu.Unlock()
	for _, e := range events {
		s.emit(e)
	}
}

// tickRestart fires pending restarts past their backoff; daemon main calls it on a 50 ms tick.
func (s *Supervisor) tickRestart() {
	now := s.cfg.Clock()
	s.mu.Lock()
	due := s.pending[:0]
	skip := make([]pendingRestart, 0, len(s.pending))
	for _, pr := range s.pending {
		if !pr.when.After(now) {
			due = append(due, pr)
		} else {
			skip = append(skip, pr)
		}
	}
	s.pending = skip
	s.mu.Unlock()

	for _, pr := range due {
		s.attemptRestart(pr.h)
	}
}

func (s *Supervisor) attemptRestart(h ProcessHandle) {
	if s.cfg.Verifier != nil {
		if v := s.cfg.Verifier.LastVerdict(); v.State == attest.Failed {
			s.mu.Lock()
			var ev SupervisorEvent
			if p, ok := s.procs[h]; ok {
				p.state = Disabled
				ev = SupervisorEvent{At: s.cfg.Clock(), Kind: "disabled", Process: p.spec.Name, Reason: "attest=Failed"}
			}
			s.mu.Unlock()
			if ev.Kind != "" {
				s.emit(ev)
			}
			return
		}
	}
	s.mu.Lock()
	p, ok := s.procs[h]
	s.mu.Unlock()
	if !ok {
		return
	}
	ctx := context.Background()
	if p.spec.StartTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.spec.StartTimeout)
		defer cancel()
	}
	if err := p.runner.Start(ctx); err != nil {
		s.emit(SupervisorEvent{At: s.cfg.Clock(), Kind: "crash", Process: p.spec.Name, Reason: err.Error()})
		s.notifyCrash(h, err)
		return
	}
	s.mu.Lock()
	p.state = Running
	p.restarts = append(p.restarts, s.cfg.Clock())
	s.mu.Unlock()
	s.emit(SupervisorEvent{At: s.cfg.Clock(), Kind: "restart", Process: p.spec.Name})
}

func (s *Supervisor) emit(e SupervisorEvent) {
	if e.At.IsZero() {
		e.At = s.cfg.Clock()
	}
	s.mu.Lock()
	subs := s.subs
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// SampleRSS feeds the §9.4 leak detector; >5 MB/min → warn, >20 MB/min → preemptive restart.
func (s *Supervisor) SampleRSS(h ProcessHandle, rssMB int) {
	s.mu.Lock()
	p, ok := s.procs[h]
	s.mu.Unlock()
	if !ok {
		return
	}
	now := s.cfg.Clock()
	s.leak.observe(p.spec.Name, rssMB, now)
	v := s.leak.classify(p.spec.Name, now)
	switch v {
	case leakWarn:
		s.emit(SupervisorEvent{At: now, Kind: "leak", Process: p.spec.Name, Reason: "slope>5MB/min"})
	case leakEvict:
		s.emit(SupervisorEvent{At: now, Kind: "leak", Process: p.spec.Name, Reason: "slope>20MB/min"})
		s.notifyCrash(h, fmt.Errorf("preemptive restart: leak slope>20MB/min"))
	}
}

func orDefault(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}
