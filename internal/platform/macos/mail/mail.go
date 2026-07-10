// Package mail is a read-only, attestation-gated SQLite reader for Mail.app
// (Envelope Index, V10). Distinct from internal/adapters/gmail/: this reads
// the local mirror so the operator's already-synced mail is queryable
// offline without a second OAuth dance.
package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/platform/contracts"
	"github.com/trilam/leah/internal/platform/macos/sqliteopen"
)

const Scope = "macos:mail:query"

var (
	ErrPermissionDenied  = errors.New("mail: permission denied")
	ErrSourceUnavailable = errors.New("mail: source unavailable")
	ErrAttestationDenied = errors.New("mail: attestation denied")
)

type MacApp interface {
	Name() string
	Available(ctx context.Context) bool
	Sync(ctx context.Context) error
	Query(ctx context.Context, q Query) ([]Item, error)
}

// Item carries Envelope Index headers only — full body decode is deferred
// per spec §non-goals until a caller files an issue citing the gap.
type Item struct {
	ID      string
	From    string
	To      []string
	Subject string
	Date    time.Time
	Snippet string
}

type Query struct {
	Since, Until time.Time
	Text         string
	Limit        int
}

type Opener func(driverName, dsn string) (*sql.DB, error)

type Config struct {
	DBPath   string
	Attestor contracts.Attestor
	Open     Opener
}

type Mail struct {
	path string
	att  contracts.Attestor
	open Opener
}

func New(cfg Config) (*Mail, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("mail: Config.Attestor required")
	}
	path := cfg.DBPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("mail: resolve home: %w", err)
		}
		path = home + "/Library/Mail/V10/MailData/Envelope Index"
	}
	open := cfg.Open
	if open == nil {
		open = sql.Open
	}
	return &Mail{path: path, att: cfg.Attestor, open: open}, nil
}

func (m *Mail) Name() string { return "Mail" }

func (m *Mail) Available(_ context.Context) bool {
	f, err := os.Open(m.path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (m *Mail) Sync(_ context.Context) error { return nil }

// Attest BEFORE open so a denied operator never attaches the Apple store.
func (m *Mail) Query(ctx context.Context, q Query) ([]Item, error) {
	if err := m.att.Attest(ctx, Scope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if _, err := os.Stat(m.path); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	db, err := m.open("sqlite", sqliteopen.RODSN(m.path))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	defer func() { _ = db.Close() }()
	return queryItems(ctx, db, q)
}

const snippetMax = 200

func queryItems(ctx context.Context, db *sql.DB, q Query) ([]Item, error) {
	where := []string{"1=1"}
	var args []any
	if !q.Since.IsZero() {
		where = append(where, "m.date_received >= ?")
		args = append(args, q.Since.Unix())
	}
	if !q.Until.IsZero() {
		where = append(where, "m.date_received < ?")
		args = append(args, q.Until.Unix())
	}
	if q.Text != "" {
		where = append(where, "s.subject LIKE ?")
		args = append(args, "%"+q.Text+"%")
	}
	sqlStr := `SELECT m.ROWID,
	                  COALESCE(s.subject,''), COALESCE(a.address,''),
	                  COALESCE(sm.summary,''), COALESCE(m.date_received,0)
	           FROM messages m
	           LEFT JOIN subjects s   ON s.ROWID  = m.subject
	           LEFT JOIN addresses a  ON a.ROWID  = m.sender
	           LEFT JOIN summaries sm ON sm.ROWID = m.summary
	           WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY m.date_received ASC`
	if q.Limit > 0 {
		sqlStr += " LIMIT ?"
		args = append(args, q.Limit)
	}
	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: query: %v", ErrSourceUnavailable, err)
	}
	defer func() { _ = rows.Close() }()
	var out []Item
	type pending struct {
		idx   int
		rowid int64
	}
	var todo []pending
	for rows.Next() {
		var (
			rowid      int64
			subj, from string
			summary    string
			recvSec    int64
		)
		if err := rows.Scan(&rowid, &subj, &from, &summary, &recvSec); err != nil {
			return nil, fmt.Errorf("%w: scan: %v", ErrSourceUnavailable, err)
		}
		out = append(out, Item{
			ID:      fmt.Sprintf("mail:%d", rowid),
			From:    from,
			Subject: subj,
			Date:    time.Unix(recvSec, 0).UTC(),
			Snippet: truncate(summary, snippetMax),
		})
		todo = append(todo, pending{idx: len(out) - 1, rowid: rowid})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: rows: %v", ErrSourceUnavailable, err)
	}
	for _, p := range todo {
		to, err := recipients(ctx, db, p.rowid)
		if err != nil {
			return nil, fmt.Errorf("%w: recipients: %v", ErrSourceUnavailable, err)
		}
		out[p.idx].To = to
	}
	return out, nil
}

// recipients.message_id is the messages.ROWID foreign key (not the RFC 5322
// Message-Id header, which Envelope Index stores in messages.message_id).
func recipients(ctx context.Context, db *sql.DB, messageRowID int64) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT a.address FROM recipients r
		 JOIN addresses a ON a.ROWID = r.address_id
		 WHERE r.message_id = ? AND a.address IS NOT NULL
		 ORDER BY r.ROWID`, messageRowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var addrs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		addrs = append(addrs, s)
	}
	return addrs, rows.Err()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
