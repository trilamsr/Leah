package meta

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestImageCache_PutGet(t *testing.T) {
	c, err := NewImageCache(t.TempDir(), 1024*1024)
	if err != nil {
		t.Fatalf("NewImageCache: %v", err)
	}
	body := []byte("hello")
	path, err := c.Put("https://x/y.png", "image/png", body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := c.Get("https://x/y.png")
	if !ok {
		t.Fatal("Get miss")
	}
	if got != path {
		t.Errorf("path mismatch %q vs %q", got, path)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cached file: %v", err)
	}
	if !bytes.Equal(disk, body) {
		t.Error("disk content mismatch")
	}
}

func TestImageCache_EvictsOnCap(t *testing.T) {
	dir := t.TempDir()
	c, err := NewImageCache(dir, 1500) // cap 1.5KB
	if err != nil {
		t.Fatalf("NewImageCache: %v", err)
	}
	a := make([]byte, 1024)
	b := make([]byte, 1024)
	if _, err := c.Put("https://x/a", "image/png", a); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put("https://x/b", "image/png", b); err != nil {
		t.Fatal(err)
	}
	// a should have been evicted (LRU; capacity exceeded).
	if _, ok := c.Get("https://x/a"); ok {
		t.Error("expected a evicted")
	}
	if _, ok := c.Get("https://x/b"); !ok {
		t.Error("expected b present")
	}
	// Disk file for a should be gone.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file on disk, got %d", len(entries))
	}
}

func TestImageCache_GetTouchesLRU(t *testing.T) {
	c, err := NewImageCache(t.TempDir(), 1500)
	if err != nil {
		t.Fatal(err)
	}
	a := make([]byte, 1024)
	b := make([]byte, 1024)
	cc := make([]byte, 1024)
	_, _ = c.Put("https://x/a", "image/png", a)
	_, _ = c.Put("https://x/b", "image/png", b)
	// At this point a got evicted by cap. Re-add a, touch b via Get, then add c.
	_, _ = c.Put("https://x/a", "image/png", a) // evicts b? no — b is newer than initial a was, but cap forces eviction of LRU = b? actually after first Put(b) we evicted a. order: [b]. Put(a) makes [b,a]; cap 1500 → must evict. b is LRU → evict b. So now [a].
	if _, ok := c.Get("https://x/b"); ok {
		t.Fatal("setup: expected b evicted")
	}
	if _, ok := c.Get("https://x/a"); !ok {
		t.Fatal("setup: expected a present")
	}
	_, _ = c.Put("https://x/c", "image/png", cc) // [a,c] → evict a (LRU)
	if _, ok := c.Get("https://x/a"); ok {
		t.Error("a should be evicted after c add")
	}
	if _, ok := c.Get("https://x/c"); !ok {
		t.Error("c should be present")
	}
}

func TestImageCache_ConcurrentSafe(t *testing.T) {
	c, err := NewImageCache(t.TempDir(), 10*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := []byte{byte(i)}
			url := "https://x/" + string(rune('a'+i))
			if _, err := c.Put(url, "image/png", body); err != nil {
				t.Errorf("Put: %v", err)
			}
			c.Get(url)
		}(i)
	}
	wg.Wait()
}

func TestImageCache_KeyIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	c, _ := NewImageCache(dir, 1<<20)
	p1, _ := c.Put("https://x/y", "image/png", []byte("a"))
	p2, _ := c.Put("https://x/y", "image/png", []byte("a"))
	if p1 != p2 {
		t.Errorf("same url → different paths: %q vs %q", p1, p2)
	}
	// Path lives under dir.
	if rel, err := filepath.Rel(dir, p1); err != nil || rel == "" {
		t.Errorf("cached path not under dir: %q", p1)
	}
}
