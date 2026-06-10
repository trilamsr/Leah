package messages

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/trilam/leah/internal/audit"
	"github.com/trilam/leah/internal/contracts"
)

// Scope for operator attestation on every Query call. Spec §7 places this
// in the high-sensitivity scope set alongside macos:photos:read.
const Scope = "macos:messages:query"

var (
	ErrPermissionDenied  = errors.New("messages: permission denied")
	ErrSourceUnavailable = errors.New("messages: source unavailable")
	ErrAttestationDenied = errors.New("messages: attestation denied")
)

// MacApp mirrors the per-app contract from spec §6. Same shape as the
// reminders + calendar adapters; the third reader keeps the duplication
// to defer abstraction until the pattern stabilizes.
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

// Opener abstracts sql.Open so tests can assert the DSN and count opens.
type Opener func(driverName, dsn string) (*sql.DB, error)

// AuditSink is the narrow audit interface this package needs. *audit.Logger
// satisfies it; tests inject an in-memory capture.
type AuditSink interface {
	Append(e audit.Entry) error
}

type Config struct {
	// DBPath defaults to ~/Library/Messages/chat.db.
	DBPath   string
	Attestor contracts.Attestor
	Open     Opener
	// Audit, when non-nil, receives one row per returned Item with the
	// body hashed (sha256[:8]) — never plaintext (spec §7 row "Photos +
	// Messages content = HIGHEST sensitivity").
	Audit AuditSink
}

type Messages struct {
	path  string
	att   contracts.Attestor
	open  Opener
	audit AuditSink
}

func New(cfg Config) (*Messages, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("messages: Config.Attestor required")
	}
	path := cfg.DBPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("messages: resolve home: %w", err)
		}
		path = home + "/Library/Messages/chat.db"
	}
	open := cfg.Open
	if open == nil {
		open = sql.Open
	}
	return &Messages{path: path, att: cfg.Attestor, open: open, audit: cfg.Audit}, nil
}

func (m *Messages) Name() string { return "Messages" }

// Available is a fail-fast probe — never errors so leah init can sweep.
func (m *Messages) Available(_ context.Context) bool {
	f, err := os.Open(m.path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// Sync is a no-op in W30; the mirror lands in a later wave.
func (m *Messages) Sync(_ context.Context) error { return nil }

// Query attests BEFORE opening the DB so a denied operator never touches
// chat.db; opens read-only + immutable per spec §4.
func (m *Messages) Query(ctx context.Context, q Query) ([]Item, error) {
	if err := m.att.Attest(ctx, Scope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	if _, err := os.Stat(m.path); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	db, err := m.open("sqlite", roDSN(m.path))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSourceUnavailable, err)
	}
	defer func() { _ = db.Close() }()
	items, err := queryItems(ctx, db, q)
	if err != nil {
		return nil, err
	}
	if m.audit != nil {
		for _, it := range items {
			// Privacy invariant: Detail carries only the body HASH, never
			// the body bytes. If this row ever shows plaintext, the
			// TestMessages_Audit_HashesBody guard must fail loudly.
			_ = m.audit.Append(audit.Entry{
				Kind:        "macos_messages_query",
				BlastRadius: 0,
				Outcome:     "ok",
				Detail:      fmt.Sprintf("id=%s bodyhash=%s", it.ID, hashBody(it.Body)),
			})
		}
	}
	return items, nil
}

// hashBody returns the first 8 hex chars (4 bytes) of sha256(body) — a
// reversible fingerprint is unacceptable for audit; truncated hash gives
// us "did two queries see the same body" without leaking content.
func hashBody(b string) string {
	sum := sha256.Sum256([]byte(b))
	return hex.EncodeToString(sum[:])[:8]
}

func roDSN(path string) string {
	return fmt.Sprintf(
		"file:%s?mode=ro&immutable=1&_journal_mode=OFF&_query_only=1",
		url.PathEscape(path),
	)
}

// macEpoch — Messages.app stores message.date as nanoseconds since
// 2001-01-01 UTC on macOS 10.13+. Legacy rows (pre-High Sierra) use
// seconds; we detect by magnitude (<1e10 = seconds).
var macEpoch = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

func macTime(raw int64) time.Time {
	if raw == 0 {
		return time.Time{}
	}
	if raw < 1_000_000_000_0 { // pre-2286 in seconds → must be seconds
		return macEpoch.Add(time.Duration(raw) * time.Second)
	}
	return macEpoch.Add(time.Duration(raw))
}

func queryItems(ctx context.Context, db *sql.DB, q Query) ([]Item, error) {
	var (
		where []string
		args  []any
	)
	if !q.Since.IsZero() {
		where = append(where, "m.date >= ?")
		args = append(args, q.Since.Sub(macEpoch).Nanoseconds())
	}
	if !q.Until.IsZero() {
		where = append(where, "m.date < ?")
		args = append(args, q.Until.Sub(macEpoch).Nanoseconds())
	}
	if q.ContactRef != "" {
		where = append(where, "h.id = ?")
		args = append(args, q.ContactRef)
	}
	if q.Text != "" {
		where = append(where, "m.text LIKE ?")
		args = append(args, "%"+q.Text+"%")
	}
	sqlStr := `SELECT m.ROWID, COALESCE(m.text,''), COALESCE(m.date,0), COALESCE(h.id,'')
	           FROM message m LEFT JOIN handle h ON m.handle_id = h.ROWID`
	if len(where) > 0 {
		sqlStr += " WHERE " + strings.Join(where, " AND ")
	}
	sqlStr += " ORDER BY m.date ASC"
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
			rowID     int64
			body, hdl string
			dateRaw   int64
		)
		if err := rows.Scan(&rowID, &body, &dateRaw, &hdl); err != nil {
			return nil, fmt.Errorf("%w: scan: %v", ErrSourceUnavailable, err)
		}
		it := Item{
			ID:        fmt.Sprintf("messages:%d", rowID),
			Body:      body,
			Timestamp: macTime(dateRaw),
			Source:    "messages",
		}
		if hdl != "" {
			it.ContactRefs = []string{hdl}
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
