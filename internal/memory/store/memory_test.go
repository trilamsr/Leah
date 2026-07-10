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

// TestContact asserts fetch-by-id returns the original row (#M2).
func TestContact(t *testing.T) {
	s := newTestStore(t)
	added, err := s.AddContact(Contact{Name: "Pat Singh"})
	if err != nil {
		t.Fatalf("AddContact: %v", err)
	}
	got, err := s.Contact(added.ID)
	if err != nil {
		t.Fatalf("Contact: %v", err)
	}
	if got.ID != added.ID || got.Name != "Pat Singh" {
		t.Errorf("Contact mismatch: %+v", got)
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
	got, err := s.Decision(out.ID)
	if err != nil {
		t.Fatalf("Decision: %v", err)
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

// TestProject_Roundtrip asserts AddProject → Project preserves all
// fields including the default-active status assignment.
func TestProject_Roundtrip(t *testing.T) {
	s := newTestStore(t)
	added, err := s.AddProject(Project{Name: "leah", Notes: "self-host"})
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	got, err := s.Project(added.ID)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got.ID != added.ID || got.Name != "leah" || got.Status != "active" || got.Notes != "self-host" {
		t.Errorf("Project mismatch: %+v", got)
	}
}

// TestProject_NotFoundErrors asserts unknown ID returns an error rather
// than a zero-value (which would silently swallow KB miss).
func TestProject_NotFoundErrors(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Project("nonexistent-id"); err == nil {
		t.Fatal("Project accepted unknown id, want error")
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

// TestListContactsByWorkspace asserts ListContactsByWorkspace filters to the
// supplied workspace, and treats empty string as the canonical "default"
// (backward-compat with rows seeded via the old WorkspaceID-defaulting
// AddContact path). Activates the dormant workspace_id column.
func TestListContactsByWorkspace(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddContact(Contact{Name: "default-1"}); err != nil {
		t.Fatalf("add default-1: %v", err)
	}
	if _, err := s.AddContact(Contact{Name: "acme-1", WorkspaceID: "acme"}); err != nil {
		t.Fatalf("add acme-1: %v", err)
	}
	if _, err := s.AddContact(Contact{Name: "acme-2", WorkspaceID: "acme"}); err != nil {
		t.Fatalf("add acme-2: %v", err)
	}

	def, err := s.ListContactsByWorkspace("default")
	if err != nil {
		t.Fatalf("list default: %v", err)
	}
	if len(def) != 1 || def[0].Name != "default-1" {
		t.Errorf("default list = %+v, want [default-1]", def)
	}

	acme, err := s.ListContactsByWorkspace("acme")
	if err != nil {
		t.Fatalf("list acme: %v", err)
	}
	if len(acme) != 2 {
		t.Errorf("acme list len = %d, want 2", len(acme))
	}

	// Empty input → default. Backward-compat with ListContacts.
	emp, err := s.ListContactsByWorkspace("")
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(emp) != 1 || emp[0].Name != "default-1" {
		t.Errorf("empty-workspace list = %+v, want [default-1]", emp)
	}
}

// TestListProjectsByWorkspace mirrors TestListContactsByWorkspace for the
// project table.
func TestListProjectsByWorkspace(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddProject(Project{Name: "p1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddProject(Project{Name: "p2", WorkspaceID: "acme"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListProjectsByWorkspace("acme")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Name != "p2" {
		t.Errorf("got %+v want [p2]", got)
	}
}

// TestListDecisionsByWorkspace mirrors TestListContactsByWorkspace for the
// decision table — newest-first order preserved per workspace.
func TestListDecisionsByWorkspace(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddDecision(Decision{Topic: "t1", Choice: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddDecision(Decision{Topic: "t2", Choice: "y", WorkspaceID: "acme"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListDecisionsByWorkspace("acme")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Topic != "t2" {
		t.Errorf("got %+v want [t2]", got)
	}
}

// TestSchemaVersion_v9_v10_OrderingCorrect pins the L11 bug: lexicographic compare ranks "10" < "9".
func TestSchemaVersion_v9_v10_OrderingCorrect(t *testing.T) {
	ten, err := parseSchemaVersion("10")
	if err != nil {
		t.Fatalf("parse 10: %v", err)
	}
	nine, err := parseSchemaVersion("9")
	if err != nil {
		t.Fatalf("parse 9: %v", err)
	}
	if ten <= nine {
		t.Fatalf("want 10 > 9, got ten=%d nine=%d", ten, nine)
	}
}

// TestSchemaVersion_v2_v4_OrderingCorrect sanity baseline — same answer under lex or int.
func TestSchemaVersion_v2_v4_OrderingCorrect(t *testing.T) {
	four, err := parseSchemaVersion("4")
	if err != nil {
		t.Fatalf("parse 4: %v", err)
	}
	two, err := parseSchemaVersion("2")
	if err != nil {
		t.Fatalf("parse 2: %v", err)
	}
	if four <= two {
		t.Fatalf("want 4 > 2, got four=%d two=%d", four, two)
	}
}

// TestSchemaVersion_NonNumericRejected refuses silent fallback to lex compare on corrupt header.
func TestSchemaVersion_NonNumericRejected(t *testing.T) {
	if _, err := parseSchemaVersion("abc"); err == nil {
		t.Fatal("parseSchemaVersion accepted non-numeric, want error")
	}
}

// TestSchemaVersion_v1_v1_Equal equal versions must not trigger migration-mismatch path.
func TestSchemaVersion_v1_v1_Equal(t *testing.T) {
	a, err := parseSchemaVersion("1")
	if err != nil {
		t.Fatalf("parse a: %v", err)
	}
	b, err := parseSchemaVersion("1")
	if err != nil {
		t.Fatalf("parse b: %v", err)
	}
	if a != b {
		t.Fatalf("want equal, got a=%d b=%d", a, b)
	}
	if a > b || b > a {
		t.Fatalf("equal versions compared unequal: a=%d b=%d", a, b)
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

// TestSchema_Consolidated_UpsertOnConflict asserts duplicate
// (class, key, slot) inserts via ON CONFLICT DO UPDATE replace in place —
// the real production path used by ConsolidatePass.
func TestSchema_Consolidated_UpsertOnConflict(t *testing.T) {
	s := newTestStore(t)
	upsert := `INSERT INTO operator_profile_consolidated
		(class, key, slot, weight, count, first_seen_ts, last_consolidated_at, source_window_end)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(class, key, slot) DO UPDATE SET
			weight=excluded.weight,
			count=excluded.count,
			last_consolidated_at=excluded.last_consolidated_at,
			source_window_end=excluded.source_window_end`
	if _, err := s.db.Exec(upsert, "time_of_day", "ask", "09", 1.5, 3,
		"2026-05-01T00:00:00Z", "2026-06-01T00:00:00Z", "2026-05-27T00:00:00Z"); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := s.db.Exec(upsert, "time_of_day", "ask", "09", 9.9, 11,
		"2026-05-01T00:00:00Z", "2026-06-10T00:00:00Z", "2026-05-27T00:00:00Z"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var count int
	if err := s.db.QueryRow(
		`SELECT count FROM operator_profile_consolidated WHERE class=? AND key=? AND slot=?`,
		"time_of_day", "ask", "09",
	).Scan(&count); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if count != 11 {
		t.Fatalf("upsert did not replace: count=%d, want 11", count)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM operator_profile_consolidated`).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("row count = %d, want 1", n)
	}
}

// TestSchema_Snapshot_UpsertOnConflict mirrors the consolidated upsert
// contract for operator_profile_snapshot (§3.3 anchor table).
func TestSchema_Snapshot_UpsertOnConflict(t *testing.T) {
	s := newTestStore(t)
	upsert := `INSERT INTO operator_profile_snapshot
		(class, key, slot, weight_at_snapshot, snapshot_ts)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(class, key, slot) DO UPDATE SET
			weight_at_snapshot=excluded.weight_at_snapshot,
			snapshot_ts=excluded.snapshot_ts`
	if _, err := s.db.Exec(upsert, "cadence", "ask", "Mon", 2.0, "2026-05-27T00:00:00Z"); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := s.db.Exec(upsert, "cadence", "ask", "Mon", 4.4, "2026-06-10T00:00:00Z"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var w float64
	if err := s.db.QueryRow(
		`SELECT weight_at_snapshot FROM operator_profile_snapshot WHERE class=? AND key=? AND slot=?`,
		"cadence", "ask", "Mon",
	).Scan(&w); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if w != 4.4 {
		t.Fatalf("upsert did not replace: w=%v, want 4.4", w)
	}
}

// TestSchema_Consolidated_MigrationIdempotent asserts re-opening a store
// preserves consolidated rows (no DDL reset of existing data).
func TestSchema_Consolidated_MigrationIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.db")
	s1, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := s1.db.Exec(`INSERT INTO operator_profile_consolidated
		(class, key, slot, weight, count, first_seen_ts, last_consolidated_at, source_window_end)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"time_of_day", "ask", "09", 1.0, 1,
		"2026-05-01T00:00:00Z", "2026-06-01T00:00:00Z", "2026-05-27T00:00:00Z"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_ = s1.Close()

	s2, err := NewStore(path)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer func() { _ = s2.Close() }()
	var n int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM operator_profile_consolidated`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("row count after re-open = %d, want 1 (DDL clobbered data)", n)
	}
}
