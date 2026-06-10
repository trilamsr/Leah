package knowledge

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// int-parse per PR #58 — lex "10" < "9" is the trap.
const schemaVersion = "1"

const ddl = `
CREATE TABLE IF NOT EXISTS schema_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS entities (
  kind         TEXT NOT NULL,
  key          TEXT NOT NULL,
  display      TEXT NOT NULL,
  aliases_json TEXT NOT NULL DEFAULT '[]',
  first_seen   INTEGER NOT NULL,
  last_touched INTEGER NOT NULL,
  PRIMARY KEY (kind, key)
);
CREATE TABLE IF NOT EXISTS entity_items (
  kind     TEXT NOT NULL,
  key      TEXT NOT NULL,
  source   TEXT NOT NULL,
  item_id  TEXT NOT NULL,
  ts       INTEGER NOT NULL,
  PRIMARY KEY (kind, key, source, item_id)
);
CREATE INDEX IF NOT EXISTS idx_entity_items_ts ON entity_items(ts);
`

// Single-writer pool keeps WAL contention bounded across daemon + CLI.
type storage struct {
	db *sql.DB
}

func openStorage(path string) (*storage, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		f, ferr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if ferr != nil {
			return nil, fmt.Errorf("create db file: %w", ferr)
		}
		_ = f.Close()
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("chmod db file: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", url.PathEscape(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &storage{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *storage) Close() error { return s.db.Close() }

func (s *storage) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("bootstrap schema_meta: %w", err)
	}
	var v string
	err := s.db.QueryRow(`SELECT value FROM schema_meta WHERE key='version'`).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read schema version: %w", err)
	}
	if v != "" {
		onDisk, perr := strconv.Atoi(strings.TrimSpace(v))
		if perr != nil {
			return fmt.Errorf("parse on-disk schema version %q: %w", v, perr)
		}
		embedded, _ := strconv.Atoi(schemaVersion)
		if onDisk > embedded {
			return fmt.Errorf("knowledge.db schema version %s newer than binary %s; upgrade leah", v, schemaVersion)
		}
	}
	if _, err := s.db.Exec(ddl); err != nil {
		return fmt.Errorf("exec ddl: %w", err)
	}
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO schema_meta(key, value) VALUES('version', ?)`, schemaVersion); err != nil {
		return fmt.Errorf("stamp version: %w", err)
	}
	return nil
}

func (s *storage) upsertEntity(e Entity, now time.Time) error {
	aliases, _ := json.Marshal(e.Aliases)
	last := e.LastTouched
	if last.IsZero() {
		last = now
	}
	_, err := s.db.Exec(`
		INSERT INTO entities (kind, key, display, aliases_json, first_seen, last_touched)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(kind, key) DO UPDATE SET
		    display = excluded.display,
		    aliases_json = excluded.aliases_json,
		    last_touched = excluded.last_touched`,
		string(e.Kind), e.Key, e.Display, string(aliases), now.UnixNano(), last.UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("upsert entity: %w", err)
	}
	for _, ref := range e.Refs {
		ts := ref.Timestamp.UnixNano()
		if _, err := s.db.Exec(`
			INSERT OR REPLACE INTO entity_items (kind, key, source, item_id, ts)
			VALUES (?, ?, ?, ?, ?)`,
			string(e.Kind), e.Key, ref.Source, ref.ID, ts,
		); err != nil {
			return fmt.Errorf("insert ref: %w", err)
		}
	}
	return nil
}

func (s *storage) getEntity(kind EntityKind, key string) (Entity, error) {
	var e Entity
	var aliasJSON string
	var firstSeen, lastTouched int64
	err := s.db.QueryRow(`
		SELECT kind, key, display, aliases_json, first_seen, last_touched
		FROM entities WHERE kind = ? AND key = ?`,
		string(kind), key,
	).Scan((*string)(&e.Kind), &e.Key, &e.Display, &aliasJSON, &firstSeen, &lastTouched)
	if errors.Is(err, sql.ErrNoRows) {
		return Entity{}, ErrUnknownEntity
	}
	if err != nil {
		return Entity{}, fmt.Errorf("scan entity: %w", err)
	}
	_ = json.Unmarshal([]byte(aliasJSON), &e.Aliases)
	e.FirstSeen = time.Unix(0, firstSeen).UTC()
	e.LastTouched = time.Unix(0, lastTouched).UTC()
	refs, err := s.listRefs(kind, key, time.Time{}, 0)
	if err != nil {
		return Entity{}, err
	}
	e.Refs = refs
	return e, nil
}

// Empty subject matches all; needle is lowercased substring over key/display/aliases.
func (s *storage) matchEntities(kind EntityKind, subject string, limit int) ([]Entity, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if kind == "" {
		rows, err = s.db.Query(`
			SELECT kind, key, display, aliases_json, first_seen, last_touched
			FROM entities ORDER BY last_touched DESC`)
	} else {
		rows, err = s.db.Query(`
			SELECT kind, key, display, aliases_json, first_seen, last_touched
			FROM entities WHERE kind = ? ORDER BY last_touched DESC`,
			string(kind))
	}
	if err != nil {
		return nil, fmt.Errorf("query entities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	needle := strings.ToLower(subject)
	var out []Entity
	for rows.Next() {
		var e Entity
		var aliasJSON string
		var firstSeen, lastTouched int64
		if err := rows.Scan((*string)(&e.Kind), &e.Key, &e.Display, &aliasJSON, &firstSeen, &lastTouched); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		_ = json.Unmarshal([]byte(aliasJSON), &e.Aliases)
		e.FirstSeen = time.Unix(0, firstSeen).UTC()
		e.LastTouched = time.Unix(0, lastTouched).UTC()
		if needle != "" && !entityMatches(e, needle) {
			continue
		}
		out = append(out, e)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

func entityMatches(e Entity, needle string) bool {
	if strings.Contains(strings.ToLower(e.Key), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Display), needle) {
		return true
	}
	for _, a := range e.Aliases {
		if strings.Contains(strings.ToLower(a), needle) {
			return true
		}
	}
	return false
}

// Newest-first; since.IsZero() means no lower bound; limit<=0 means no cap.
func (s *storage) listRefs(kind EntityKind, key string, since time.Time, limit int) ([]ItemRef, error) {
	q := `SELECT source, item_id, ts FROM entity_items WHERE kind = ? AND key = ?`
	args := []any{string(kind), key}
	if !since.IsZero() {
		q += ` AND ts >= ?`
		args = append(args, since.UnixNano())
	}
	q += ` ORDER BY ts DESC`
	if limit > 0 {
		q += fmt.Sprintf(` LIMIT %d`, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query refs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ItemRef
	for rows.Next() {
		var r ItemRef
		var ts int64
		if err := rows.Scan(&r.Source, &r.ID, &ts); err != nil {
			return nil, fmt.Errorf("scan ref: %w", err)
		}
		r.Timestamp = time.Unix(0, ts).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *storage) deleteEntity(kind EntityKind, key string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM entities WHERE kind = ? AND key = ?`, string(kind), key)
	if err != nil {
		return false, fmt.Errorf("delete entity: %w", err)
	}
	n, _ := res.RowsAffected()
	if _, err := s.db.Exec(`DELETE FROM entity_items WHERE kind = ? AND key = ?`, string(kind), key); err != nil {
		return false, fmt.Errorf("delete refs: %w", err)
	}
	return n > 0, nil
}
