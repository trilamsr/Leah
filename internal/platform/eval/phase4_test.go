package eval

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestPhase4Smoke_OfflineMode exercises every Phase-4 subsystem check in
// the offline runner. Offline mode short-circuits any subsystem that needs
// real network/iCloud/cgo so the test stays green in CI; what we verify
// here is that the orchestration walks all nine subsystem hooks, records
// a result per hook, and fails-fast on the first error.
func TestPhase4Smoke_OfflineMode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := RunPhase4Smoke(ctx, Phase4Options{Mode: Phase4ModeOffline})
	if err != nil {
		t.Fatalf("RunPhase4Smoke offline: %v", err)
	}
	want := []string{
		"voice-duplex",
		"vision-route",
		"learn-pass2",
		"budget-ladder",
		"sync-bonjour",
		"a2a-frame",
		"plugin-load",
		"dashboard-cards",
		"supervisor",
	}
	if len(res.Subsystems) != len(want) {
		t.Fatalf("subsystem count: got %d want %d (%+v)", len(res.Subsystems), len(want), res.Subsystems)
	}
	for i, sub := range res.Subsystems {
		if sub.Name != want[i] {
			t.Errorf("subsystem[%d] name: got %q want %q", i, sub.Name, want[i])
		}
		if sub.Status != Phase4StatusSkipOffline && sub.Status != Phase4StatusPass {
			t.Errorf("subsystem %s: status %q neither pass nor offline-skip", sub.Name, sub.Status)
		}
		if sub.Evidence == "" {
			t.Errorf("subsystem %s: empty evidence line — every subsystem MUST emit one expected line", sub.Name)
		}
	}
}

// TestPhase4Smoke_FailsFast confirms a single subsystem failure aborts
// the remaining hooks. Fail-fast is load-bearing — the operator should
// not have to spelunk a 9-section log to find the broken subsystem.
func TestPhase4Smoke_FailsFast(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := Phase4Options{
		Mode: Phase4ModeOffline,
		// Force voice-duplex (the first hook) to fail.
		ForceFail: map[string]string{"voice-duplex": "synthetic-failure"},
	}
	_, err := RunPhase4Smoke(ctx, opts)
	if err == nil {
		t.Fatalf("expected synthetic failure to propagate")
	}
	if !strings.Contains(err.Error(), "voice-duplex") {
		t.Errorf("error %q should name failing subsystem", err)
	}
	if !strings.Contains(err.Error(), "synthetic-failure") {
		t.Errorf("error %q should carry root cause", err)
	}
}
