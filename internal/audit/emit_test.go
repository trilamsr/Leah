package audit

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/trilam/leah/internal/obs"
)

// TestAppendEmitsAuditAppendToBroadcaster drives a real Append and asserts the
// live SSE fan-out (the bus events_timeline.js subscribes to) receives an
// audit.append frame — proving the dashboard subscription is no longer dead.
func TestAppendEmitsAuditAppendToBroadcaster(t *testing.T) {
	b := obs.NewBroadcaster()
	obs.SetDefaultBroadcaster(b)
	t.Cleanup(func() { obs.SetDefaultBroadcaster(nil) })
	sub, err := b.Subscribe(context.Background(), []string{"audit.append"})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = sub.Close() })

	l := &Logger{Path: filepath.Join(t.TempDir(), "audit.jsonl")}
	if err := l.Append(Entry{Kind: "ship", Outcome: "ok"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	select {
	case e := <-sub.Events():
		if e.Kind != "audit.append" {
			t.Fatalf("kind: got %q want audit.append", e.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("no audit.append frame on the live bus")
	}
}
