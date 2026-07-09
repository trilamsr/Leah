package feeds

import (
	"sync"
	"time"
)

// Cache is the TTL-bounded read-through surface synthesizers + the morning
// brief share. SQLite-backed implementation is a follow-up; the current
// in-memory map gives the weather adapter a place to land without a schema.
type Cache interface {
	Get(key string) (any, bool)
	Set(key string, val any, ttl time.Duration)
}

// MemCache is a process-local sync.Mutex-guarded map. Expiry is lazy — a stale
// entry is only evicted when next Get'd; bounded memory is a follow-up.
type MemCache struct {
	mu      sync.Mutex
	entries map[string]memEntry
	now     func() time.Time
}

type memEntry struct {
	val     any
	expires time.Time
}

// NewMemCache returns a fresh in-memory cache. Constructor (vs zero-value)
// keeps the lazy-init seam available if SQLite swap-in needs setup.
func NewMemCache() *MemCache {
	return &MemCache{entries: make(map[string]memEntry), now: time.Now}
}

// Get returns the cached value or ok=false on miss / expired. Expired entries
// are dropped on read so a hot key doesn't pin freed memory.
func (c *MemCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if c.now().After(e.expires) {
		delete(c.entries, key)
		return nil, false
	}
	return e.val, true
}

// Set writes val with a TTL; non-positive ttl writes an already-expired entry
// (callers can use that to invalidate). Compile-time guard on the Cache iface.
func (c *MemCache) Set(key string, val any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = memEntry{val: val, expires: c.now().Add(ttl)}
}

var _ Cache = (*MemCache)(nil)
