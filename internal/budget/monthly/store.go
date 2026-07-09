// Package monthly persists the operator's monthly LLM spend to
// ~/.leah-state/cost-month.json so the spec §7 cost circuit breaker
// survives daemon restarts. One goroutine — the goroutine holding the
// Store mutex — owns the file; writes go via tmp+rename so a crash
// mid-update never leaves a torn JSON blob on disk.
package monthly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// monthFmt is the canonical year_month layout (`2006-01`); compared
// against time.Now().UTC() to detect rollover.
const monthFmt = "2006-01"

// operatorIDFromEnv reads LEAH_OPERATOR_ID (spec §6.1 reserves the field
// for multi-op); empty / unset falls back to "default".
func operatorIDFromEnv() string {
	if v := os.Getenv("LEAH_OPERATOR_ID"); v != "" {
		return v
	}
	return "default"
}

// state is the on-disk shape (spec §6.1). Fields tagged with the exact
// JSON keys the spec example uses so audit-replay tooling stays stable.
type state struct {
	YearMonth       string             `json:"year_month"`
	OperatorID      string             `json:"operator_id"`
	TotalsByKind    map[string]float64 `json:"totals_by_kind"`
	MonthCapDollars float64            `json:"month_cap_dollars"`
	LastChargeAt    time.Time          `json:"last_charge_at,omitempty"`
	RolloverAt      time.Time          `json:"rollover_at"`
}

// Store is the in-memory + file-backed monthly cost gauge. The mutex
// guards both the totals and the file write so concurrent Charges from
// different LLM call-sites cannot interleave.
type Store struct {
	mu   sync.Mutex
	path string
	st   state
}

// OpenAt loads (or creates) the state at path with cap dollars and the
// supplied "now" — taking now as a parameter keeps the rollover test
// deterministic without monkey-patching time.Now.
func OpenAt(path string, capDollars float64, now time.Time) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	s := &Store{path: path}
	if err := s.loadOrInit(capDollars, now); err != nil {
		return nil, err
	}
	return s, nil
}

// loadOrInit reads the on-disk state, archives the prior month on a YM
// mismatch (§6.3), and sidecars a corrupt file before starting fresh.
func (s *Store) loadOrInit(capDollars float64, now time.Time) error {
	currentYM := now.UTC().Format(monthFmt)
	b, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.st = freshState(currentYM, capDollars, now)
		return s.writeLocked()
	case err != nil:
		return fmt.Errorf("read state: %w", err)
	}
	var loaded state
	if jerr := json.Unmarshal(b, &loaded); jerr != nil {
		// Corrupt JSON: archive sidecar (forensics) and start fresh.
		sidecar := filepath.Join(filepath.Dir(s.path),
			fmt.Sprintf("cost-month-corrupt-%d.json", now.UnixNano()))
		_ = os.Rename(s.path, sidecar)
		s.st = freshState(currentYM, capDollars, now)
		return s.writeLocked()
	}
	if loaded.YearMonth != currentYM {
		if err := s.archive(loaded); err != nil {
			return err
		}
		s.st = freshState(currentYM, capDollars, now)
		return s.writeLocked()
	}
	// Resume; the spec lets operators bump the cap via env between
	// boots, so the caller's capDollars always wins.
	loaded.MonthCapDollars = capDollars
	s.st = loaded
	return nil
}

// freshState builds a zero-totals state for ym with rolloverAt at the
// first instant of the following month (UTC, per spec §15 P2 carve-out).
func freshState(ym string, capDollars float64, now time.Time) state {
	utc := now.UTC()
	next := time.Date(utc.Year(), utc.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return state{
		YearMonth:       ym,
		OperatorID:      operatorIDFromEnv(),
		TotalsByKind:    map[string]float64{},
		MonthCapDollars: capDollars,
		RolloverAt:      next,
	}
}

// archive moves prior into cost-month-archive/<year_month>.json so the
// `leah whoami --full` history surface (§6.3) has an append-only feed.
func (s *Store) archive(prior state) error {
	dir := filepath.Join(filepath.Dir(s.path), "cost-month-archive")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir archive: %w", err)
	}
	out := filepath.Join(dir, prior.YearMonth+".json")
	b, err := json.MarshalIndent(prior, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal archive: %w", err)
	}
	return os.WriteFile(out, b, 0o600)
}

// ErrNegativeCharge surfaces a negative-dollar Charge — refunds belong
// in `leah cost refund`, not the hot-path Charge call.
var ErrNegativeCharge = errors.New("cost-month: negative charge — use refund flow")

// Charge adds dollars to kind's bucket and flushes to disk. Zero is a
// silent no-op (cost-free LLM cache hits); negative returns ErrNegativeCharge.
func (s *Store) Charge(kind string, dollars float64) error {
	if dollars < 0 {
		return ErrNegativeCharge
	}
	if dollars == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.TotalsByKind == nil {
		s.st.TotalsByKind = map[string]float64{}
	}
	s.st.TotalsByKind[kind] += dollars
	s.st.LastChargeAt = time.Now().UTC()
	return s.writeLocked()
}

// Spent is the numerator of the breaker's spent/cap ratio.
func (s *Store) Spent() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sum float64
	for _, v := range s.st.TotalsByKind {
		sum += v
	}
	return sum
}

// Cap is the breaker ratio denominator; the breaker reads it instead of
// re-parsing LEAH_COST_MONTH_CAP from env on every State() call.
func (s *Store) Cap() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.MonthCapDollars
}

// YearMonth is exposed for the 1-minute rollover timer to compare against
// wall-clock UTC without re-locking the inner state.
func (s *Store) YearMonth() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st.YearMonth
}

// CheckRollover archives + resets the state when now's YM no longer
// matches the persisted YM. Called from a 1-minute timer in the daemon
// so daemon-idle midnight rolls within ≤60 s (§6.3).
func (s *Store) CheckRollover(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	currentYM := now.UTC().Format(monthFmt)
	if currentYM == s.st.YearMonth {
		return nil
	}
	if err := s.archive(s.st); err != nil {
		return err
	}
	s.st = freshState(currentYM, s.st.MonthCapDollars, now)
	return s.writeLocked()
}

// RunRolloverLoop ticks every interval, calling CheckRollover so a
// daemon-idle midnight rolls within ≤interval (spec §6.3 targets ≤60 s).
// Cancellation wins over a fired tick: ctx.Done in the same select frees
// the worst-case shutdown wait of one CheckRollover (single mutex-guarded
// archive + rewrite — bounded, but worth not blocking on at SIGTERM).
func (s *Store) RunRolloverLoop(ctx context.Context, interval time.Duration, onErr func(error)) {
	if interval <= 0 {
		interval = time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if err := s.CheckRollover(now); err != nil && onErr != nil {
				onErr(err)
			}
		}
	}
}

// writeLocked serializes s.st via write-temp-then-rename for crash
// atomicity. Caller MUST hold s.mu.
func (s *Store) writeLocked() error {
	b, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".cost-month-*.tmp")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, werr := tmp.Write(b); werr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write tmp: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close tmp: %w", cerr)
	}
	// CreateTemp honors umask, so a permissive umask can leak 0o644 on
	// the tmp file before the rename. Re-chmod 0o600 before the swap so
	// the operator's spend ledger is never world-readable on disk.
	if cherr := os.Chmod(tmpPath, 0o600); cherr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod tmp: %w", cherr)
	}
	if rerr := os.Rename(tmpPath, s.path); rerr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename tmp: %w", rerr)
	}
	return nil
}
