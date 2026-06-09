package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendWritesOneLinePerEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{Path: path, Now: func() time.Time { return time.Unix(1700000000, 0).UTC() }}

	e1 := Entry{Kind: "ask", ArgsHash: "abc", BlastRadius: 1, Outcome: "success"}
	e2 := Entry{Kind: "ship", ArgsHash: "def", BlastRadius: 3, Outcome: "pending"}

	if err := logger.Append(e1); err != nil {
		t.Fatalf("append e1: %v", err)
	}
	if err := logger.Append(e2); err != nil {
		t.Fatalf("append e2: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	lines := splitLines(string(data))
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), data)
	}

	var got1, got2 Entry
	if err := json.Unmarshal([]byte(lines[0]), &got1); err != nil {
		t.Fatalf("parse line 1: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &got2); err != nil {
		t.Fatalf("parse line 2: %v", err)
	}

	if got1.Kind != "ask" || got1.ArgsHash != "abc" || got1.BlastRadius != 1 || got1.Outcome != "success" {
		t.Errorf("line 1 mismatch: %+v", got1)
	}
	if got2.Kind != "ship" || got2.ArgsHash != "def" || got2.BlastRadius != 3 || got2.Outcome != "pending" {
		t.Errorf("line 2 mismatch: %+v", got2)
	}
	if got1.Timestamp != "2023-11-14T22:13:20Z" {
		t.Errorf("line 1 timestamp wrong: %q", got1.Timestamp)
	}
}

func TestAppendAppendsNotTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{Path: path, Now: func() time.Time { return time.Unix(1700000000, 0).UTC() }}

	for i := 0; i < 10; i++ {
		if err := logger.Append(Entry{Kind: "ask", ArgsHash: "x"}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	data, _ := os.ReadFile(path)
	if got := len(splitLines(string(data))); got != 10 {
		t.Fatalf("want 10 lines, got %d", got)
	}
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
