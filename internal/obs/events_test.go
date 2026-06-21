package obs_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
)

var osStat = os.Stat

func newStore(t *testing.T) *obs.SQLiteEventStore {
	t.Helper()
	dir := t.TempDir()
	s, err := obs.OpenEventStore(filepath.Join(dir, "events.db"), obs.EventStoreOptions{
		BatchInterval: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("OpenEventStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestEventStore_Emit_Persists(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	e := obs.Event{
		TS:        time.Now().UTC(),
		Kind:      "dispatch.ship",
		Actor:     "daemon",
		Outcome:   "ok",
		LatencyMS: 42,
	}
	if err := s.Emit(ctx, e); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	rows, err := s.Query(ctx, obs.EventQuery{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	got := rows[0]
	if got.Kind != "dispatch.ship" || got.Actor != "daemon" || got.Outcome != "ok" || got.LatencyMS != 42 {
		t.Fatalf("row mismatch: %+v", got)
	}
}

func TestEventStore_Query_ByKind(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	emits := []obs.Event{
		{TS: now, Kind: "dispatch.ship", Actor: "daemon", Outcome: "ok"},
		{TS: now.Add(1 * time.Millisecond), Kind: "audit.append", Actor: "daemon", Outcome: "ok"},
		{TS: now.Add(2 * time.Millisecond), Kind: "dispatch.ship", Actor: "daemon", Outcome: "ok"},
	}
	for _, e := range emits {
		if err := s.Emit(ctx, e); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	rows, err := s.Query(ctx, obs.EventQuery{Kinds: []string{"dispatch.ship"}})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows kind=dispatch.ship, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Kind != "dispatch.ship" {
			t.Fatalf("unexpected kind %q in filtered result", r.Kind)
		}
	}
}

func TestEventStore_Query_ByRefID_TreeAssembly(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	refID := obs.NewRefID()
	ctx = obs.WithRefID(ctx, refID)
	otherID := obs.NewRefID()
	now := time.Now().UTC()

	chain := []obs.Event{
		{TS: now, Kind: "dispatch.ship", Actor: "daemon", Outcome: "begin"},
		{TS: now.Add(1 * time.Millisecond), Kind: "attestation.attempt", Actor: "daemon", Outcome: "ok", Scope: "dispatcher.ship"},
		{TS: now.Add(2 * time.Millisecond), Kind: "reasoner.call", Actor: "daemon", Outcome: "ok"},
		{TS: now.Add(3 * time.Millisecond), Kind: "subagent.spawn", Actor: "daemon", Outcome: "ok"},
		{TS: now.Add(4 * time.Millisecond), Kind: "audit.append", Actor: "daemon", Outcome: "ok"},
		{TS: now.Add(5 * time.Millisecond), Kind: "dispatch.ship", Actor: "daemon", Outcome: "ok", LatencyMS: 250},
	}
	for _, e := range chain {
		// RefID flows from ctx.
		if err := s.Emit(ctx, e); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	// Unrelated event with a different RefID — must NOT appear in the query.
	if err := s.Emit(obs.WithRefID(context.Background(), otherID), obs.Event{
		TS:      now.Add(6 * time.Millisecond),
		Kind:    "voice.speak",
		Actor:   "daemon",
		Outcome: "ok",
	}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	rows, err := s.Query(ctx, obs.EventQuery{RefID: refID})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != len(chain) {
		t.Fatalf("got %d rows for RefID, want %d", len(rows), len(chain))
	}
	// Order MUST match emission order — causal chain reconstruction.
	for i, r := range rows {
		if r.Kind != chain[i].Kind {
			t.Fatalf("row %d: got kind %q, want %q", i, r.Kind, chain[i].Kind)
		}
		if r.RefID != refID {
			t.Fatalf("row %d: got RefID %q, want %q", i, r.RefID, refID)
		}
	}
}

func TestEventStore_ConcurrentSafe(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	const goroutines = 10
	const perGoroutine = 50
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				_ = s.Emit(ctx, obs.Event{
					Kind:    "reasoner.call",
					Actor:   "daemon",
					Outcome: "ok",
					Detail:  "g" + strconv.Itoa(g) + ":i" + strconv.Itoa(i),
				})
			}
		}(g)
	}
	wg.Wait()
	if err := s.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	rows, err := s.Query(ctx, obs.EventQuery{Limit: goroutines * perGoroutine * 2})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	// Some events MAY drop under load; assert no panic, no corruption, and
	// total + dropped == emitted (conservation).
	got := int64(len(rows)) + int64(s.Dropped())
	want := int64(goroutines * perGoroutine)
	if got != want {
		t.Fatalf("conservation broken: persisted=%d dropped=%d sum=%d want=%d",
			len(rows), s.Dropped(), got, want)
	}
}

func TestEventStore_BufferFull_DropsLogged(t *testing.T) {
	dir := t.TempDir()
	// Tiny buffer + slow batch interval forces overflow on synchronous emits.
	s, err := obs.OpenEventStore(filepath.Join(dir, "events.db"), obs.EventStoreOptions{
		BufferSize:    2,
		BatchInterval: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("OpenEventStore: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	// Emit far past capacity in a tight loop — non-blocking by contract.
	const tries = 200
	var dropErrs int
	for i := 0; i < tries; i++ {
		err := s.Emit(ctx, obs.Event{Kind: "memory.query", Actor: "daemon", Outcome: "ok"})
		if err != nil {
			dropErrs++
		}
	}
	if dropErrs == 0 {
		t.Fatal("expected ErrEventDropped on at least one Emit under overflow")
	}
	if s.Dropped() == 0 {
		t.Fatal("Dropped() counter did not increment on overflow")
	}
}

func TestEventStore_RetentionTrimsOlderThan30d(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.Add(-60 * 24 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	for _, ts := range []time.Time{old, old.Add(time.Second), recent} {
		if err := s.Emit(ctx, obs.Event{TS: ts, Kind: "audit.append", Actor: "daemon", Outcome: "ok"}); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	if err := s.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	cutoff := now.Add(-30 * 24 * time.Hour)
	n, err := s.PruneOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("PruneOlderThan: %v", err)
	}
	if n != 2 {
		t.Fatalf("pruned %d, want 2", n)
	}
	rows, err := s.Query(ctx, obs.EventQuery{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("post-prune rows=%d, want 1", len(rows))
	}
	if !rows[0].TS.Equal(recent) {
		t.Fatalf("surviving ts=%v, want %v", rows[0].TS, recent)
	}
}

func TestEventStore_0600Mode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")
	s, err := obs.OpenEventStore(path, obs.EventStoreOptions{})
	if err != nil {
		t.Fatalf("OpenEventStore: %v", err)
	}
	defer func() { _ = s.Close() }()

	info, err := osStat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Fatalf("got mode %o, want 0600", mode)
	}
}

func TestEventStore_SchemaVersion_IntParse(t *testing.T) {
	// Round-trip the parse helper indirectly via re-opening the same DB —
	// PR #58 lesson: lex compare ranks "10" < "9", so the on-disk integer
	// MUST be parsed numerically.
	dir := t.TempDir()
	path := filepath.Join(dir, "events.db")
	s1, err := obs.OpenEventStore(path, obs.EventStoreOptions{})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	_ = s1.Close()
	// Second open must accept the existing stamp without error.
	s2, err := obs.OpenEventStore(path, obs.EventStoreOptions{})
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	_ = s2.Close()
}

// Guards against re-introduction of a second Event type (e.g. W77 SSE pre-W75 stub):
// the spec mandates ONE canonical struct. Build alone catches duplicate symbols;
// this test catches silent shape drift if a future PR reshapes the canonical type.
func TestEvent_CanonicalSchemaShape(t *testing.T) {
	type field struct{ name, typ, tag string }
	want := []field{
		{"TS", "time.Time", `json:"ts"`},
		{"Kind", "string", `json:"kind"`},
		{"Actor", "string", `json:"actor"`},
		{"Target", "string", `json:"target,omitempty"`},
		{"Scope", "string", `json:"scope,omitempty"`},
		{"LatencyMS", "int64", `json:"latency_ms,omitempty"`},
		{"Outcome", "string", `json:"outcome"`},
		{"RefID", "string", `json:"ref_id,omitempty"`},
		{"Detail", "string", `json:"detail,omitempty"`},
		{"Payload", "interface {}", `json:"payload,omitempty"`},
	}
	rt := reflect.TypeOf(obs.Event{})
	if rt.NumField() != len(want) {
		t.Fatalf("Event fields: got %d, want %d", rt.NumField(), len(want))
	}
	for i, w := range want {
		f := rt.Field(i)
		if f.Name != w.name || f.Type.String() != w.typ || string(f.Tag) != w.tag {
			t.Errorf("field[%d]: got {%s %s `%s`}, want {%s %s `%s`}",
				i, f.Name, f.Type, f.Tag, w.name, w.typ, w.tag)
		}
	}
}

func TestEventKinds_FrozenList(t *testing.T) {
	want := []string{
		"dispatch.ship",
		"dispatch.review",
		"dispatch.merge",
		"attestation.attempt",
		"attestation.granted",
		"attestation.revoked",
		"audit.append",
		"audit.rotate",
		"connect.exchange",
		"connect.refresh",
		"connect.api_call",
		"voice.speak",
		"voice.fallback",
		"subagent.spawn",
		"subagent.complete",
		"reasoner.call",
		"reasoner.retry",
		"memory.query",
		"memory.upsert",
		"recommendation.propose",
		"recommendation.accept",
		"recommendation.reject",
		"recommendation.apply",
		"obs.snapshot",
		"obs.selfcheck",
		"obs.panic",
		"hud.state",
		"workspace.active_app_changed",
		"contact_store_changed",
		"messages_changed",
		"mail_changed",
		"notes_changed",
		"safari.history_changed",
		"focus.state_changed",
	}
	if !reflect.DeepEqual(obs.KnownEventKinds, want) {
		t.Fatalf("KnownEventKinds drifted from frozen list:\n got: %v\nwant: %v",
			obs.KnownEventKinds, want)
	}
}

func TestSafeDetail_StripsPII(t *testing.T) {
	in := "hello@example.com path with spaces and unicode é"
	out := obs.SafeDetail(in)
	if containsSubstr(out, "@") {
		t.Fatalf("SafeDetail kept @: %q", out)
	}
	if containsSubstr(out, " ") {
		t.Fatalf("SafeDetail kept space: %q", out)
	}
}

func TestSafeDetailHashed_Stable(t *testing.T) {
	a := obs.SafeDetailHashed("operator-message-text")
	b := obs.SafeDetailHashed("operator-message-text")
	if a != b {
		t.Fatalf("hash unstable: %q vs %q", a, b)
	}
	if !startsWith(a, "h:") {
		t.Fatalf("missing h: prefix: %q", a)
	}
}

func TestEmitEvent_NoStore_NoOp(t *testing.T) {
	// Package-level EmitEvent must not panic when no default store is set.
	obs.SetDefaultEventStore(nil)
	obs.EmitEvent(context.Background(), obs.Event{Kind: "obs.selfcheck", Actor: "daemon", Outcome: "ok"})
}

func TestEmitEvent_WithDefaultStore_Persists(t *testing.T) {
	s := newStore(t)
	obs.SetDefaultEventStore(s)
	t.Cleanup(func() { obs.SetDefaultEventStore(nil) })

	ctx := obs.WithRefID(context.Background(), obs.NewRefID())
	obs.EmitEvent(ctx, obs.Event{Kind: "obs.selfcheck", Actor: "daemon", Outcome: "ok"})
	if err := s.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	rows, err := s.Query(ctx, obs.EventQuery{Kinds: []string{"obs.selfcheck"}})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].RefID == "" {
		t.Fatalf("RefID not propagated from ctx")
	}
}

// --- tiny helpers — keep here to avoid pulling in strings/os in main file ---

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
