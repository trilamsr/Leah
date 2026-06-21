package calendar

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

// Scope for operator attestation on every Query call.
const Scope = "macos:calendar:query"

var (
	ErrPermissionDenied  = errors.New("calendar: permission denied")
	ErrSourceUnavailable = errors.New("calendar: source unavailable")
	ErrAttestationDenied = errors.New("calendar: attestation denied")
)

// MacApp is the per-app adapter contract from spec §6. Defined here as the
// first macOS package; subsequent adapters (W26+) will lift it into a shared
// internal/macos package once a second implementer exists.
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

// Opener abstracts sql.Open so tests can count DB opens and assert the DSN.
// Production callers leave it nil; the package defaults to database/sql.
type Opener func(driverName, dsn string) (*sql.DB, error)

type Config struct {
	// DBPath defaults to ~/Library/Calendars/Calendar Cache.
	DBPath   string
	Attestor contracts.Attestor
	Open     Opener
}

type Calendar struct {
	path string
	att  contracts.Attestor
	open Opener
}

func New(cfg Config) (*Calendar, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("calendar: Config.Attestor required")
	}
	path := cfg.DBPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("calendar: resolve home: %w", err)
		}
		path = home + "/Library/Calendars/Calendar Cache"
	}
	open := cfg.Open
	if open == nil {
		open = sql.Open
	}
	return &Calendar{path: path, att: cfg.Attestor, open: open}, nil
}

func (c *Calendar) Name() string { return "Calendar" }

// Available is a fail-fast probe — never errors so leah init can sweep.
func (c *Calendar) Available(_ context.Context) bool {
	f, err := os.Open(c.path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// Sync is a no-op in W25; the mirror lands in W26.
func (c *Calendar) Sync(_ context.Context) error { return nil }

// Query attests BEFORE opening the DB so a denied operator never touches
// the Apple store; opens read-only + immutable per spec §4.
func (c *Calendar) Query(ctx context.Context, q Query) ([]Item, error) {
	if err := c.att.Attest(ctx, Scope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if _, err := os.Stat(c.path); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	db, err := c.open("sqlite", sqliteopen.RODSN(c.path))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	defer func() { _ = db.Close() }()
	return queryItems(ctx, db, q)
}

// macEpoch — Calendar.app stores ZSTARTDATE/ZENDDATE as seconds since
// 2001-01-01 UTC (CoreData absolute time).
var macEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

func queryItems(ctx context.Context, db *sql.DB, q Query) ([]Item, error) {
	var (
		where []string
		args  []any
	)
	if !q.Since.IsZero() {
		where = append(where, "ZSTARTDATE >= ?")
		args = append(args, q.Since.Sub(macEpoch).Seconds())
	}
	if !q.Until.IsZero() {
		where = append(where, "ZSTARTDATE < ?")
		args = append(args, q.Until.Sub(macEpoch).Seconds())
	}
	if q.Text != "" {
		where = append(where, "(ZSUMMARY LIKE ? OR ZNOTES LIKE ?)")
		like := "%" + q.Text + "%"
		args = append(args, like, like)
	}
	sqlStr := `SELECT ZUNIQUEIDENTIFIER, COALESCE(ZSUMMARY,''), COALESCE(ZNOTES,''), COALESCE(ZSTARTDATE,0)
	           FROM ZCALENDARITEM`
	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}
	sqlStr += " ORDER BY ZSTARTDATE ASC"
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
			id, title, body string
			startSec        float64
		)
		if err := rows.Scan(&id, &title, &body, &startSec); err != nil {
			return nil, fmt.Errorf("%w: scan: %v", ErrSourceUnavailable, err)
		}
		out = append(out, Item{
			ID:        "calendar:" + id,
			Title:     title,
			Body:      body,
			Timestamp: macEpoch.Add(time.Duration(startSec * float64(time.Second))),
			Source:    "calendar",
		})
	}
	return out, rows.Err()
}
