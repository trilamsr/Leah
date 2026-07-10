package onboarding_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/platform/onboarding"
)

// TestMarkInstalled_Idempotent — second call must NOT overwrite first timestamp; A9 SLA anchor cannot reset.
func TestMarkInstalled_Idempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	first := time.Unix(1_700_000_000, 0).UTC()
	second := time.Unix(1_700_000_999, 0).UTC()

	if err := onboarding.MarkInstalled(dir, first); err != nil {
		t.Fatalf("first MarkInstalled: %v", err)
	}
	if err := onboarding.MarkInstalled(dir, second); err != nil {
		t.Fatalf("second MarkInstalled: %v", err)
	}
	got, ok, err := onboarding.InstalledAt(dir)
	if err != nil || !ok {
		t.Fatalf("InstalledAt: ok=%v err=%v", ok, err)
	}
	if !got.Equal(first) {
		t.Fatalf("marker drifted: got %v want %v", got, first)
	}
}

// TestInstalledAt_AbsentBeforeMark — fresh stateDir reports no marker.
func TestInstalledAt_AbsentBeforeMark(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, ok, err := onboarding.InstalledAt(dir); err != nil || ok {
		t.Fatalf("expected ok=false err=nil, got ok=%v err=%v", ok, err)
	}
}

// TestMarkInstalled_RoundTrip — written-at round-trips at second precision.
func TestMarkInstalled_RoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Unix(1_700_123_456, 0).UTC()
	if err := onboarding.MarkInstalled(dir, now); err != nil {
		t.Fatalf("MarkInstalled: %v", err)
	}
	got, ok, err := onboarding.InstalledAt(dir)
	if err != nil || !ok {
		t.Fatalf("InstalledAt: ok=%v err=%v", ok, err)
	}
	if !got.Equal(now) {
		t.Fatalf("round-trip drift: got %v want %v", got, now)
	}
}

// TestMarkInstalled_CreatesOnboardingDir — onboarding/ subdir auto-created under stateDir.
func TestMarkInstalled_CreatesOnboardingDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := onboarding.MarkInstalled(dir, time.Now()); err != nil {
		t.Fatalf("MarkInstalled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "onboarding", "installed_at")); err != nil {
		t.Fatalf("installed_at not written: %v", err)
	}
}
