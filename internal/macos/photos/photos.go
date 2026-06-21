// Package photos: read-only SQLite reader for Photos.app (W22m, spec §7
// HIGHEST). Metadata only; audit rows hash UUID and elide lat/lng.
package photos

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/contracts"
	"github.com/trilam/leah/internal/macos/sqliteopen"
)

const Scope = "macos:photos:query"

var (
	ErrSourceUnavailable = errors.New("photos: source unavailable")
	ErrAttestationDenied = errors.New("photos: attestation denied")
)

type MacApp interface {
	Name() string
	Available(ctx context.Context) bool
	Sync(ctx context.Context) error
	Query(ctx context.Context, q Query) ([]Item, error)
}

// Latitude/Longitude pointer-typed so NULL is distinguishable from (0,0).
type Item struct {
	ID        string
	UUID      string
	Filename  string
	Timestamp time.Time
	Latitude  *float64
	Longitude *float64
	Source    string
}

type Query struct {
	Since, Until time.Time
	Limit        int
}

type Opener func(driverName, dsn string) (*sql.DB, error)

type AuditSink interface {
	Append(e audit.Entry) error
}

type Config struct {
	DBPath   string
	Attestor contracts.Attestor
	Open     Opener
	Audit    AuditSink
}

type Photos struct {
	path  string
	att   contracts.Attestor
	open  Opener
	audit AuditSink
}

func New(cfg Config) (*Photos, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("photos: Config.Attestor required")
	}
	path := cfg.DBPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("photos: resolve home: %w", err)
		}
		path = home + "/Pictures/Photos Library.photoslibrary/database/Photos.sqlite"
	}
	open := cfg.Open
	if open == nil {
		open = sql.Open
	}
	return &Photos{path: path, att: cfg.Attestor, open: open, audit: cfg.Audit}, nil
}

func (p *Photos) Name() string { return "Photos" }

func (p *Photos) Available(_ context.Context) bool {
	f, err := os.Open(p.path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (p *Photos) Sync(_ context.Context) error { return nil }

// Attest BEFORE open: denied operator never attaches Photos.sqlite.
func (p *Photos) Query(ctx context.Context, q Query) ([]Item, error) {
	if err := p.att.Attest(ctx, Scope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if _, err := os.Stat(p.path); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	db, err := p.open("sqlite", sqliteopen.RODSN(p.path))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	defer func() { _ = db.Close() }()
	items, err := queryItems(ctx, db, q)
	if err != nil {
		return nil, err
	}
	if p.audit != nil {
		for _, it := range items {
			// Detail: uuidhash + hasloc only; plaintext UUID/lat/lng forbidden.
			_ = p.audit.Append(audit.Entry{
				Kind:        "macos_photos_query",
				BlastRadius: 0,
				Outcome:     "ok",
				Detail: fmt.Sprintf("uuidhash=%s hasloc=%t",
					hashUUID(it.UUID), it.Latitude != nil),
			})
		}
	}
	return items, nil
}

func hashUUID(u string) string {
	sum := sha256.Sum256([]byte(u))
	return hex.EncodeToString(sum[:])[:8]
}

var macEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

// LEFT JOIN so mid-iCloud-sync orphans surface with empty Filename.
func queryItems(ctx context.Context, db *sql.DB, q Query) ([]Item, error) {
	var (
		where []string
		args  []any
	)
	if !q.Since.IsZero() {
		where = append(where, "a.ZDATECREATED >= ?")
		args = append(args, q.Since.Sub(macEpoch).Seconds())
	}
	if !q.Until.IsZero() {
		where = append(where, "a.ZDATECREATED < ?")
		args = append(args, q.Until.Sub(macEpoch).Seconds())
	}
	sqlStr := `SELECT COALESCE(a.ZUUID,''), COALESCE(a.ZDATECREATED,0),
	                  a.ZLATITUDE, a.ZLONGITUDE,
	                  COALESCE(x.ZORIGINALFILENAME,'')
	           FROM ZASSET a
	           LEFT JOIN ZADDITIONALASSETATTRIBUTES x
	             ON a.ZADDITIONALATTRIBUTES = x.Z_PK`
	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}
	sqlStr += " ORDER BY a.ZDATECREATED ASC"
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
			uuid, fn string
			dateSec  float64
			lat, lng sql.NullFloat64
		)
		if err := rows.Scan(&uuid, &dateSec, &lat, &lng, &fn); err != nil {
			return nil, fmt.Errorf("%w: scan: %v", ErrSourceUnavailable, err)
		}
		it := Item{
			ID:        "photos:" + uuid,
			UUID:      uuid,
			Filename:  fn,
			Timestamp: macEpoch.Add(time.Duration(dateSec * float64(time.Second))),
			Source:    "photos",
		}
		if lat.Valid && lng.Valid {
			la, lo := lat.Float64, lng.Float64
			it.Latitude, it.Longitude = &la, &lo
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
