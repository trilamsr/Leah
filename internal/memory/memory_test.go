package memory

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestStore opens a fresh Store in a temp dir.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestNewStore_CreatesTables asserts schema migration runs on first open (#M2).
func TestNewStore_CreatesTables(t *testing.T) {
	s := newTestStore(t)
	for _, tbl := range []string{"contact", "project", "decision", "schema_meta"} {
		var name string
		err := s.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %q missing: %v", tbl, err)
		}
	}
}

// TestAddContact_Roundtrip asserts Add → List preserves fields (#M2).
func TestAddContact_Roundtrip(t *testing.T) {
	s := newTestStore(t)
	c := Contact{Name: "Maya Chen", Email: "maya@example.com", Notes: "intros"}
	out, err := s.AddContact(c)
	if err != nil {
		t.Fatalf("AddContact: %v", err)
	}
	if out.ID == "" {
		t.Fatal("AddContact: empty id")
	}
	if out.CreatedAt == "" || out.UpdatedAt == "" {
		t.Fatal("AddContact: empty timestamps")
	}

	list, err := s.ListContacts()
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListContacts: want 1 got %d", len(list))
	}
	got := list[0]
	if got.Name != c.Name || got.Email != c.Email || got.Notes != c.Notes {
		t.Errorf("roundtrip mismatch: %+v vs %+v", got, c)
	}
	if got.WorkspaceID != "default" {
		t.Errorf("workspace_id = %q, want default", got.WorkspaceID)
	}
}

// TestGetContact asserts fetch-by-id returns the original row (#M2).
func TestGetContact(t *testing.T) {
	s := newTestStore(t)
	added, err := s.AddContact(Contact{Name: "Pat Singh"})
	if err != nil {
		t.Fatalf("AddContact: %v", err)
	}
	got, err := s.GetContact(added.ID)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if got.ID != added.ID || got.Name != "Pat Singh" {
		t.Errorf("GetContact mismatch: %+v", got)
	}
}

// TestAddContact_EmptyName rejects whitespace-only names (#M2 critic MED).
func TestAddContact_EmptyName(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddContact(Contact{Name: "   "}); err == nil {
		t.Fatal("AddContact accepted whitespace-only name")
	}
	if _, err := s.AddContact(Contact{Name: ""}); err == nil {
		t.Fatal("AddContact accepted empty name")
	}
}

// TestAddProject_Roundtrip asserts Add → List preserves fields (#M2).
func TestAddProject_Roundtrip(t *testing.T) {
	s := newTestStore(t)
	p := Project{Name: "leah", Status: "active", Notes: "self-host"}
	out, err := s.AddProject(p)
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if out.ID == "" {
		t.Fatal("AddProject: empty id")
	}
	list, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(list) != 1 || list[0].Name != "leah" || list[0].Status != "active" {
		t.Errorf("ListProjects mismatch: %+v", list)
	}
}

// TestAddProject_DefaultStatus inserts 'active' when status omitted (#M2).
func TestAddProject_DefaultStatus(t *testing.T) {
	s := newTestStore(t)
	out, err := s.AddProject(Project{Name: "p"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if out.Status != "active" {
		t.Errorf("default status = %q, want active", out.Status)
	}
}

// TestAddDecision_Roundtrip asserts Add → List preserves fields (#M2).
func TestAddDecision_Roundtrip(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC().Format(time.RFC3339)
	d := Decision{Topic: "billing-stack", Choice: "Stripe", Rationale: "familiar", DecidedAt: now}
	out, err := s.AddDecision(d)
	if err != nil {
		t.Fatalf("AddDecision: %v", err)
	}
	if out.ID == "" || out.CreatedAt == "" {
		t.Fatal("AddDecision: empty id/created_at")
	}
	got, err := s.GetDecision(out.ID)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.Topic != d.Topic || got.Choice != d.Choice || got.Rationale != d.Rationale {
		t.Errorf("decision roundtrip mismatch: %+v vs %+v", got, d)
	}
}

// TestAddDecision_RequiredFields rejects empty topic or choice (#M2 critic MED).
func TestAddDecision_RequiredFields(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddDecision(Decision{Choice: "x"}); err == nil {
		t.Error("AddDecision accepted empty topic")
	}
	if _, err := s.AddDecision(Decision{Topic: "x"}); err == nil {
		t.Error("AddDecision accepted empty choice")
	}
}

// TestPersistence asserts data survives close + reopen (#M2).
func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.db")
	s1, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := s1.AddContact(Contact{Name: "persisted"}); err != nil {
		t.Fatalf("AddContact: %v", err)
	}
	_ = s1.Close()

	s2, err := NewStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = s2.Close() }()
	list, err := s2.ListContacts()
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if len(list) != 1 || list[0].Name != "persisted" {
		t.Errorf("persistence lost: %+v", list)
	}
}

// TestSchemaIdempotent asserts NewStore is safe to call twice (#M2).
func TestSchemaIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.db")
	for i := 0; i < 3; i++ {
		s, err := NewStore(path)
		if err != nil {
			t.Fatalf("NewStore iter %d: %v", i, err)
		}
		_ = s.Close()
	}
}

// TestGetProject_Roundtrip asserts AddProject → GetProject preserves all
// fields including the default-active status assignment.
func TestGetProject_Roundtrip(t *testing.T) {
	s := newTestStore(t)
	added, err := s.AddProject(Project{Name: "leah", Notes: "self-host"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	got, err := s.GetProject(added.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.ID != added.ID || got.Name != "leah" || got.Status != "active" || got.Notes != "self-host" {
		t.Errorf("GetProject mismatch: %+v", got)
	}
}

// TestGetProject_NotFoundErrors asserts unknown ID returns an error rather
// than a zero-value (which would silently swallow KB miss).
func TestGetProject_NotFoundErrors(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetProject("nonexistent-id"); err == nil {
		t.Fatal("GetProject accepted unknown id, want error")
	}
}

// TestListDecisions_NewestFirst asserts the list is ordered by DecidedAt desc
// (operator wants newest decisions surfaced first).
func TestListDecisions_NewestFirst(t *testing.T) {
	s := newTestStore(t)
	older := "2026-06-01T10:00:00Z"
	newer := "2026-06-09T10:00:00Z"
	if _, err := s.AddDecision(Decision{Topic: "a", Choice: "x", DecidedAt: older}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddDecision(Decision{Topic: "b", Choice: "y", DecidedAt: newer}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListDecisions()
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 decisions, got %d", len(list))
	}
	if list[0].DecidedAt != newer || list[1].DecidedAt != older {
		t.Errorf("want newest first, got: %+v", list)
	}
}

// TestMemoryStore_ConcurrentWritesNoRace exercises AddContact/AddProject/
// AddDecision concurrently — race detector must stay clean and all 30 rows
// must land (MaxOpenConns=1 serializes the writers).
func TestMemoryStore_ConcurrentWritesNoRace(t *testing.T) {
	s := newTestStore(t)
	var wg sync.WaitGroup
	const N = 10
	wg.Add(3 * N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			if _, err := s.AddContact(Contact{Name: "c-" + intToStr(i)}); err != nil {
				t.Errorf("AddContact: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.AddProject(Project{Name: "p-" + intToStr(i)}); err != nil {
				t.Errorf("AddProject: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := s.AddDecision(Decision{Topic: "t-" + intToStr(i), Choice: "x"}); err != nil {
				t.Errorf("AddDecision: %v", err)
			}
		}()
	}
	wg.Wait()
	contacts, _ := s.ListContacts()
	projects, _ := s.ListProjects()
	decisions, _ := s.ListDecisions()
	if len(contacts) != N || len(projects) != N || len(decisions) != N {
		t.Errorf("want %d each, got contacts=%d projects=%d decisions=%d",
			N, len(contacts), len(projects), len(decisions))
	}
}

// intToStr avoids strconv import in the test helper; pulls a single digit
// from 0-9 (N=10 in the caller).
func intToStr(i int) string {
	return string(rune('0' + i))
}

// TestNewStore_RejectsNewerSchemaOnDisk asserts the "newer DB than binary"
// guard fires when the on-disk schema_meta.version exceeds the embedded
// version. Wave3-P regression: schema.sql previously stamped the version
// unconditionally on every open, silently overwriting any operator-bumped
// value before the version check could fire.
func TestNewStore_RejectsNewerSchemaOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/memory.db"

	s, err := NewStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	_ = s.Close()

	// Operator manually bumps schema_meta to a future version (simulating
	// a newer binary having touched this DB then operator downgraded).
	s2, err := NewStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	db := s2.DB()
	if _, err := db.Exec(`UPDATE schema_meta SET value='999' WHERE key='version'`); err != nil {
		t.Fatalf("bump: %v", err)
	}
	_ = s2.Close()

	// Re-open should REFUSE — DB is from a newer binary.
	_, err = NewStore(path)
	if err == nil {
		t.Fatalf("expected error opening DB with newer schema version, got nil")
	}
	if !strings.Contains(err.Error(), "schema") || !strings.Contains(err.Error(), "999") {
		t.Errorf("expected schema-version error mentioning 999, got: %v", err)
	}
}

// TestAuditHook fires the audit callback on each mutation (#M2).
func TestAuditHook(t *testing.T) {
	s := newTestStore(t)
	var seen []string
	s.OnWrite = func(table, id string) {
		seen = append(seen, table+":"+id)
	}
	if _, err := s.AddContact(Contact{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProject(Project{Name: "p"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddDecision(Decision{Topic: "t", Choice: "c", DecidedAt: "2026-06-09T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 {
		t.Fatalf("OnWrite fired %d times, want 3 (%v)", len(seen), seen)
	}
	for i, prefix := range []string{"contact:", "project:", "decision:"} {
		if !strings.HasPrefix(seen[i], prefix) {
			t.Errorf("seen[%d]=%q, want prefix %q", i, seen[i], prefix)
		}
	}
}
