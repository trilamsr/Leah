package telemetry

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/sqlstore"
)

// Event is one row in the causal timeline — internal sibling to audit.jsonl;
// captures denied/failed paths audit elides (spec §2.1). Payload carries
// transport-only structured data (e.g. HUD state snapshot) on the SSE path;
// SQLite persistence ignores it so the row schema stays narrow.
type Event struct {
	TS        time.Time   `json:"ts"`
	Kind      string      `json:"kind"`
	Actor     string      `json:"actor"`
	Target    string      `json:"target,omitempty"`
	Scope     string      `json:"scope,omitempty"`
	LatencyMS int64       `json:"latency_ms,omitempty"`
	Outcome   string      `json:"outcome"`
	RefID     string      `json:"ref_id,omitempty"`
	Detail    string      `json:"detail,omitempty"`
	Payload   interface{} `json:"payload,omitempty"`
}

// HUDStateEvent is the Payload schema for `hud.state` events consumed by
// ambient.js. Field names MUST stay in sync with the JS reader
// (`p.value`, `p.listening`, `p.thinking`) — anything else freezes the
// state pill at its default.
type HUDStateEvent struct {
	Value     string `json:"value"`
	Listening bool   `json:"listening"`
	Thinking  bool   `json:"thinking"`
}

// WorkspaceActiveAppEvent is the Payload schema for
// `workspace.active_app_changed`. Separate kind from `hud.state` so the
// active-app push pump cannot reset the HUD pill or inject phantom
// recommender signals (V9 reviewer B1).
type WorkspaceActiveAppEvent struct {
	BundleID string `json:"bundle_id"`
	Name     string `json:"name"`
}

// ContactStoreChangedEvent is the Payload schema for `contact_store_changed`.
// Empty struct — the notification itself is the signal; CNContactStore does
// not name the changed records (privacy-by-design).
type ContactStoreChangedEvent struct{}

// MessagesChangedEvent is the Payload for `messages_changed`. The FSEvents
// WAL watcher never opens chat.db — consumers re-query under their own
// macos:messages:query scope, so no row data ships in the payload.
type MessagesChangedEvent struct{}

// MailChangedEvent — Envelope Index WAL mutation; consumers re-query under
// macos:mail:query.
type MailChangedEvent struct{}

// NotesChangedEvent — NoteStore.sqlite WAL mutation; consumers re-query under
// macos:notes:query.
type NotesChangedEvent struct{}

// SafariHistoryChangedEvent is the Payload schema for `safari.history_changed`.
// Empty struct — the FSEvent fires once History.db WAL has flushed; row ids
// are intentionally not surfaced (PII; downstream re-reads the db).
type SafariHistoryChangedEvent struct{}

// FocusStateChangedEvent is the Payload schema for `focus.state_changed`. Empty
// struct — pmset / DND-Assertions FSEvents only signal that focus toggled;
// consumers re-read state under macos:focus:query for the active mode.
type FocusStateChangedEvent struct{}

// CalendarStoreChangedEvent is the Payload for `calendar.store_changed`.
// Empty struct — the FSEvent fires once Calendar.sqlitedb WAL has flushed;
// event ids stay implicit so consumers re-query under macos:calendar:query.
type CalendarStoreChangedEvent struct{}

// PhotosLibraryChangedEvent — Photos.sqlite WAL mutation; consumers re-query
// under macos:photos:query. UUIDs are not surfaced (PII).
type PhotosLibraryChangedEvent struct{}

// RemindersStoreChangedEvent — Reminders Group Container store WAL mutation;
// consumers re-query under macos:reminders:query.
type RemindersStoreChangedEvent struct{}

// EventQuery is a Query filter. Mutually-additive fields AND together.
type EventQuery struct {
	Since    time.Time
	Until    time.Time
	Kinds    []string
	Actors   []string
	Outcomes []string
	RefID    string
	Limit    int
}

// EventStore is the timeline backend; SQLite impl needs Close to drain.
type EventStore interface {
	Emit(ctx context.Context, e Event) error
	Query(ctx context.Context, q EventQuery) ([]Event, error)
	Close() error
}

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

type refIDKeyType struct{}

var refIDKey refIDKeyType

// WithRefID stamps refID onto ctx; explicit (not goroutine-local) per spec §4.
func WithRefID(ctx context.Context, refID string) context.Context {
	return context.WithValue(ctx, refIDKey, refID)
}

// RefID reads the RefID stamped on ctx ("" if absent).
func RefID(ctx context.Context) string {
	v, _ := ctx.Value(refIDKey).(string)
	return v
}

// NewRefID returns a fresh 128-bit hex RefID for operation roots.
func NewRefID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		// Pseudo-ID fallback — diagnostic correlation OK if /dev/urandom broken.
		now := time.Now().UnixNano()
		for i := 0; i < 8; i++ {
			b[i] = byte(now >> (i * 8))
		}
	}
	return hex.EncodeToString(b[:])
}

var (
	defaultStoreMu sync.RWMutex
	defaultStore   EventStore
)

// SetDefaultEventStore wires EmitEvent to store; nil detaches (no-op mode).
func SetDefaultEventStore(store EventStore) {
	defaultStoreMu.Lock()
	defaultStore = store
	defaultStoreMu.Unlock()
}

// EmitEvent enqueues e against the default store AND fans it out to the
// default Broadcaster's live SSE subscribers. Either side is a no-op when
// its sink is unset, so this stays safe for callers that only wire one.
func EmitEvent(ctx context.Context, e Event) {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	if e.RefID == "" {
		if id := RefID(ctx); id != "" {
			e.RefID = id
		}
	}
	Publish(e)
	defaultStoreMu.RLock()
	s := defaultStore
	defaultStoreMu.RUnlock()
	if s == nil {
		return
	}
	_ = s.Emit(ctx, e)
}

// KnownEventKinds — frozen enum; drift-gated by TestEventKinds_FrozenList.
var KnownEventKinds = []string{
	"dispatch.ship", "dispatch.review", "dispatch.merge",
	"attestation.attempt", "attestation.granted", "attestation.revoked",
	"audit.append", "audit.rotate",
	"connect.exchange", "connect.refresh", "connect.api_call",
	"voice.speak", "voice.fallback",
	"subagent.spawn", "subagent.complete",
	"reasoner.call", "reasoner.retry",
	"memory.query", "memory.upsert",
	"recommendation.propose", "recommendation.accept",
	"recommendation.reject", "recommendation.apply",
	"obs.snapshot", "obs.selfcheck", "obs.panic",
	"hud.state",
	"workspace.active_app_changed",
	"contact_store_changed",
	"messages_changed",
	"mail_changed",
	"notes_changed",
	"safari.history_changed",
	"calendar.store_changed",
	"focus.state_changed",
	"photos.library_changed",
	"reminders.store_changed",
}

// SafeDetail strips chars outside [\w\-\.:/], truncates 128r (spec §9 PII).
func SafeDetail(s string) string {
	const maxRunes = 128
	var b strings.Builder
	b.Grow(len(s))
	n := 0
	for _, r := range s {
		if n >= maxRunes {
			break
		}
		if isDetailRune(r) {
			b.WriteRune(r)
			n++
		}
	}
	return b.String()
}

func isDetailRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '_' || r == '-' || r == '.' || r == ':' || r == '/':
		return true
	}
	return false
}

// SafeDetailHashed returns "h:<FNV-1a hex>" — use for any PII-bearing value.
func SafeDetailHashed(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("h:%x", h.Sum64())
}
