// Package coord is the multi-device sync coordinator (phase4 §2.4.1).
package coord

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/trilam/leah/internal/sync/crdt"
)

// OutboxMaxBytes caps the on-disk outbox at 50 MB (§2.7 "Outbox > 50 MB"). Once the
// estimate exceeds this, Enqueue truncates the oldest ack'd payloads then warns.
const OutboxMaxBytes int64 = 50 * 1024 * 1024

// Outbox persists gzipped CRDT delta batches per peer into sync_outbox (§2.5).
type Outbox struct {
	store crdt.Store
}

// NewOutbox binds an Outbox to the same Store the CRDT log uses, so enqueue +
// apply land in a single SQLite-WAL writer pool with no cross-pool deadlock risk.
func NewOutbox(store crdt.Store) *Outbox { return &Outbox{store: store} }

// Enqueue gzips entries and writes one sync_outbox row for peer. The encoding is
// JSON inside gzip — payload-bytes-per-entry is small (lamport+ids+small payload),
// JSON keeps the wire schema introspectable from sqlite3 cli during debugging,
// and gzip's prefix-table reclaims most of the JSON overhead.
//
// If the total outbox size after this enqueue would exceed OutboxMaxBytes, the
// oldest already-sent (sent_at NOT NULL) rows are dropped until the projected
// size fits. Unsent rows are preserved — losing those would lose a delta forever.
func (o *Outbox) Enqueue(ctx context.Context, peer crdt.DeviceID, entries []crdt.LogEntry) error {
	if peer == "" {
		return errors.New("empty peer id")
	}
	if len(entries) == 0 {
		return nil
	}
	payload, err := gzipJSON(entries)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	tx, err := o.store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := truncateAckdToFit(ctx, tx, int64(len(payload))); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO sync_outbox(peer_id, payload, enqueued_at) VALUES(?,?,?)`,
		string(peer), payload, time.Now().UTC().Unix()); err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	return tx.Commit()
}

// Ack marks an outbox row as sent so future truncation can reclaim its bytes.
// Untouched rows remain pending until a peer acknowledges receipt.
func (o *Outbox) Ack(ctx context.Context, id int64) error {
	tx, err := o.store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`UPDATE sync_outbox SET sent_at=? WHERE id=?`,
		time.Now().UTC().Unix(), id); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	return tx.Commit()
}

// SizeBytes returns the current total outbox storage cost — used by tests and
// the disk watcher (§2.7).
func (o *Outbox) SizeBytes(ctx context.Context) (int64, error) {
	tx, err := o.store.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return 0, fmt.Errorf("begin ro: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var n sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(LENGTH(payload)),0) FROM sync_outbox`).Scan(&n); err != nil {
		return 0, fmt.Errorf("sum: %w", err)
	}
	return n.Int64, nil
}

// truncateAckdToFit drops oldest ack'd rows until projected total (existing + incoming)
// fits OutboxMaxBytes. If the projection still over-runs after exhausting ack'd rows,
// the function returns nil — Enqueue proceeds; the disk watcher will surface the warn.
// (Per spec §2.7: "warn if still > 50 MB after compression"; we never silently drop
// an unsent delta.)
func truncateAckdToFit(ctx context.Context, tx *sql.Tx, incoming int64) error {
	var current int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(LENGTH(payload)),0) FROM sync_outbox`).Scan(&current); err != nil {
		return fmt.Errorf("sum: %w", err)
	}
	if current+incoming <= OutboxMaxBytes {
		return nil
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT id, LENGTH(payload) FROM sync_outbox WHERE sent_at IS NOT NULL ORDER BY enqueued_at ASC`)
	if err != nil {
		return fmt.Errorf("scan ackd: %w", err)
	}
	var victims []int64
	freed := int64(0)
	for rows.Next() {
		var id int64
		var size int64
		if err := rows.Scan(&id, &size); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan ackd row: %w", err)
		}
		victims = append(victims, id)
		freed += size
		if current+incoming-freed <= OutboxMaxBytes {
			break
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	for _, id := range victims {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sync_outbox WHERE id=?`, id); err != nil {
			return fmt.Errorf("delete %d: %w", id, err)
		}
	}
	return nil
}

func gzipJSON(entries []crdt.LogEntry) ([]byte, error) {
	raw, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}
	return buf.Bytes(), nil
}
