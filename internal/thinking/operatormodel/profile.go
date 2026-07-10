package operatormodel

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/platform/audit"
	"github.com/trilam/leah/internal/platform/activectx"
	"github.com/trilam/leah/internal/memory/store"
)

// SwitchSource yields the operator's context switches at-or-after a cutoff,
// ascending — the shape ObserveContextTransitions consumes. Satisfied by
// *ctxmgr.Manager; the interface lets tests inject canned switches without
// a real SQLite file.
//
// Optional: a nil SwitchSource on Profile keeps the 2/3 observation classes
// (time_of_day + cadence) running and degrades context_transition to zero —
// the path leah-daemon ran on before #10 wired this up.
type SwitchSource interface {
	Since(time.Time) ([]activectx.Switch, error)
}

// DefaultWindowDays bounds the audit slice read by Profile.Update.
// Configurable via env LEAH_OPERATOR_MODEL_WINDOW_DAYS.
const DefaultWindowDays = 30

// DefaultHalflifeDays controls per-event decay weighting (§3.3).
// 14d picked to match the personal-use 4-task-per-week cadence;
// recent observations dominate, fading habits lose recommendation
// power within ~2 weeks.
const DefaultHalflifeDays = 14.0

// ColdStartMinRows is the row-count gate (§3.5).
const ColdStartMinRows = 50

// ColdStartMinDays is the day-span gate (§3.5).
const ColdStartMinDays = 7

// ProfileRow is one persisted (class, key, slot) cell with decay-adjusted
// weight, suitable for sorting in Recommend.
type ProfileRow struct {
	Class       string
	Key         string
	Slot        string
	Count       int
	Weight      float64
	WindowStart time.Time
	WindowEnd   time.Time
}

// Profile is the in-memory snapshot rebuilt by Update from the audit log.
// Ready is the cold-start gate result (§3.5). When !Ready, Recommend
// returns no recommendations.
type Profile struct {
	Ready        bool
	RowsObserved int
	DaysObserved int
	WindowStart  time.Time
	WindowEnd    time.Time
	Rows         []ProfileRow

	// Now defaults to time.Now().UTC() — overridden in tests.
	Now func() time.Time
	// TZ defaults to time.Local — observers honor operator-laptop tz.
	TZ *time.Location
	// SwitchSource feeds ObserveContextTransitions; nil keeps the class silent.
	SwitchSource SwitchSource
	// Feedback drains ship-outcome observations from the selflearn edge into
	// the rebuild; nil keeps the ship_outcome class silent.
	Feedback *FeedbackObserver
}

func (p *Profile) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now().UTC()
}

func (p *Profile) tz() *time.Location {
	if p.TZ != nil {
		return p.TZ
	}
	return time.Local
}

// Update rebuilds the profile from audit.jsonl entries with ts >= since,
// computes the three observation classes, decays per-event weight, and
// replaces the operator_profile table contents in a single tx. Updates
// operator_profile_meta with row + day counts and timestamps.
//
// The store's DB handle is borrowed; Update does NOT close it.
func (p *Profile) Update(ctx context.Context, store *memory.Store, auditPath string, since time.Time) error {
	rows, err := readAudit(auditPath, since)
	if err != nil {
		return fmt.Errorf("read audit: %w", err)
	}
	now := p.now()
	tz := p.tz()
	window := windowDays()

	p.WindowStart = since
	p.WindowEnd = now
	p.RowsObserved = len(rows)
	p.DaysObserved = daySpan(rows)
	p.Ready = p.RowsObserved >= ColdStartMinRows && p.DaysObserved >= ColdStartMinDays

	var switches []activectx.Switch
	if p.SwitchSource != nil {
		s, err := p.SwitchSource.Since(since)
		if err != nil {
			return fmt.Errorf("ctx switches: %w", err)
		}
		switches = s
	}

	all := ObserveTimeOfDay(rows, tz)
	all = append(all, ObserveContextTransitions(rows, switches)...)
	all = append(all, ObserveCadence(rows, tz)...)
	if p.Feedback != nil {
		all = append(all, p.Feedback.Drain()...)
	}

	halflife := halflifeDays()
	out := make([]ProfileRow, 0, len(all))
	for _, o := range all {
		out = append(out, ProfileRow{
			Class:       o.Class,
			Key:         o.Key,
			Slot:        o.Slot,
			Count:       o.Count,
			Weight:      decayedWeight(o.Times, now, halflife),
			WindowStart: since,
			WindowEnd:   now,
		})
	}
	p.Rows = out

	return persist(ctx, store.DB(), out, now, since, p.RowsObserved, p.DaysObserved, window)
}

// readAudit streams auditPath and returns entries with Timestamp >= since.
// Malformed JSON lines are skipped (mirror patterns.Detect posture).
func readAudit(path string, since time.Time) ([]audit.Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []audit.Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var e audit.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		if ts.Before(since) {
			continue
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// daySpan returns the count of distinct calendar dates covered by rows
// (in UTC). Used by the cold-start day-floor gate.
func daySpan(rows []audit.Entry) int {
	if len(rows) == 0 {
		return 0
	}
	days := map[string]struct{}{}
	for _, r := range rows {
		ts, err := time.Parse(time.RFC3339, r.Timestamp)
		if err != nil {
			continue
		}
		days[ts.UTC().Format("2006-01-02")] = struct{}{}
	}
	return len(days)
}

// decayedWeight applies exponential decay per event: w = sum exp(-age/halflife).
// Older events contribute less to the weight; recent events dominate.
func decayedWeight(times []time.Time, now time.Time, halflifeDays float64) float64 {
	if halflifeDays <= 0 || len(times) == 0 {
		return float64(len(times))
	}
	var w float64
	for _, t := range times {
		ageDays := now.Sub(t).Hours() / 24.0
		if ageDays < 0 {
			ageDays = 0
		}
		w += math.Exp(-ageDays / halflifeDays)
	}
	return w
}

// persist replaces operator_profile rows + meta keys in a single tx.
// Rebuild-from-scratch contract: stale rows from prior runs are wiped.
func persist(
	ctx context.Context, db *sql.DB, rows []ProfileRow,
	now, since time.Time, rowsObserved, daysObserved, windowDays int,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM operator_profile WHERE class IN ('time_of_day','context_transition','cadence','ship_outcome')`); err != nil {
		return fmt.Errorf("wipe operator_profile: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO operator_profile(class, key, slot, count, weight, window_start, window_end, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	winStart := since.UTC().Format(time.RFC3339)
	winEnd := now.UTC().Format(time.RFC3339)
	updated := winEnd
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx,
			r.Class, r.Key, r.Slot, r.Count, r.Weight, winStart, winEnd, updated); err != nil {
			return fmt.Errorf("insert row: %w", err)
		}
	}
	metaSets := []struct{ k, v string }{
		{"rows_observed", strconv.Itoa(rowsObserved)},
		{"days_observed", strconv.Itoa(daysObserved)},
		{"window_days", strconv.Itoa(windowDays)},
		{"last_full_rebuild_at", updated},
	}
	for _, m := range metaSets {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO operator_profile_meta(key, value) VALUES(?, ?)
			 ON CONFLICT(key) DO UPDATE SET value=excluded.value`, m.k, m.v); err != nil {
			return fmt.Errorf("upsert meta %s: %w", m.k, err)
		}
	}
	return tx.Commit()
}

func windowDays() int {
	if v := os.Getenv("LEAH_OPERATOR_MODEL_WINDOW_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return DefaultWindowDays
}

func halflifeDays() float64 {
	if v := os.Getenv("LEAH_OPERATOR_MODEL_HALFLIFE_DAYS"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			return n
		}
	}
	return DefaultHalflifeDays
}

// Load reads operator_profile + operator_profile_meta (populated by
// Update / UpdateProfile) and returns a Profile ready to feed Recommend.
// Symmetric with Update: write-side rebuilds, read-side hydrates.
//
// Missing tables (fresh install before the first daemon tick) are not
// an error — Load returns an empty, not-Ready Profile so callers can
// surface the cold-start message instead of a stack trace.
func Load(ctx context.Context, db *sql.DB) (Profile, error) {
	p := Profile{}
	meta := map[string]string{}
	mrows, err := db.QueryContext(ctx, `SELECT key, value FROM operator_profile_meta`)
	if err != nil {
		return p, nil
	}
	defer func() { _ = mrows.Close() }()
	for mrows.Next() {
		var k, v string
		if err := mrows.Scan(&k, &v); err != nil {
			return p, fmt.Errorf("scan meta: %w", err)
		}
		meta[k] = v
	}
	if n, err := strconv.Atoi(meta["rows_observed"]); err == nil {
		p.RowsObserved = n
	}
	if n, err := strconv.Atoi(meta["days_observed"]); err == nil {
		p.DaysObserved = n
	}
	p.Ready = p.RowsObserved >= ColdStartMinRows && p.DaysObserved >= ColdStartMinDays

	rrows, err := db.QueryContext(ctx, `
		SELECT class, key, slot, count, weight, window_start, window_end
		FROM operator_profile`)
	if err != nil {
		return p, nil
	}
	defer func() { _ = rrows.Close() }()
	for rrows.Next() {
		var r ProfileRow
		var ws, we string
		if err := rrows.Scan(&r.Class, &r.Key, &r.Slot, &r.Count, &r.Weight, &ws, &we); err != nil {
			return p, fmt.Errorf("scan row: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, ws); err == nil {
			r.WindowStart = t
		}
		if t, err := time.Parse(time.RFC3339, we); err == nil {
			r.WindowEnd = t
		}
		p.Rows = append(p.Rows, r)
	}
	return p, nil
}

// UpdateProfile is the daemon-task entrypoint. switches may be nil — the
// daemon caller is expected to pass a *ctxmgr.Manager so context_transition
// observations land in operator_profile (closes #10). feedback may be nil;
// when set, drained ship-outcome observations join the rebuild.
func UpdateProfile(ctx context.Context, store *memory.Store, auditPath string, switches SwitchSource, feedback *FeedbackObserver) error {
	since := time.Now().UTC().Add(-time.Duration(windowDays()) * 24 * time.Hour)
	p := &Profile{SwitchSource: switches, Feedback: feedback}
	return p.Update(ctx, store, auditPath, since)
}
