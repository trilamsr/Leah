package router

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// dbConsent is the DB-backed ConsentStore. Persistent grants survive across
// process restarts via the vision_consent table (shipped by T05); session and
// until-quit grants live only in the in-memory cache. Granted() is cache-only
// so the hot path stays lock+map — DB never touched per Ask.
//
// Schema mapping: ConsentScope ↔ scope TEXT —
//
//	ScopeThisSession → "this_session"
//	ScopeUntilQuit   → "until_quit"
//	ScopePersistent  → "persistent"
type dbConsent struct {
	db *sql.DB
	mu sync.Mutex
	m  map[string]ConsentScope
}

func scopeToString(s ConsentScope) string {
	switch s {
	case ScopeThisSession:
		return "this_session"
	case ScopeUntilQuit:
		return "until_quit"
	case ScopePersistent:
		return "persistent"
	}
	return "this_session"
}

// NewDBConsent returns a ConsentStore backed by db.vision_consent for the
// Persistent scope. Session/UntilQuit grants stay in-memory by design — they
// MUST NOT persist across restarts. Init loads existing persistent rows so
// Granted() reflects the prior process's choices on first access.
func NewDBConsent(db *sql.DB) (ConsentStore, error) {
	c := &dbConsent{db: db, m: map[string]ConsentScope{}}
	if err := c.loadPersistent(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *dbConsent) loadPersistent() error {
	rows, err := c.db.Query(`SELECT mode FROM vision_consent WHERE scope='persistent'`)
	if err != nil {
		return fmt.Errorf("load persistent consent: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var mode string
		if err := rows.Scan(&mode); err != nil {
			return fmt.Errorf("scan consent: %w", err)
		}
		c.m[mode] = ScopePersistent
	}
	return rows.Err()
}

func (c *dbConsent) Granted(mode string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.m[mode]
	return ok
}

// Grant writes Persistent scopes through to the DB (replacing any prior row
// for the same mode) and clears any prior persistent row when downgrading to
// Session/UntilQuit — otherwise a downgrade would leave a stale persistent
// row that re-grants the mode on next process start.
func (c *dbConsent) Grant(mode string, scope ConsentScope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[mode] = scope
	if scope == ScopePersistent {
		// DELETE+INSERT keeps the row count at 1 per mode without depending
		// on a UNIQUE index that the T05 schema doesn't declare.
		if _, err := c.db.Exec(`DELETE FROM vision_consent WHERE mode=?`, mode); err != nil {
			slog.Default().Error("vision: consent persist DELETE failed",
				"mode", mode, "err", err)
			return
		}
		if _, err := c.db.Exec(
			`INSERT INTO vision_consent(mode, granted_at, scope) VALUES(?,?,?)`,
			mode, time.Now().Unix(), scopeToString(scope),
		); err != nil {
			slog.Default().Error("vision: consent persist INSERT failed",
				"mode", mode, "err", err)
		}
	} else {
		// Downgrade: drop any pre-existing persistent row so the lower-tier
		// grant is the source of truth on next process start.
		if _, err := c.db.Exec(`DELETE FROM vision_consent WHERE mode=? AND scope='persistent'`, mode); err != nil {
			slog.Default().Error("vision: consent downgrade DELETE failed",
				"mode", mode, "err", err)
		}
	}
}

func (c *dbConsent) Revoke(mode string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, mode)
	if _, err := c.db.Exec(`DELETE FROM vision_consent WHERE mode=?`, mode); err != nil {
		slog.Default().Error("vision: consent revoke DELETE failed",
			"mode", mode, "err", err)
	}
}
