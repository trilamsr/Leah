package mirror

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trilam/leah/internal/testutil"
)

type fakeApp struct {
	name        string
	available   bool
	syncCalls   atomic.Int32
	syncErr     error
	syncPanic   any
	beforeSync  func() // hook for synchronization in tests
}

func (f *fakeApp) Name() string                       { return f.name }
func (f *fakeApp) Available(_ context.Context) bool   { return f.available }
func (f *fakeApp) Sync(_ context.Context) error {
	if f.beforeSync != nil {
		f.beforeSync()
	}
	f.syncCalls.Add(1)
	if f.syncPanic != nil {
		panic(f.syncPanic)
	}
	return f.syncErr
}

func TestMirror_TickIntervalElapsed_SyncsApps(t *testing.T) {
	t.Parallel()
	a := &fakeApp{name: "alpha", available: true}
	b := &fakeApp{name: "beta", available: true}
	m := New(Config{TickInterval: 5 * time.Millisecond})
	if err := m.Register(a); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := m.Register(b); err != nil {
		t.Fatalf("register b: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = m.Run(ctx)
		close(done)
	}()
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		return a.syncCalls.Load() >= 2 && b.syncCalls.Load() >= 2
	})
	cancel()
	<-done
}

func TestMirror_AppFails_LoopContinues(t *testing.T) {
	t.Parallel()
	bad := &fakeApp{name: "bad", available: true, syncErr: errors.New("kaboom")}
	good := &fakeApp{name: "good", available: true}
	panicker := &fakeApp{name: "panicker", available: true, syncPanic: "boom"}
	m := New(Config{TickInterval: 5 * time.Millisecond, Out: &bytes.Buffer{}})
	for _, a := range []MacApp{bad, good, panicker} {
		if err := m.Register(a); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()
	testutil.Eventually(t, 2*time.Second, 5*time.Millisecond, func() bool {
		return bad.syncCalls.Load() >= 2 &&
			good.syncCalls.Load() >= 2 &&
			panicker.syncCalls.Load() >= 2
	})
	st := m.Status()
	if st.Apps["bad"].LastErr == "" {
		t.Errorf("bad app: expected LastErr, got empty")
	}
	if st.Apps["panicker"].LastErr == "" {
		t.Errorf("panicker app: expected LastErr, got empty")
	}
	if st.Apps["good"].LastErr != "" {
		t.Errorf("good app: unexpected LastErr=%q", st.Apps["good"].LastErr)
	}
	if st.Apps["good"].SuccessCount < 2 {
		t.Errorf("good app: SuccessCount=%d, want >= 2", st.Apps["good"].SuccessCount)
	}
}

func TestMirror_CtxCancel_StopsLoop(t *testing.T) {
	t.Parallel()
	a := &fakeApp{name: "a", available: true}
	m := New(Config{TickInterval: 5 * time.Millisecond})
	if err := m.Register(a); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx) }()
	testutil.Eventually(t, time.Second, 2*time.Millisecond, func() bool {
		return a.syncCalls.Load() >= 1
	})
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned err: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

func TestMirror_Status_TracksLastSyncPerApp(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	a := &fakeApp{name: "a", available: true}
	b := &fakeApp{name: "b", available: true, syncErr: errors.New("nope")}
	m := New(Config{
		TickInterval: 5 * time.Millisecond,
		Now:          func() time.Time { return clock },
	})
	if err := m.Register(a); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(b); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = m.Run(ctx) }()
	testutil.Eventually(t, time.Second, 2*time.Millisecond, func() bool {
		st := m.Status()
		return st.Apps["a"].SuccessCount >= 1 && st.Apps["b"].ErrCount >= 1
	})
	st := m.Status()
	if !st.Apps["a"].LastSync.Equal(clock) {
		t.Errorf("a.LastSync=%v, want %v", st.Apps["a"].LastSync, clock)
	}
	if !st.Apps["b"].LastSync.IsZero() {
		t.Errorf("b.LastSync=%v, want zero (only success updates LastSync)", st.Apps["b"].LastSync)
	}
}

func TestMirror_Register_RejectsDuplicate(t *testing.T) {
	t.Parallel()
	m := New(Config{})
	a1 := &fakeApp{name: "dup", available: true}
	a2 := &fakeApp{name: "dup", available: true}
	if err := m.Register(a1); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := m.Register(a2)
	if !errors.Is(err, ErrDuplicateApp) {
		t.Fatalf("second register err=%v, want ErrDuplicateApp", err)
	}
}

func TestMirror_Unavailable_SkipsWithoutSync(t *testing.T) {
	t.Parallel()
	a := &fakeApp{name: "off", available: false}
	m := New(Config{TickInterval: 5 * time.Millisecond})
	if err := m.Register(a); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = m.Run(ctx)
	if a.syncCalls.Load() != 0 {
		t.Fatalf("Sync called %d times on unavailable app; want 0", a.syncCalls.Load())
	}
	st := m.Status()
	if st.Apps["off"].ErrCount != 0 || st.Apps["off"].SuccessCount != 0 {
		t.Fatalf("unavailable app should not record success/err; got %+v", st.Apps["off"])
	}
}

func TestMirror_New_DefaultsTickInterval(t *testing.T) {
	t.Parallel()
	m := New(Config{})
	if m.tick != DefaultTickInterval {
		t.Fatalf("tick=%v, want %v", m.tick, DefaultTickInterval)
	}
}

func TestMirror_Register_NilApp(t *testing.T) {
	t.Parallel()
	m := New(Config{})
	if err := m.Register(nil); err == nil {
		t.Fatal("nil app: want err, got nil")
	}
}
