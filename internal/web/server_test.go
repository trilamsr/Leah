package web

import (
	"testing"

	"github.com/trilam/leah/internal/obs"
)

// TestEnsureBroadcaster_AdoptsExistingDefault guards the daemon→dashboard
// ownership invariant: BuildMux must not orphan a pre-set default broadcaster
// (else obs.EmitEvent fans out to a bus the dispatcher never subscribes).
func TestEnsureBroadcaster_AdoptsExistingDefault(t *testing.T) {
	want := telemetry.NewBroadcaster()
	telemetry.SetDefaultBroadcaster(want)
	t.Cleanup(func() { telemetry.SetDefaultBroadcaster(nil) })

	s := &Server{Addr: "127.0.0.1:0"}
	if _, err := s.BuildMux(); err != nil {
		t.Fatalf("BuildMux: %v", err)
	}
	if s.bcast != want {
		t.Fatalf("bus identity not preserved: got %p want %p", s.bcast, want)
	}
	if got := telemetry.DefaultBroadcaster(); got != want {
		t.Fatalf("default broadcaster mutated: got %p want %p", got, want)
	}
}

func TestEnsureBroadcaster_CreatesWhenUnset(t *testing.T) {
	telemetry.SetDefaultBroadcaster(nil)
	t.Cleanup(func() { telemetry.SetDefaultBroadcaster(nil) })

	s := &Server{Addr: "127.0.0.1:0"}
	if _, err := s.BuildMux(); err != nil {
		t.Fatalf("BuildMux: %v", err)
	}
	if s.bcast == nil {
		t.Fatal("expected fallback broadcaster")
	}
	if telemetry.DefaultBroadcaster() != s.bcast {
		t.Fatal("fallback should be installed as default")
	}
}
