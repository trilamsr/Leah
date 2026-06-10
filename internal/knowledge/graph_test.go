package knowledge

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/audit"
)

func newTestGraph(t *testing.T, now func() time.Time) *Graph {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{Path: filepath.Join(dir, "knowledge.db"), Now: now}
	g, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func TestGraph_Add_PersistsEntity(t *testing.T) {
	clock := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	g := newTestGraph(t, func() time.Time { return clock })

	e := Entity{
		Kind:    KindPerson,
		Key:     "sarah@example.com",
		Display: "Sarah",
		Aliases: []string{"+15551234567"},
		Refs: []ItemRef{
			{Source: "messages", ID: "m1", Timestamp: clock.Add(-1 * time.Hour)},
		},
	}
	if err := g.Add(e); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := g.store.getEntity(KindPerson, "sarah@example.com")
	if err != nil {
		t.Fatalf("getEntity: %v", err)
	}
	if got.Display != "Sarah" {
		t.Errorf("display: want Sarah, got %q", got.Display)
	}
	if len(got.Refs) != 1 || got.Refs[0].Source != "messages" {
		t.Errorf("refs: %+v", got.Refs)
	}
}

func TestGraph_Resolve_HappyPath(t *testing.T) {
	clock := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	g := newTestGraph(t, func() time.Time { return clock })
	if err := g.Add(Entity{
		Kind: KindPerson, Key: "sarah@example.com", Display: "Sarah Connor",
		Aliases: []string{"sarah"},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	out, err := g.Resolve(context.Background(), "sarah")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(out) != 1 || out[0].Key != "sarah@example.com" {
		t.Fatalf("resolve: %+v", out)
	}
}

func TestGraph_Resolve_UnknownReturnsSentinel(t *testing.T) {
	g := newTestGraph(t, nil)
	_, err := g.Resolve(context.Background(), "nobody")
	if !errors.Is(err, ErrUnknownEntity) {
		t.Fatalf("want ErrUnknownEntity, got %v", err)
	}
}

func TestGraph_Query_AggregatesAcrossSources(t *testing.T) {
	clock := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	g := newTestGraph(t, func() time.Time { return clock })
	e := Entity{
		Kind: KindPerson, Key: "sarah@example.com", Display: "Sarah",
		Refs: []ItemRef{
			{Source: "messages", ID: "m1", Timestamp: clock.Add(-2 * time.Hour)},
			{Source: "messages", ID: "m2", Timestamp: clock.Add(-1 * time.Hour)},
			{Source: "calendar", ID: "c1", Timestamp: clock.Add(-3 * time.Hour)},
		},
	}
	if err := g.Add(e); err != nil {
		t.Fatalf("Add: %v", err)
	}
	res, err := g.Query(context.Background(), KnowledgeQuery{
		Subject: "sarah@example.com",
		Within:  24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if res.Scalars["source_messages"] != 2 {
		t.Errorf("messages scalar: want 2, got %d", res.Scalars["source_messages"])
	}
	if res.Scalars["source_calendar"] != 1 {
		t.Errorf("calendar scalar: want 1, got %d", res.Scalars["source_calendar"])
	}
	if res.Scalars["item_count"] != 3 {
		t.Errorf("item_count: want 3, got %d", res.Scalars["item_count"])
	}
	// Refs sorted newest-first.
	if !res.Items[0].Timestamp.After(res.Items[1].Timestamp) {
		t.Errorf("items not newest-first: %+v", res.Items)
	}
}

func TestGraph_Query_WindowFilters(t *testing.T) {
	clock := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	g := newTestGraph(t, func() time.Time { return clock })
	e := Entity{
		Kind: KindPerson, Key: "k", Display: "k",
		Refs: []ItemRef{
			{Source: "x", ID: "old", Timestamp: clock.Add(-10 * 24 * time.Hour)},
			{Source: "x", ID: "new", Timestamp: clock.Add(-1 * time.Hour)},
		},
	}
	if err := g.Add(e); err != nil {
		t.Fatalf("Add: %v", err)
	}
	res, err := g.Query(context.Background(), KnowledgeQuery{
		Subject: "k", Within: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "new" {
		t.Fatalf("expected only newest in window: %+v", res.Items)
	}
}

func TestGraph_0600Mode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge.db")
	g, err := New(Config{Path: path})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })
	info, err := statFile(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("want mode 0600, got %#o", info.Mode().Perm())
	}
}

func TestGraph_LastTouchedTTL_AgedOut(t *testing.T) {
	clock := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	g := newTestGraph(t, now)

	stale := clock.Add(-100 * 24 * time.Hour)
	if err := g.Add(Entity{
		Kind: KindPerson, Key: "stale@example.com", Display: "Stale",
		FirstSeen: stale, LastTouched: stale,
		Refs: []ItemRef{{Source: "messages", ID: "m1", Timestamp: stale}},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	res, err := g.Query(context.Background(), KnowledgeQuery{Subject: "stale@example.com"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if res.Scalars["entity_age_days"] < 90 {
		t.Fatalf("want age >= 90, got %d", res.Scalars["entity_age_days"])
	}
	// Entity demoted, not deleted — Query still finds it.
	if res.Entity.Key != "stale@example.com" {
		t.Fatalf("want entity preserved, got %q", res.Entity.Key)
	}
}

func TestGraph_ForgetEntity_Removes(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	logger := &audit.Logger{Path: auditPath}

	clock := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	g, err := New(Config{
		Path:        filepath.Join(dir, "knowledge.db"),
		AuditLogger: logger,
		Now:         func() time.Time { return clock },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	if err := g.Add(Entity{
		Kind: KindPerson, Key: "doomed@example.com", Display: "Doomed",
		Refs: []ItemRef{{Source: "messages", ID: "m1", Timestamp: clock}},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := g.Forget(context.Background(), KindPerson, "doomed@example.com"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, err := g.Query(context.Background(), KnowledgeQuery{Subject: "doomed@example.com"}); !errors.Is(err, ErrUnknownEntity) {
		t.Fatalf("want ErrUnknownEntity after forget, got %v", err)
	}
	// Forget a second time → ErrUnknownEntity.
	if err := g.Forget(context.Background(), KindPerson, "doomed@example.com"); !errors.Is(err, ErrUnknownEntity) {
		t.Fatalf("second forget: want ErrUnknownEntity, got %v", err)
	}
	// audit.jsonl appended once.
	body := readFile(t, auditPath)
	if !strings.Contains(body, `"knowledge_forget"`) {
		t.Fatalf("audit row missing kind: %s", body)
	}
}

func TestGraph_Add_MergesAliases(t *testing.T) {
	clock := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	g := newTestGraph(t, func() time.Time { return clock })
	if err := g.Add(Entity{
		Kind: KindPerson, Key: "k", Display: "K", Aliases: []string{"a", "b"},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := g.Add(Entity{
		Kind: KindPerson, Key: "k", Display: "K", Aliases: []string{"b", "c"},
	}); err != nil {
		t.Fatalf("Add 2: %v", err)
	}
	got, err := g.store.getEntity(KindPerson, "k")
	if err != nil {
		t.Fatalf("getEntity: %v", err)
	}
	if len(got.Aliases) != 3 {
		t.Fatalf("aliases: want 3 unique, got %v", got.Aliases)
	}
}

type stubResolver struct {
	kind EntityKind
	out  []Entity
	err  error
}

func (s *stubResolver) Kind() EntityKind { return s.kind }
func (s *stubResolver) Resolve(ctx context.Context, _ string) ([]Entity, error) {
	return s.out, s.err
}

func TestGraph_Resolve_InvokesRegisteredResolver(t *testing.T) {
	clock := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	g := newTestGraph(t, func() time.Time { return clock })
	g.Register(&stubResolver{
		kind: KindPerson,
		out: []Entity{{
			Kind: KindPerson, Key: "fresh@example.com", Display: "Fresh",
		}},
	})
	out, err := g.Resolve(context.Background(), "fresh")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected at least one entity from resolver")
	}
	// Persisted via Add → subsequent Query sees it without resolver.
	res, err := g.Query(context.Background(), KnowledgeQuery{Subject: "fresh@example.com"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if res.Entity.Key != "fresh@example.com" {
		t.Fatalf("query missed persisted entity: %+v", res.Entity)
	}
}

func TestGraph_ScalarsNoRawKeys(t *testing.T) {
	// Defensive regression test for spec §5 — Scalar keys must not leak
	// raw entity Key/Display values.
	clock := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	g := newTestGraph(t, func() time.Time { return clock })
	if err := g.Add(Entity{
		Kind: KindPerson, Key: "sarah@example.com", Display: "Sarah",
		Refs: []ItemRef{{Source: "messages", ID: "m1", Timestamp: clock}},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	res, err := g.Query(context.Background(), KnowledgeQuery{Subject: "sarah@example.com"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	for k := range res.Scalars {
		if strings.Contains(k, "sarah") || strings.Contains(k, "example.com") {
			t.Fatalf("scalar key leaks raw entity: %q", k)
		}
	}
}
