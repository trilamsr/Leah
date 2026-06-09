package ctxmgr

import (
	"errors"
	"path/filepath"
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
