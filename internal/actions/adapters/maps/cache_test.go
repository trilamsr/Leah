package maps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func tempCachePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "maps-cache.db")
}

func TestCache_OpenCreates0600File(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("file mode semantics differ on windows")
	}
	path := tempCachePath(t)
	c, err := OpenCache(path)
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	defer func() { _ = c.Close() }()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("file mode = %o, want 0600", mode)
	}
}

func TestCache_GetMiss(t *testing.T) {
	t.Parallel()
	c, err := OpenCache(tempCachePath(t))
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	defer func() { _ = c.Close() }()
	if _, ok, err := c.Get(context.Background(), ScopeRoute, "missing"); err != nil || ok {
		t.Fatalf("Get miss: ok=%v err=%v", ok, err)
	}
}

func TestCache_SetGetRoundTrip(t *testing.T) {
	t.Parallel()
	c, err := OpenCache(tempCachePath(t))
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	defer func() { _ = c.Close() }()
	ctx := context.Background()
	payload := []byte(`{"hello":"world"}`)
	if err := c.Set(ctx, ScopeGeocode, "k1", payload, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := c.Get(ctx, ScopeGeocode, "k1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch: %q", got)
	}
}

func TestCache_TTLEnforced(t *testing.T) {
	t.Parallel()
	c, err := OpenCache(tempCachePath(t))
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	defer func() { _ = c.Close() }()
	ctx := context.Background()

	base := time.Now()
	c.now = func() time.Time { return base }
	if err := c.Set(ctx, ScopeTraffic, "k", []byte("v"), 60*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	c.now = func() time.Time { return base.Add(30 * time.Second) }
	if _, ok, err := c.Get(ctx, ScopeTraffic, "k"); err != nil || !ok {
		t.Fatalf("within TTL: ok=%v err=%v", ok, err)
	}

	c.now = func() time.Time { return base.Add(61 * time.Second) }
	_, ok, err := c.Get(ctx, ScopeTraffic, "k")
	if err != nil {
		t.Fatalf("expired Get err: %v", err)
	}
	if ok {
		t.Fatalf("entry past TTL still ok=true")
	}
}

func TestCache_ScopeIsolation(t *testing.T) {
	t.Parallel()
	c, err := OpenCache(tempCachePath(t))
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	defer func() { _ = c.Close() }()
	ctx := context.Background()
	if err := c.Set(ctx, ScopeGeocode, "same", []byte("A"), time.Minute); err != nil {
		t.Fatalf("Set A: %v", err)
	}
	if err := c.Set(ctx, ScopeRoute, "same", []byte("B"), time.Minute); err != nil {
		t.Fatalf("Set B: %v", err)
	}
	a, _, _ := c.Get(ctx, ScopeGeocode, "same")
	b, _, _ := c.Get(ctx, ScopeRoute, "same")
	if string(a) != "A" || string(b) != "B" {
		t.Fatalf("scope collision: a=%q b=%q", a, b)
	}
}

func TestCache_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	c, err := OpenCache(tempCachePath(t))
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	defer func() { _ = c.Close() }()
	ctx := context.Background()
	var wg sync.WaitGroup
	const workers = 8
	const ops = 50
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := keyHash("worker", id, i)
				if err := c.Set(ctx, ScopePOI, key, []byte("x"), time.Minute); err != nil {
					t.Errorf("Set: %v", err)
					return
				}
				if _, ok, err := c.Get(ctx, ScopePOI, key); err != nil || !ok {
					t.Errorf("Get: ok=%v err=%v", ok, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
}

func TestCache_OpenInvalidPath(t *testing.T) {
	t.Parallel()
	if _, err := OpenCache("/this/path/should/not/exist/maps-cache.db"); err == nil {
		t.Fatalf("expected error opening unreachable path")
	}
}

func TestCache_DefaultPathExpands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	p, err := defaultCachePath()
	if err != nil {
		t.Fatalf("defaultCachePath: %v", err)
	}
	want := filepath.Join(dir, ".leah-state", "maps-cache.db")
	if p != want {
		t.Fatalf("path = %q, want %q", p, want)
	}
}

func TestCache_RejectEmptyScope(t *testing.T) {
	t.Parallel()
	c, err := OpenCache(tempCachePath(t))
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Set(context.Background(), "", "k", []byte("v"), time.Minute); !errors.Is(err, ErrCacheInvalidScope) {
		t.Fatalf("err = %v, want ErrCacheInvalidScope", err)
	}
}
