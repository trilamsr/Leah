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

// TestAppendPreservesWorkspace asserts the new Workspace field roundtrips
// through JSON so downstream readers (status, retro, patterns) can filter
// audit rows by active workspace. Backward-compat: omitempty keeps legacy
// rows valid JSON.
func TestAppendPreservesWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{Path: path, Now: func() time.Time { return time.Unix(1700000000, 0).UTC() }}

	if err := logger.Append(Entry{Kind: "ask", Workspace: "acme", Outcome: "success"}); err != nil {
		t.Fatalf("append tagged: %v", err)
	}
	if err := logger.Append(Entry{Kind: "ask", Outcome: "success"}); err != nil {
		t.Fatalf("append untagged: %v", err)
	}

	data, _ := os.ReadFile(path)
	lines := splitLines(string(data))
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}

	var tagged, untagged Entry
	if err := json.Unmarshal([]byte(lines[0]), &tagged); err != nil {
		t.Fatalf("parse tagged: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &untagged); err != nil {
		t.Fatalf("parse untagged: %v", err)
	}
	if tagged.Workspace != "acme" {
		t.Errorf("Workspace lost: %q", tagged.Workspace)
	}
	if untagged.Workspace != "" {
		t.Errorf("untagged Workspace = %q, want empty", untagged.Workspace)
	}
	// Untagged row must not contain the "workspace" JSON key (omitempty).
	if !json.Valid([]byte(lines[1])) {
		t.Fatalf("line 2 not valid JSON: %s", lines[1])
	}
	if contains(lines[1], `"workspace"`) {
		t.Errorf("untagged row leaked workspace key: %s", lines[1])
	}
}

// TestAppendDefaultWorkspaceCallback asserts Logger.DefaultWorkspace stamps
// an unset Entry.Workspace at append time — the runAsk / runShip wiring
// uses this to tag every audit row with the active workspace without
// passing it through every callsite.
func TestAppendDefaultWorkspaceCallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{
		Path:             path,
		Now:              func() time.Time { return time.Unix(1700000000, 0).UTC() },
		DefaultWorkspace: func() string { return "acme" },
	}

	// Unset Workspace → default fires.
	if err := logger.Append(Entry{Kind: "ask"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	// Caller-set Workspace wins over default.
	if err := logger.Append(Entry{Kind: "ask", Workspace: "personal"}); err != nil {
		t.Fatalf("append explicit: %v", err)
	}

	data, _ := os.ReadFile(path)
	lines := splitLines(string(data))
	var defaulted, explicit Entry
	_ = json.Unmarshal([]byte(lines[0]), &defaulted)
	_ = json.Unmarshal([]byte(lines[1]), &explicit)
	if defaulted.Workspace != "acme" {
		t.Errorf("default not applied: %q", defaulted.Workspace)
	}
	if explicit.Workspace != "personal" {
		t.Errorf("explicit overridden: %q", explicit.Workspace)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
