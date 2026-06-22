package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trilam/leah/internal/ipc"
	"github.com/trilam/leah/internal/knowledge"
	"github.com/trilam/leah/internal/reasoner"
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

// streamFn error must surface as a handler error so the ipc server can
// emit an error frame; partial frames must not be delivered.
func TestIPCHandlerStreamError(t *testing.T) {
	db := newTestTurnDB(t)
	failStream := func(_ context.Context, _, _ string) (<-chan ipc.Frame, error) {
		return nil, errors.New("upstream down")
	}
	h := newIPCHandlerForTest(db, failStream)
	in := ipc.Frame{Kind: "ask", TurnID: "te", Payload: json.RawMessage(`{"text":"x"}`)}
	out, err := h(context.Background(), in)
	if err == nil {
		t.Fatal("want error from streamFn failure")
	}
	if out != nil {
		t.Fatalf("want nil channel on error, got %v", out)
	}
}

// RecordTurn failure (broken schema) must be logged but not crash the
// handler goroutine or block frame delivery to the consumer.
func TestIPCHandlerRecordTurnFailureIsNonFatal(t *testing.T) {
	db, err := sqlstore.OpenWAL(filepath.Join(t.TempDir(), "broken.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	// Intentionally do NOT create conversation_turn — RecordTurn must fail.
	fakeStream := func(_ context.Context, turnID, _ string) (<-chan ipc.Frame, error) {
		out := make(chan ipc.Frame, 2)
		out <- ipc.Frame{Kind: "prose.delta", TurnID: turnID, Seq: 1, Payload: json.RawMessage(`{"text":"hi"}`)}
		out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: 2, Payload: json.RawMessage(`{}`)}
		close(out)
		return out, nil
	}
	h := newIPCHandlerForTest(db, fakeStream)
	in := ipc.Frame{Kind: "ask", TurnID: "nofs", Payload: json.RawMessage(`{"text":"hi"}`)}
	out, err := h(context.Background(), in)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var frames []ipc.Frame
	done := make(chan struct{})
	go func() {
		for f := range out {
			frames = append(frames, f)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler goroutine deadlocked after RecordTurn failure")
	}
	if frames[len(frames)-1].Kind != "turn.end" {
		t.Fatalf("turn.end must still surface despite RecordTurn failure; got %q", frames[len(frames)-1].Kind)
	}
}

// Haiku classify returning widget kind must emit a widget.mount frame and
// skip the Sonnet stream entirely.
func TestIPCHandlerClassifiesWidget(t *testing.T) {
	db := newTestTurnDB(t)
	// streamFn must NOT be called for widget queries.
	sonnetCalled := false
	sonnetStream := func(_ context.Context, _, _ string) (<-chan ipc.Frame, error) {
		sonnetCalled = true
		out := make(chan ipc.Frame, 1)
		close(out)
		return out, nil
	}
	widgetClassify := func(_ context.Context, _ string) reasoner.Intent {
		return reasoner.Intent{Kind: "widget", Widget: "stat", Confidence: 0.95}
	}
	h := newIPCHandlerWithClassify(db, sonnetStream, sonnetStream, widgetClassify,
		func(_ context.Context, _ string) error { return nil }, nil, time.Time{})

	in := ipc.Frame{Kind: "ask", TurnID: "w1", Payload: json.RawMessage(`{"text":"what is my daily cost?"}`)}
	out, err := h(context.Background(), in)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var frames []ipc.Frame
	for f := range out {
		frames = append(frames, f)
	}
	if sonnetCalled {
		t.Fatal("Sonnet stream must not be called for widget intent")
	}
	if len(frames) == 0 {
		t.Fatal("want at least one frame for widget intent")
	}
	if frames[0].Kind != "widget.mount" {
		t.Fatalf("first frame kind: want widget.mount, got %q", frames[0].Kind)
	}
	var wp struct {
		WidgetType string `json:"widget_type"`
	}
	if err := json.Unmarshal(frames[0].Payload, &wp); err != nil {
		t.Fatalf("unmarshal widget.mount payload: %v", err)
	}
	if wp.WidgetType != "stat" {
		t.Fatalf("widget_type: want stat, got %q", wp.WidgetType)
	}
}

// escalate_opus: true in the ask payload must route to the opus streamFn.
func TestIPCHandlerHonorsOpusEscalation(t *testing.T) {
	db := newTestTurnDB(t)
	sonnetCalled := false
	opusCalled := false
	sonnetStream := func(_ context.Context, turnID, _ string) (<-chan ipc.Frame, error) {
		sonnetCalled = true
		out := make(chan ipc.Frame, 1)
		out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: 1, Payload: json.RawMessage(`{}`)}
		close(out)
		return out, nil
	}
	opusStream := func(_ context.Context, turnID, _ string) (<-chan ipc.Frame, error) {
		opusCalled = true
		out := make(chan ipc.Frame, 1)
		out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: 1, Payload: json.RawMessage(`{}`)}
		close(out)
		return out, nil
	}
	chatClassify := func(_ context.Context, _ string) reasoner.Intent {
		return reasoner.Intent{Kind: "chat"}
	}
	h := newIPCHandlerWithClassify(db, sonnetStream, opusStream, chatClassify,
		func(_ context.Context, _ string) error { return nil }, nil, time.Time{})

	in := ipc.Frame{
		Kind:    "ask",
		TurnID:  "op1",
		Payload: json.RawMessage(`{"text":"complex reasoning task","escalate_opus":true}`),
	}
	out, err := h(context.Background(), in)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	for range out {
	}
	if !opusCalled {
		t.Fatal("opusStream must be called when escalate_opus=true")
	}
	if sonnetCalled {
		t.Fatal("sonnetStream must not be called when escalate_opus=true")
	}
}

// Context cancellation mid-stream must close the output channel without
// goroutine leak.
func TestIPCHandlerContextCancelClosesChannel(t *testing.T) {
	db := newTestTurnDB(t)
	block := make(chan struct{})
	blockingStream := func(ctx context.Context, turnID, _ string) (<-chan ipc.Frame, error) {
		out := make(chan ipc.Frame)
		go func() {
			defer close(out)
			select {
			case <-ctx.Done():
				return
			case <-block:
				return
			}
		}()
		return out, nil
	}
	h := newIPCHandlerForTest(db, blockingStream)
	ctx, cancel := context.WithCancel(context.Background())
	in := ipc.Frame{Kind: "ask", TurnID: "cx", Payload: json.RawMessage(`{"text":"x"}`)}
	out, err := h(ctx, in)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	cancel()
	close(block)
	select {
	case _, ok := <-out:
		if ok {
			for range out {
			}
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("output channel did not close after ctx cancel")
	}
}

// SearchRelevant is called before streaming and context is prepended to prompt.
func TestIPCHandlerFetchesContext(t *testing.T) {
	db := newTestTurnDB(t)

	fetchCalled := false
	spyFetch := func(_ context.Context, query string, k int) ([]knowledge.Chunk, error) {
		fetchCalled = true
		if query != "what is leah" {
			t.Errorf("fetch query: got %q, want %q", query, "what is leah")
		}
		if k != 5 {
			t.Errorf("fetch k: got %d, want 5", k)
		}
		return []knowledge.Chunk{{ID: "c1", Text: "Leah is a personal AI assistant."}}, nil
	}

	var capturedPrompt string
	spyStream := func(_ context.Context, turnID, prompt string) (<-chan ipc.Frame, error) {
		capturedPrompt = prompt
		out := make(chan ipc.Frame, 1)
		out <- ipc.Frame{Kind: "turn.end", TurnID: turnID, Seq: 1, Payload: json.RawMessage(`{}`)}
		close(out)
		return out, nil
	}

	noClassify := func(_ context.Context, _ string) reasoner.Intent { return reasoner.Intent{Kind: "chat"} }
	h := newIPCHandlerWithClassify(db, spyStream, spyStream, noClassify,
		func(_ context.Context, _ string) error { return nil }, spyFetch, time.Time{})
	in := ipc.Frame{Kind: "ask", TurnID: "t2", Payload: json.RawMessage(`{"text":"what is leah"}`)}
	out, err := h(context.Background(), in)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	for range out {
	}

	if !fetchCalled {
		t.Fatal("SearchRelevant was not called")
	}
	if !strings.Contains(capturedPrompt, "Leah is a personal AI assistant.") {
		t.Fatalf("prompt missing retrieved context: %q", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "what is leah") {
		t.Fatalf("prompt missing original query: %q", capturedPrompt)
	}
}
