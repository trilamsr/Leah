package crdt

import (
	"context"
	"fmt"
	"time"
)

// GCTombstones deletes sync_tombstone rows older than cutoff. Spec §2.3 mandates a
// 90-day retention "after both peers have ack'd consumption" — the ack tracking
// lives in the coordinator's outbox/cursor; GC here trusts the cutoff its caller
// computes (typically time.Now().Add(-90*24*time.Hour) clamped by the lowest
// per-peer ack lamport recorded in sync_outbox.sent_at).
func (l *Log) GCTombstones(ctx context.Context, cutoff time.Time) (int, error) {
	tx, err := l.store.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`DELETE FROM sync_tombstone WHERE deleted_at < ?`, cutoff.Unix())
	if err != nil {
		return 0, fmt.Errorf("delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return int(n), nil
}
