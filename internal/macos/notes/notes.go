// Package notes is a read-only, attestation-gated SQLite reader for Notes.app (W28).
package notes

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

const Scope = "macos:notes:query"

var (
	ErrPermissionDenied  = errors.New("notes: permission denied")
	ErrSourceUnavailable = errors.New("notes: source unavailable")
	ErrAttestationDenied = errors.New("notes: attestation denied")
)

type MacApp interface {
	Name() string
	Available(ctx context.Context) bool
	Sync(ctx context.Context) error
	Query(ctx context.Context, q Query) ([]Item, error)
}

type Item struct {
	ID          string
	Title       string
	Body        string
	Timestamp   time.Time
	Source      string
	ContactRefs []string
	Tags        []string
}

type Query struct {
	Since, Until time.Time
	ContactRef   string
	Text         string
	Limit        int
}

type Opener func(driverName, dsn string) (*sql.DB, error)

type Config struct {
	DBPath   string
	Attestor contracts.Attestor
	Open     Opener
}

type Notes struct {
	path string
	att  contracts.Attestor
	open Opener
}

func New(cfg Config) (*Notes, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("notes: Config.Attestor required")
	}
	path := cfg.DBPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("notes: resolve home: %w", err)
		}
		path = home + "/Library/Group Containers/group.com.apple.notes/NoteStore.sqlite"
	}
	open := cfg.Open
	if open == nil {
		open = sql.Open
	}
	return &Notes{path: path, att: cfg.Attestor, open: open}, nil
}

func (n *Notes) Name() string { return "Notes" }

func (n *Notes) Available(_ context.Context) bool {
	f, err := os.Open(n.path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (n *Notes) Sync(_ context.Context) error { return nil }

// Attest BEFORE open so a denied operator never attaches the Apple store.
func (n *Notes) Query(ctx context.Context, q Query) ([]Item, error) {
	if err := n.att.Attest(ctx, Scope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if _, err := os.Stat(n.path); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	db, err := n.open("sqlite", sqliteopen.RODSN(n.path))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	defer func() { _ = db.Close() }()
	return queryItems(ctx, db, q)
}

var macEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// ZNOTEDATA NOT NULL discriminates note rows from folder/account rows
// in the co-tabled ZICCLOUDSYNCINGOBJECT row store.
func queryItems(ctx context.Context, db *sql.DB, q Query) ([]Item, error) {
	where := []string{"ZNOTEDATA IS NOT NULL"}
	var args []any
	if !q.Since.IsZero() {
		where = append(where, "ZMODIFICATIONDATE1 >= ?")
		args = append(args, q.Since.Sub(macEpoch).Seconds())
	}
	if !q.Until.IsZero() {
		where = append(where, "ZMODIFICATIONDATE1 < ?")
		args = append(args, q.Until.Sub(macEpoch).Seconds())
	}
	if q.Text != "" {
		where = append(where, "ZTITLE1 LIKE ?")
		args = append(args, "%"+q.Text+"%")
	}
	sqlStr := `SELECT COALESCE(ZIDENTIFIER,''), COALESCE(ZTITLE1,''), COALESCE(ZMODIFICATIONDATE1,0)
	           FROM ZICCLOUDSYNCINGOBJECT WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY ZMODIFICATIONDATE1 ASC`
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
			id, title string
			modSec    float64
		)
		if err := rows.Scan(&id, &title, &modSec); err != nil {
			return nil, fmt.Errorf("%w: scan: %v", ErrSourceUnavailable, err)
		}
		out = append(out, Item{
			ID:        "notes:" + id,
			Title:     title,
			Timestamp: macEpoch.Add(time.Duration(modSec * float64(time.Second))),
			Source:    "notes",
		})
	}
	return out, rows.Err()
}
