package mirror

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// MacApp is the loop-side subset of the per-app adapter contract from spec
// §6 — the mirror only schedules Sync; Query is per-app type-different and
// lives on each adapter directly. Lifting this to a shared spot is deferred
// until a Query consumer needs it (W31 leah-init wizard).
type MacApp interface {
	Name() string
	Available(ctx context.Context) bool
	Sync(ctx context.Context) error
}

// DefaultTickInterval is the spec §4 cadence — 60 s.
const DefaultTickInterval = 60 * time.Second

// ErrDuplicateApp is returned by Register when an app with the same Name
// is already registered. Names are the user-visible adapter handle (spec §6)
// so collisions are a bug, not a runtime condition to recover from.
var ErrDuplicateApp = errors.New("mirror: duplicate app")

// Status is the per-app sync record the daemon dashboard surfaces.
type Status struct {
	Apps map[string]AppStatus
}

type AppStatus struct {
	LastSync     time.Time
	LastErr      string
	ErrCount     int
	SuccessCount int
}

type Config struct {
	// TickInterval defaults to DefaultTickInterval when zero.
	TickInterval time.Duration
	// Out is the daemon's stdout sink (matches daemonloop.Loop.Out).
	Out io.Writer
	// Now is injected by tests; production leaves nil → time.Now.
	Now func() time.Time
}

type Mirror struct {
	tick   time.Duration
	out    io.Writer
	now    func() time.Time
	mu     sync.Mutex // guards apps + status
	apps   []MacApp
	status map[string]*AppStatus
}

func New(cfg Config) *Mirror {
	tick := cfg.TickInterval
	if tick <= 0 {
		tick = DefaultTickInterval
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	out := cfg.Out
	if out == nil {
		out = io.Discard
	}
	return &Mirror{
		tick:   tick,
		out:    out,
		now:    now,
		status: map[string]*AppStatus{},
	}
}

// Register adds app to the sync set. Duplicate Name is ErrDuplicateApp.
func (m *Mirror) Register(app MacApp) error {
	if app == nil {
		return errors.New("mirror: nil app")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	name := app.Name()
	if _, exists := m.status[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateApp, name)
	}
	m.apps = append(m.apps, app)
	m.status[name] = &AppStatus{}
	return nil
}

// Run blocks until ctx is canceled, calling syncAll on every tick. First
// sync fires immediately so callers do not wait a full interval to see
// liveness. Per-app failures are isolated — one bad adapter never halts
// the loop (spec §4 hammer-protection contract).
func (m *Mirror) Run(ctx context.Context) error {
	_, _ = fmt.Fprintf(m.out, "macos-mirror: starting; tick interval: %s\n", m.tick)
	m.syncAll(ctx)
	t := time.NewTicker(m.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintln(m.out, "macos-mirror: shutdown")
			return nil
		case <-t.C:
			m.syncAll(ctx)
		}
	}
}

func (m *Mirror) syncAll(ctx context.Context) {
	lg := obs.LoggerFromCtx(ctx).With("package", "macos/mirror")
	m.mu.Lock()
	apps := append([]MacApp(nil), m.apps...)
	m.mu.Unlock()
	for _, app := range apps {
		m.syncOne(ctx, lg, app)
	}
}

func (m *Mirror) syncOne(ctx context.Context, lg *slog.Logger, app MacApp) {
	name := app.Name()
	if !app.Available(ctx) {
		lg.Info("mirror.skip_unavailable", "app", name)
		return
	}
	// Per-app recover — a panicking adapter MUST NOT kill the loop or its
	// siblings. Spec §4 "per-app failure does not halt the loop" is the
	// adversarial-review hinge: tested via TestMirror_AppFails_LoopContinues.
	func() {
		defer func() {
			if r := recover(); r != nil {
				m.recordErr(name, fmt.Sprintf("panic: %v", r))
				lg.Error("mirror.sync_panic", "app", name, "panic", r)
			}
		}()
		err := app.Sync(ctx)
		if err != nil {
			m.recordErr(name, err.Error())
			lg.Error("mirror.sync_error", "app", name, "err", err)
			return
		}
		m.recordOK(name)
	}()
}

func (m *Mirror) recordOK(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status[name]
	s.LastSync = m.now()
	s.LastErr = ""
	s.SuccessCount++
}

func (m *Mirror) recordErr(name, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status[name]
	s.LastErr = msg
	s.ErrCount++
}

// Status snapshots the per-app sync record. Returned map is a copy —
// callers may mutate freely without racing the loop.
func (m *Mirror) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := Status{Apps: make(map[string]AppStatus, len(m.status))}
	for k, v := range m.status {
		out.Apps[k] = *v
	}
	return out
}
