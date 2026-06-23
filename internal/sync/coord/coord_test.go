package coord

import (
	"context"
	"database/sql"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/sync/crdt"
	_ "modernc.org/sqlite"
)

// fakePeer implements crdt.Peer for tests with no network surface.
type fakePeer struct {
	id     crdt.DeviceID
	status crdt.PeerStatus
}

func (p *fakePeer) ID() crdt.DeviceID        { return p.id }
func (p *fakePeer) Endpoint() netip.AddrPort { return netip.AddrPort{} }
func (p *fakePeer) LastSeenAt() time.Time    { return time.Unix(0, 0) }
func (p *fakePeer) Status() crdt.PeerStatus  { return p.status }

// fakeCRDT lets tests assert on Coord wiring without touching SQLite for the
// non-outbox path (outbox tests below use a real DB).
type fakeCRDT struct {
	applyEntries [][]crdt.LogEntry
	emitReturn   []crdt.LogEntry
	stats        crdt.DeltaStats
	emitErr      error
}

func (f *fakeCRDT) ApplyLog(_ context.Context, e []crdt.LogEntry) (crdt.DeltaStats, error) {
	f.applyEntries = append(f.applyEntries, e)
	return f.stats, nil
}
func (f *fakeCRDT) EmitLog(_ context.Context, _ crdt.Lamport, _ int) ([]crdt.LogEntry, error) {
	return f.emitReturn, f.emitErr
}
func (f *fakeCRDT) GCTombstones(_ context.Context, _ time.Time) (int, error) { return 0, nil }

type fakePairer struct{ peer crdt.Peer }

func (p *fakePairer) Pair(_ context.Context, _ string) (crdt.Peer, error) { return p.peer, nil }
func (p *fakePairer) Disconnect(_ context.Context, _ crdt.Peer) error     { return nil }

// Pause must short-circuit ApplyRemote without touching the CRDT log — otherwise the
// kill-switch is racy and a paused peer can still mutate state (§2.6).
func TestCoord_PauseBlocksApply(t *testing.T) {
	log := &fakeCRDT{}
	c := NewCoord(&fakePairer{}, log, nil)
	p := &fakePeer{id: "node-a"}

	if err := c.Pause(context.Background(), p); err != nil {
		t.Fatalf("pause: %v", err)
	}
	_, err := c.ApplyRemote(context.Background(), p, []crdt.LogEntry{{Table: "x", RowID: 1, Node: "node-a", Lamport: 1, Op: crdt.OpUpdate}})
	if err != ErrPaused {
		t.Fatalf("err = %v, want ErrPaused", err)
	}
	if len(log.applyEntries) != 0 {
		t.Fatalf("paused peer reached CRDT log: %+v", log.applyEntries)
	}
}

// Resume after Pause must restore replication and not require a re-pair (§2.6).
func TestCoord_ResumeRestoresApply(t *testing.T) {
	log := &fakeCRDT{}
	c := NewCoord(&fakePairer{}, log, nil)
	p := &fakePeer{id: "node-a"}
	_ = c.Pause(context.Background(), p)
	_ = c.Resume(context.Background(), p)
	if _, err := c.ApplyRemote(context.Background(), p, []crdt.LogEntry{{Table: "x", RowID: 1, Node: "node-a", Lamport: 1, Op: crdt.OpUpdate}}); err != nil {
		t.Fatalf("apply after resume: %v", err)
	}
	if len(log.applyEntries) != 1 {
		t.Fatalf("apply count = %d, want 1", len(log.applyEntries))
	}
}

// Conflict stats from ApplyLog must surface as EventConflict, not EventDeltaApplied —
// the HUD toast UX (§2.7) keys off the event kind to show the 24h-undo affordance.
func TestCoord_ConflictEventKind(t *testing.T) {
	log := &fakeCRDT{stats: crdt.DeltaStats{Applied: 0, Conflicts: 1}}
	c := NewCoord(&fakePairer{}, log, nil)
	sub := c.Subscribe()
	p := &fakePeer{id: "node-a"}
	if _, err := c.ApplyRemote(context.Background(), p, []crdt.LogEntry{{Table: "x", RowID: 1, Node: "node-a", Lamport: 1, Op: crdt.OpUpdate}}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	select {
	case ev := <-sub:
		if ev.Kind != EventConflict {
			t.Fatalf("event kind = %d, want EventConflict (%d)", ev.Kind, EventConflict)
		}
	case <-time.After(time.Second):
		t.Fatal("no event after conflict")
	}
}

// Pair routes through the transport, records the peer for later Pause/Unpair, and
// emits EventPaired.
func TestCoord_PairRecordsAndEmits(t *testing.T) {
	peer := &fakePeer{id: "node-b"}
	c := NewCoord(&fakePairer{peer: peer}, &fakeCRDT{}, nil)
	sub := c.Subscribe()
	got, err := c.Pair(context.Background(), "123456")
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	if got.ID() != "node-b" {
		t.Fatalf("paired %s, want node-b", got.ID())
	}
	select {
	case ev := <-sub:
		if ev.Kind != EventPaired {
			t.Fatalf("kind = %d, want EventPaired", ev.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("no paired event")
	}
}

// --- Outbox ---

const outboxSchema = `
CREATE TABLE sync_peer (
    id TEXT PRIMARY KEY, name TEXT NOT NULL, paired_at INTEGER NOT NULL,
    paused INTEGER NOT NULL DEFAULT 0, last_seen_at INTEGER, fingerprint BLOB NOT NULL
);
CREATE TABLE sync_outbox (
    id INTEGER PRIMARY KEY, peer_id TEXT NOT NULL REFERENCES sync_peer(id) ON DELETE CASCADE,
    payload BLOB NOT NULL, enqueued_at INTEGER NOT NULL, sent_at INTEGER
);`

type testStore struct{ db *sql.DB }

func (t *testStore) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return t.db.BeginTx(ctx, opts)
}

func newOutbox(t *testing.T) (*Outbox, *sql.DB) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "sync.db") + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(outboxSchema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sync_peer(id,name,paired_at,fingerprint) VALUES('node-a','A',0,x'00')`); err != nil {
		t.Fatalf("seed peer: %v", err)
	}
	return NewOutbox(&testStore{db: db}), db
}

// gzip must shrink JSON of repetitive log entries — the §2.5 schema choice of
// gzipped CRDT delta batches assumes a meaningful compression ratio.
func TestOutbox_GzipCompresses(t *testing.T) {
	ob, db := newOutbox(t)
	entries := make([]crdt.LogEntry, 200)
	for i := range entries {
		entries[i] = crdt.LogEntry{Table: "memory", RowID: int64(i), Node: "node-a", Lamport: crdt.Lamport(i), Op: crdt.OpInsert, Payload: []byte("payload payload payload")}
	}
	if err := ob.Enqueue(context.Background(), "node-a", entries); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	var size int64
	if err := db.QueryRow(`SELECT LENGTH(payload) FROM sync_outbox`).Scan(&size); err != nil {
		t.Fatalf("size: %v", err)
	}
	// Conservative ceiling — repetitive JSON of 200 entries pre-gzip is roughly
	// 25 KB; gzip must bring it well under 5 KB or our wire budget (§2.8 idle
	// 4 KB/min) goes negative on the first heartbeat.
	if size >= 5_000 {
		t.Fatalf("gzipped size %d bytes, want < 5000 (compression broken)", size)
	}
}

// When projected size exceeds 50 MB, Enqueue truncates the oldest sent_at rows.
// Unsent rows must survive — losing them would lose a delta forever.
func TestOutbox_TruncateOldestAckdAt50MB(t *testing.T) {
	ob, db := newOutbox(t)
	ctx := context.Background()

	// Fabricate ack'd large rows directly so we exercise the truncation arithmetic
	// without spending 50 MB of test time on real gzip. Each row is 10 MB.
	bigPayload := make([]byte, 10*1024*1024)
	for i := 0; i < 5; i++ {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO sync_outbox(peer_id,payload,enqueued_at,sent_at) VALUES('node-a',?,?,?)`,
			bigPayload, int64(i), int64(i+1000)); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	// Seed one unsent row at the head — it must NOT be deleted by truncation.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO sync_outbox(peer_id,payload,enqueued_at) VALUES('node-a',?,?)`,
		bigPayload[:1024], int64(0)); err != nil {
		t.Fatalf("seed unsent: %v", err)
	}

	// Now enqueue 10 MB more — projected = 50 MB + 10 MB > cap; oldest ack'd rows drop.
	entries := []crdt.LogEntry{{Table: "memory", RowID: 1, Node: "node-a", Lamport: 1, Op: crdt.OpUpdate, Payload: bigPayload}}
	if err := ob.Enqueue(ctx, "node-a", entries); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var sentCount, unsentCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sync_outbox WHERE sent_at IS NOT NULL`).Scan(&sentCount); err != nil {
		t.Fatalf("sent: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sync_outbox WHERE sent_at IS NULL`).Scan(&unsentCount); err != nil {
		t.Fatalf("unsent: %v", err)
	}
	if sentCount >= 5 {
		t.Fatalf("sent rows after truncation = %d, want < 5 (some ack'd should be reclaimed)", sentCount)
	}
	if unsentCount < 2 {
		t.Fatalf("unsent rows = %d, want >= 2 (truncation must preserve unsent)", unsentCount)
	}

	size, err := ob.SizeBytes(ctx)
	if err != nil {
		t.Fatalf("size: %v", err)
	}
	if size > OutboxMaxBytes {
		t.Fatalf("size %d > cap %d after truncation", size, OutboxMaxBytes)
	}
}
