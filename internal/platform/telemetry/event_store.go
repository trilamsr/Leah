package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/memory/sqlstore"
)

// Integer-parsed — lex compare ranks "10" < "9" (PR #58 lesson).
const embeddedEventSchemaVersion = "1"

const eventSchemaSQL = `CREATE TABLE IF NOT EXISTS events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  ts         INTEGER NOT NULL,
  kind       TEXT    NOT NULL,
  actor      TEXT    NOT NULL,
  target     TEXT,
  scope      TEXT,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  outcome    TEXT    NOT NULL,
  ref_id     TEXT,
  detail     TEXT
);
CREATE INDEX IF NOT EXISTS events_ts_idx      ON events (ts);
CREATE INDEX IF NOT EXISTS events_ref_idx     ON events (ref_id);
CREATE INDEX IF NOT EXISTS events_kind_ts_idx ON events (kind, ts);`

// Buffer drops on overflow rather than blocking the caller (spec §5).
const (
	defaultBufferSize    = 1024
	defaultBatchSize     = 100
	defaultBatchInterval = 100 * time.Millisecond
	defaultQueryLimit    = 1000
)

// SQLiteEventStore implements EventStore against modernc.org/sqlite.
type SQLiteEventStore struct {
	db     *sql.DB
	ch     chan Event
	done   chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger

	dropped uint64
	dropMu  sync.Mutex
}

// EventStoreOptions tunes OpenEventStore; zero values use spec defaults.
type EventStoreOptions struct {
	BufferSize    int
	BatchSize     int
	BatchInterval time.Duration
	Logger        *slog.Logger
}

// OpenEventStore opens events.db at path (mode 0600), migrates, starts the writer.
func OpenEventStore(path string, opts EventStoreOptions) (*SQLiteEventStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	// synchronous=NORMAL trades last-few-ms-on-crash for throughput (spec §2.2).
	db, err := sqlstore.OpenWAL(path, "synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("chmod 0600: %w", err)
	}
	if err := migrateEvents(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	bs := opts.BufferSize
	if bs <= 0 {
		bs = defaultBufferSize
	}
	batchN := opts.BatchSize
	if batchN <= 0 {
		batchN = defaultBatchSize
	}
	batchInt := opts.BatchInterval
	if batchInt <= 0 {
		batchInt = defaultBatchInterval
	}
	lg := opts.Logger
	if lg == nil {
		lg = slog.Default()
	}
	s := &SQLiteEventStore{
		db:     db,
		ch:     make(chan Event, bs),
		done:   make(chan struct{}),
		logger: lg,
	}
	s.wg.Add(1)
	go s.writeLoop(batchN, batchInt)
	return s, nil
}

func migrateEvents(db *sql.DB) error {
	embedded, err := parseEventSchemaVersion(embeddedEventSchemaVersion)
	if err != nil {
		return fmt.Errorf("parse embedded schema version %q: %w", embeddedEventSchemaVersion, err)
	}
	if err := sqlstore.EnsureSchemaVersion(db, "events.db", embedded); err != nil {
		return err
	}
	if _, err := db.Exec(eventSchemaSQL); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	if _, err := db.Exec(
		`INSERT OR REPLACE INTO schema_meta(key, value) VALUES('version', ?)`,
		embeddedEventSchemaVersion,
	); err != nil {
		return fmt.Errorf("stamp schema version: %w", err)
	}
	return nil
}

func parseEventSchemaVersion(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

// Emit enqueues e; non-blocking. Full buffer → ErrEventDropped + logged warn.
func (s *SQLiteEventStore) Emit(ctx context.Context, e Event) error {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	if e.RefID == "" {
		if id := RefID(ctx); id != "" {
			e.RefID = id
		}
	}
	select {
	case s.ch <- e:
		return nil
	default:
		s.dropMu.Lock()
		s.dropped++
		dropped := s.dropped
		s.dropMu.Unlock()
		s.logger.Warn("obs.event dropped — buffer full",
			"kind", e.Kind, "dropped_total", dropped)
		return ErrEventDropped
	}
}

var ErrEventDropped = errors.New("obs: event dropped, buffer full")

// Dropped returns the running drop count.
func (s *SQLiteEventStore) Dropped() uint64 {
	s.dropMu.Lock()
	defer s.dropMu.Unlock()
	return s.dropped
}

func (s *SQLiteEventStore) writeLoop(batchN int, batchInt time.Duration) {
	defer s.wg.Done()
	// Same-package: cannot SafeGo via obs.SafeGo without recursive import.
	// A panic here loses all observability — log to slog directly so the
	// daemon at least sees the failure even if the events ring is the bug.
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("events writeLoop panic", "panic", r)
		}
	}()
	buf := make([]Event, 0, batchN)
	tick := time.NewTicker(batchInt)
	defer tick.Stop()
	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := s.flushBatch(buf); err != nil {
			s.logger.Error("obs.event flush failed", "err", err, "n", len(buf))
		}
		buf = buf[:0]
	}
	for {
		select {
		case <-s.done:
			for { // drain pending entries before exit

				select {
				case e := <-s.ch:
					buf = append(buf, e)
					if len(buf) >= batchN {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case e := <-s.ch:
			buf = append(buf, e)
			if len(buf) >= batchN {
				flush()
			}
		case <-tick.C:
			flush()
		}
	}
}

func (s *SQLiteEventStore) flushBatch(events []Event) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Umbrella rollback — Rollback after Commit is a no-op per database/sql
	// contract. Forward-safe if a future error return forgets explicit roll.
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(
		`INSERT INTO events(ts, kind, actor, target, scope, latency_ms, outcome, ref_id, detail)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, e := range events {
		if _, err := stmt.Exec(
			e.TS.UnixNano(), e.Kind, e.Actor,
			nullable(e.Target), nullable(e.Scope),
			e.LatencyMS, e.Outcome,
			nullable(e.RefID), nullable(e.Detail),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// nullable maps "" → SQL NULL so IS NULL queries behave correctly.
func nullable(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// Query returns events ordered ts ASC. Eventually-consistent — tests Sync first.
func (s *SQLiteEventStore) Query(ctx context.Context, q EventQuery) ([]Event, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	var (
		clauses []string
		args    []interface{}
	)
	if !q.Since.IsZero() {
		clauses = append(clauses, "ts >= ?")
		args = append(args, q.Since.UnixNano())
	}
	if !q.Until.IsZero() {
		clauses = append(clauses, "ts < ?")
		args = append(args, q.Until.UnixNano())
	}
	if len(q.Kinds) > 0 {
		clauses = append(clauses, "kind IN ("+placeholders(len(q.Kinds))+")")
		for _, k := range q.Kinds {
			args = append(args, k)
		}
	}
	if len(q.Actors) > 0 {
		clauses = append(clauses, "actor IN ("+placeholders(len(q.Actors))+")")
		for _, a := range q.Actors {
			args = append(args, a)
		}
	}
	if len(q.Outcomes) > 0 {
		clauses = append(clauses, "outcome IN ("+placeholders(len(q.Outcomes))+")")
		for _, o := range q.Outcomes {
			args = append(args, o)
		}
	}
	if q.RefID != "" {
		clauses = append(clauses, "ref_id = ?")
		args = append(args, q.RefID)
	}
	sqlStr := `SELECT ts, kind, actor, target, scope, latency_ms, outcome, ref_id, detail FROM events`
	if len(clauses) > 0 {
		sqlStr += " WHERE " + strings.Join(clauses, " AND ")
	}
	sqlStr += " ORDER BY ts ASC, id ASC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Event
	for rows.Next() {
		var (
			tsNanos                      int64
			kind, actor, outcome         string
			target, scope, refID, detail sql.NullString
			latencyMS                    int64
		)
		if err := rows.Scan(&tsNanos, &kind, &actor, &target, &scope, &latencyMS, &outcome, &refID, &detail); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, Event{
			TS:        time.Unix(0, tsNanos).UTC(),
			Kind:      kind,
			Actor:     actor,
			Target:    target.String,
			Scope:     scope.String,
			LatencyMS: latencyMS,
			Outcome:   outcome,
			RefID:     refID.String,
			Detail:    detail.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?,", n-1) + "?"
}

// Sync blocks until in-flight emits have been written. Test-only helper.
func (s *SQLiteEventStore) Sync(ctx context.Context) error {
	deadline := time.Now().Add(5 * time.Second)
	for len(s.ch) > 0 {
		if time.Now().After(deadline) {
			return errors.New("obs.event sync: timeout draining channel")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Millisecond): // allow-sleep: drain-quiescence wait; bounded by 5s deadline
		}
	}
	// Slack covers a dequeued-but-pre-commit batch.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(defaultBatchInterval + 50*time.Millisecond): // allow-sleep: covers in-flight batch commit; deterministic upper bound
	}
	return nil
}

// PruneOlderThan deletes rows ts < cutoff (spec §8 retention).
func (s *SQLiteEventStore) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE ts < ?`, cutoff.UnixNano())
	if err != nil {
		return 0, fmt.Errorf("prune: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return n, nil
}

// Close drains pending events and closes the DB.
func (s *SQLiteEventStore) Close() error {
	close(s.done)
	s.wg.Wait()
	return s.db.Close()
}
