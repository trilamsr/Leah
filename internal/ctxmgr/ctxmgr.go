// Package ctxmgr is the single-active-context manager.
//
// One row in operator_state identifies the operator's current context;
// switches append to context_switch_log. Backed by the shared memory.db
// (see internal/memory/schema.sql, schema_version 2).
//
// Personal-use Phase 1: zero concurrent contexts, no auto-infer, no
// cross-context queries. See docs/specs/2026-06-09-context-manager.md.
package ctxmgr

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	_ "modernc.org/sqlite"
)

// nameRe constrains context names: lowercase, digits, dashes;
// must lead with a letter; 1-32 chars. Keeps shells / CLI / SQL safe.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// ErrUnknownContext is returned by Switch when the target context name
// has no row in the context table.
var ErrUnknownContext = errors.New("ctxmgr: unknown context")

// ErrDuplicateContext is returned by NewContext when a context with the
// requested name already exists.
var ErrDuplicateContext = errors.New("ctxmgr: context already exists")

// ErrInvalidName is returned by NewContext when the name fails nameRe.
var ErrInvalidName = errors.New("ctxmgr: invalid context name")

// Context is a stored named lane (workspace dimension; see overview §3.5).
type Context struct {
	Name        string
	Description string
	CreatedAt   time.Time
}

// Switch is one row of context_switch_log.
type Switch struct {
	ID          int64
	From        string // empty on first switch
	To          string
	SwitchedAt  time.Time
	Reason      string
}

// Manager owns a *sql.DB handle pointing at the shared memory.db file.
// It is safe for concurrent use (database/sql pool handles serialization;
// SQLite is opened in WAL mode for daemon + CLI co-existence).
type Manager struct {
	db  *sql.DB
	now func() time.Time
}

// Open returns a Manager backed by the SQLite file at path. The file is
// created if absent; schema (context, operator_state, context_switch_log)
// is applied via CREATE IF NOT EXISTS so re-open is idempotent.
//
// WAL mode + busy_timeout=5000ms permit safe co-existence with leah-daemon
// reading from the same file (spec §5.2).
func Open(path string) (*Manager, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	m := &Manager{db: db, now: func() time.Time { return time.Now().UTC() }}
	if err := m.ensureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return m, nil
}

// Close releases the underlying *sql.DB.
func (m *Manager) Close() error { return m.db.Close() }

// SetNow overrides the time source (test-only).
func (m *Manager) SetNow(fn func() time.Time) { m.now = fn }

// ensureSchema applies the additive ctxmgr tables. Idempotent. Kept inline
// (not a //go:embed of schema.sql) so the ctxmgr package compiles standalone
// — the memory pkg owns the canonical schema.sql for full-file initialization.
func (m *Manager) ensureSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS context (
			name         TEXT PRIMARY KEY,
			created_at   TEXT NOT NULL,
			description  TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS operator_state (
			id              INTEGER PRIMARY KEY CHECK(id=1),
			active_context  TEXT NOT NULL REFERENCES context(name),
			updated_at      TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS context_switch_log (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			from_context  TEXT,
			to_context    TEXT NOT NULL,
			switched_at   TEXT NOT NULL,
			reason        TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_switch_time ON context_switch_log(switched_at)`,
	}
	for _, s := range stmts {
		if _, err := m.db.Exec(s); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	ts := m.now().Format(time.RFC3339)
	if _, err := m.db.Exec(
		`INSERT OR IGNORE INTO context(name, created_at, description) VALUES (?, ?, ?)`,
		"default", ts, "Implicit context for unsegmented work",
	); err != nil {
		return fmt.Errorf("seed default context: %w", err)
	}
	if _, err := m.db.Exec(
		`INSERT OR IGNORE INTO operator_state(id, active_context, updated_at) VALUES (1, ?, ?)`,
		"default", ts,
	); err != nil {
		return fmt.Errorf("seed operator_state: %w", err)
	}
	return nil
}

// NewContext creates a context row. Returns ErrInvalidName on regex miss,
// ErrDuplicateContext on collision.
func (m *Manager) NewContext(name, description string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	ts := m.now().Format(time.RFC3339)
	res, err := m.db.Exec(
		`INSERT OR IGNORE INTO context(name, created_at, description) VALUES (?, ?, ?)`,
		name, ts, description,
	)
	if err != nil {
		return fmt.Errorf("insert context: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: %q", ErrDuplicateContext, name)
	}
	return nil
}

// Switch points operator_state at name and appends to context_switch_log.
// reason is free-form ("cli", "voice-intent", "auto-infer"). Empty allowed.
// Idempotent on already-active target — still records a log row so the
// audit trail captures the user-visible action.
func (m *Manager) Switch(name, reason string) error {
	// Look up target so we return ErrUnknownContext before mutating state.
	var exists int
	if err := m.db.QueryRow(`SELECT 1 FROM context WHERE name = ?`, name).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q", ErrUnknownContext, name)
		}
		return fmt.Errorf("lookup context: %w", err)
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var from string
	if err := tx.QueryRow(`SELECT active_context FROM operator_state WHERE id = 1`).Scan(&from); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read active: %w", err)
	}

	ts := m.now().Format(time.RFC3339)
	if _, err := tx.Exec(
		`INSERT OR REPLACE INTO operator_state(id, active_context, updated_at) VALUES (1, ?, ?)`,
		name, ts,
	); err != nil {
		return fmt.Errorf("update operator_state: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO context_switch_log(from_context, to_context, switched_at, reason) VALUES (?, ?, ?, ?)`,
		nullableString(from), name, ts, reason,
	); err != nil {
		return fmt.Errorf("append switch log: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Active returns the currently active context. On a fresh DB the schema
// seed guarantees "default", so this never errors with "no row".
func (m *Manager) Active() (Context, error) {
	var c Context
	var createdAt string
	var desc sql.NullString
	row := m.db.QueryRow(`
		SELECT c.name, c.created_at, c.description
		FROM operator_state os
		JOIN context c ON c.name = os.active_context
		WHERE os.id = 1
	`)
	if err := row.Scan(&c.Name, &createdAt, &desc); err != nil {
		return Context{}, fmt.Errorf("read active: %w", err)
	}
	c.Description = desc.String
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		c.CreatedAt = t
	}
	return c, nil
}

// History returns up to limit switches, most-recent first. limit<=0 → 20.
func (m *Manager) History(limit int) ([]Switch, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := m.db.Query(`
		SELECT id, from_context, to_context, switched_at, reason
		FROM context_switch_log
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Switch
	for rows.Next() {
		var s Switch
		var from, reason sql.NullString
		var switchedAt string
		if err := rows.Scan(&s.ID, &from, &s.To, &switchedAt, &reason); err != nil {
			return nil, fmt.Errorf("scan history row: %w", err)
		}
		s.From = from.String
		s.Reason = reason.String
		if t, err := time.Parse(time.RFC3339, switchedAt); err == nil {
			s.SwitchedAt = t
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter history: %w", err)
	}
	return out, nil
}

// Since returns switches with switched_at >= cutoff, ascending — the shape
// operatormodel.activeAt requires for an "active context at ts" walk.
func (m *Manager) Since(cutoff time.Time) ([]Switch, error) {
	rows, err := m.db.Query(`
		SELECT id, from_context, to_context, switched_at, reason
		FROM context_switch_log
		WHERE switched_at >= ?
		ORDER BY switched_at ASC, id ASC
	`, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("query since: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Switch
	for rows.Next() {
		var s Switch
		var from, reason sql.NullString
		var switchedAt string
		if err := rows.Scan(&s.ID, &from, &s.To, &switchedAt, &reason); err != nil {
			return nil, fmt.Errorf("scan since row: %w", err)
		}
		s.From = from.String
		s.Reason = reason.String
		if t, err := time.Parse(time.RFC3339, switchedAt); err == nil {
			s.SwitchedAt = t
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter since: %w", err)
	}
	return out, nil
}

// List returns every known context sorted by name.
func (m *Manager) List() ([]Context, error) {
	rows, err := m.db.Query(`SELECT name, created_at, description FROM context ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query contexts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Context
	for rows.Next() {
		var c Context
		var createdAt string
		var desc sql.NullString
		if err := rows.Scan(&c.Name, &createdAt, &desc); err != nil {
			return nil, fmt.Errorf("scan context row: %w", err)
		}
		c.Description = desc.String
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			c.CreatedAt = t
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter contexts: %w", err)
	}
	return out, nil
}

// nullableString returns sql.NullString{Valid:false} when s is empty so
// the column stores NULL — preserves the "first switch has no from" shape.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
