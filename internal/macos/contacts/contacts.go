// Package contacts is the macOS Contacts.app read adapter — SQLite read-only
// over ~/Library/Application Support/AddressBook/AddressBook-v22.abcddb,
// gated by operator attestation at the macos:contacts:query scope.
package contacts

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

var (
	ErrAttestationDenied = errors.New("contacts: attestation denied")
	ErrSourceUnavailable = errors.New("contacts: source unavailable")
)

const ScopeQuery = "macos:contacts:query"

// Item mirrors the spec §6 cross-app normalized shape so the future
// internal/macos package can hoist it without changing this adapter.
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
	Limit int
}

type Config struct {
	Attestor contracts.Attestor
	DBPath   string
}

type Adapter struct {
	att    contracts.Attestor
	dbPath string
}

func New(cfg Config) (*Adapter, error) {
	if cfg.Attestor == nil {
		return nil, errors.New("contacts: Config.Attestor required")
	}
	if cfg.DBPath == "" {
		return nil, errors.New("contacts: Config.DBPath required")
	}
	return &Adapter{att: cfg.Attestor, dbPath: cfg.DBPath}, nil
}

func (a *Adapter) Name() string { return "Contacts" }

func (a *Adapter) Available(_ context.Context) bool {
	st, err := os.Stat(a.dbPath)
	return err == nil && !st.IsDir()
}

// Query gates on attestation BEFORE opening the DB — a denied operator must
// not see a sqlite handle materialize even briefly.
func (a *Adapter) Query(ctx context.Context, q Query) ([]Item, error) {
	if err := a.att.Attest(ctx, ScopeQuery); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttestationDenied, err)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}

	db, err := sql.Open("sqlite", sqliteopen.RODSN(a.dbPath))
	if err != nil {
		return nil, fmt.Errorf("%w: open: %v", ErrSourceUnavailable, err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.QueryContext(ctx, `
		SELECT Z_PK, COALESCE(ZFIRSTNAME,''), COALESCE(ZLASTNAME,''), COALESCE(ZORGANIZATION,'')
		FROM ZABCDRECORD
		ORDER BY Z_PK
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: select: %v", ErrSourceUnavailable, err)
	}
	defer func() { _ = rows.Close() }()

	var items []Item
	for rows.Next() {
		var pk int64
		var first, last, org string
		if err := rows.Scan(&pk, &first, &last, &org); err != nil {
			return nil, fmt.Errorf("%w: scan: %v", ErrSourceUnavailable, err)
		}
		title := strings.TrimSpace(first + " " + last)
		if title == "" {
			title = org
		}
		items = append(items, Item{
			ID:     fmt.Sprintf("contacts:%d", pk),
			Title:  title,
			Source: "contacts",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: rows: %v", ErrSourceUnavailable, err)
	}

	for i := range items {
		refs, err := contactRefs(ctx, db, idOf(items[i].ID))
		if err != nil {
			return nil, fmt.Errorf("%w: refs: %v", ErrSourceUnavailable, err)
		}
		items[i].ContactRefs = refs
	}
	return items, nil
}

func idOf(adapterID string) int64 {
	var n int64
	_, _ = fmt.Sscanf(adapterID, "contacts:%d", &n)
	return n
}

func contactRefs(ctx context.Context, db *sql.DB, pk int64) ([]string, error) {
	var refs []string
	emails, err := db.QueryContext(ctx, `SELECT ZADDRESS FROM ZABCDEMAILADDRESS WHERE ZOWNER=? AND ZADDRESS IS NOT NULL`, pk)
	if err != nil {
		return nil, err
	}
	for emails.Next() {
		var s string
		if err := emails.Scan(&s); err != nil {
			_ = emails.Close()
			return nil, err
		}
		refs = append(refs, s)
	}
	_ = emails.Close()
	if err := emails.Err(); err != nil {
		return nil, err
	}

	phones, err := db.QueryContext(ctx, `SELECT ZFULLNUMBER FROM ZABCDPHONENUMBER WHERE ZOWNER=? AND ZFULLNUMBER IS NOT NULL`, pk)
	if err != nil {
		return nil, err
	}
	defer func() { _ = phones.Close() }()
	for phones.Next() {
		var s string
		if err := phones.Scan(&s); err != nil {
			return nil, err
		}
		refs = append(refs, s)
	}
	return refs, phones.Err()
}
