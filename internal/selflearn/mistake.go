package selflearn

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var mistakeSchema string

// Mistake is an operator-annotated negative outcome (spec §3.1).
type Mistake struct {
	ID         string
	CreatedAt  time.Time
	AuditTS    string // RFC3339 — original audit row Timestamp
	AuditKind  string // original audit row Kind
	AuditHash  string // original audit row ArgsHash
	RootCause  string
	Prevention string
}

// MistakeStore wraps the mistake_log table.
type MistakeStore struct {
	db  *sql.DB
	Now func() time.Time
}

// OpenMistakeStore opens (and migrates) the sqlite db at path.
func OpenMistakeStore(path string) (*MistakeStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := db.Exec(mistakeSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &MistakeStore{db: db}, nil
}

// Close releases the underlying db handle.
func (s *MistakeStore) Close() error { return s.db.Close() }

// Add inserts a mistake row + returns its ulid.
func (s *MistakeStore) Add(m Mistake) (string, error) {
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now()
	}
	id := ulid.MustNew(ulid.Timestamp(now), rand.Reader).String()
	_, err := s.db.Exec(
		`INSERT INTO mistake_log (id, created_at, audit_ts, audit_kind, audit_hash, root_cause, prevention)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, now.Format(time.RFC3339), m.AuditTS, m.AuditKind, m.AuditHash, m.RootCause, m.Prevention,
	)
	if err != nil {
		return "", fmt.Errorf("insert mistake: %w", err)
	}
	return id, nil
}

// ListOptions narrows List to a single ISO week (YYYY-WW) when set.
type ListOptions struct {
	Week string // ISO "YYYY-WW"; empty = all
}

// List returns mistakes sorted oldest-first.
func (s *MistakeStore) List(opts ListOptions) ([]Mistake, error) {
	rows, err := s.db.Query(`SELECT id, created_at, audit_ts, audit_kind, audit_hash, root_cause, prevention FROM mistake_log ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Mistake
	for rows.Next() {
		var m Mistake
		var createdStr string
		if err := rows.Scan(&m.ID, &createdStr, &m.AuditTS, &m.AuditKind, &m.AuditHash, &m.RootCause, &m.Prevention); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		t, err := time.Parse(time.RFC3339, createdStr)
		if err != nil {
			return nil, fmt.Errorf("parse created_at %q: %w", createdStr, err)
		}
		m.CreatedAt = t.UTC()
		if opts.Week != "" {
			y, w := m.CreatedAt.ISOWeek()
			if fmt.Sprintf("%04d-%02d", y, w) != opts.Week {
				continue
			}
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
