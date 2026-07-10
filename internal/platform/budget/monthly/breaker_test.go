package monthly

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/reasoner"
)

// Below the 80 % warn threshold the breaker reports reasoner.StateOK; at or above
// it (and below 100 %) it reports reasoner.StateWarn — the spec §7.1 ratio table.
func TestBreaker_DegradeBelowCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cost-month.json")
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	s, err := OpenAt(path, 50.0, now)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	b := NewBreaker(s)

	if got := b.State(); got != reasoner.StateOK {
		t.Errorf("State zero spend = %d, want %d (reasoner.StateOK)", got, reasoner.StateOK)
	}

	// Pull spend to 79 % (under the 80 % warn floor) — still ok.
	if err := s.Charge("reasoner", 39.5); err != nil {
		t.Fatalf("Charge 39.5: %v", err)
	}
	if got := b.State(); got != reasoner.StateOK {
		t.Errorf("State at 0.79 = %d, want %d (reasoner.StateOK)", got, reasoner.StateOK)
	}

	// Cross the 80 % warn threshold — spec §7.1: 0.80–1.00 ⇒ warn.
	if err := s.Charge("reasoner", 1.0); err != nil {
		t.Fatalf("Charge 1.0: %v", err)
	}
	if got := b.State(); got != reasoner.StateWarn {
		t.Errorf("State at 0.81 = %d, want %d (reasoner.StateWarn)", got, reasoner.StateWarn)
	}
}

// At ≥100 % of cap, State() returns reasoner.StateDeny — spec §7.1.
func TestBreaker_DenyAtCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cost-month.json")
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	s, err := OpenAt(path, 50.0, now)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	b := NewBreaker(s)
	if err := s.Charge("reasoner", 50.0); err != nil {
		t.Fatalf("Charge 50.0: %v", err)
	}
	if got := b.State(); got != reasoner.StateDeny {
		t.Errorf("State at 1.0 = %d, want %d (reasoner.StateDeny)", got, reasoner.StateDeny)
	}
}

// Override lifts deny → ok for one hour, then re-imposes deny — spec §7.3.
func TestBreaker_OverrideAttestation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cost-month.json")
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	s, err := OpenAt(path, 50.0, now)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	b := NewBreaker(s)
	if err := s.Charge("reasoner", 50.0); err != nil {
		t.Fatalf("Charge 50.0: %v", err)
	}
	if got := b.State(); got != reasoner.StateDeny {
		t.Fatalf("pre-override = %d, want %d (reasoner.StateDeny)", got, reasoner.StateDeny)
	}

	b.GrantOverrideAt(now)
	if got := b.StateAt(now.Add(30 * time.Minute)); got != reasoner.StateOK {
		t.Errorf("within-TTL = %d, want %d (reasoner.StateOK)", got, reasoner.StateOK)
	}
	if got := b.StateAt(now.Add(time.Hour + time.Second)); got != reasoner.StateDeny {
		t.Errorf("post-TTL = %d, want %d (reasoner.StateDeny re-imposed)", got, reasoner.StateDeny)
	}
}
