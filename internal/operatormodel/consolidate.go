package operatormodel

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/memory"
)

// ConsolidatePass is the W125 nightly summary pass. Stability-gated
// (class, key, slot) cells collapse into operator_profile_consolidated;
// raw audit rows older than the age floor migrate to ArchivePath; the
// live audit.jsonl is rewritten in place via tmp+rename under the
// audit.Logger quiesce contract so concurrent Append calls cannot
// orphan rows on the pre-rename inode (spec §4 step 7).
type ConsolidatePass struct {
	Now         func() time.Time
	Store       *memory.Store
	AuditPath   string
	ArchivePath string
	// Audit, when non-nil, is held under QuiesceForConsolidation during the
	// audit.jsonl rewrite. Tests + cold-start paths pass nil — the rename
	// still happens, but without the live-Append cross-process guard.
	Audit *audit.Logger
	// TZ defaults to time.Local; slot derivation honors operator-laptop tz
	// the same way ObserveTimeOfDay does.
	TZ *time.Location
}

// DefaultConsolidationAgeDays is the §2-rule-3 age floor. Cells whose
// first_seen is more recent than now-AgeDays are skipped.
const DefaultConsolidationAgeDays = 14

// DefaultStabilityDelta is the §2-rule-2 weight delta gate.
const DefaultStabilityDelta = 0.05

// Run executes one consolidation pass. Returns nil when LEAH_CONSOLIDATION=0
// disables the pass at task-fire time.
func (cp *ConsolidatePass) Run(ctx context.Context) error {
	if os.Getenv("LEAH_CONSOLIDATION") == "0" {
		return nil
	}
	now := cp.now()
	ageDays := consolidationAgeDays()
	cutoff := now.Add(-time.Duration(ageDays) * 24 * time.Hour)
	tz := cp.tz()

	rows, err := readAudit(cp.AuditPath, time.Time{})
	if err != nil {
		return fmt.Errorf("read audit: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	cells := groupCells(rows, tz)
	stabilityDelta := stabilityDelta()
	halflife := halflifeDays()

	var consolidated []consolidatedCell
	for _, c := range cells {
		var oldTimes []time.Time
		var firstSeen time.Time
		for _, ts := range c.times {
			if firstSeen.IsZero() || ts.Before(firstSeen) {
				firstSeen = ts
			}
			if ts.Before(cutoff) {
				oldTimes = append(oldTimes, ts)
			}
		}
		if len(oldTimes) == 0 {
			continue
		}
		if firstSeen.After(cutoff) {
			continue
		}
		wNow := decayedWeight(c.times, now, halflife)
		anchor, hasAnchor, err := loadSnapshot(ctx, cp.Store.DB(), c.class, c.key, c.slot)
		if err != nil {
			return fmt.Errorf("load snapshot: %w", err)
		}
		if hasAnchor {
			denom := wNow
			if anchor > denom {
				denom = anchor
			}
			if denom < 1e-6 {
				denom = 1e-6
			}
			delta := wNow - anchor
			if delta < 0 {
				delta = -delta
			}
			if delta/denom >= stabilityDelta {
				// Volatile — record fresh snapshot so the next pass can
				// reconverge, but skip the consolidation write.
				if err := upsertSnapshot(ctx, cp.Store.DB(), c.class, c.key, c.slot, wNow, now); err != nil {
					return fmt.Errorf("upsert snapshot: %w", err)
				}
				continue
			}
		}
		weight := decayedWeight(oldTimes, cutoff, halflife)
		consolidated = append(consolidated, consolidatedCell{
			cell:      c,
			weight:    weight,
			count:     len(oldTimes),
			firstSeen: firstSeen,
			oldTimes:  oldTimes,
			wNow:      wNow,
		})
	}

	if len(consolidated) == 0 {
		return nil
	}

	tx, err := cp.Store.DB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, cc := range consolidated {
		if err := upsertConsolidatedTx(ctx, tx, cc, now); err != nil {
			return err
		}
		if err := upsertSnapshotTx(ctx, tx, cc.cell.class, cc.cell.key, cc.cell.slot, cc.wNow, now); err != nil {
			return err
		}
	}
	if err := appendArchive(cp.ArchivePath, consolidated, rows, tz); err != nil {
		return fmt.Errorf("archive append: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	release := func() {}
	if cp.Audit != nil {
		release = cp.Audit.QuiesceForConsolidation()
	}
	defer release()
	if err := rewriteAudit(cp.AuditPath, rows, consolidated, tz); err != nil {
		return fmt.Errorf("rewrite audit: %w", err)
	}
	return nil
}

func (cp *ConsolidatePass) now() time.Time {
	if cp.Now != nil {
		return cp.Now()
	}
	return time.Now().UTC()
}

func (cp *ConsolidatePass) tz() *time.Location {
	if cp.TZ != nil {
		return cp.TZ
	}
	return time.Local
}

// cellAggregate groups raw audit rows by the three observation classes the
// W125 pass cares about. context_transition is folded in by W126 once the
// SwitchSource plumbing matches; W124/W125 cover the two slot-derivable
// classes that need no extra DB read.
type cellAggregate struct {
	class string
	key   string
	slot  string
	times []time.Time
}

func groupCells(rows []audit.Entry, tz *time.Location) []cellAggregate {
	type k struct{ class, key, slot string }
	out := map[k]*cellAggregate{}
	for _, r := range rows {
		ts, err := time.Parse(time.RFC3339, r.Timestamp)
		if err != nil {
			continue
		}
		tot := k{"time_of_day", r.Kind, fmt.Sprintf("%02d", ts.In(tz).Hour())}
		c := out[tot]
		if c == nil {
			c = &cellAggregate{class: tot.class, key: tot.key, slot: tot.slot}
			out[tot] = c
		}
		c.times = append(c.times, ts)

		cad := k{"cadence", r.Kind, ts.In(tz).Weekday().String()[:3]}
		c2 := out[cad]
		if c2 == nil {
			c2 = &cellAggregate{class: cad.class, key: cad.key, slot: cad.slot}
			out[cad] = c2
		}
		c2.times = append(c2.times, ts)
	}
	res := make([]cellAggregate, 0, len(out))
	for _, v := range out {
		res = append(res, *v)
	}
	return res
}

// rowMatches reports whether an audit row belongs to a consolidated cell —
// the §3.4 slot-derivation contract.
func rowMatches(r audit.Entry, c cellAggregate, tz *time.Location) bool {
	if r.Kind != c.key {
		return false
	}
	ts, err := time.Parse(time.RFC3339, r.Timestamp)
	if err != nil {
		return false
	}
	switch c.class {
	case "time_of_day":
		return fmt.Sprintf("%02d", ts.In(tz).Hour()) == c.slot
	case "cadence":
		return ts.In(tz).Weekday().String()[:3] == c.slot
	}
	return false
}

type consolidatedCell struct {
	cell      cellAggregate
	weight    float64
	count     int
	firstSeen time.Time
	oldTimes  []time.Time
	wNow      float64
}

func upsertConsolidatedTx(ctx context.Context, tx *sql.Tx, cc consolidatedCell, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO operator_profile_consolidated
			(class, key, slot, weight, count, first_seen_ts, last_consolidated_at, source_window_end)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(class, key, slot) DO UPDATE SET
			weight=excluded.weight,
			count=excluded.count,
			last_consolidated_at=excluded.last_consolidated_at,
			source_window_end=excluded.source_window_end`,
		cc.cell.class, cc.cell.key, cc.cell.slot, cc.weight, cc.count,
		cc.firstSeen.UTC().Format(time.RFC3339),
		now.UTC().Format(time.RFC3339),
		now.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upsert consolidated: %w", err)
	}
	return nil
}

func upsertSnapshotTx(ctx context.Context, tx *sql.Tx, class, key, slot string, weight float64, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO operator_profile_snapshot
			(class, key, slot, weight_at_snapshot, snapshot_ts)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(class, key, slot) DO UPDATE SET
			weight_at_snapshot=excluded.weight_at_snapshot,
			snapshot_ts=excluded.snapshot_ts`,
		class, key, slot, weight, now.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}
	return nil
}

func upsertSnapshot(ctx context.Context, db *sql.DB, class, key, slot string, weight float64, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO operator_profile_snapshot
			(class, key, slot, weight_at_snapshot, snapshot_ts)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(class, key, slot) DO UPDATE SET
			weight_at_snapshot=excluded.weight_at_snapshot,
			snapshot_ts=excluded.snapshot_ts`,
		class, key, slot, weight, now.UTC().Format(time.RFC3339),
	)
	return err
}

func loadSnapshot(ctx context.Context, db *sql.DB, class, key, slot string) (float64, bool, error) {
	var w float64
	err := db.QueryRowContext(ctx,
		`SELECT weight_at_snapshot FROM operator_profile_snapshot
		 WHERE class=? AND key=? AND slot=?`,
		class, key, slot,
	).Scan(&w)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return w, true, nil
}

// appendArchive durably writes every old-side raw row matching any
// consolidated cell to ArchivePath. fsync barrier per §4 step 4.
func appendArchive(path string, consolidated []consolidatedCell, rows []audit.Entry, tz *time.Location) error {
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = f.Close() }()
	w := bufio.NewWriter(f)
	for _, r := range rows {
		if !archiveRow(r, consolidated, tz) {
			continue
		}
		buf, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("marshal row: %w", err)
		}
		if _, err := w.Write(append(buf, '\n')); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("fsync: %w", err)
	}
	return nil
}

// archiveRow reports whether r matches any consolidated cell AND is older
// than the cell's age floor — duplicate matches across classes (time_of_day
// + cadence both match) still write one archive line because the dedupe
// happens at the audit-row identity layer.
func archiveRow(r audit.Entry, consolidated []consolidatedCell, tz *time.Location) bool {
	ts, err := time.Parse(time.RFC3339, r.Timestamp)
	if err != nil {
		return false
	}
	for _, cc := range consolidated {
		if !rowMatches(r, cc.cell, tz) {
			continue
		}
		for _, ot := range cc.oldTimes {
			if ot.Equal(ts) {
				return true
			}
		}
	}
	return false
}

// rewriteAudit replaces audit.jsonl with the subset of rows that did NOT
// migrate to the archive. tmp+rename keeps the live file atomic from a
// reader's POV; the audit.Logger quiesce on the caller side keeps writers
// from racing the swap.
func rewriteAudit(path string, rows []audit.Entry, consolidated []consolidatedCell, tz *time.Location) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open tmp: %w", err)
	}
	w := bufio.NewWriter(f)
	for _, r := range rows {
		if archiveRow(r, consolidated, tz) {
			continue
		}
		buf, err := json.Marshal(r)
		if err != nil {
			_ = f.Close()
			return fmt.Errorf("marshal: %w", err)
		}
		if _, err := w.Write(append(buf, '\n')); err != nil {
			_ = f.Close()
			return fmt.Errorf("write: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return fmt.Errorf("flush: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fsync tmp: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func consolidationAgeDays() int {
	if v := os.Getenv("LEAH_CONSOLIDATION_AGE_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return DefaultConsolidationAgeDays
}

func stabilityDelta() float64 {
	if v := os.Getenv("LEAH_CONSOLIDATION_STABILITY_DELTA"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			return n
		}
	}
	return DefaultStabilityDelta
}
