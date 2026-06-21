// Package safari is a read-only, attestation-gated SQLite reader for
// Safari's History.db. Bookmarks.plist is deferred per W31 MVP scope —
// the binary-plist decode lands when a caller files an issue citing the gap.
package safari

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/macos/sqliteopen"
)

const Scope = "macos:safari:history"

var (
	ErrPermissionDenied  = errors.New("safari: permission denied")
	ErrSourceUnavailable = errors.New("safari: source unavailable")
	ErrAttestationDenied = errors.New("safari: attestation denied")
)

type MacApp interface {
	Name() string
	Available(ctx context.Context) bool
	Sync(ctx context.Context) error
	Query(ctx context.Context, q Query) ([]Item, error)
}

type Item struct {
	URL        string
	Title      string
	VisitTime  time.Time
	VisitCount int
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

type Safari struct {
	path string
	att  contracts.Attestor
	open Opener
}

func New(cfg Config) (*Safari, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("safari: Config.Attestor required")
	}
	path := cfg.DBPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("safari: resolve home: %w", err)
		}
		path = home + "/Library/Safari/History.db"
	}
	open := cfg.Open
	if open == nil {
		open = sql.Open
	}
	return &Safari{path: path, att: cfg.Attestor, open: open}, nil
}

func (s *Safari) Name() string { return "Safari" }

func (s *Safari) Available(_ context.Context) bool {
	f, err := os.Open(s.path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (s *Safari) Sync(_ context.Context) error { return nil }

// Attest BEFORE open: History.db can carry private-browsing residue, so a
// denied operator must never attach the store.
func (s *Safari) Query(ctx context.Context, q Query) ([]Item, error) {
	if err := s.att.Attest(ctx, Scope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if _, err := os.Stat(s.path); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	db, err := s.open("sqlite", sqliteopen.RODSN(s.path))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	defer func() { _ = db.Close() }()
	return queryItems(ctx, db, q)
}

// cocoaEpoch is 2001-01-01 UTC; Safari stores visit_time as REAL seconds
// since this anchor (Foundation's NSDate reference date).
var cocoaEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

func queryItems(ctx context.Context, db *sql.DB, q Query) ([]Item, error) {
	where := []string{"1=1"}
	var args []any
	if !q.Since.IsZero() {
		where = append(where, "v.visit_time >= ?")
		args = append(args, q.Since.Sub(cocoaEpoch).Seconds())
	}
	if !q.Until.IsZero() {
		where = append(where, "v.visit_time < ?")
		args = append(args, q.Until.Sub(cocoaEpoch).Seconds())
	}
	if q.Text != "" {
		where = append(where, "(v.title LIKE ? OR i.url LIKE ?)")
		args = append(args, "%"+q.Text+"%", "%"+q.Text+"%")
	}
	sqlStr := `SELECT i.url, COALESCE(v.title,''), v.visit_time, i.visit_count
	           FROM history_visits v
	           JOIN history_items i ON i.id = v.history_item
	           WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY v.visit_time ASC`
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
	for rows.Next() {
		var (
			u, title   string
			visitCocoa float64
			visitCount int
		)
		if err := rows.Scan(&u, &title, &visitCocoa, &visitCount); err != nil {
			return nil, fmt.Errorf("%w: scan: %v", ErrSourceUnavailable, err)
		}
		out = append(out, Item{
			URL:        u,
			Title:      title,
			VisitTime:  cocoaEpoch.Add(time.Duration(visitCocoa * float64(time.Second))),
			VisitCount: visitCount,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: rows: %v", ErrSourceUnavailable, err)
	}
	return out, nil
}
