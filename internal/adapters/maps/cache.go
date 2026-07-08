package maps

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrCacheInvalidScope = errors.New("maps cache: empty scope")
	ErrCacheClosed       = errors.New("maps cache: closed")
)

var defaultTTLs = map[string]time.Duration{
	ScopeGeocode: 30 * 24 * time.Hour,
	ScopeRoute:   5 * time.Minute,
	ScopePOI:     6 * time.Hour,
	ScopeDetails: 24 * time.Hour,
	ScopeTraffic: 60 * time.Second,
}

// TTLFor — unknown scopes default to 5min so a new RPC never silently serves stale.
func TTLFor(scope string) time.Duration {
	if d, ok := defaultTTLs[scope]; ok {
		return d
	}
	return 5 * time.Minute
}

type Cache struct {
	db     *sql.DB
	mu     sync.Mutex
	now    func() time.Time
	stop   chan struct{}
	stopWG sync.WaitGroup
}

func OpenCache(path string) (*Cache, error) {
	if path == "" {
		return nil, errors.New("maps cache: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("maps cache: mkdir: %w", err)
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("maps cache: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("maps cache: ping: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS entries (
			scope     TEXT NOT NULL,
			key_hash  TEXT NOT NULL,
			value     BLOB NOT NULL,
			expires_at INTEGER NOT NULL,
			PRIMARY KEY(scope, key_hash)
		)`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("maps cache: schema: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("maps cache: chmod: %w", err)
	}
	c := &Cache{db: db, now: time.Now, stop: make(chan struct{})}
	c.startReaper(10 * time.Minute)
	return c, nil
}

// startReaper drains expired rows in the background so cold entries do not
// accumulate. Purge-on-read only removes the row when the same key is queried;
// keys never queried again would otherwise grow the DB unbounded.
func (c *Cache) startReaper(interval time.Duration) {
	c.stopWG.Add(1)
	go func() {
		defer c.stopWG.Done()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-c.stop:
				return
			case <-t.C:
				_ = c.ReapExpired(context.Background())
			}
		}
	}()
}

// ReapExpired deletes every row whose expires_at is in the past. Safe to call
// concurrently with Get/Set — SQLite serialises through the shared DB handle.
func (c *Cache) ReapExpired(ctx context.Context) error {
	if c == nil || c.db == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.ExecContext(ctx, `DELETE FROM entries WHERE expires_at <= ?`, c.now().UnixNano())
	if err != nil {
		return fmt.Errorf("maps cache: reap: %w", err)
	}
	return nil
}

func defaultCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".leah-state", "maps-cache.db"), nil
}

func (c *Cache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	if c.stop != nil {
		select {
		case <-c.stop:
		default:
			close(c.stop)
		}
		c.stopWG.Wait()
	}
	return c.db.Close()
}

func (c *Cache) Get(ctx context.Context, scope, key string) ([]byte, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	if scope == "" {
		return nil, false, ErrCacheInvalidScope
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var value []byte
	var expiresAt int64
	row := c.db.QueryRowContext(ctx, `SELECT value, expires_at FROM entries WHERE scope = ? AND key_hash = ?`, scope, key)
	if err := row.Scan(&value, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("maps cache: select: %w", err)
	}
	if c.now().UnixNano() >= expiresAt {
		_, _ = c.db.ExecContext(ctx, `DELETE FROM entries WHERE scope = ? AND key_hash = ?`, scope, key)
		return nil, false, nil
	}
	return value, true, nil
}

func (c *Cache) Set(ctx context.Context, scope, key string, value []byte, ttl time.Duration) error {
	if c == nil {
		return nil
	}
	if scope == "" {
		return ErrCacheInvalidScope
	}
	if ttl <= 0 {
		return nil
	}
	expires := c.now().Add(ttl).UnixNano()
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO entries(scope, key_hash, value, expires_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(scope, key_hash) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
		scope, key, value, expires)
	if err != nil {
		return fmt.Errorf("maps cache: upsert: %w", err)
	}
	return nil
}

func keyHash(parts ...any) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			_, _ = h.Write([]byte{0})
		}
		switch v := p.(type) {
		case string:
			_, _ = h.Write([]byte(v))
		case int:
			_, _ = h.Write([]byte(strconv.Itoa(v)))
		case int64:
			_, _ = h.Write([]byte(strconv.FormatInt(v, 10)))
		case float64:
			_, _ = h.Write([]byte(strconv.FormatFloat(v, 'f', -1, 64)))
		default:
			_, _ = fmt.Fprintf(h, "%v", v)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
