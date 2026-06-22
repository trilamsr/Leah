package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/sqlstore"
)

// newTestTurnDB opens a WAL DB with only the conversation_turn table —
// mirrors what memory.RecordTurn expects without pulling in the full migration.
func newTestTurnDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlstore.OpenWAL(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE conversation_turn(
		id              TEXT PRIMARY KEY,
		user_text       TEXT NOT NULL,
		assistant_text  TEXT NOT NULL,
		created_at      TEXT NOT NULL
	);`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func TestIPCHandlerEmitsTurnEndAndPersistsTurn(t *testing.T) {
	db := newTestTurnDB(t)

	// Fake stream: emits two prose.delta tokens + turn.end.
	fakeStream := func(_ context.Context, turnID, _ string) (<-chan ipc.Frame, error) {
		out := make(chan ipc.Frame, 3)
		out <- ipc.Frame{Kind: "prose.delta", TurnID: turnID, Seq: 1, Payload: json.RawMessage(`{"text":"hi "}`)}
		out <- ipc.Frame{Kind: "prose.delta", TurnID: turnID, Seq: 2, Payload: json.RawMessage(`{"text":"there"}`)}
		out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: 3, Payload: json.RawMessage(`{}`)}
		close(out)
		return out, nil
	}
	h := newIPCHandlerForTest(db, fakeStream)

	in := ipc.Frame{Kind: "ask", TurnID: "t1", Seq: 0, Payload: json.RawMessage(`{"text":"hello"}`)}
	out, err := h(context.Background(), in)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var frames []ipc.Frame
	for f := range out {
		frames = append(frames, f)
	}
	if len(frames) < 3 {
		t.Fatalf("want >=3 frames, got %d", len(frames))
	}
	last := frames[len(frames)-1]
	if last.Kind != "turn.end" {
		t.Fatalf("last frame must be turn.end, got %q", last.Kind)
	}

	// Verify memory persistence.
	var got string
	if err := db.QueryRow(`SELECT assistant_text FROM conversation_turn WHERE id='t1'`).Scan(&got); err != nil {
		t.Fatalf("query turn: %v", err)
	}
	if got != "hi there" {
		t.Fatalf("assembled assistant text: %q, want %q", got, "hi there")
	}
}

func TestIPCHandlerVerifyKey(t *testing.T) {
	db := newTestTurnDB(t)

	// verify-key path uses a one-shot ping; stub returns immediate success.
	pingOK := func(_ context.Context, _ string) error { return nil }
	h := newIPCHandlerWithPingForTest(db, nil, pingOK)

	in := ipc.Frame{Kind: "verify-key", TurnID: "vk1", Seq: 0, Payload: json.RawMessage(`{"key":"sk-test"}`)}
	out, err := h(context.Background(), in)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var frames []ipc.Frame
	for f := range out {
		frames = append(frames, f)
	}
	if len(frames) != 1 {
		t.Fatalf("verify-key: want 1 frame, got %d", len(frames))
	}
	if frames[0].Kind != "verify-key.result" {
		t.Fatalf("verify-key.result: got kind %q", frames[0].Kind)
	}
	var p struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(frames[0].Payload, &p); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !p.OK {
		t.Fatal("verify-key: expected ok=true")
	}
}

func TestIPCHandlerVerifyKeyFailure(t *testing.T) {
	db := newTestTurnDB(t)

	pingFail := func(_ context.Context, _ string) error {
		return errVerifyFailed
	}
	h := newIPCHandlerWithPingForTest(db, nil, pingFail)

	in := ipc.Frame{Kind: "verify-key", TurnID: "vk2", Seq: 0, Payload: json.RawMessage(`{"key":"bad-key"}`)}
	out, err := h(context.Background(), in)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var frames []ipc.Frame
	for f := range out {
		frames = append(frames, f)
	}
	if len(frames) != 1 {
		t.Fatalf("verify-key: want 1 frame, got %d", len(frames))
	}
	var p struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(frames[0].Payload, &p)
	if p.OK {
		t.Fatal("verify-key: expected ok=false on bad key")
	}
}
