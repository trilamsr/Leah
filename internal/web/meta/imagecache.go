package meta

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const DefaultCacheCapBytes = 200 * 1024 * 1024

type cacheEntry struct {
	url  string
	path string
	size int64
}

type ImageCache struct {
	mu    sync.Mutex
	dir   string
	cap   int64
	used  int64
	order *list.List
	index map[string]*list.Element
}

func NewImageCache(dir string, capBytes int64) (*ImageCache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("imagecache: mkdir: %w", err)
	}
	return &ImageCache{
		dir:   dir,
		cap:   capBytes,
		order: list.New(),
		index: map[string]*list.Element{},
	}, nil
}

func keyForURL(url string) string {
	h := sha256.Sum256([]byte(url))
	return hex.EncodeToString(h[:])
}

func (c *ImageCache) pathFor(url string) string {
	return filepath.Join(c.dir, keyForURL(url))
}

func (c *ImageCache) Get(url string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.index[url]
	if !ok {
		return "", false
	}
	c.order.MoveToFront(el)
	return el.Value.(*cacheEntry).path, true
}

func (c *ImageCache) Put(url, mime string, body []byte) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := c.pathFor(url)
	if el, ok := c.index[url]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*cacheEntry).path, nil
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", fmt.Errorf("imagecache: write: %w", err)
	}
	size := int64(len(body))
	entry := &cacheEntry{url: url, path: path, size: size}
	el := c.order.PushFront(entry)
	c.index[url] = el
	c.used += size
	c.evictLocked()
	return path, nil
}

func (c *ImageCache) evictLocked() {
	for c.used > c.cap && c.order.Len() > 0 {
		back := c.order.Back()
		if back == nil {
			return
		}
		ent := back.Value.(*cacheEntry)
		c.order.Remove(back)
		delete(c.index, ent.url)
		c.used -= ent.size
		_ = os.Remove(ent.path)
	}
}
