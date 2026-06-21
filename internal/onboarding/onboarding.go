// Package onboarding owns the first-launch install marker that anchors the
// MAY-A9 SLA (brew install → first useful reply ≤5 min). The marker lives at
// $stateDir/onboarding/installed_at and is written exactly once — any later
// write is a no-op so the elapsed-time span cannot silently reset.
//
// The package is deliberately tiny and dependency-free so cmd/leah (first
// launch) and cmd/leah ask (first reply) can import it without dragging
// daemon-side wiring.
package onboarding

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const markerRel = "onboarding/installed_at"

// MarkInstalled writes now as the install anchor iff no marker already exists.
// Idempotent on purpose: a re-run of `leah` (relaunch, crash recovery, second
// daemon) must not reset the A9 span. File body is the unix-seconds string —
// O_EXCL guarantees the first writer wins across racing daemons.
func MarkInstalled(stateDir string, now time.Time) error {
	if stateDir == "" {
		return errors.New("onboarding: stateDir required")
	}
	path := filepath.Join(stateDir, markerRel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(strconv.FormatInt(now.Unix(), 10))
	return err
}

// InstalledAt returns the recorded install timestamp. ok=false (err=nil) means
// the marker has not been written yet — callers treat that as "first launch".
func InstalledAt(stateDir string) (time.Time, bool, error) {
	if stateDir == "" {
		return time.Time{}, false, errors.New("onboarding: stateDir required")
	}
	raw, err := os.ReadFile(filepath.Join(stateDir, markerRel))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return time.Time{}, false, err
	}
	return time.Unix(sec, 0).UTC(), true, nil
}
