package crdt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/trilam/leah/internal/sync/discovery"
)

// LogEntry is one row on the add-only replicated log (§2.4.1).
type LogEntry struct {
	Table   string
	RowID   int64
	Node    discovery.DeviceID
	Lamport Lamport
	Op      Op
	Payload []byte
}

// DeltaStats summarizes one ApplyLog batch (§2.4.1).
type DeltaStats struct {
	Applied   int
	Skipped   int
	Conflicts int
}

// CRDT is the storage-bound replication surface (§2.4.1).
type CRDT interface {
	ApplyLog(ctx context.Context, entries []LogEntry) (DeltaStats, error)
	EmitLog(ctx context.Context, since Lamport, limit int) ([]LogEntry, error)
	GCTombstones(ctx context.Context, cutoff time.Time) (int, error)
}

// Store is the CRDT-bound surface of sqlstore.DB; an interface keeps tests off SQLite.
type Store interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Log persists LogEntry batches into sync_clock + sync_tombstone (§2.5)
// and provides idempotent replay against the (table_name, row_id, node_uuid) PK.
type Log struct {
	store Store
	self  discovery.DeviceID
}

// NewLog binds a Log to a sqlstore and the local DeviceID. The DeviceID is used
// as the deterministic tiebreaker in Resolve and as deleted_by on emitted tombstones.
func NewLog(store Store, self discovery.DeviceID) *Log {
	return &Log{store: store, self: self}
}

// ApplyLog is idempotent: re-applying the same (Table, RowID, Node, Lamport) leaves
// state unchanged (§2.3 "tombstones are honored idempotently"). Conflicts are counted
// when a remote write loses LWW resolution — they're not errors, just observability.
func (l *Log) ApplyLog(ctx context.Context, entries []LogEntry) (DeltaStats, error) {
	if len(entries) == 0 {
		return DeltaStats{}, nil
	}
	tx, err := l.store.BeginTx(ctx, nil)
	if err != nil {
		return DeltaStats{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stats := DeltaStats{}
	for _, e := range entries {
		if err := e.validate(); err != nil {
			return stats, fmt.Errorf("entry %s/%d: %w", e.Table, e.RowID, err)
		}
		applied, conflict, err := l.applyOne(ctx, tx, e)
		if err != nil {
			return stats, err
		}
		switch {
		case applied:
			stats.Applied++
		case conflict:
			stats.Conflicts++
		default:
			stats.Skipped++
		}
	}
	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("commit: %w", err)
	}
	return stats, nil
}

// applyOne upserts a single LogEntry. Returns (applied, conflict, err).
//   - applied: the row was newly recorded or replaced an older clock.
//   - conflict: an existing local write has a higher (lamport, device) — remote
//     loses LWW; the caller should NOT replay the payload (it's already stale).
//   - skipped (both false): idempotent — same (node, lamport) already on disk.
func (l *Log) applyOne(ctx context.Context, tx *sql.Tx, e LogEntry) (bool, bool, error) {
	var existing Lamport
	err := tx.QueryRowContext(ctx,
		`SELECT lamport FROM sync_clock WHERE table_name=? AND row_id=? AND node_uuid=?`,
		e.Table, e.RowID, string(e.Node)).Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Fresh row from this node — record clock + tombstone if op is delete.
	case err != nil:
		return false, false, fmt.Errorf("read clock: %w", err)
	default:
		if existing >= e.Lamport {
			return false, false, nil // idempotent or stale-from-same-node
		}
	}

	// Cross-node LWW: does another node hold a competing higher (lamport, device)?
	conflict, err := l.lwwConflict(ctx, tx, e)
	if err != nil {
		return false, false, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO sync_clock(table_name, row_id, node_uuid, lamport) VALUES(?,?,?,?)
		 ON CONFLICT(table_name, row_id, node_uuid) DO UPDATE SET lamport=excluded.lamport`,
		e.Table, e.RowID, string(e.Node), int64(e.Lamport)); err != nil {
		return false, false, fmt.Errorf("upsert clock: %w", err)
	}

	if e.Op == OpDelete {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sync_tombstone(table_name, row_id, deleted_at, deleted_by) VALUES(?,?,?,?)
			 ON CONFLICT(table_name, row_id) DO NOTHING`,
			e.Table, e.RowID, time.Now().UTC().Unix(), string(e.Node)); err != nil {
			return false, false, fmt.Errorf("upsert tombstone: %w", err)
		}
	}
	return !conflict, conflict, nil
}

// lwwConflict checks whether ANY OTHER node has a clock that would beat e on
// (lamport DESC, device lex ASC). The check uses Resolve so the tiebreaker rule
// stays in one place.
func (l *Log) lwwConflict(ctx context.Context, tx *sql.Tx, e LogEntry) (bool, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT node_uuid, lamport FROM sync_clock WHERE table_name=? AND row_id=? AND node_uuid<>?`,
		e.Table, e.RowID, string(e.Node))
	if err != nil {
		return false, fmt.Errorf("scan clocks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	incoming := LWWValue{Payload: e.Payload, Lamport: e.Lamport, Device: e.Node}
	for rows.Next() {
		var nodeStr string
		var lam int64
		if err := rows.Scan(&nodeStr, &lam); err != nil {
			return false, fmt.Errorf("scan row: %w", err)
		}
		competitor := LWWValue{Payload: []byte{0}, Lamport: Lamport(lam), Device: discovery.DeviceID(nodeStr)}
		if winner := Resolve(competitor, incoming); winner.Device != e.Node {
			return true, rows.Err()
		}
	}
	return false, rows.Err()
}

// EmitLog returns up to limit entries with Lamport strictly greater than since,
// ordered by (lamport ASC, node_uuid ASC) so a peer can resume catch-up
// deterministically (§2.4.1 EmitLog).
//
// Payload is left nil — the caller (coordinator) materializes payloads from the
// underlying table via Table/RowID. Tombstones surface with Op = OpDelete and
// no payload by design.
func (l *Log) EmitLog(ctx context.Context, since Lamport, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := l.store.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin ro: %w", err)
	}
	defer func() { _ = rows.Rollback() }()
	q := `SELECT table_name, row_id, node_uuid, lamport FROM sync_clock WHERE lamport > ? ORDER BY lamport ASC, node_uuid ASC LIMIT ?`
	cur, err := rows.QueryContext(ctx, q, int64(since), limit)
	if err != nil {
		return nil, fmt.Errorf("query clocks: %w", err)
	}
	defer func() { _ = cur.Close() }()
	var out []LogEntry
	for cur.Next() {
		var e LogEntry
		var node string
		var lam int64
		if err := cur.Scan(&e.Table, &e.RowID, &node, &lam); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		e.Node = discovery.DeviceID(node)
		e.Lamport = Lamport(lam)
		e.Op = OpUpdate // default; tombstone lookup below promotes to OpDelete
		out = append(out, e)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	if err := l.markTombstones(ctx, rows, out); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Lamport != out[j].Lamport {
			return out[i].Lamport < out[j].Lamport
		}
		return out[i].Node < out[j].Node
	})
	return out, nil
}

// markTombstones flips Op to OpDelete for entries that have a sync_tombstone row.
func (l *Log) markTombstones(ctx context.Context, tx *sql.Tx, entries []LogEntry) error {
	for i := range entries {
		var deletedAt int64
		err := tx.QueryRowContext(ctx,
			`SELECT deleted_at FROM sync_tombstone WHERE table_name=? AND row_id=?`,
			entries[i].Table, entries[i].RowID).Scan(&deletedAt)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("tombstone lookup: %w", err)
		}
		entries[i].Op = OpDelete
	}
	return nil
}

func (e LogEntry) validate() error {
	if e.Table == "" {
		return errors.New("empty table")
	}
	if e.Node == "" {
		return errors.New("empty node")
	}
	switch e.Op {
	case OpInsert, OpUpdate, OpDelete:
	default:
		return fmt.Errorf("unknown op %q", e.Op)
	}
	return nil
}
