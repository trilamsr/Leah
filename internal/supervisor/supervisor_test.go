package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/trilam/leah/internal/attest"
)

type stubVerifier struct{ v attest.Attestation }

func (s stubVerifier) VerifySelf(context.Context) (attest.Attestation, error) {
	return s.v, nil
}
func (s stubVerifier) VerifyArtifact(context.Context, string, attest.ManifestRef) (attest.Attestation, error) {
	return s.v, nil
}
func (s stubVerifier) VerifyPlugin(context.Context, string) (attest.Attestation, error) {
	return s.v, nil
}
func (s stubVerifier) LastVerdict() attest.Attestation      { return s.v }
func (s stubVerifier) Subscribe() <-chan attest.Attestation { return nil }

type fakeRunner struct {
	starts   int
	stops    int
	failNext bool
	rssMB    int
}

func (f *fakeRunner) Start(context.Context) error {
	f.starts++
	if f.failNext {
		f.failNext = false
		return errors.New("boom")
	}
	return nil
}
func (f *fakeRunner) Stop(context.Context) error { f.stops++; return nil }
func (f *fakeRunner) RSSmb() int                 { return f.rssMB }

func newTestSupervisor(t *testing.T, v attest.Verifier) (*Supervisor, *fakeClock) {
	t.Helper()
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	sv := New(Config{
		Clock:    clk.Now,
		Verifier: v,
	})
	t.Cleanup(func() { _ = sv.Close() })
	return sv, clk
}

func TestSupervisor_RegisterStartsProcess(t *testing.T) {
	sv, _ := newTestSupervisor(t, stubVerifier{attest.Attestation{State: attest.Verified}})
	r := &fakeRunner{}
	h := sv.Register(ProcessSpec{Name: "voice-stt"}, r)
	if err := sv.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if r.starts != 1 {
		t.Fatalf("starts = %d, want 1", r.starts)
	}
	s := sv.Status()
	if len(s) != 1 || s[0].State != Running {
		t.Fatalf("status: %+v", s)
	}
}

func TestSupervisor_RestartOnCrashEmitsEvent(t *testing.T) {
	sv, clk := newTestSupervisor(t, stubVerifier{attest.Attestation{State: attest.Verified}})
	r := &fakeRunner{}
	h := sv.Register(ProcessSpec{
		Name:       "voice-stt",
		Restart:    CrashOnly,
		BackoffMin: 200 * time.Millisecond,
		BackoffMax: 30 * time.Second,
	}, r)
	if err := sv.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	sub := sv.Subscribe()

	sv.notifyCrash(h, errors.New("segfault"))
	clk.advance(250 * time.Millisecond)
	sv.tickRestart()

	if r.starts != 2 {
		t.Fatalf("after crash + tick, starts = %d, want 2", r.starts)
	}
	if got := drainKinds(sub, 250*time.Millisecond); !hasKind(got, "crash") || !hasKind(got, "restart") {
		t.Fatalf("missing event kinds; got %v", got)
	}
}

func TestSupervisor_FailedAttestBlocksRestart(t *testing.T) {
	sv, clk := newTestSupervisor(t, stubVerifier{attest.Attestation{State: attest.Failed}})
	r := &fakeRunner{}
	h := sv.Register(ProcessSpec{
		Name:       "voice-stt",
		Restart:    CrashOnly,
		BackoffMin: 200 * time.Millisecond,
		BackoffMax: 30 * time.Second,
	}, r)
	if err := sv.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	sv.notifyCrash(h, errors.New("segfault"))
	clk.advance(time.Hour)
	sv.tickRestart()

	if r.starts != 1 {
		t.Fatalf("Failed attest must block restart; starts = %d", r.starts)
	}
}

func TestSupervisor_CircuitBreakerDisables(t *testing.T) {
	sv, clk := newTestSupervisor(t, stubVerifier{attest.Attestation{State: attest.Verified}})
	r := &fakeRunner{}
	h := sv.Register(ProcessSpec{
		Name:       "vision-live",
		Restart:    CrashOnly,
		BackoffMin: 10 * time.Millisecond,
		BackoffMax: time.Second,
		Circuit:    CircuitPolicy{MaxCrashes: 3, Window: time.Minute},
	}, r)
	_ = sv.Start(context.Background(), h)

	for i := 0; i < 3; i++ {
		sv.notifyCrash(h, errors.New("x"))
		clk.advance(50 * time.Millisecond)
		sv.tickRestart()
	}
	sv.notifyCrash(h, errors.New("x"))
	clk.advance(time.Second)
	sv.tickRestart()

	st := sv.Status()
	if st[0].State != CircuitOpen && st[0].State != Disabled {
		t.Fatalf("circuit-breaker should disable subsystem; state = %s", st[0].State)
	}
}

func TestSupervisor_NeverPolicyDoesNotRestart(t *testing.T) {
	sv, clk := newTestSupervisor(t, stubVerifier{attest.Attestation{State: attest.Verified}})
	r := &fakeRunner{}
	h := sv.Register(ProcessSpec{Name: "p", Restart: Never, BackoffMin: time.Millisecond, BackoffMax: time.Second}, r)
	_ = sv.Start(context.Background(), h)
	sv.notifyCrash(h, errors.New("die"))
	clk.advance(time.Hour)
	sv.tickRestart()
	if r.starts != 1 {
		t.Fatalf("Never policy must not restart; got %d", r.starts)
	}
}

func hasKind(events []string, k string) bool {
	for _, e := range events {
		if e == k {
			return true
		}
	}
	return false
}

func drainKinds(ch <-chan SupervisorEvent, budget time.Duration) []string {
	var out []string
	deadline := time.After(budget)
	for {
		select {
		case e := <-ch:
			out = append(out, e.Kind)
		case <-deadline:
			return out
		}
	}
}
