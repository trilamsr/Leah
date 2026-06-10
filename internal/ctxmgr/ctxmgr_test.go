package ctxmgr

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func openTestManager(t *testing.T) *Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	m, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestActiveOnFreshDBReturnsDefault(t *testing.T) {
	m := openTestManager(t)
	c, err := m.Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if c.Name != "default" {
		t.Fatalf("want default, got %q", c.Name)
	}
	if c.Description == "" {
		t.Errorf("want non-empty default description, got empty")
	}
}

func TestSwitchPersistsActive(t *testing.T) {
	m := openTestManager(t)
	if err := m.NewContext("acme", "Acme Corp"); err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := m.Switch("acme", "cli"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	c, err := m.Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if c.Name != "acme" || c.Description != "Acme Corp" {
		t.Fatalf("want acme/Acme Corp, got %q/%q", c.Name, c.Description)
	}
}

func TestSwitchPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.db")
	m1, err := Open(path)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	if err := m1.NewContext("acme", ""); err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := m1.Switch("acme", "cli"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if err := m1.Close(); err != nil {
		t.Fatalf("close1: %v", err)
	}

	m2, err := Open(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	defer func() { _ = m2.Close() }()
	c, err := m2.Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if c.Name != "acme" {
		t.Fatalf("active did not persist across reopen: got %q", c.Name)
	}
}

func TestHistoryReturnsLastSwitches(t *testing.T) {
	m := openTestManager(t)
	if err := m.NewContext("acme", ""); err != nil {
		t.Fatalf("new acme: %v", err)
	}
	if err := m.NewContext("personal", ""); err != nil {
		t.Fatalf("new personal: %v", err)
	}

	// Pin time so SwitchedAt parses deterministically.
	base := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	tick := 0
	m.SetNow(func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Second)
	})

	for _, target := range []string{"acme", "personal", "acme"} {
		if err := m.Switch(target, "cli"); err != nil {
			t.Fatalf("switch %s: %v", target, err)
		}
	}

	hist, err := m.History(10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("want 3 entries, got %d", len(hist))
	}
	// Most-recent first.
	if hist[0].To != "acme" || hist[0].From != "personal" {
		t.Errorf("hist[0] = %+v, want personal→acme", hist[0])
	}
	if hist[1].To != "personal" || hist[1].From != "acme" {
		t.Errorf("hist[1] = %+v, want acme→personal", hist[1])
	}
	if hist[2].To != "acme" || hist[2].From != "default" {
		t.Errorf("hist[2] = %+v, want default→acme", hist[2])
	}
	for i, s := range hist {
		if s.Reason != "cli" {
			t.Errorf("hist[%d].Reason = %q, want cli", i, s.Reason)
		}
		if s.SwitchedAt.IsZero() {
			t.Errorf("hist[%d].SwitchedAt is zero", i)
		}
	}
}

func TestHistoryDefaultLimitWhenZero(t *testing.T) {
	m := openTestManager(t)
	if err := m.NewContext("acme", ""); err != nil {
		t.Fatalf("new: %v", err)
	}
	for i := 0; i < 25; i++ {
		if err := m.Switch("acme", "cli"); err != nil {
			t.Fatalf("switch %d: %v", i, err)
		}
	}
	hist, err := m.History(0)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 20 {
		t.Fatalf("default limit should be 20, got %d", len(hist))
	}
}

func TestNewContextRejectsDuplicate(t *testing.T) {
	m := openTestManager(t)
	if err := m.NewContext("acme", ""); err != nil {
		t.Fatalf("first new: %v", err)
	}
	err := m.NewContext("acme", "")
	if !errors.Is(err, ErrDuplicateContext) {
		t.Fatalf("want ErrDuplicateContext, got %v", err)
	}
}

func TestNewContextRejectsInvalidName(t *testing.T) {
	m := openTestManager(t)
	cases := []string{"", "Acme", "1acme", "ac me", "ac.me", "ac/me", "this-name-is-way-too-long-for-the-regex-limit"}
	for _, name := range cases {
		err := m.NewContext(name, "")
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q: want ErrInvalidName, got %v", name, err)
		}
	}
}

func TestSwitchUnknownErrors(t *testing.T) {
	m := openTestManager(t)
	err := m.Switch("nope", "cli")
	if !errors.Is(err, ErrUnknownContext) {
		t.Fatalf("want ErrUnknownContext, got %v", err)
	}
	// State unchanged.
	c, _ := m.Active()
	if c.Name != "default" {
		t.Errorf("unknown switch should not mutate active; got %q", c.Name)
	}
}

// TestSwitchIdempotentTargetStillLogs asserts a Switch to the already-active
// context still appends to context_switch_log so the audit trail captures
// the user-visible action (spec §2 — idempotent target still records).
func TestSwitchIdempotentTargetStillLogs(t *testing.T) {
	m := openTestManager(t)
	if err := m.NewContext("acme", ""); err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := m.Switch("acme", "cli"); err != nil {
		t.Fatalf("switch1: %v", err)
	}
	if err := m.Switch("acme", "cli"); err != nil {
		t.Fatalf("switch2: %v", err)
	}
	hist, err := m.History(10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 2 {
		t.Errorf("idempotent switch should still log; want 2 entries, got %d", len(hist))
	}
	// Second switch: from=acme, to=acme.
	if hist[0].From != "acme" || hist[0].To != "acme" {
		t.Errorf("hist[0] = %+v, want acme→acme", hist[0])
	}
}

// TestConcurrentSwitchesNoRace runs N goroutines toggling between two
// contexts; race detector + operator_state singleton invariant guard
// against state corruption under contention. SQLite SQLITE_BUSY rejections
// are expected (the busy_timeout(5000) + WAL stops most but not all);
// the test asserts only that the SUCCESSFUL switches landed coherently
// (active matches one of the two contexts, history count == success count).
func TestConcurrentSwitchesNoRace(t *testing.T) {
	m := openTestManager(t)
	for _, n := range []string{"acme", "personal"} {
		if err := m.NewContext(n, ""); err != nil {
			t.Fatalf("new %s: %v", n, err)
		}
	}
	var wg sync.WaitGroup
	var succeeded sync.Map // map[int]bool
	const N = 20
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			target := "acme"
			if i%2 == 0 {
				target = "personal"
			}
			if err := m.Switch(target, "race-test"); err == nil {
				succeeded.Store(i, true)
			}
			// SQLITE_BUSY is acceptable under contention; we only assert
			// invariants on the rows that DID commit.
		}()
	}
	wg.Wait()
	// Singleton row must still resolve to one of the two contexts.
	c, err := m.Active()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if c.Name != "acme" && c.Name != "personal" {
		t.Errorf("active corrupted under race: %q", c.Name)
	}
	// History rows match successful Switch returns.
	successCount := 0
	succeeded.Range(func(_, _ any) bool { successCount++; return true })
	hist, _ := m.History(N + 10)
	if len(hist) != successCount {
		t.Errorf("history rows %d != successful switches %d", len(hist), successCount)
	}
}

// TestHistoryReturnsEmptyOnFreshDB asserts a freshly-opened DB with no
// explicit switches returns an empty history slice (the seed default
// context lands via INSERT-OR-IGNORE only, no switch row).
func TestHistoryReturnsEmptyOnFreshDB(t *testing.T) {
	m := openTestManager(t)
	hist, err := m.History(10)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 0 {
		t.Errorf("fresh DB should have empty history, got %d entries", len(hist))
	}
}

// TestSinceReturnsAscendingFiltered asserts Since returns switches at-or-after
// the cutoff in ascending order — the shape operatormodel.activeAt requires.
func TestSinceReturnsAscendingFiltered(t *testing.T) {
	m := openTestManager(t)
	for _, n := range []string{"acme", "personal"} {
		if err := m.NewContext(n, ""); err != nil {
			t.Fatalf("new %s: %v", n, err)
		}
	}
	base := time.Date(2026, 6, 9, 10, 0, 0, 0, time.UTC)
	tick := 0
	m.SetNow(func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Minute)
	})
	for _, target := range []string{"acme", "personal", "acme", "personal"} {
		if err := m.Switch(target, "cli"); err != nil {
			t.Fatalf("switch %s: %v", target, err)
		}
	}

	got, err := m.Since(base.Add(2*time.Minute + 30*time.Second))
	if err != nil {
		t.Fatalf("since: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 switches at-or-after cutoff, got %d", len(got))
	}
	if !got[0].SwitchedAt.Before(got[1].SwitchedAt) {
		t.Errorf("Since must return ascending: %v then %v", got[0].SwitchedAt, got[1].SwitchedAt)
	}
	if got[0].To != "acme" || got[1].To != "personal" {
		t.Errorf("want acme then personal, got %s then %s", got[0].To, got[1].To)
	}
}

func TestListReturnsAllContexts(t *testing.T) {
	m := openTestManager(t)
	for _, n := range []string{"zebra", "acme", "personal"} {
		if err := m.NewContext(n, ""); err != nil {
			t.Fatalf("new %s: %v", n, err)
		}
	}
	list, err := m.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []string{"acme", "default", "personal", "zebra"}
	if len(list) != len(want) {
		t.Fatalf("want %d, got %d", len(want), len(list))
	}
	for i, c := range list {
		if c.Name != want[i] {
			t.Errorf("list[%d] = %q, want %q", i, c.Name, want[i])
		}
	}
}
