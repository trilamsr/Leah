package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func mustOpen(t *testing.T) *MailDir {
	t.Helper()
	m, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return m
}

func sampleItem(id string) Item {
	return Item{
		Schema:  1,
		ID:      id,
		Channel: "linkedin",
		Slot:    "commit",
		Text:    "ship the maildir store today.\n",
		Origin:  "abc1234",
		Created: time.Date(2026, 6, 21, 21, 39, 0, 0, time.UTC),
	}
}

func TestEnqueueNextRoundtrip(t *testing.T) {
	m := mustOpen(t)
	in := sampleItem("01J8AAAAAA")
	if err := m.Enqueue(in); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	got, err := m.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got.ID != in.ID || got.Channel != in.Channel || got.Slot != in.Slot || got.Text != in.Text {
		t.Fatalf("roundtrip mismatch: got %+v want %+v", got, in)
	}
	if got.Schema != 1 {
		t.Fatalf("schema = %d, want 1", got.Schema)
	}
	if err := m.Sent(got.ID); err != nil {
		t.Fatalf("Sent: %v", err)
	}
	// After Sent, queue is empty.
	if _, err := m.Next(); !errors.Is(err, ErrEmptyQueue) {
		t.Fatalf("Next after Sent: err = %v, want ErrEmptyQueue", err)
	}
}

func TestEnqueueIdempotentRejectsDuplicateID(t *testing.T) {
	m := mustOpen(t)
	it := sampleItem("01J8DUPDUP")
	if err := m.Enqueue(it); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	err := m.Enqueue(it)
	if err == nil {
		t.Fatal("second Enqueue: expected error, got nil")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("err = %v, want os.ErrExist (O_CREATE|O_EXCL contract)", err)
	}
}

func TestRejectsSchemaZero(t *testing.T) {
	m := mustOpen(t)
	it := sampleItem("01J8SCHEMA0")
	it.Schema = 0
	err := m.Enqueue(it)
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("Enqueue schema=0: err = %v, want schema error", err)
	}
}

func TestLoaderRejectsSchemaTwo(t *testing.T) {
	// Hand-craft a queue/<id>.md with schema: 2 and assert ListQueue
	// returns "unknown schema" — forward-compat: v2 fields must NOT
	// silently misparse against v1 loader.
	m := mustOpen(t)
	path := filepath.Join(m.root, "queue", "01J8SCHEMA2.md")
	body := "---\nschema: 2\nid: 01J8SCHEMA2\nchannel: linkedin\nslot: commit\norigin: abc\ncreated: 2026-06-21T21:39:00Z\n---\n\nbody\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := m.ListQueue()
	if err == nil || !strings.Contains(err.Error(), "unknown schema") {
		t.Fatalf("ListQueue with schema 2: err = %v, want unknown schema", err)
	}
}

func TestInboxApproveMovesToQueue(t *testing.T) {
	m := mustOpen(t)
	it := sampleItem("01J8INBOX01")
	if err := m.Inbox(it); err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	pending, err := m.ListInbox()
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("ListInbox len = %d, want 1", len(pending))
	}
}

func TestConcurrentEnqueueExactlyOneWins(t *testing.T) {
	// Spec § "Idempotency / collision" — Enqueue uses O_CREATE|O_EXCL
	// so two callers racing on the same ULID see exactly-one-success
	// and exactly-one os.ErrExist. This is the atomicity guarantee the
	// store relies on; verifying it under -race protects the contract
	// the generator (PR-3) will lean on.
	m := mustOpen(t)
	it := sampleItem("01J8RACE0001")
	var wg sync.WaitGroup
	var errA, errB error
	wg.Add(2)
	go func() { defer wg.Done(); errA = m.Enqueue(it) }()
	go func() { defer wg.Done(); errB = m.Enqueue(it) }()
	wg.Wait()
	wins := 0
	if errA == nil {
		wins++
	} else if !errors.Is(errA, os.ErrExist) {
		t.Fatalf("Enqueue A loser err = %v, want os.ErrExist", errA)
	}
	if errB == nil {
		wins++
	} else if !errors.Is(errB, os.ErrExist) {
		t.Fatalf("Enqueue B loser err = %v, want os.ErrExist", errB)
	}
	if wins != 1 {
		t.Fatalf("wins = %d, want exactly 1 (errA=%v errB=%v)", wins, errA, errB)
	}
}

func TestSentLoserSeesErrNotExist(t *testing.T) {
	// Per-rename atomicity: once Sent moved queue/<id>.md → sent/<id>.md,
	// a second Sent for the same id finds nothing to rename.
	m := mustOpen(t)
	it := sampleItem("01J8RACE0002")
	if err := m.Enqueue(it); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := m.Sent(it.ID); err != nil {
		t.Fatalf("first Sent: %v", err)
	}
	err := m.Sent(it.ID)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second Sent: err = %v, want ErrNotExist", err)
	}
}
