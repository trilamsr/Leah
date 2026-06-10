package audit

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLogger_Subscribe_ReceivesAppendedEntries(t *testing.T) {
	dir := t.TempDir()
	logger := &Logger{Path: filepath.Join(dir, "audit.jsonl")}

	ch, cancel := logger.Subscribe(8)
	defer cancel()

	if err := logger.Append(Entry{Kind: "ask", ArgsHash: "abc", Outcome: "success"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	select {
	case e := <-ch:
		if e.Kind != "ask" || e.ArgsHash != "abc" || e.Outcome != "success" {
			t.Errorf("entry mismatch: %+v", e)
		}
		if e.Timestamp == "" {
			t.Errorf("timestamp not stamped on pushed entry")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber received nothing within 1s")
	}
}

func TestLogger_Subscribe_DropOnFull(t *testing.T) {
	dir := t.TempDir()
	logger := &Logger{Path: filepath.Join(dir, "audit.jsonl")}

	ch, cancel := logger.Subscribe(2)
	defer cancel()

	for i := 0; i < 6; i++ {
		if err := logger.Append(Entry{Kind: "ask", ArgsHash: "x"}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	if got := logger.Dropped(); got != 4 {
		t.Errorf("Dropped() = %d, want 4", got)
	}

	drained := 0
	for {
		select {
		case <-ch:
			drained++
		default:
			if drained != 2 {
				t.Errorf("drained = %d, want 2 (buffer cap)", drained)
			}
			return
		}
	}
}

func TestLogger_Subscribe_Unsubscribe_StopsFanout(t *testing.T) {
	dir := t.TempDir()
	logger := &Logger{Path: filepath.Join(dir, "audit.jsonl")}

	ch, cancel := logger.Subscribe(4)

	if err := logger.Append(Entry{Kind: "ask"}); err != nil {
		t.Fatalf("append pre-cancel: %v", err)
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("no pre-cancel delivery")
	}

	cancel()

	for i := 0; i < 3; i++ {
		if err := logger.Append(Entry{Kind: "ask"}); err != nil {
			t.Fatalf("append post-cancel %d: %v", i, err)
		}
	}

	if got := logger.Dropped(); got != 0 {
		t.Errorf("cancelled subscriber must not contribute to Dropped(); got %d", got)
	}

	select {
	case e, ok := <-ch:
		if ok {
			t.Errorf("post-cancel delivery on closed sub: %+v", e)
		}
	default:
	}
}

// Regression for the cancel-vs-fanout send-on-closed-channel panic: cancel
// path closing s.ch while a concurrent Append was sending into it crashed
// the audit hot path. Must run clean under -race.
func TestLogger_Subscribe_ConcurrentCancelAndAppend_NoRace(t *testing.T) {
	dir := t.TempDir()
	logger := &Logger{Path: filepath.Join(dir, "audit.jsonl")}

	const cancellers = 10
	const appends = 100

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < cancellers; i++ {
		_, cancel := logger.Subscribe(2)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			cancel()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < appends; i++ {
			if err := logger.Append(Entry{Kind: "ask", ArgsHash: "x"}); err != nil {
				t.Errorf("append %d: %v", i, err)
				return
			}
		}
	}()

	close(start)
	wg.Wait()
}

// TotalDropped is cumulative across the logger's lifetime — it must not
// flicker down when a subscriber cancels (unlike the live-only Dropped()).
func TestLogger_TotalDropped_SurvivesCancel(t *testing.T) {
	dir := t.TempDir()
	logger := &Logger{Path: filepath.Join(dir, "audit.jsonl")}

	_, cancel := logger.Subscribe(1)

	for i := 0; i < 5; i++ {
		if err := logger.Append(Entry{Kind: "ask"}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	pre := logger.TotalDropped()
	if pre == 0 {
		t.Fatalf("TotalDropped should be >0 after overflow; got 0")
	}

	cancel()

	if got := logger.TotalDropped(); got != pre {
		t.Errorf("TotalDropped flickered on cancel: pre=%d post=%d", pre, got)
	}
}

func TestLogger_MultipleSubscribers_Independent(t *testing.T) {
	dir := t.TempDir()
	logger := &Logger{Path: filepath.Join(dir, "audit.jsonl")}

	fast, cancelFast := logger.Subscribe(16)
	defer cancelFast()
	slow, cancelSlow := logger.Subscribe(1)
	defer cancelSlow()

	const N = 8
	var wg sync.WaitGroup
	wg.Add(1)
	got := 0
	go func() {
		defer wg.Done()
		deadline := time.After(2 * time.Second)
		for got < N {
			select {
			case <-fast:
				got++
			case <-deadline:
				return
			}
		}
	}()

	for i := 0; i < N; i++ {
		if err := logger.Append(Entry{Kind: "ask"}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	wg.Wait()
	if got != N {
		t.Errorf("fast subscriber got %d/%d — slow consumer blocked fanout", got, N)
	}

	select {
	case <-slow:
	default:
		t.Errorf("slow subscriber buffer should hold the most recent entry")
	}
	if logger.Dropped() == 0 {
		t.Errorf("slow subscriber should have recorded drops; Dropped() = 0")
	}
}
