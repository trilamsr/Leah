package sqlstore

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const schemaMetaSQL = `CREATE TABLE IF NOT EXISTS schema_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);`

// OpenWAL opens a writable WAL SQLite DB with a single-writer pool; extraPragmas append as `_pragma=<p>`.
func OpenWAL(path string, extraPragmas ...string) (*sql.DB, error) {
	frags := []string{"_pragma=journal_mode(WAL)", "_pragma=busy_timeout(5000)"}
	for _, p := range extraPragmas {
		frags = append(frags, "_pragma="+p)
	}
	dsn := fmt.Sprintf("file:%s?%s", url.PathEscape(path), strings.Join(frags, "&"))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}

// EnsureSchemaVersion bootstraps schema_meta and refuses an on-disk version newer than embedded; caller owns DDL + stamp.
func EnsureSchemaVersion(db *sql.DB, dbName string, embedded int) error {
	if _, err := db.Exec(schemaMetaSQL); err != nil {
		return fmt.Errorf("bootstrap schema_meta: %w", err)
	}
	var v string
	err := db.QueryRow(`SELECT value FROM schema_meta WHERE key='version'`).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read schema version: %w", err)
	}
	if v == "" {
		return nil
	}
	onDisk, perr := strconv.Atoi(strings.TrimSpace(v))
	if perr != nil {
		return fmt.Errorf("parse on-disk schema version %q: %w", v, perr)
	}
	if onDisk > embedded {
		return fmt.Errorf("%s schema version %s newer than binary %d; upgrade leah", dbName, v, embedded)
	}
	return nil
}
