package reminders

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/contracts"
)

// Scope for operator attestation on every Query call.
const Scope = "macos:reminders:query"

var (
	ErrPermissionDenied  = errors.New("reminders: permission denied")
	ErrSourceUnavailable = errors.New("reminders: source unavailable")
	ErrAttestationDenied = errors.New("reminders: attestation denied")
)

// MacApp is the per-app adapter contract from spec §6. Calendar declares
// the same shape today; a shared internal/macos/macapp.go lift waits for
// the third implementer to avoid an abstraction before the pattern stabilizes.
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
type Opener func(driverName, dsn string) (*sql.DB, error)

type Config struct {
	// DBPath defaults to
	// ~/Library/Group Containers/group.com.apple.reminders/Container_v1/Stores/Data-local.sqlite.
	DBPath   string
	Attestor contracts.Attestor
	Open     Opener
}

type Reminders struct {
	path string
	att  contracts.Attestor
	open Opener
}

func New(cfg Config) (*Reminders, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("reminders: Config.Attestor required")
	}
	path := cfg.DBPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("reminders: resolve home: %w", err)
		}
		path = home + "/Library/Group Containers/group.com.apple.reminders/Container_v1/Stores/Data-local.sqlite"
	}
	open := cfg.Open
	if open == nil {
		open = sql.Open
	}
	return &Reminders{path: path, att: cfg.Attestor, open: open}, nil
}

func (r *Reminders) Name() string { return "Reminders" }

// Available is a fail-fast probe — never errors so leah init can sweep.
func (r *Reminders) Available(_ context.Context) bool {
	f, err := os.Open(r.path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// Sync is a no-op in W26; the mirror lands in W27.
func (r *Reminders) Sync(_ context.Context) error { return nil }

// Query attests BEFORE opening the DB so a denied operator never touches
// the Apple store; opens read-only + immutable per spec §4.
func (r *Reminders) Query(ctx context.Context, q Query) ([]Item, error) {
	if err := r.att.Attest(ctx, Scope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if _, err := os.Stat(r.path); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	db, err := r.open("sqlite", roDSN(r.path))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	defer func() { _ = db.Close() }()
	return queryItems(ctx, db, q)
}

// roDSN — immutable=1 sidesteps WAL-recovery contention with iCloud-sync
// writes (spec §4, §7 risk row 1).
func roDSN(path string) string {
	return fmt.Sprintf(
		"file:%s?mode=ro&immutable=1&_journal_mode=OFF&_query_only=1",
		url.PathEscape(path),
	)
}

// macEpoch — Reminders.app stores ZDUEDATE as seconds since 2001-01-01 UTC
// (CoreData absolute time), same convention as Calendar.
var macEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

func queryItems(ctx context.Context, db *sql.DB, q Query) ([]Item, error) {
	var (
		where []string
		args  []any
	)
	if !q.Since.IsZero() {
		where = append(where, "ZDUEDATE >= ?")
		args = append(args, q.Since.Sub(macEpoch).Seconds())
	}
	if !q.Until.IsZero() {
		where = append(where, "ZDUEDATE < ?")
		args = append(args, q.Until.Sub(macEpoch).Seconds())
	}
	if q.Text != "" {
		where = append(where, "(ZTITLE LIKE ? OR ZNOTES LIKE ?)")
		like := "%" + q.Text + "%"
		args = append(args, like, like)
	}
	sqlStr := `SELECT ZUNIQUEIDENTIFIER, COALESCE(ZTITLE,''), COALESCE(ZNOTES,''), COALESCE(ZDUEDATE,0)
	           FROM ZREMCDREMINDER`
	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}
	sqlStr += " ORDER BY ZDUEDATE ASC"
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
			dueSec          float64
		)
		if err := rows.Scan(&id, &title, &body, &dueSec); err != nil {
			return nil, fmt.Errorf("%w: scan: %v", ErrSourceUnavailable, err)
		}
		out = append(out, Item{
			ID:        "reminders:" + id,
			Title:     title,
			Body:      body,
			Timestamp: macEpoch.Add(time.Duration(dueSec * float64(time.Second))),
			Source:    "reminders",
		})
	}
	return out, rows.Err()
}
